package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"log/slog"
	"strings"

	"github.com/hooperits/hitstor/internal/cron"
	"github.com/hooperits/hitstor/internal/server"
)

type backupJobJSON struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	AgentName         string   `json:"agent_name"`
	RepoName          string   `json:"repo_name"`
	Paths             []string `json:"paths"`
	Excludes          []string `json:"excludes,omitempty"`
	ScheduleEnabled   bool     `json:"schedule_enabled"`
	ScheduleHours     int      `json:"schedule_hours"`
	CronExpression    string   `json:"cron_expression,omitempty"`
	Timezone          string   `json:"timezone,omitempty"`
	CronDescription   string   `json:"cron_description,omitempty"`
	NextRun           string   `json:"next_run,omitempty"`
	LastBackup        *string  `json:"last_backup"`
	RunNow            bool     `json:"run_now"`
	Status            string   `json:"status"`
	CreatedAt         string   `json:"created_at"`
	FullRescanDays    int      `json:"full_rescan_days"`
	LastFullBackup    *string  `json:"last_full_backup"`
	UseVSS            bool     `json:"use_vss"`
	JobType           string   `json:"job_type,omitempty"`
	VerifyAfterBackup bool     `json:"verify_after_backup"`
	LastVerifyAt      *string  `json:"last_verify_at,omitempty"`
	LastVerifyStatus  string   `json:"last_verify_status,omitempty"`
	LastVerifySnap    string   `json:"last_verify_snapshot,omitempty"`
	LastVerifyDetail  string   `json:"last_verify_detail,omitempty"`
	LastVerifyFiles   int      `json:"last_verify_files,omitempty"`
	LastVerifyBytes   uint64   `json:"last_verify_bytes,omitempty"`
}

func jobToJSON(j server.BackupJob) backupJobJSON {
	var lastBackup *string
	if j.LastBackup != nil {
		ts := j.LastBackup.Format(time.RFC3339)
		lastBackup = &ts
	}
	var lastFullBackup *string
	if j.LastFullBackup != nil {
		ts := j.LastFullBackup.Format(time.RFC3339)
		lastFullBackup = &ts
	}
	status := "never"
	if j.RunNow {
		status = "pending"
	} else if !j.ScheduleEnabled {
		status = "disabled"
	} else if j.LastBackup != nil {
		window := time.Duration(j.ScheduleHours)*time.Hour + time.Hour
		if j.CronExpression != "" {
			// For cron jobs, use 2x the interval to the next occurrence as the overdue window.
			window = 25 * time.Hour
		}
		if time.Since(*j.LastBackup) > window {
			status = "overdue"
		} else {
			status = "ok"
		}
	}

	result := backupJobJSON{
		ID:                j.ID,
		Name:              j.Name,
		AgentName:         j.AgentName,
		RepoName:          j.RepoName,
		Paths:             j.Paths,
		Excludes:          j.Excludes,
		ScheduleEnabled:   j.ScheduleEnabled,
		ScheduleHours:     j.ScheduleHours,
		CronExpression:    j.CronExpression,
		Timezone:          j.Timezone,
		LastBackup:        lastBackup,
		RunNow:            j.RunNow,
		Status:            status,
		CreatedAt:         j.CreatedAt.Format(time.RFC3339),
		FullRescanDays:    j.FullRescanDays,
		LastFullBackup:    lastFullBackup,
		UseVSS:            j.UseVSS,
		JobType:           j.JobType,
		VerifyAfterBackup: server.JobWantsVerify(j),
		LastVerifyStatus:  j.LastVerifyStatus,
		LastVerifySnap:    j.LastVerifySnap,
		LastVerifyDetail:  j.LastVerifyDetail,
		LastVerifyFiles:   j.LastVerifyFiles,
		LastVerifyBytes:   j.LastVerifyBytes,
	}
	if j.LastVerifyAt != nil {
		ts := j.LastVerifyAt.Format(time.RFC3339)
		result.LastVerifyAt = &ts
	}

	// Compute cron description and next run time.
	if j.CronExpression != "" {
		if expr, err := cron.Parse(j.CronExpression); err == nil {
			result.CronDescription = expr.Describe()

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
				from = time.Now().In(loc)
			}
			next := expr.Next(from)
			if !next.IsZero() {
				result.NextRun = next.In(loc).Format(time.RFC3339)
			}
		}
	} else if j.ScheduleEnabled && j.ScheduleHours > 0 && j.LastBackup != nil {
		nextRun := j.LastBackup.Add(time.Duration(j.ScheduleHours) * time.Hour)
		result.NextRun = nextRun.Format(time.RFC3339)
	}

	return result
}

