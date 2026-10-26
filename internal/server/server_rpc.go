package server

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/cron"
	"github.com/hooperits/hitstor/internal/hook"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Ping implements the health check RPC.
func (s *Server) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	repoID := ""
	if repos := s.repos.List(); len(repos) > 0 {
		repoID = repos[0].Repo.Cfg().ID
	}
	return &pb.PingResponse{
		Version: s.opts.Version,
		RepoId:  repoID,
	}, nil
}

// BeginSession starts a backup session for an authenticated agent.
func (s *Server) BeginSession(ctx context.Context, req *pb.BeginSessionRequest) (*pb.BeginSessionResponse, error) {
	agentName, ok := s.registry.VerifyKey(req.ApiKey)
	if !ok {
		slog.Warn("authentication failed", "hostname", req.Hostname)
		return nil, status.Error(codes.Unauthenticated, "invalid API key")
	}

	// Resolve target repository. If a job_id is provided, use the job's repo.
	var mr *ManagedRepo
	var jobID string
	if req.JobId != "" {
		job := s.registry.GetJob(req.JobId)
		if job == nil {
			return nil, status.Errorf(codes.NotFound, "backup job %q not found", req.JobId)
		}
		if job.AgentName != agentName {
			return nil, status.Error(codes.PermissionDenied, "job does not belong to this agent")
		}
		jobID = req.JobId
		var err error
		mr, err = s.repos.Get(job.RepoName)
		if err != nil {
			// Fall back to first repo if job's repo not found.
			repos := s.repos.List()
			if len(repos) == 0 {
				return nil, status.Error(codes.FailedPrecondition, "no repositories configured")
			}
			mr = repos[0]
		}
		// Inject job tag for snapshot association.
		req.Tags = append(req.Tags, "job:"+req.JobId)
	} else {
		// Legacy path: resolve from agent record.
		var err error
		mr, err = s.resolveAgentRepo(agentName)
		if err != nil {
			return nil, err
		}
	}

	// Check maintenance lock.
	if locked, reason := mr.MaintenanceLock.IsLocked(); locked {
		return nil, status.Errorf(codes.Unavailable,
			"repository %q locked for maintenance: %s", mr.Name, reason)
	}

	// Run pre-backup hooks (blocking — abort on error).
	if hooks := s.getHooks(); len(hooks) > 0 {
		evt := hook.HookEvent{
			Agent:    agentName,
			Hostname: req.Hostname,
			Paths:    req.Paths,
			Excludes: req.Excludes,
			Tags:     req.Tags,
		}
		for _, h := range hooks {
			if err := h.PreBackup(ctx, evt); err != nil {
				slog.Warn("pre-backup hook rejected session",
					"agent", agentName, "error", err)
				return nil, status.Errorf(codes.FailedPrecondition, "pre-backup hook: %v", err)
			}
		}
	}

	if err := mr.Repo.LockWithOptions(repository.LockOptions{
		Exclusive:  true,
		BreakStale: true,
	}); err != nil {
		return nil, status.Errorf(codes.Unavailable, "lock repository %q: %v", mr.Name, err)
	}

	session := NewSession(mr.Repo, agentName, req.Hostname, req.Paths, req.Excludes, req.Tags)
	session.RepoName = mr.Name
	session.JobID = jobID
	session.repoLocked = true

	// Determine backup type: find previous snapshot for incremental.
	var previousFiles []*pb.PreviousFileEntry
	var parentSnapshotID string
	forceFull := false

	if jobID != "" {
		job := s.registry.GetJob(jobID)
		if job != nil {
			// Check force-full conditions.
			if job.ForceFullScan {
				forceFull = true
				slog.Info("force full scan requested", "job", jobID)
			} else if job.FullRescanDays > 0 && job.LastFullBackup != nil {
				if time.Since(*job.LastFullBackup) > time.Duration(job.FullRescanDays)*24*time.Hour {
					forceFull = true
					slog.Info("full rescan interval exceeded", "job", jobID,
						"days", job.FullRescanDays, "last_full", job.LastFullBackup)
				}
			}
		}

		if !forceFull {
			prevSnap, prevID, found := findPreviousSnapshot(mr.Repo, jobID)
			if found {
				parentSnapshotID = hex.EncodeToString(prevID[:])
				files, err := extractPreviousFiles(mr.Repo, backend.ID(prevSnap.Tree), "")
				if err != nil {
					slog.Warn("failed to extract previous files, falling back to full backup",
						"job", jobID, "error", err)
				} else {
					previousFiles = files
				}
			}
		}
	}

	if len(previousFiles) > 0 {
		session.BackupType = "incremental"
		session.ParentSnapshotID = parentSnapshotID
	} else {
		session.BackupType = "full"
	}

	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()

	cfg := mr.Repo.Cfg()
	idKey := mr.Repo.Key().IDKey

	slog.Info("backup session started",
		"agent", agentName,
		"hostname", req.Hostname,
		"session", session.ID,
		"repo", mr.Name,
		"paths", req.Paths,
		"job_id", jobID,
		"backup_type", session.BackupType,
		"parent_snapshot", session.ParentSnapshotID,
		"previous_files", len(previousFiles),
	)

	return &pb.BeginSessionResponse{
		SessionId:        session.ID,
		IdKey:            idKey[:],
		ChunkerMin:       uint32(cfg.ChunkerParams.MinSize),
		ChunkerAvg:       uint32(cfg.ChunkerParams.AvgSize),
		ChunkerMax:       uint32(cfg.ChunkerParams.MaxSize),
		PreviousFiles:    previousFiles,
		ParentSnapshotId: parentSnapshotID,
	}, nil
}

