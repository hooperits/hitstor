package dashboard

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

// DisasterJobUnit is one backup job's contribution to a full-system recovery.
type DisasterJobUnit struct {
	JobID           string    `json:"job_id"`
	JobName         string    `json:"job_name"`
	Paths           []string  `json:"paths"`
	RepoName        string    `json:"repo_name"`
	SnapshotID      string    `json:"snapshot_id,omitempty"`
	SnapshotTime    time.Time `json:"snapshot_time,omitempty"`
	SnapshotTimeStr string    `json:"snapshot_time_str,omitempty"`
	TotalFiles      uint64    `json:"total_files,omitempty"`
	TotalSize       uint64    `json:"total_size,omitempty"`
	VerifyStatus    string    `json:"verify_status,omitempty"`
	VerifyDetail    string    `json:"verify_detail,omitempty"`
	Ready           bool      `json:"ready"`
	SkipReason      string    `json:"skip_reason,omitempty"`
}

// DisasterPlan is a guided recovery package for one protected system (source agent).
type DisasterPlan struct {
	SourceAgent      string            `json:"source_agent"`
	JobCount         int               `json:"job_count"`
	ReadyUnits       int               `json:"ready_units"`
	TotalSize        uint64            `json:"total_size"`
	TotalFiles       uint64            `json:"total_files"`
	Units            []DisasterJobUnit `json:"units"`
	Checklist        []string          `json:"checklist"`
	Warnings         []string          `json:"warnings,omitempty"`
	LastBackup       *time.Time        `json:"last_backup,omitempty"`
	LastBackupStr    string            `json:"last_backup_str,omitempty"`
	SourceOnlineHint string            `json:"source_online_hint,omitempty"`
}

// DisasterPlanSummary is a list-row view of a plan.
type DisasterPlanSummary struct {
	SourceAgent   string `json:"source_agent"`
	JobCount      int    `json:"job_count"`
	ReadyUnits    int    `json:"ready_units"`
	TotalSize     uint64 `json:"total_size"`
	LastBackupStr string `json:"last_backup_str,omitempty"`
	Ready         bool   `json:"ready"`
}

// DisasterRecoveryUnit tracks one job restore within a DR run.
type DisasterRecoveryUnit struct {
	JobID      string `json:"job_id"`
	JobName    string `json:"job_name"`
	SnapshotID string `json:"snapshot_id"`
	RestoreID  string `json:"restore_id,omitempty"`
	Status     string `json:"status"` // pending|running|completed|failed|skipped
	Error      string `json:"error,omitempty"`
}

// DisasterRecovery is an orchestrated multi-job full-system restore.
type DisasterRecovery struct {
	ID          string                 `json:"id"`
	SourceAgent string                 `json:"source_agent"`
	TargetAgent string                 `json:"target_agent"`
	Mode        string                 `json:"mode"` // original|staging
	StagingRoot string                 `json:"staging_root,omitempty"`
	Status      string                 `json:"status"` // running|completed|failed
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time,omitempty"`
	Units       []DisasterRecoveryUnit `json:"units"`
	Checklist   []string               `json:"checklist"`
	mu          sync.Mutex
}

// DisasterManager tracks active disaster recoveries.
type DisasterManager struct {
	mu   sync.Mutex
	byID map[string]*DisasterRecovery
}

func NewDisasterManager() *DisasterManager {
	return &DisasterManager{byID: make(map[string]*DisasterRecovery)}
}

func (m *DisasterManager) Put(r *DisasterRecovery) {
	m.mu.Lock()
	m.byID[r.ID] = r
	m.mu.Unlock()
}

func (m *DisasterManager) Get(id string) *DisasterRecovery {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byID[id]
}

func (m *DisasterManager) List() []*DisasterRecovery {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*DisasterRecovery, 0, len(m.byID))
	for _, r := range m.byID {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTime.After(out[j].StartTime)
	})
	return out
}

// DisasterRecoveryView is a lock-free, serializable copy of a DisasterRecovery.
// DisasterRecovery itself carries a mutex and must never be copied by value.
type DisasterRecoveryView struct {
	ID          string                 `json:"id"`
	SourceAgent string                 `json:"source_agent"`
	TargetAgent string                 `json:"target_agent"`
	Mode        string                 `json:"mode"`
	StagingRoot string                 `json:"staging_root,omitempty"`
	Status      string                 `json:"status"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time,omitempty"`
	Units       []DisasterRecoveryUnit `json:"units"`
	Checklist   []string               `json:"checklist"`
}

