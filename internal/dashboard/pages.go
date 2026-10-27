package dashboard

import (
	"net/http"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/cron"
	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

type pageData struct {
	Title           string // English message key, translated in the layout
	TitleRaw        bool   // Title is user data (job, agent, repo name): shown as is
	TitleID         string // value for the {id} placeholder in Title
	Subtitle        string
	Version         string
	RepoID          string
	Active          string // current nav item
	Content         any
	HasAuth         bool
	UpdateAvailable bool
	LatestVersion   string
	UpdateURL       string
}

func (d *Dashboard) newPageData(title, subtitle, active string, content any) pageData {
	pd := pageData{
		Title:    title,
		Subtitle: subtitle,
		Version:  d.opts.Version,
		RepoID:   d.opts.RepoID,
		Active:   active,
		Content:  content,
		HasAuth:  d.opts.Password != "",
	}
	if v, ok := d.latestVersion.Load().(string); ok && v != "" {
		pd.UpdateAvailable = true
		pd.LatestVersion = v
		if u, ok := d.updateURL.Load().(string); ok {
			pd.UpdateURL = u
		}
	}
	return pd
}

type jobView struct {
	ID               string
	Name             string
	JobType          string
	JobTypeLabel     string
	AgentName        string
	Paths            []string
	SourceLabel      string
	RepoName         string
	ScheduleEnabled  bool
	ScheduleHours    int
	CronExpression   string
	Timezone         string
	ScheduleDisplay  string
	NextRun          string
	LastBackup       *time.Time
	RunNow           bool
	Status           string
	CreatedAt        time.Time
	Running          bool
	Stale            bool
	ElapsedHuman     string
	BytesHuman       string
	Phase            string
	SpeedHuman       string
	DedupPercent     float64
	FilesProcessed   uint64
	FilesNew         uint64
	VerifyEnabled    bool
	LastVerifyAt     *time.Time
	LastVerifyStatus string
	LastVerifyDetail string
	LastVerifySnap   string
	LastVerifyFiles  int
	LastVerifyBytes  uint64
	VerifyOK         bool
	VerifyFailed     bool
}

// buildJobView prepares a job for display; schedule and date text are in
// catalog c's language.
func buildJobView(c *i18n.Catalog, j server.BackupJob, sess *server.SessionInfo, now time.Time) jobView {
	status := "never"
	if j.RunNow {
		status = "pending"
	} else if !j.ScheduleEnabled {
		status = "disabled"
	} else if j.LastBackup != nil {
		window := time.Duration(j.ScheduleHours)*time.Hour + time.Hour
		if j.CronExpression != "" {
			window = 25 * time.Hour
		}
		if now.Sub(*j.LastBackup) > window {
			status = "overdue"
		} else {
			status = "ok"
		}
	}

	schedDisplay := c.Tf("every {n}h", "n", j.ScheduleHours)
	var nextRun string
	if j.CronExpression != "" {
		if expr, err := cron.Parse(j.CronExpression); err == nil {
			schedDisplay = describeCron(c, expr)
			loc := time.Local
			if j.Timezone != "" {
				if tzLoc, tzErr := time.LoadLocation(j.Timezone); tzErr == nil {
					loc = tzLoc
				}
			}
			var from time.Time
			if j.LastBackup != nil {
				from = j.LastBackup.In(loc)
			} else {
				from = now.In(loc)
			}
			next := expr.Next(from)
			if !next.IsZero() {
				nextRun = dateTimeIn(c, next.In(loc))
			}
		}
	} else if j.ScheduleEnabled && j.ScheduleHours > 0 && j.LastBackup != nil {
		next := j.LastBackup.Add(time.Duration(j.ScheduleHours) * time.Hour)
		nextRun = dateTimeIn(c, next)
	}

	v := jobView{
		ID:               j.ID,
		Name:             j.Name,
		JobType:          j.JobType,
		JobTypeLabel:     "Files",
		AgentName:        j.AgentName,
		Paths:            j.Paths,
		SourceLabel:      formatJobSource(c, j),
		RepoName:         j.RepoName,
		ScheduleEnabled:  j.ScheduleEnabled,
		ScheduleHours:    j.ScheduleHours,
		CronExpression:   j.CronExpression,
		VerifyEnabled:    server.JobWantsVerify(j),
		LastVerifyAt:     j.LastVerifyAt,
		LastVerifyStatus: j.LastVerifyStatus,
		LastVerifyDetail: j.LastVerifyDetail,
		LastVerifySnap:   j.LastVerifySnap,
		LastVerifyFiles:  j.LastVerifyFiles,
		LastVerifyBytes:  j.LastVerifyBytes,
		VerifyOK:         j.LastVerifyStatus == "ok",
		VerifyFailed:     j.LastVerifyStatus == "failed" || j.LastVerifyStatus == "error",
		Timezone:         j.Timezone,
		ScheduleDisplay:  schedDisplay,
		NextRun:          nextRun,
		LastBackup:       j.LastBackup,
		RunNow:           j.RunNow,
		Status:           status,
		CreatedAt:        j.CreatedAt,
	}

	if sess != nil {
		v.Status = "running"
		v.Running = true
		v.Stale = time.Since(sess.LastActivity) > staleThreshold
		elapsed := time.Since(sess.StartTime)
		v.ElapsedHuman = FormatDuration(elapsed)
		v.BytesHuman = formatBytes(sess.BytesAdded)
		v.Phase = sess.Phase
		v.FilesProcessed = sess.FilesProcessed
		v.FilesNew = sess.FilesNew
		elapsedSec := elapsed.Seconds()
		if elapsedSec > 0 {
			v.SpeedHuman = formatBytes(uint64(float64(sess.BytesAdded)/elapsedSec)) + "/s"
		}
		if sess.FilesProcessed > 0 {
			v.DedupPercent = float64(sess.FilesProcessed-sess.FilesNew) / float64(sess.FilesProcessed) * 100
		}
	}

	return v
}

func formatJobSource(c *i18n.Catalog, job server.BackupJob) string {
	if len(job.Paths) > 0 {
		if len(job.Paths) > 1 {
			return c.Tf("{path} +{n} more", "path", job.Paths[0], "n", len(job.Paths)-1)
		}
		return job.Paths[0]
	}

	return "—"
}

func (d *Dashboard) handleOverview(w http.ResponseWriter, r *http.Request) {
	agents := d.registry.ListAgents()
	var snapIDs []repository.ID
	if d.repo != nil {
		snapIDs, _ = d.repo.ListSnapshots()
	}

	// Find last backup time across all agents.
	var lastBackup *time.Time
	for _, a := range agents {
		if a.LastBackup != nil {
			if lastBackup == nil || a.LastBackup.After(*lastBackup) {
				lb := *a.LastBackup
				lastBackup = &lb
			}
		}
	}

	// Count stale/ok agents.
	now := time.Now()
	staleThreshold := 25 * time.Hour
	var okCount, staleCount, neverCount int
	for _, a := range agents {
		if a.LastBackup == nil {
			neverCount++
		} else if now.Sub(*a.LastBackup) > staleThreshold {
			staleCount++
		} else {
			okCount++
		}
	}

	// Count active backup sessions.
	var activeBackups int
	var activeAgents []string
	if d.srv != nil {
		sessions := d.srv.AllSessions()
		activeBackups = len(sessions)
		seen := make(map[string]bool)
		for _, s := range sessions {
			if !seen[s.AgentName] {
				seen[s.AgentName] = true
				activeAgents = append(activeAgents, s.AgentName)
			}
		}
	}

	lastBackupTone := "never"
	if lastBackup != nil {
		if now.Sub(*lastBackup) > staleThreshold {
			lastBackupTone = "stale"
		} else {
			lastBackupTone = "ok"
		}
	}

	alerts := d.registry.ListAlerts()
	data := d.newPageData("Overview", "System status at a glance", "overview", map[string]any{
		"AgentCount":     len(agents),
		"SnapshotCount":  len(snapIDs),
		"LastBackup":     lastBackup,
		"LastBackupTone": lastBackupTone,
		"OKCount":        okCount,
		"StaleCount":     staleCount,
		"NeverCount":     neverCount,
		"ActiveBackups":  activeBackups,
		"ActiveAgents":   activeAgents,
		"Alerts":         alerts,
	})

	d.render(w, r, "overview", data)
}

type agentView struct {
	Name         string
	CreatedAt    time.Time
	Managed      bool
	Suspended    bool
	TypeLabel    string
	AgentAddr    string
	LastBackup   *time.Time
	JobCount     int
	BackupStatus string
}

func buildAgentView(a server.AgentRecord, jobCount int, now time.Time) agentView {
	typeLabel := "cli"
	if a.Managed {
		typeLabel = "managed"
	}

	status := "ok"
	if a.Suspended {
		status = "suspended"
	} else if a.LastBackup == nil {
		status = "never"
	} else if now.Sub(*a.LastBackup) > 25*time.Hour {
		status = "stale"
	}

	return agentView{
		Name:         a.Name,
		CreatedAt:    a.CreatedAt,
		Managed:      a.Managed,
		Suspended:    a.Suspended,
		TypeLabel:    typeLabel,
		AgentAddr:    a.AgentAddr,
		LastBackup:   a.LastBackup,
		JobCount:     jobCount,
		BackupStatus: status,
	}
}

func (d *Dashboard) handleAgents(w http.ResponseWriter, r *http.Request) {
	agents := d.registry.ListAgents()
	now := time.Now()

	jobCounts := make(map[string]int)
	for _, j := range d.registry.ListJobs() {
		jobCounts[j.AgentName]++
	}

	views := make([]agentView, 0, len(agents))
	var okCount, staleCount, neverCount, suspendedCount, managedCount int
	for _, a := range agents {
		v := buildAgentView(a, jobCounts[a.Name], now)
		views = append(views, v)
		if v.Managed {
			managedCount++
		}
		if v.Suspended {
			suspendedCount++
		}
		switch v.BackupStatus {
		case "ok":
			okCount++
		case "stale":
			staleCount++
		case "never":
			neverCount++
		}
	}

	data := d.newPageData("Agents", "Registered backup agents", "agents", map[string]any{
		"Agents":         views,
		"Total":          len(views),
		"OKCount":        okCount,
		"StaleCount":     staleCount,
		"NeverCount":     neverCount,
		"SuspendedCount": suspendedCount,
		"ManagedCount":   managedCount,
	})
	d.render(w, r, "agents", data)
}

func (d *Dashboard) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	agent := d.registry.GetAgent(name)
	if agent == nil {
		http.NotFound(w, r)
		return
	}

	now := time.Now()
	jobs := d.registry.ListJobsByAgent(name)

	activeSessions := make(map[string]server.SessionInfo)
	if d.srv != nil {
		for _, s := range d.srv.AllSessions() {
			if s.JobID != "" {
				activeSessions[s.JobID] = s
			}
		}
	}

	jobViews := make([]jobView, 0, len(jobs))
	var okCount, overdueCount, runningCount, pendingCount int
	for _, j := range jobs {
		var sess *server.SessionInfo
		if s, ok := activeSessions[j.ID]; ok {
			sCopy := s
			sess = &sCopy
		}
		jv := buildJobView(d.catalog(r), j, sess, now)
		jobViews = append(jobViews, jv)
		switch jv.Status {
		case "ok":
			okCount++
		case "overdue":
			overdueCount++
		case "running":
			runningCount++
		case "pending":
			pendingCount++
		}
	}

	av := buildAgentView(*agent, len(jobs), now)

	data := d.newPageData(name, "Agent information", "agents", map[string]any{
		"Agent":        av,
		"Jobs":         jobViews,
		"TotalJobs":    len(jobViews),
		"OKCount":      okCount,
		"OverdueCount": overdueCount,
		"ActiveCount":  runningCount + pendingCount,
	})
	data.TitleRaw = true
	d.render(w, r, "agent_detail", data)
}