// resolveAgentRepo finds the target repo for an agent.
// If the agent has a RepoName set, that repo is used.
// Otherwise, falls back to the first available repo.
func (s *Server) resolveAgentRepo(agentName string) (*ManagedRepo, error) {
	agent := s.registry.GetAgent(agentName)
	if agent == nil {
		return nil, status.Errorf(codes.NotFound, "agent %q not found", agentName)
	}

	if agent.RepoName != "" {
		mr, err := s.repos.Get(agent.RepoName)
		if err != nil {
			return nil, status.Errorf(codes.NotFound,
				"repo %q assigned to agent %q not found", agent.RepoName, agentName)
		}
		return mr, nil
	}

	// Fallback: use first repo.
	repos := s.repos.List()
	if len(repos) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "no repositories configured")
	}
	return repos[0], nil
}

// CheckBlobs checks which blob IDs the server already has.
func (s *Server) CheckBlobs(ctx context.Context, req *pb.CheckBlobsRequest) (*pb.CheckBlobsResponse, error) {
	session, err := s.getSession(req.SessionId)
	if err != nil {
		return nil, err
	}

	var needed [][]byte
	for _, idBytes := range req.BlobIds {
		if len(idBytes) != 32 {
			continue
		}
		var blobID [32]byte
		copy(blobID[:], idBytes)

		// Check session's own known set first, then repo index.
		if session.IsKnown(blobID) {
			continue
		}
		if existing, _ := session.repo.LookupBlob(blobID); existing != nil {
			session.MarkKnown(blobID)
			continue
		}
		needed = append(needed, idBytes)
	}

	return &pb.CheckBlobsResponse{
		NeededIds: needed,
	}, nil
}

// UploadChunks receives a stream of compressed chunks from the agent.
func (s *Server) UploadChunks(stream pb.BackupService_UploadChunksServer) error {
	var session *Session
	var chunksReceived uint32
	var bytesReceived uint64

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" || status.Code(err) == codes.OK {
				break
			}
			// Check for clean stream end.
			if s, ok := status.FromError(err); ok && s.Code() == codes.OK {
				break
			}
			// Agent disconnected — clean up.
			if session != nil {
				slog.Warn("agent disconnected during upload",
					"session", session.ID,
					"agent", session.AgentName,
					"chunks_received", chunksReceived,
				)
				s.backupsFailed.Add(1)
				s.notifyBackupFailed(session.AgentName, session.Hostname,
					"agent disconnected during upload",
					map[string]any{
						"session_id":      session.ID,
						"chunks_received": chunksReceived,
					})
				s.cleanupSession(session.ID)
			}
			return err
		}

		// Lazy session lookup on first message.
		if session == nil {
			var sErr error
			session, sErr = s.getSession(msg.SessionId)
			if sErr != nil {
				return sErr
			}
		}

		if len(msg.BlobId) != 32 {
			return status.Error(codes.InvalidArgument, "blob_id must be 32 bytes")
		}

		var blobID [32]byte
		copy(blobID[:], msg.BlobId)

		// Skip if already known (idempotent).
		if session.IsKnown(blobID) {
			continue
		}

		if err := session.AddChunk(blobID, msg.CompressedData, int(msg.UncompressedLength)); err != nil {
			return status.Errorf(codes.Internal, "add chunk: %v", err)
		}

		chunksReceived++
		bytesReceived += uint64(len(msg.CompressedData))
		s.chunksReceived.Add(1)
		s.bytesReceived.Add(uint64(len(msg.CompressedData)))
	}

	return stream.SendAndClose(&pb.UploadChunksResponse{
		ChunksReceived: chunksReceived,
		BytesReceived:  bytesReceived,
	})
}