func (r *DisasterRecovery) snapshot() DisasterRecoveryView {
	r.mu.Lock()
	defer r.mu.Unlock()
	return DisasterRecoveryView{
		ID:          r.ID,
		SourceAgent: r.SourceAgent,
		TargetAgent: r.TargetAgent,
		Mode:        r.Mode,
		StagingRoot: r.StagingRoot,
		Status:      r.Status,
		StartTime:   r.StartTime,
		EndTime:     r.EndTime,
		Units:       append([]DisasterRecoveryUnit(nil), r.Units...),
		Checklist:   append([]string(nil), r.Checklist...),
	}
}

// snapRef is a snapshot located in a named repository.
type snapRef struct {
	repo     *repository.Repository
	repoName string
	id       string
	summary  cachedSnapshotSummary
}

// buildDisasterPlan constructs a recovery plan for sourceAgent from jobs + snapshots.
func (d *Dashboard) buildDisasterPlan(sourceAgent string) (*DisasterPlan, error) {
	if d.registry == nil {
		return nil, fmt.Errorf("registry not available")
	}
	jobs := d.registry.ListJobsByAgent(sourceAgent)
	// Also include jobs whose AgentName matches case-insensitively.
	if len(jobs) == 0 {
		for _, j := range d.registry.ListJobs() {
			if strings.EqualFold(j.AgentName, sourceAgent) {
				jobs = append(jobs, j)
			}
		}
	}

	plan := &DisasterPlan{
		SourceAgent: sourceAgent,
		Units:       make([]DisasterJobUnit, 0, len(jobs)),
		Checklist:   defaultDRChecklist(sourceAgent),
	}

	if agent := d.registry.GetAgent(sourceAgent); agent != nil {
		if agent.Suspended {
			plan.Warnings = append(plan.Warnings, "source agent is suspended (OK if recovering onto a replacement)")
		}
		if agent.AgentAddr == "" {
			plan.SourceOnlineHint = "source agent has no gRPC address on file"
		} else {
			plan.SourceOnlineHint = "source agent registered at " + agent.AgentAddr
		}
		if agent.LastBackup != nil {
			t := *agent.LastBackup
			plan.LastBackup = &t
			plan.LastBackupStr = t.UTC().Format(time.RFC3339)
		}
	} else {
		plan.Warnings = append(plan.Warnings, "source agent is not in the registry; plan uses hostname matching on snapshots only")
	}

	// Index snapshots by repo for job matching.
	var all []snapRef
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			sums, err := d.getSnapshotSummaries(mr)
			if err != nil {
				continue
			}
			for _, cs := range sums {
				all = append(all, snapRef{repo: mr.Repo, repoName: mr.Name, id: cs.ID, summary: cs})
			}
		}
	} else if d.repo != nil {
		// Single-repo fallback: load via summaries path if possible
		// (getSnapshotSummaries needs ManagedRepo — skip if no manager)
	}

	if len(jobs) == 0 {
		// Fallback: treat hostname-matched latest snapshot as a single unit.
		best := pickLatestForHost(all, sourceAgent)
		if best == nil {
			plan.Warnings = append(plan.Warnings, "no backup jobs or snapshots found for this agent")
			return plan, nil
		}
		u := DisasterJobUnit{
			JobID:           "",
			JobName:         "all snapshots (" + sourceAgent + ")",
			Paths:           best.summary.Paths,
			RepoName:        best.repoName,
			SnapshotID:      best.id,
			SnapshotTime:    best.summary.Time,
			SnapshotTimeStr: best.summary.Time.UTC().Format(time.RFC3339),
			TotalFiles:      best.summary.Summary.TotalFiles,
			TotalSize:       best.summary.Summary.TotalSize,
			Ready:           true,
		}
		plan.Units = append(plan.Units, u)
		plan.JobCount = 1
		plan.ReadyUnits = 1
		plan.TotalFiles = u.TotalFiles
		plan.TotalSize = u.TotalSize
		return plan, nil
	}

	for _, j := range jobs {
		u := DisasterJobUnit{
			JobID:        j.ID,
			JobName:      j.Name,
			Paths:        append([]string(nil), j.Paths...),
			RepoName:     j.RepoName,
			VerifyStatus: j.LastVerifyStatus,
			VerifyDetail: j.LastVerifyDetail,
		}
		if !server.IsFilesystemJobType(j.JobType) {
			u.SkipReason = "non-filesystem job type"
			plan.Units = append(plan.Units, u)
			continue
		}
		ref := pickSnapshotForJob(all, j)
		if ref == nil {
			u.SkipReason = "no matching snapshot found"
			plan.Units = append(plan.Units, u)
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("job %q has no restorable snapshot", j.Name))
			continue
		}
		u.SnapshotID = ref.id
		u.SnapshotTime = ref.summary.Time
		u.SnapshotTimeStr = ref.summary.Time.UTC().Format(time.RFC3339)
		u.TotalFiles = ref.summary.Summary.TotalFiles
		u.TotalSize = ref.summary.Summary.TotalSize
		if u.RepoName == "" {
			u.RepoName = ref.repoName
		}
		u.Ready = true
		if j.LastVerifyStatus == "failed" || j.LastVerifyStatus == "error" {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("job %q last restore verification was %s", j.Name, j.LastVerifyStatus))
		}
		plan.Units = append(plan.Units, u)
		plan.ReadyUnits++
		plan.TotalFiles += u.TotalFiles
		plan.TotalSize += u.TotalSize
		if plan.LastBackup == nil || ref.summary.Time.After(*plan.LastBackup) {
			t := ref.summary.Time
			plan.LastBackup = &t
			plan.LastBackupStr = t.UTC().Format(time.RFC3339)
		}
	}
	plan.JobCount = len(jobs)
	return plan, nil
}

