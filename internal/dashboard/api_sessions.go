package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hooperits/hitstor/internal/server"
)

// sessionProgressJSON is the JSON representation of an active backup session's progress.
type sessionProgressJSON struct {
	SessionID      string  `json:"session_id"`
	AgentName      string  `json:"agent_name"`
	Hostname       string  `json:"hostname"`
	JobID          string  `json:"job_id"`
	RepoName       string  `json:"repo_name"`
	BackupType     string  `json:"backup_type"`
	Phase          string  `json:"phase"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	ElapsedHuman   string  `json:"elapsed_human"`
	BytesUploaded  uint64  `json:"bytes_uploaded"`
	BytesHuman     string  `json:"bytes_human"`
	FilesProcessed uint64  `json:"files_processed"`
	FilesNew       uint64  `json:"files_new"`
	Stale          bool    `json:"stale"`
	SpeedBps       uint64  `json:"speed_bps"`
	SpeedHuman     string  `json:"speed_human"`
	DedupPercent   float64 `json:"dedup_percent"`
	Warning        string  `json:"warning,omitempty"`
}

const staleThreshold = 10 * time.Minute

func sessionInfoToProgress(si server.SessionInfo) sessionProgressJSON {
	elapsed := time.Since(si.StartTime)
	stale := time.Since(si.LastActivity) > staleThreshold

	var speedBps uint64
	var speedHuman string
	elapsedSec := elapsed.Seconds()
	if elapsedSec > 0 {
		speedBps = uint64(float64(si.BytesAdded) / elapsedSec)
		speedHuman = formatBytes(speedBps) + "/s"
	}

	var dedupPercent float64
	if si.FilesProcessed > 0 {
		dedupPercent = float64(si.FilesProcessed-si.FilesNew) / float64(si.FilesProcessed) * 100
	}

	return sessionProgressJSON{
		SessionID:      si.ID,
		AgentName:      si.AgentName,
		Hostname:       si.Hostname,
		JobID:          si.JobID,
		RepoName:       si.RepoName,
		BackupType:     si.BackupType,
		Phase:          si.Phase,
		ElapsedSeconds: elapsedSec,
		ElapsedHuman:   FormatDuration(elapsed),
		BytesUploaded:  si.BytesAdded,
		BytesHuman:     formatBytes(si.BytesAdded),
		FilesProcessed: si.FilesProcessed,
		FilesNew:       si.FilesNew,
		Stale:          stale,
		SpeedBps:       speedBps,
		SpeedHuman:     speedHuman,
		DedupPercent:   dedupPercent,
	}
}

// handleAPISessions returns active backup sessions for a repo with full progress.
func (d *Dashboard) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if d.srv == nil {
		writeJSON(w, []any{})
		return
	}

	sessions := d.srv.SessionsByRepo(name)
	result := make([]sessionProgressJSON, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, sessionInfoToProgress(s))
	}
	writeJSON(w, result)
}

// handleAPIActiveBackups returns a summary of all active backup sessions.
func (d *Dashboard) handleAPIActiveBackups(w http.ResponseWriter, r *http.Request) {
	if d.srv == nil {
		writeJSON(w, map[string]any{"count": 0, "sessions": []any{}})
		return
	}

	sessions := d.srv.AllSessions()
	result := make([]sessionProgressJSON, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, sessionInfoToProgress(s))
	}
	writeJSON(w, map[string]any{
		"count":    len(result),
		"sessions": result,
	})
}

// handleAPIBackupProgressEvents streams SSE events for backup progress.
func (d *Dashboard) handleAPIBackupProgressEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	idleCount := 0
	const maxIdle = 150 // 150 * 2s = 5 minutes

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		var sessions []sessionProgressJSON
		if d.srv != nil {
			for _, s := range d.srv.AllSessions() {
				sessions = append(sessions, sessionInfoToProgress(s))
			}
		}
		data, _ := json.Marshal(map[string]any{"sessions": sessions})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if len(sessions) == 0 {
			idleCount++
			if idleCount >= maxIdle {
				return
			}
		} else {
			idleCount = 0
		}
	}
}