// FinalizeBackup builds tree blobs, creates a snapshot, and saves everything.
func (s *Server) FinalizeBackup(ctx context.Context, req *pb.FinalizeBackupRequest) (*pb.FinalizeBackupResponse, error) {
	session, err := s.getSession(req.SessionId)
	if err != nil {
		return nil, err
	}

	session.SetPhase("finalizing")
	resp, err := session.Finalize(req.Files, req.SystemProfile)
	if err != nil {
		s.backupsFailed.Add(1)
		s.notifyBackupFailed(session.AgentName, session.Hostname,
			fmt.Sprintf("finalize failed: %v", err),
			map[string]any{"session_id": session.ID})
		s.cleanupSession(session.ID)
		return nil, status.Errorf(codes.Internal, "finalize backup: %v", err)
	}

	// Update last backup time on the agent and job (when applicable).
	now := time.Now()
	if updateErr := s.registry.UpdateLastBackup(session.AgentName, now); updateErr != nil {
		slog.Warn("failed to update agent last backup time", "agent", session.AgentName, "error", updateErr)
	}
	if session.JobID != "" {
		if updateErr := s.registry.UpdateJobLastBackup(session.JobID, now); updateErr != nil {
			slog.Warn("failed to update job last backup time", "job", session.JobID, "error", updateErr)
		}
		if session.BackupType == "full" {
			if updateErr := s.registry.UpdateJobLastFullBackup(session.JobID, now); updateErr != nil {
				slog.Warn("failed to update job last full backup time", "job", session.JobID, "error", updateErr)
			}
		}
	}

	s.backupsCompleted.Add(1)

	slog.Info("backup completed",
		"agent", session.AgentName,
		"hostname", session.Hostname,
		"session", session.ID,
		"snapshot", resp.SnapshotId,
		"files_new", resp.FilesNew,
		"bytes_added", resp.BytesAdded,
	)

	// Run post-backup hooks (non-blocking — log errors).
	if hooks := s.getHooks(); len(hooks) > 0 {
		evt := hook.HookEvent{
			Agent:      session.AgentName,
			Hostname:   session.Hostname,
			Paths:      session.Paths,
			Excludes:   session.Excludes,
			Tags:       session.Tags,
			SnapshotID: resp.SnapshotId,
			Details: map[string]string{
				"files_new":       fmt.Sprintf("%d", resp.FilesNew),
				"files_unchanged": fmt.Sprintf("%d", resp.FilesUnchanged),
				"bytes_added":     fmt.Sprintf("%d", resp.BytesAdded),
			},
		}
		for _, h := range hooks {
			if err := h.PostBackup(ctx, evt); err != nil {
				slog.Warn("post-backup hook error",
					"agent", session.AgentName, "error", err)
			}
		}
	}

	// Active Ransomware Sentinel check.
	go s.runSentinelCheck(session, resp)

	// Automated restore verification (filesystem excellence).
	if session.JobID != "" && resp.SnapshotId != "" {
		jobID, repoName, snapID := session.JobID, session.RepoName, resp.SnapshotId
		go s.runRestoreVerify(jobID, repoName, snapID)
	}

	s.cleanupSession(session.ID)
	return resp, nil
}