func pickLatestForHost(all []snapRef, host string) *snapRef {
	var best *snapRef
	for i := range all {
		if !strings.EqualFold(all[i].summary.Hostname, host) {
			continue
		}
		if best == nil || all[i].summary.Time.After(best.summary.Time) {
			best = &all[i]
		}
	}
	return best
}

func pickSnapshotForJob(all []snapRef, j server.BackupJob) *snapRef {
	tag := "job:" + j.ID
	var best *snapRef
	for i := range all {
		s := &all[i]
		if j.RepoName != "" && s.repoName != "" && j.RepoName != s.repoName {
			// Prefer matching repo but allow fallback later.
		}
		matchTag := false
		for _, t := range s.summary.Tags {
			if t == tag {
				matchTag = true
				break
			}
		}
		hostMatch := strings.EqualFold(s.summary.Hostname, j.AgentName)
		if !matchTag && !hostMatch {
			continue
		}
		// Prefer tag match over host-only; among equals pick newest.
		if best == nil {
			best = s
			continue
		}
		bestTag := false
		for _, t := range best.summary.Tags {
			if t == tag {
				bestTag = true
				break
			}
		}
		if matchTag && !bestTag {
			best = s
			continue
		}
		if matchTag == bestTag && s.summary.Time.After(best.summary.Time) {
			best = s
		}
	}
	// Repo filter preference: if we have a tagged match in the right repo, use newest among those.
	if j.RepoName != "" && best != nil {
		var repoBest *snapRef
		for i := range all {
			s := &all[i]
			if s.repoName != j.RepoName {
				continue
			}
			for _, t := range s.summary.Tags {
				if t == tag {
					if repoBest == nil || s.summary.Time.After(repoBest.summary.Time) {
						repoBest = s
					}
				}
			}
		}
		if repoBest != nil {
			return repoBest
		}
	}
	return best
}

func defaultDRChecklist(sourceAgent string) []string {
	return []string{
		"Provision replacement hardware or VM with matching OS (Windows or Linux).",
		"Install the HitStor agent on the recovery host and register it with this server.",
		"Ensure the recovery host has enough free disk for all protected paths (or use a staging root).",
		"Stop applications that would lock files on the recovery host before restore.",
		"Run guided recovery: original paths (in-place) or staging root (review then cut over).",
		"After restore completes, verify data and re-enable schedules on the recovery agent.",
		"Optional: run hitstor verify-restore on the restored snapshot for an extra integrity check.",
		fmt.Sprintf("Optional: retire or re-register source agent %q once the new host is production.", sourceAgent),
	}
}

