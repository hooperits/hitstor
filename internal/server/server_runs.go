package server

import (
	"log/slog"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/runlog"
)

// SetRunLog records every finished backup run in runs, for reports.
func (s *Server) SetRunLog(runs *runlog.Log) {
	s.handlersMu.Lock()
	s.runs = runs
	s.handlersMu.Unlock()
}

func (s *Server) appendRun(r runlog.Run) {
	s.handlersMu.RLock()
	runs := s.runs
	s.handlersMu.RUnlock()
	if runs == nil {
		return
	}
	if err := runs.Append(r); err != nil {
		slog.Warn("could not record the backup run", "agent", r.Agent, "job", r.JobName, "error", err)
	}
}

// recordRun records a session that ended: stored (resp, with outcomes for
// the objects that produced no item) or failed with errMsg.
func (s *Server) recordRun(session *Session, resp *pb.FinalizeBackupResponse, outcomes []ObjectOutcome, errMsg string) {
	r := runlog.Run{Agent: session.AgentName, Repo: session.RepoName, JobID: session.JobID, Kind: KindFromTags(session.Tags)}
	if job := s.registry.GetJob(session.JobID); job != nil {
		r.JobName, r.Plugin = job.Name, job.Plugin
		if job.JobType == JobTypeWorkload {
			r.Kind = workloadKind(*job, r.Kind)
		}
	}
	r.FailedObjects = failedObjectList(outcomes)
	switch {
	case resp == nil:
		r.Result, r.Error = runlog.ResultFailed, errMsg
	case len(r.FailedObjects) > 0:
		r.Result = runlog.ResultPartial
	default:
		r.Result = runlog.ResultOK
	}
	if resp != nil {
		r.SnapshotID, r.FilesNew, r.BytesAdded = resp.SnapshotId, resp.FilesNew, resp.BytesAdded
		r.BytesProcessed, r.Duration = resp.BytesProcessed, resp.DurationSeconds
	}
	s.appendRun(r)
}

// failedObjectList lists the failed objects of outcomes as "object: reason".
func failedObjectList(outcomes []ObjectOutcome) []string {
	var out []string
	for _, o := range outcomes {
		if o.Status != "failed" {
			continue
		}
		why := o.Message
		if why == "" {
			why = o.Reason
		}
		if why != "" {
			out = append(out, o.Object+": "+why)
		} else {
			out = append(out, o.Object)
		}
	}
	return out
}