func (d *Dashboard) handleAPIBackupJobs(w http.ResponseWriter, r *http.Request) {
	jobs := d.registry.ListJobs()
	result := make([]backupJobJSON, 0, len(jobs))
	for _, j := range jobs {
		result = append(result, jobToJSON(j))
	}
	writeJSON(w, result)
}

type createBackupJobRequest struct {
	Name            string   `json:"name"`
	AgentName       string   `json:"agent_name"`
	RepoName        string   `json:"repo_name"`
	Paths           []string `json:"paths"`
	Excludes        []string `json:"excludes"`
	ScheduleEnabled bool     `json:"schedule_enabled"`
	ScheduleHours   int      `json:"schedule_hours"`
	CronExpression  string   `json:"cron_expression"`
	Timezone        string   `json:"timezone"`
	FullRescanDays  int      `json:"full_rescan_days"`
	UseVSS          bool     `json:"use_vss"`
	JobType         string   `json:"job_type"`
}

// validateJobType rejects anything but filesystem jobs: HitStor backs up
// filesystems only.
func validateJobType(jobType string) error {
	if jobType != "" && jobType != "files" {
		return fmt.Errorf("unsupported job type %q: HitStor supports filesystem backup jobs only", jobType)
	}
	return nil
}

func (d *Dashboard) handleAPICreateBackupJob(w http.ResponseWriter, r *http.Request) {
	var req createBackupJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.RepoName == "" {
		http.Error(w, "name and repo_name are required", http.StatusBadRequest)
		return
	}
	if err := validateJobType(req.JobType); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.AgentName == "" {
		http.Error(w, "agent_name is required for backup jobs", http.StatusBadRequest)
		return
	}
	if len(req.Paths) == 0 {
		http.Error(w, "paths are required for filesystem backup jobs", http.StatusBadRequest)
		return
	}

	// Validate agent exists.
	if d.registry.GetAgent(req.AgentName) == nil {
		http.Error(w, "agent not found", http.StatusBadRequest)
		return
	}
	// Validate repo exists.
	if d.repos != nil {
		if _, err := d.repos.Get(req.RepoName); err != nil {
			http.Error(w, "repository not found", http.StatusBadRequest)
			return
		}
	}

	// Validate cron expression if provided.
	if req.CronExpression != "" {
		if _, err := cron.Parse(req.CronExpression); err != nil {
			http.Error(w, fmt.Sprintf("invalid cron expression: %v", err), http.StatusBadRequest)
			return
		}
	}
	// Validate timezone if provided.
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			http.Error(w, fmt.Sprintf("unknown timezone: %s", req.Timezone), http.StatusBadRequest)
			return
		}
	}

	var idBytes [8]byte
	rand.Read(idBytes[:])

	job := server.BackupJob{
		ID:              hex.EncodeToString(idBytes[:]),
		Name:            req.Name,
		AgentName:       req.AgentName,
		RepoName:        req.RepoName,
		Paths:           req.Paths,
		Excludes:        req.Excludes,
		ScheduleEnabled: req.ScheduleEnabled,
		ScheduleHours:   req.ScheduleHours,
		CronExpression:  req.CronExpression,
		Timezone:        req.Timezone,
		UseVSS:          req.UseVSS,
		CreatedAt:       time.Now(),
		JobType:         "files",
	}

	if err := d.registry.AddJob(job); err != nil {
		http.Error(w, fmt.Sprintf("create job: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, jobToJSON(job))
}

func (d *Dashboard) handleAPIGetBackupJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := d.registry.GetJob(id)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, jobToJSON(*job))
}