func (d *Dashboard) handleBackups(w http.ResponseWriter, r *http.Request) {
	jobs := d.registry.ListJobs()
	now := time.Now()

	// Build map of active sessions by JobID.
	activeSessions := make(map[string]server.SessionInfo)
	if d.srv != nil {
		for _, s := range d.srv.AllSessions() {
			if s.JobID != "" {
				activeSessions[s.JobID] = s
			}
		}
	}

	views := make([]jobView, 0, len(jobs))
	for _, j := range jobs {
		var sess *server.SessionInfo
		if s, ok := activeSessions[j.ID]; ok {
			sCopy := s
			sess = &sCopy
		}
		views = append(views, buildJobView(d.catalog(r), j, sess, now))
	}

	var okCount, overdueCount, runningCount, pendingCount int
	for _, v := range views {
		switch v.Status {
		case "ok":
			okCount++
		case "overdue":
			overdueCount++
		case "running":
			runningCount++
		case "pending":
			pendingCount++
		}
	}

	data := d.newPageData("Backups", "Backup jobs and schedules", "backups", map[string]any{
		"Jobs":         views,
		"Total":        len(views),
		"OKCount":      okCount,
		"OverdueCount": overdueCount,
		"RunningCount": runningCount,
		"PendingCount": pendingCount,
		"ActiveCount":  runningCount + pendingCount,
	})
	d.render(w, r, "backups", data)
}