// GetSchedule returns backup jobs configured for an authenticated agent.
func (s *Server) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	agentName, ok := s.registry.VerifyKey(req.ApiKey)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid API key")
	}

	if s.registry.GetAgent(agentName) == nil {
		return nil, status.Error(codes.NotFound, "agent not found")
	}

	jobs := s.registry.ListJobsByAgent(agentName)
	resp := &pb.GetScheduleResponse{}

	for _, j := range jobs {
		sj := &pb.ScheduledJob{
			JobId:          j.ID,
			Name:           j.Name,
			Paths:          j.Paths,
			Excludes:       j.Excludes,
			RepoName:       j.RepoName,
			Enabled:        j.ScheduleEnabled,
			IntervalHours:  int32(j.ScheduleHours),
			RunNow:         j.RunNow,
			FullRescanDays: int32(j.FullRescanDays),
			UseVss:         j.UseVSS,
			JobType:        j.JobType,
		}
		if j.LastFullBackup != nil {
			sj.LastFullBackupUnix = j.LastFullBackup.Unix()
		}

		if j.ScheduleEnabled && j.CronExpression != "" {
			// Cron mode: parse expression and compute next run in job's timezone.
			expr, err := cron.Parse(j.CronExpression)
			if err != nil {
				slog.Warn("invalid cron expression", "job", j.ID, "expr", j.CronExpression, "error", err)
			} else {
				loc := time.Local
				if j.Timezone != "" {
					if tzLoc, tzErr := time.LoadLocation(j.Timezone); tzErr != nil {
						slog.Warn("unknown timezone, falling back to UTC", "job", j.ID, "timezone", j.Timezone, "error", tzErr)
						loc = time.UTC
					} else {
						loc = tzLoc
					}
				}
				slog.Debug("cron schedule evaluation", "job", j.ID, "expr", j.CronExpression, "timezone", loc)
				var from time.Time
				if j.LastBackup != nil {
					from = j.LastBackup.In(loc)
				} else {
					// First-ever run: anchor 1 minute in the past so that if
					// we are inside a matching cron window, expr.Next returns
					// ~now and the job becomes immediately due.
					from = time.Now().Add(-1 * time.Minute).In(loc)
				}
				next := expr.Next(from)
				if !next.IsZero() {
					sj.NextRunUnix = next.Unix()
					slog.Debug("cron next run computed", "job", j.ID, "next_run", next.Format(time.RFC3339))
				}
			}
			sj.CronExpression = j.CronExpression
			sj.Timezone = j.Timezone
		} else if j.ScheduleEnabled && j.ScheduleHours > 0 && j.LastBackup != nil {
			// Legacy interval mode.
			nextRun := j.LastBackup.Add(time.Duration(j.ScheduleHours) * time.Hour)
			sj.NextRunUnix = nextRun.Unix()
		}

		resp.Jobs = append(resp.Jobs, sj)
	}

	// Populate deprecated legacy fields from the first enabled job.
	for _, sj := range resp.Jobs {
		if sj.Enabled {
			resp.Enabled = true
			resp.IntervalHours = sj.IntervalHours
			resp.NextRunUnix = sj.NextRunUnix
			break
		}
	}

	return resp, nil
}

// getSession retrieves an active session by ID.
func (s *Server) getSession(sessionID string) (*Session, error) {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	return session, nil
}

// cleanupSession removes a session from the active map and preserves it
// as a completed session for 30 seconds so SSE clients can display completion.
func (s *Server) cleanupSession(sessionID string) {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	var info SessionInfo
	if ok {
		info = sessionToInfo(sess)
		info.Phase = "done"
	}
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	if ok {
		if sess.repoLocked {
			if err := sess.repo.Unlock(); err != nil {
				slog.Warn("failed to unlock repository after session", "session", sessionID, "error", err)
			}
			sess.repoLocked = false
		}
		s.completedMu.Lock()
		s.completedSessions[sessionID] = completedSession{
			Info:      info,
			ExpiresAt: time.Now().Add(30 * time.Second),
		}
		// Prune expired entries.
		now := time.Now()
		for id, cs := range s.completedSessions {
			if now.After(cs.ExpiresAt) {
				delete(s.completedSessions, id)
			}
		}
		s.completedMu.Unlock()
	}
}

// findPreviousSnapshot finds the most recent snapshot tagged with "job:<jobID>"
// in the given repository. Returns the snapshot, its ID, and true if found.
func findPreviousSnapshot(repo *repository.Repository, jobID string) (*repository.Snapshot, backend.ID, bool) {
	tag := "job:" + jobID
	snapIDs, err := repo.ListSnapshots()
	if err != nil || len(snapIDs) == 0 {
		return nil, backend.ID{}, false
	}

	var bestSnap *repository.Snapshot
	var bestID backend.ID
	var bestTime time.Time

	for _, sid := range snapIDs {
		snap, err := repo.LoadSnapshot(sid)
		if err != nil {
			continue
		}
		for _, t := range snap.Tags {
			if t == tag {
				if bestSnap == nil || snap.Time.After(bestTime) {
					bestSnap = snap
					bestID = sid
					bestTime = snap.Time
				}
				break
			}
		}
	}

	if bestSnap == nil {
		return nil, backend.ID{}, false
	}
	return bestSnap, bestID, true
}