func (d *Dashboard) handleAPIUpdateBackupJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing := d.registry.GetJob(id)
	if existing == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	var req createBackupJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := validateJobType(req.JobType); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate cron expression if provided.
	if req.CronExpression != "" {
		if _, err := cron.Parse(req.CronExpression); err != nil {
			http.Error(w, fmt.Sprintf("invalid cron expression: %v", err), http.StatusBadRequest)
			return
		}
	}
	// Validate timezone if provided.
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			http.Error(w, fmt.Sprintf("unknown timezone: %s", req.Timezone), http.StatusBadRequest)
			return
		}
	}

	// Apply updates, keeping immutable fields.
	updated := *existing
	if req.Name != "" {
		updated.Name = req.Name
	}
	if req.AgentName != "" {
		updated.AgentName = req.AgentName
	}
	if req.RepoName != "" {
		updated.RepoName = req.RepoName
	}
	if len(req.Paths) > 0 {
		updated.Paths = req.Paths
	}
	updated.Excludes = req.Excludes
	updated.ScheduleEnabled = req.ScheduleEnabled
	updated.ScheduleHours = req.ScheduleHours
	updated.CronExpression = req.CronExpression
	updated.Timezone = req.Timezone
	updated.FullRescanDays = req.FullRescanDays
	updated.UseVSS = req.UseVSS
	updated.JobType = "files"

	if err := d.registry.UpdateJob(updated); err != nil {
		http.Error(w, fmt.Sprintf("update job: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, jobToJSON(updated))
}

func (d *Dashboard) handleAPIDeleteBackupJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := d.registry.RemoveJob(id); err != nil {
		http.Error(w, fmt.Sprintf("delete job: %v", err), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": "Backup job deleted"})
}

// handleAPIRunBackupJob triggers an immediate backup for a job.
func (d *Dashboard) handleAPIRunBackupJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := d.registry.GetJob(id)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if len(job.Paths) == 0 {
		http.Error(w, "job has no paths configured", http.StatusBadRequest)
		return
	}
	if job.RunNow {
		http.Error(w, "backup already pending", http.StatusConflict)
		return
	}

	var body struct {
		ForceFull bool `json:"force_full"`
	}
	// Body is optional; ignore decode errors for empty body.
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.ForceFull {
		if err := d.registry.SetJobForceFullScan(id, true); err != nil {
			http.Error(w, fmt.Sprintf("set force full: %v", err), http.StatusInternalServerError)
			return
		}
	}

	if err := d.registry.SetJobRunNow(id, true); err != nil {
		http.Error(w, fmt.Sprintf("set run now: %v", err), http.StatusInternalServerError)
		return
	}

	// Wake up the agent so it picks up the job instantly.
	msg := "Backup triggered."
	agent := d.registry.GetAgent(job.AgentName)
	if agent != nil && agent.AgentAddr != "" {
		go func() {
			token, err := d.registry.DecryptToken(job.AgentName)
			if err != nil {
				slog.Warn("wakeup: decrypt token failed", "agent", job.AgentName, "error", err)
				return
			}
			client, err := d.dialManagedAgent(job.AgentName, agent.AgentAddr)
			if err != nil {
				slog.Warn("wakeup: dial agent failed", "agent", job.AgentName, "error", err)
				return
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := client.WakeUp(ctx, token); err != nil {
				slog.Warn("wakeup: RPC failed", "agent", job.AgentName, "error", err)
			} else {
				slog.Info("agent woken up for immediate backup", "agent", job.AgentName, "job", job.Name)
			}
		}()
		msg = "Backup triggered. Agent notified."
	} else {
		msg += " The agent will start within 5 minutes."
	}
	if body.ForceFull {
		msg = "Full " + strings.ToLower(msg)
	}
	writeJSON(w, map[string]any{"ok": true, "message": msg})
}

// handleAPICancelBackupJob cancels a running or pending backup for a job.
func (d *Dashboard) handleAPICancelBackupJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := d.registry.GetJob(id)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	// Clear RunNow flag (handles PENDING state).
	if job.RunNow {
		if err := d.registry.SetJobRunNow(id, false); err != nil {
			slog.Warn("cancel: clear RunNow failed", "job", id, "error", err)
		}
	}

	// Cancel the active session if one exists.
	var cancelled bool
	if d.srv != nil {
		cancelled = d.srv.CancelSessionByJobID(id)
	}

	msg := "No active backup to cancel."
	if cancelled || job.RunNow {
		msg = "Backup cancelled."
		slog.Info("backup cancelled by dashboard", "job_id", id, "job_name", job.Name, "had_session", cancelled)
	}

	writeJSON(w, map[string]any{"ok": true, "message": msg})
}

func (d *Dashboard) handleAPIValidateCron(w http.ResponseWriter, r *http.Request) {
	expr := r.URL.Query().Get("expr")
	if expr == "" {
		writeJSON(w, map[string]any{"valid": false, "error": "expr parameter required"})
		return
	}

	parsed, err := cron.Parse(expr)
	if err != nil {
		writeJSON(w, map[string]any{"valid": false, "error": err.Error()})
		return
	}

	loc := time.Local
	if tz := r.URL.Query().Get("tz"); tz != "" {
		if tzLoc, tzErr := time.LoadLocation(tz); tzErr == nil {
			loc = tzLoc
		}
	}

	now := time.Now().In(loc)
	var next3 []string
	t := now
	for i := 0; i < 3; i++ {
		next := parsed.Next(t)
		if next.IsZero() {
			break
		}
		next3 = append(next3, next.In(loc).Format(time.RFC3339))
		t = next
	}

	writeJSON(w, map[string]any{
		"valid":       true,
		"description": parsed.Describe(),
		"next_3":      next3,
	})
}

func (d *Dashboard) handleAPITimezones(w http.ResponseWriter, r *http.Request) {
	// Common IANA timezones. The full list from the embedded tzdata would
	// require parsing the zip, so we provide a curated set covering all
	// major regions.
	zones := []string{
		"Africa/Cairo", "Africa/Johannesburg", "Africa/Lagos", "Africa/Nairobi",
		"America/Anchorage", "America/Argentina/Buenos_Aires", "America/Bogota",
		"America/Chicago", "America/Denver", "America/Halifax",
		"America/Los_Angeles", "America/Mexico_City", "America/New_York",
		"America/Phoenix", "America/Santiago", "America/Sao_Paulo",
		"America/St_Johns", "America/Toronto", "America/Vancouver",
		"Asia/Bangkok", "Asia/Colombo", "Asia/Dubai", "Asia/Hong_Kong",
		"Asia/Istanbul", "Asia/Jakarta", "Asia/Karachi", "Asia/Kolkata",
		"Asia/Kuala_Lumpur", "Asia/Manila", "Asia/Seoul", "Asia/Shanghai",
		"Asia/Singapore", "Asia/Taipei", "Asia/Tehran", "Asia/Tokyo",
		"Atlantic/Reykjavik",
		"Australia/Adelaide", "Australia/Brisbane", "Australia/Melbourne",
		"Australia/Perth", "Australia/Sydney",
		"Europe/Amsterdam", "Europe/Athens", "Europe/Berlin", "Europe/Brussels",
		"Europe/Bucharest", "Europe/Dublin", "Europe/Helsinki", "Europe/Kiev",
		"Europe/Lisbon", "Europe/London", "Europe/Madrid", "Europe/Moscow",
		"Europe/Oslo", "Europe/Paris", "Europe/Prague", "Europe/Rome",
		"Europe/Stockholm", "Europe/Vienna", "Europe/Warsaw", "Europe/Zurich",
		"Pacific/Auckland", "Pacific/Fiji", "Pacific/Honolulu",
		"UTC",
	}
	sort.Strings(zones)
	writeJSON(w, zones)
}
