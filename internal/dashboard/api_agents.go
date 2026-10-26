package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"log/slog"
	"strings"
)

type agentJSON struct {
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
	LastBackup      *string `json:"last_backup"`
	SnapshotCount   int     `json:"snapshot_count"`
	ScheduleEnabled bool    `json:"schedule_enabled"`
	ScheduleHours   int     `json:"schedule_hours"`
	Suspended       bool    `json:"suspended"`
}

func (d *Dashboard) handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	agents := d.registry.ListAgents()

	// Count snapshots per host across all repositories.
	hostCounts := make(map[string]int)
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			summaries, err := d.getSnapshotSummaries(mr)
			if err != nil {
				continue
			}
			for _, cs := range summaries {
				for _, a := range agents {
					if d.snapshotMatchesAgent(cs, a.Name) {
						hostCounts[a.Name]++
					}
				}
			}
		}
	} else if d.repo != nil {
		snapIDs, _ := d.repo.ListSnapshots()
		for _, id := range snapIDs {
			snap, err := d.repo.LoadSnapshot(id)
			if err != nil {
				continue
			}
			cs := cachedSnapshotSummary{Hostname: snap.Hostname, Tags: snap.Tags}
			for _, a := range agents {
				if d.snapshotMatchesAgent(cs, a.Name) {
					hostCounts[a.Name]++
				}
			}
		}
	}

	now := time.Now()
	staleThreshold := 25 * time.Hour

	result := make([]agentJSON, 0, len(agents))
	for _, a := range agents {
		status := "never"
		var lastBackup *string
		if a.LastBackup != nil {
			ts := a.LastBackup.Format(time.RFC3339)
			lastBackup = &ts
			if now.Sub(*a.LastBackup) > staleThreshold {
				status = "stale"
			} else {
				status = "ok"
			}
		}
		if a.Suspended {
			status = "suspended"
		}

		result = append(result, agentJSON{
			Name:            a.Name,
			Status:          status,
			CreatedAt:       a.CreatedAt.Format(time.RFC3339),
			LastBackup:      lastBackup,
			SnapshotCount:   hostCounts[a.Name],
			ScheduleEnabled: a.ScheduleEnabled,
			ScheduleHours:   a.ScheduleHours,
			Suspended:       a.Suspended,
		})
	}

	writeJSON(w, result)
}

type scheduleUpdateRequest struct {
	Enabled       bool `json:"enabled"`
	IntervalHours int  `json:"interval_hours"`
}