// listDisasterPlanSummaries builds a summary row per agent that has jobs or snapshots.
func (d *Dashboard) listDisasterPlanSummaries() []DisasterPlanSummary {
	agents := map[string]struct{}{}
	if d.registry != nil {
		for _, j := range d.registry.ListJobs() {
			if j.AgentName != "" && server.IsFilesystemJobType(j.JobType) {
				agents[j.AgentName] = struct{}{}
			}
		}
		for _, a := range d.registry.ListAgents() {
			agents[a.Name] = struct{}{}
		}
	}
	out := make([]DisasterPlanSummary, 0, len(agents))
	for name := range agents {
		plan, err := d.buildDisasterPlan(name)
		if err != nil || plan == nil {
			continue
		}
		if plan.JobCount == 0 && plan.ReadyUnits == 0 {
			continue
		}
		out = append(out, DisasterPlanSummary{
			SourceAgent:   plan.SourceAgent,
			JobCount:      plan.JobCount,
			ReadyUnits:    plan.ReadyUnits,
			TotalSize:     plan.TotalSize,
			LastBackupStr: plan.LastBackupStr,
			Ready:         plan.ReadyUnits > 0,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SourceAgent < out[j].SourceAgent
	})
	return out
}

// resolveRepoForSnapshot finds the repository containing snapID prefix/full.
func (d *Dashboard) resolveRepoForSnapshot(snapIDStr string) (*repository.Repository, repository.ID, string, error) {
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			ids, err := mr.Repo.ListSnapshots()
			if err != nil {
				continue
			}
			for _, sid := range ids {
				if sid.String() == snapIDStr || strings.HasPrefix(sid.String(), snapIDStr) {
					return mr.Repo, sid, mr.Name, nil
				}
			}
		}
	}
	if d.repo != nil {
		ids, err := d.repo.ListSnapshots()
		if err == nil {
			for _, sid := range ids {
				if sid.String() == snapIDStr || strings.HasPrefix(sid.String(), snapIDStr) {
					return d.repo, sid, "default", nil
				}
			}
		}
	}
	return nil, repository.ID{}, "", fmt.Errorf("snapshot %q not found", snapIDStr)
}

// startDisasterRecovery validates the request, creates a recovery session, and
// runs job restores sequentially on the target agent.
func (d *Dashboard) startDisasterRecovery(sourceAgent, targetAgent, mode, stagingRoot string, jobIDs []string) (*DisasterRecovery, error) {
	if d.restores == nil {
		return nil, fmt.Errorf("restore manager not initialized")
	}
	if d.disaster == nil {
		d.disaster = NewDisasterManager()
	}
	if targetAgent == "" {
		return nil, fmt.Errorf("target_agent is required")
	}
	agent := d.registry.GetAgent(targetAgent)
	if agent == nil {
		return nil, fmt.Errorf("target agent %q not found — register the recovery host first", targetAgent)
	}
	if agent.AgentAddr == "" {
		if agent.Recovering {
			return nil, fmt.Errorf(
				"target agent %q is marked recovering but announced no address; "+
					"re-run the rescue media's recovery step", targetAgent)
		}
		return nil, fmt.Errorf("target agent %q has no gRPC address", targetAgent)
	}
	if agent.Suspended {
		return nil, fmt.Errorf("target agent %q is suspended", targetAgent)
	}
	if d.restores.HasActiveRestore(targetAgent) {
		return nil, fmt.Errorf("a restore is already running on target agent %q", targetAgent)
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "staging"
	}
	if mode != "original" && mode != "staging" {
		return nil, fmt.Errorf("mode must be original or staging")
	}
	if mode == "staging" && strings.TrimSpace(stagingRoot) == "" {
		return nil, fmt.Errorf("staging_root is required when mode=staging")
	}

	plan, err := d.buildDisasterPlan(sourceAgent)
	if err != nil {
		return nil, err
	}

	want := map[string]bool{}
	for _, id := range jobIDs {
		want[id] = true
	}

	rec := &DisasterRecovery{
		ID:          generateRestoreID(),
		SourceAgent: sourceAgent,
		TargetAgent: targetAgent,
		Mode:        mode,
		StagingRoot: stagingRoot,
		Status:      "running",
		StartTime:   time.Now(),
		Checklist:   plan.Checklist,
	}

	for _, u := range plan.Units {
		if !u.Ready {
			continue
		}
		if len(want) > 0 {
			if u.JobID != "" && !want[u.JobID] {
				continue
			}
			if u.JobID == "" && len(jobIDs) > 0 {
				continue
			}
		}
		rec.Units = append(rec.Units, DisasterRecoveryUnit{
			JobID:      u.JobID,
			JobName:    u.JobName,
			SnapshotID: u.SnapshotID,
			Status:     "pending",
		})
	}
	if len(rec.Units) == 0 {
		return nil, fmt.Errorf("no ready recovery units for agent %q", sourceAgent)
	}

	d.disaster.Put(rec)
	go d.runDisasterRecovery(rec, plan)
	return rec, nil
}