func (d *Dashboard) handleBackupDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := d.registry.GetJob(id)
	if job == nil {
		http.NotFound(w, r)
		return
	}

	// Build lists for dropdowns.
	var agentNames []string
	for _, a := range d.registry.ListAgents() {
		agentNames = append(agentNames, a.Name)
	}
	var repoNames []string
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			repoNames = append(repoNames, mr.Name)
		}
	}

	// Check if agent is managed (for filesystem browsing).
	agent := d.registry.GetAgent(job.AgentName)
	managed := agent != nil && agent.Managed

	// Check for active backup session for this job.
	var progress *sessionProgressJSON
	var sess *server.SessionInfo
	if d.srv != nil {
		for _, s := range d.srv.AllSessions() {
			if s.JobID == id {
				sCopy := s
				sess = &sCopy
				p := sessionInfoToProgress(s)
				progress = &p
				break
			}
		}
	}

	jv := buildJobView(d.catalog(r), *job, sess, time.Now())

	data := d.newPageData(job.Name, "Backup job details", "backups", map[string]any{
		"Job":        job,
		"JobView":    jv,
		"AgentNames": agentNames,
		"RepoNames":  repoNames,
		"Managed":    managed,
		"Progress":   progress,
	})
	data.TitleRaw = true
	d.render(w, r, "backup_detail", data)
}