func (d *Dashboard) handleAPIAgentSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Verify agent exists.
	agents := d.registry.ListAgents()
	var found bool
	for _, a := range agents {
		if a.Name == name {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	var req scheduleUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Enabled && req.IntervalHours < 1 {
		http.Error(w, "interval_hours must be at least 1", http.StatusBadRequest)
		return
	}

	if err := d.registry.UpdateSchedule(name, req.Enabled, req.IntervalHours); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

type repoUpdateRequest struct {
	RepoName string `json:"repo_name"`
}

func (d *Dashboard) handleAPIAgentRepo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	agents := d.registry.ListAgents()
	var found bool
	for _, a := range agents {
		if a.Name == name {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	var req repoUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate repo exists.
	if req.RepoName != "" {
		if _, err := d.repos.Get(req.RepoName); err != nil {
			http.Error(w, "repository not found", http.StatusBadRequest)
			return
		}
	}

	if err := d.registry.UpdateRepoName(name, req.RepoName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

type addAgentRequest struct {
	Name      string `json:"name"`
	AgentAddr string `json:"agent_addr"`
	Token     string `json:"token"`
}

// handleAPIAddAgent registers a new agent from the dashboard by connecting
// to the agent's gRPC server, authenticating with its token, and sending
// it the API key and server address for backup pushes.
func (d *Dashboard) handleAPIAddAgent(w http.ResponseWriter, r *http.Request) {
	var req addAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.AgentAddr == "" || req.Token == "" {
		http.Error(w, "name, agent_addr, and token are required", http.StatusBadRequest)
		return
	}
	// Auto-append default agent port if not specified.
	if !strings.Contains(req.AgentAddr, ":") {
		req.AgentAddr = req.AgentAddr + ":8422"
	}
	if len(req.Token) != 64 {
		http.Error(w, "token must be a 64-character hex string", http.StatusBadRequest)
		return
	}

	// Ensure registry has a backing store. If the server started in setup
	// mode and a repo was created later, the registry may still lack backing.
	if !d.registry.HasBacking() {
		if repoList := d.repos.List(); len(repoList) > 0 {
			mr := repoList[0]
			if bp, ok := mr.Backend.(interface{ BasePath() string }); ok {
				d.registry.InitBacking(mr.Repo.Key().EncryptKey, bp.BasePath())
			}
		}
		if !d.registry.HasBacking() {
			http.Error(w, "no repository configured — create a repository before adding agents", http.StatusBadRequest)
			return
		}
	}

	// Check for existing agent — support re-registration after reinstall.
	var apiKey string
	existing := d.registry.GetAgent(req.Name)
	if existing != nil {
		// Same name — re-register (update token + generate new API key).
		var err error
		apiKey, err = d.registry.ReRegisterAgent(req.Name, req.Token)
		if err != nil {
			http.Error(w, fmt.Sprintf("re-register agent: %v", err), http.StatusInternalServerError)
			return
		}
		slog.Info("agent re-registered", "name", req.Name, "addr", req.AgentAddr)
	} else {
		// New agent — register fresh.
		var err error
		apiKey, err = d.registry.AddManagedAgent(req.Name, req.AgentAddr, req.Token)
		if err != nil {
			http.Error(w, fmt.Sprintf("register agent: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Connect to the agent and send it the API key + server address.
	// The agent record now exists, so this first dial records the TOFU pin.
	client, err := d.dialManagedAgent(req.Name, req.AgentAddr)
	if err != nil {
		// Registration succeeded in registry but we can't reach the agent.
		writeJSON(w, map[string]any{
			"name":    req.Name,
			"warning": fmt.Sprintf("Agent registered but unreachable: %v. It will connect when online.", err),
		})
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	resp, err := client.Register(ctx, req.Token, apiKey, d.serverGRPCAddr, nil)
	if err != nil {
		writeJSON(w, map[string]any{
			"name":    req.Name,
			"warning": fmt.Sprintf("Agent registered but registration RPC failed: %v", err),
		})
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"name":     req.Name,
		"hostname": resp.Hostname,
		"version":  resp.Version,
		"os":       resp.Os,
		"arch":     resp.Arch,
		"message":  "Agent registered successfully",
	})
}

type browseRequest struct {
	Path string `json:"path"`
}

// handleAPIAgentBrowse proxies a ListDirectory call to the agent.
func (d *Dashboard) handleAPIAgentBrowse(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	agent := d.registry.GetAgent(name)
	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !agent.Managed {
		http.Error(w, "filesystem browsing is only available for dashboard-managed agents", http.StatusBadRequest)
		return
	}

	var req browseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	token, err := d.registry.DecryptToken(name)
	if err != nil {
		http.Error(w, fmt.Sprintf("decrypt token: %v", err), http.StatusInternalServerError)
		return
	}

	client, err := d.dialManagedAgent(name, agent.AgentAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("connect to agent: %v", err), http.StatusBadGateway)
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	entries, err := client.ListDirectory(ctx, token, req.Path)
	if err != nil {
		http.Error(w, fmt.Sprintf("list directory: %v", err), http.StatusBadGateway)
		return
	}

	// Convert proto entries to JSON-friendly format.
	type dirEntry struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		IsDir       bool   `json:"is_dir"`
		Size        int64  `json:"size"`
		Mode        uint32 `json:"mode"`
		Readable    bool   `json:"readable"`
		HasChildren bool   `json:"has_children"`
	}

	result := make([]dirEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, dirEntry{
			Name:        e.Name,
			Path:        e.Path,
			IsDir:       e.IsDir,
			Size:        e.Size,
			Mode:        e.Mode,
			Readable:    e.Readable,
			HasChildren: e.HasChildren,
		})
	}

	writeJSON(w, result)
}

type configureAgentRequest struct {
	Paths    []string `json:"paths"`
	Excludes []string `json:"excludes"`
}

// handleAPIAgentConfigure sends backup paths and excludes to the agent.
func (d *Dashboard) handleAPIAgentConfigure(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	agent := d.registry.GetAgent(name)
	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !agent.Managed {
		http.Error(w, "configuration is only available for dashboard-managed agents", http.StatusBadRequest)
		return
	}

	var req configureAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.Paths) == 0 {
		http.Error(w, "at least one backup path is required", http.StatusBadRequest)
		return
	}

	token, err := d.registry.DecryptToken(name)
	if err != nil {
		http.Error(w, fmt.Sprintf("decrypt token: %v", err), http.StatusInternalServerError)
		return
	}

	client, err := d.dialManagedAgent(name, agent.AgentAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("connect to agent: %v", err), http.StatusBadGateway)
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := client.Configure(ctx, token, req.Paths, req.Excludes); err != nil {
		http.Error(w, fmt.Sprintf("configure agent: %v", err), http.StatusBadGateway)
		return
	}

	writeJSON(w, map[string]any{"ok": true, "message": "Backup paths configured"})
}

// handleAPIAgentInfo returns the agent's current status and configuration.
func (d *Dashboard) handleAPIAgentInfo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	agent := d.registry.GetAgent(name)
	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !agent.Managed {
		http.Error(w, "agent info is only available for dashboard-managed agents", http.StatusBadRequest)
		return
	}

	token, err := d.registry.DecryptToken(name)
	if err != nil {
		http.Error(w, fmt.Sprintf("decrypt token: %v", err), http.StatusInternalServerError)
		return
	}

	client, err := d.dialManagedAgent(name, agent.AgentAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("connect to agent: %v", err), http.StatusBadGateway)
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	info, err := client.AgentInfo(ctx, token)
	if err != nil {
		http.Error(w, fmt.Sprintf("agent info: %v", err), http.StatusBadGateway)
		return
	}

	writeJSON(w, map[string]any{
		"hostname":     info.Hostname,
		"version":      info.Version,
		"os":           info.Os,
		"arch":         info.Arch,
		"backup_paths": info.BackupPaths,
		"excludes":     info.Excludes,
		"registered":   info.Registered,
		"server_addr":  info.ServerAddr,
	})
}

// handleAPIRemoveAgent removes an agent from the registry.
// Query param ?keep_jobs=true (default true) preserves associated jobs.
func (d *Dashboard) handleAPIRemoveAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	keepJobs := true // safer default
	if v := r.URL.Query().Get("keep_jobs"); v != "" {
		if v == "false" || v == "0" {
			keepJobs = false
		}
	}

	if err := d.registry.RemoveAgent(name, !keepJobs); err != nil {
		http.Error(w, fmt.Sprintf("remove agent: %v", err), http.StatusNotFound)
		return
	}

	msg := fmt.Sprintf("Agent %q removed", name)
	if keepJobs {
		msg += " (jobs preserved)"
	} else {
		msg += " (jobs deleted)"
	}
	writeJSON(w, map[string]any{"ok": true, "message": msg})
}

// handleAPIAgentSuspend handles POST /api/agents/{name}/suspend.
func (d *Dashboard) handleAPIAgentSuspend(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if err := d.registry.SuspendAgent(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"success": true})
}

// handleAPIAgentUnsuspend handles POST /api/agents/{name}/unsuspend.
func (d *Dashboard) handleAPIAgentUnsuspend(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if err := d.registry.UnsuspendAgent(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"success": true})
}

// handleAPIGetAlerts handles GET /api/alerts.
func (d *Dashboard) handleAPIGetAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := d.registry.ListAlerts()
	writeJSON(w, alerts)
}

// handleAPIResolveAlert handles POST /api/alerts/{id}/resolve.
func (d *Dashboard) handleAPIResolveAlert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := d.registry.ResolveAlert(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"success": true})
}