func (d *Dashboard) runDisasterRecovery(rec *DisasterRecovery, plan *DisasterPlan) {
	unitByJob := map[string]DisasterJobUnit{}
	for _, u := range plan.Units {
		unitByJob[u.JobID+"|"+u.SnapshotID] = u
	}

	failed := false
	for i := range rec.Units {
		rec.mu.Lock()
		rec.Units[i].Status = "running"
		rec.mu.Unlock()

		u := rec.Units[i]
		planUnit := unitByJob[u.JobID+"|"+u.SnapshotID]
		// Find plan unit by snapshot if key miss.
		if planUnit.SnapshotID == "" {
			for _, pu := range plan.Units {
				if pu.SnapshotID == u.SnapshotID {
					planUnit = pu
					break
				}
			}
		}

		repo, snapID, _, err := d.resolveRepoForSnapshot(u.SnapshotID)
		if err != nil {
			rec.mu.Lock()
			rec.Units[i].Status = "failed"
			rec.Units[i].Error = err.Error()
			rec.mu.Unlock()
			failed = true
			break
		}

		targetPath := ""
		if rec.Mode == "staging" {
			// Stage each job under staging_root/<job-or-snap>/
			name := planUnit.JobName
			if name == "" {
				name = u.SnapshotID
				if len(name) > 12 {
					name = name[:12]
				}
			}
			name = sanitizePathSegment(name)
			targetPath = strings.TrimRight(rec.StagingRoot, `/\`) + "/" + name
		}

		if d.restores.HasActiveRestore(rec.TargetAgent) {
			// Wait briefly for prior unit to release.
			for n := 0; n < 120 && d.restores.HasActiveRestore(rec.TargetAgent); n++ {
				time.Sleep(500 * time.Millisecond)
			}
		}

		sess := d.restores.StartRestore(snapID.String(), rec.TargetAgent, targetPath, nil)
		rec.mu.Lock()
		rec.Units[i].RestoreID = sess.ID
		rec.mu.Unlock()

		slog.Info("disaster recovery: starting unit",
			"recovery", rec.ID,
			"job", u.JobName,
			"snapshot", u.SnapshotID[:min(12, len(u.SnapshotID))],
			"target_agent", rec.TargetAgent,
			"target_path", targetPath,
		)

		// Run synchronously so units stay sequential on one agent.
		RunRestore(sess, repo, d.registry)

		// Wait until session leaves running (RunRestore sets final status).
		for {
			s := d.restores.GetSession(sess.ID)
			if s == nil {
				break
			}
			s.mu.Lock()
			st := s.Status
			s.mu.Unlock()
			if st != "running" {
				rec.mu.Lock()
				rec.Units[i].Status = st
				if st == "failed" && len(s.Errors) > 0 {
					rec.Units[i].Error = s.Errors[len(s.Errors)-1]
				}
				rec.mu.Unlock()
				if st == "failed" {
					failed = true
				}
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if failed {
			break
		}
	}

	rec.mu.Lock()
	if failed {
		rec.Status = "failed"
	} else {
		rec.Status = "completed"
	}
	rec.EndTime = time.Now()
	rec.mu.Unlock()

	slog.Info("disaster recovery finished",
		"recovery", rec.ID,
		"status", rec.Status,
		"duration", time.Since(rec.StartTime),
	)

	// A bare-metal target stops being a rescue system once the restore it
	// booted for reaches a terminal state, success or not: the rescue agent
	// exits and its address and token stop meaning anything.
	if target := d.registry.GetAgent(rec.TargetAgent); target != nil && target.Recovering {
		if err := d.registry.EndRecovery(rec.TargetAgent); err != nil {
			slog.Warn("clear recovering flag", "agent", rec.TargetAgent, "error", err)
		}
	}
}

func sanitizePathSegment(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "unit"
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