type snapshotAgentGroup struct {
	Agent     string         `json:"agent"`
	Snapshots []snapshotJSON `json:"snapshots"`
}

func (d *Dashboard) handleStorageDetail(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("name")
	hostFilter := r.URL.Query().Get("host")

	if d.repos == nil {
		http.NotFound(w, r)
		return
	}
	if _, err := d.repos.Get(repoName); err != nil {
		http.NotFound(w, r)
		return
	}

	// Check for active restores.
	var activeRestores []restoreProgress
	if d.restores != nil {
		for _, s := range d.restores.ListSessions() {
			if s.Status == "running" {
				activeRestores = append(activeRestores, s.Progress())
			}
		}
	}

	data := d.newPageData(repoName, "Repository details and snapshots", "storage_detail", map[string]any{
		"RepoName":       repoName,
		"HostFilter":     hostFilter,
		"ActiveRestores": activeRestores,
	})
	data.TitleRaw = true
	d.render(w, r, "storage_detail", data)
}

func (d *Dashboard) handleSnapshotDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")

	// Resolve snapshot across all repos (prefix matching).
	var found *snapshotJSON
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			snapIDs, _ := mr.Repo.ListSnapshots()
			for _, sid := range snapIDs {
				if !strings.HasPrefix(sid.String(), idStr) {
					continue
				}
				snap, err := mr.Repo.LoadSnapshot(sid)
				if err != nil {
					continue
				}
				snapType := snap.Type
				if snapType == "" {
					snapType = "full"
				}
				found = &snapshotJSON{
					ID:         sid.String(),
					Time:       snap.Time.Format(time.RFC3339),
					Hostname:   snap.Hostname,
					Tags:       snap.Tags,
					Paths:      snap.Paths,
					TotalFiles: snap.Summary.TotalFiles,
					TotalSize:  snap.Summary.TotalSize,
					Type:       snapType,
					RepoName:   mr.Name,
				}
				break
			}
			if found != nil {
				break
			}
		}
	}

	if found == nil {
		http.NotFound(w, r)
		return
	}

	// Build agent list for restore dropdown.
	var agentNames []string
	for _, a := range d.registry.ListAgents() {
		if a.AgentAddr != "" {
			agentNames = append(agentNames, a.Name)
		}
	}

	data := d.newPageData("Snapshot {id}", "Recover files from this backup point", "restores", map[string]any{
		"Snapshot": found,
		"Agents":   agentNames,
		"RepoName": found.RepoName,
	})
	data.TitleID = shortID(found.ID)
	d.render(w, r, "snapshot_detail", data)
}

func (d *Dashboard) handleRestores(w http.ResponseWriter, r *http.Request) {
	data := d.newPageData(
		"Restores",
		"Browse snapshots by agent and recover files",
		"restores",
		nil,
	)
	d.render(w, r, "restores", data)
}

func (d *Dashboard) handleStorage(w http.ResponseWriter, r *http.Request) {
	data := d.newPageData("Storage", "Repository storage and health", "storage", nil)
	d.render(w, r, "storage", data)
}
