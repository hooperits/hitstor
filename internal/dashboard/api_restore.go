package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"strings"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/repository"
)

// handleAPIStartRestore handles POST /api/snapshots/{id}/restore.
func (d *Dashboard) handleAPIStartRestore(w http.ResponseWriter, r *http.Request) {
	snapIDStr := r.PathValue("id")

	var req struct {
		AgentName  string   `json:"agent_name"`
		TargetPath string   `json:"target_path"`
		Includes   []string `json:"includes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.AgentName == "" {
		http.Error(w, "agent_name is required", http.StatusBadRequest)
		return
	}

	// Validate agent exists and has gRPC address.
	agent := d.registry.GetAgent(req.AgentName)
	if agent == nil {
		http.Error(w, fmt.Sprintf("agent %q not found", req.AgentName), http.StatusNotFound)
		return
	}
	if agent.AgentAddr == "" {
		http.Error(w, fmt.Sprintf("agent %q has no gRPC address", req.AgentName), http.StatusBadRequest)
		return
	}

	// Check for concurrent restore on this agent (FR-011).
	if d.restores != nil && d.restores.HasActiveRestore(req.AgentName) {
		http.Error(w, "a restore is already in progress on this agent", http.StatusConflict)
		return
	}

	// Resolve snapshot across repos.
	var repo *repository.Repository
	var snapID repository.ID
	var found bool
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			snapIDs, _ := mr.Repo.ListSnapshots()
			for _, sid := range snapIDs {
				if strings.HasPrefix(sid.String(), snapIDStr) {
					repo = mr.Repo
					snapID = sid
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	if !found {
		http.Error(w, "snapshot not found", http.StatusNotFound)
		return
	}

	sess := d.restores.StartRestore(snapID.String(), req.AgentName, req.TargetPath, req.Includes)

	go RunRestore(sess, repo, d.registry)

	writeJSON(w, map[string]any{
		"restore_id":  sess.ID,
		"status":      sess.Status,
		"files_total": sess.FilesTotal,
		"bytes_total": sess.BytesTotal,
	})
}

// handleAPIRestoreEvents handles GET /api/restore/{id}/events (SSE).
func (d *Dashboard) handleAPIRestoreEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if d.restores == nil {
		http.Error(w, "restore manager not initialized", http.StatusInternalServerError)
		return
	}
	sess := d.restores.GetSession(id)
	if sess == nil {
		http.Error(w, "restore session not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			progress := sess.Progress()
			data, _ := json.Marshal(progress)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			if progress.Status == "completed" || progress.Status == "failed" {
				return
			}
		}
	}
}

// handleAPISnapshotTree handles GET /api/snapshots/{id}/tree.
func (d *Dashboard) handleAPISnapshotTree(w http.ResponseWriter, r *http.Request) {
	snapIDStr := r.PathValue("id")
	treePath := r.URL.Query().Get("path")

	// Resolve snapshot.
	var repo *repository.Repository
	var snapID repository.ID
	var found bool
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			snapIDs, _ := mr.Repo.ListSnapshots()
			for _, sid := range snapIDs {
				if strings.HasPrefix(sid.String(), snapIDStr) {
					repo = mr.Repo
					snapID = sid
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	if !found {
		http.Error(w, "snapshot not found", http.StatusNotFound)
		return
	}

	snap, err := repo.LoadSnapshot(snapID)
	if err != nil {
		http.Error(w, "load snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rootTree, err := loadRestoreTree(repo, backend.ID(snap.Tree))
	if err != nil {
		http.Error(w, "load tree: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Navigate to the requested path if specified.
	targetTree := rootTree
	if treePath != "" {
		parts := strings.Split(treePath, "/")
		for _, part := range parts {
			if part == "" || part == "." {
				continue
			}
			var foundNode bool
			for _, node := range targetTree.Nodes {
				if node.Name == part && node.Type == "dir" {
					sub, err := loadRestoreTree(repo, backend.ID(node.Subtree))
					if err != nil {
						http.Error(w, "load subtree: "+err.Error(), http.StatusInternalServerError)
						return
					}
					targetTree = sub
					foundNode = true
					break
				}
			}
			if !foundNode {
				http.Error(w, "path not found in tree", http.StatusNotFound)
				return
			}
		}
	}

	type treeEntry struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Size uint64 `json:"size,omitempty"`
		Path string `json:"path"`
	}

	entries := make([]treeEntry, 0, len(targetTree.Nodes))
	for _, node := range targetTree.Nodes {
		// Skip self-referencing "." entries from legacy snapshots.
		if node.Name == "." {
			continue
		}
		p := node.Name
		if treePath != "" {
			p = treePath + "/" + node.Name
		}
		entries = append(entries, treeEntry{
			Name: node.Name,
			Type: string(node.Type),
			Size: node.Size,
			Path: p,
		})
	}

	writeJSON(w, entries)
}

// handleAPIRestores handles GET /api/restores.
func (d *Dashboard) handleAPIRestores(w http.ResponseWriter, r *http.Request) {
	if d.restores == nil {
		writeJSON(w, []any{})
		return
	}
	sessions := d.restores.ListSessions()
	result := make([]restoreProgress, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, s.Progress())
	}
	writeJSON(w, result)
}

type agentSnapshotsResponse struct {
	Agent     string         `json:"agent"`
	Repos     []string       `json:"repos"`
	Snapshots []snapshotJSON `json:"snapshots"`
}

type restoreSourceJSON struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	BackupType    string `json:"backup_type"`
	SnapshotCount int    `json:"snapshot_count"`
	Status        string `json:"status,omitempty"`
}

// handleAPIRestoreSources lists recoverable sources for the Restores page sidebar.
func (d *Dashboard) handleAPIRestoreSources(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	counts := make(map[string]int)
	labels := make(map[string]string)
	types := make(map[string]string)
	statuses := make(map[string]string)

	agentStatus := make(map[string]string)
	for _, a := range d.registry.ListAgents() {
		st := "never"
		if a.LastBackup != nil {
			st = "ok"
		}
		if a.Suspended {
			st = "suspended"
		}
		agentStatus[a.Name] = st
	}

	if d.repos != nil {
		for _, mr := range d.repos.List() {
			summaries, err := d.getSnapshotSummaries(mr)
			if err != nil {
				continue
			}
			for _, cs := range summaries {
				bt := d.snapshotBackupType(cs)
				if typeFilter != "" && bt != typeFilter {
					continue
				}
				id := cs.Hostname
				label := cs.Hostname
				counts[id]++
				labels[id] = label
				types[id] = bt
				if st, ok := agentStatus[cs.Hostname]; ok {
					statuses[id] = st
				}
			}
		}
	}

	result := make([]restoreSourceJSON, 0, len(counts))
	for id, n := range counts {
		result = append(result, restoreSourceJSON{
			ID:            id,
			Label:         labels[id],
			BackupType:    types[id],
			SnapshotCount: n,
			Status:        statuses[id],
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].BackupType != result[j].BackupType {
			return result[i].BackupType < result[j].BackupType
		}
		return result[i].Label < result[j].Label
	})
	writeJSON(w, result)
}

// handleAPIAgentSnapshots returns snapshots for one agent across all repositories.
func (d *Dashboard) handleAPIAgentSnapshots(w http.ResponseWriter, r *http.Request) {
	agentName := r.PathValue("name")
	if agentName == "" {
		http.Error(w, "agent name is required", http.StatusBadRequest)
		return
	}
	if d.repos == nil {
		if d.repo != nil {
			snapIDs, _ := d.repo.ListSnapshots()
			var snapshots []snapshotJSON
			for _, sid := range snapIDs {
				snap, err := d.repo.LoadSnapshot(sid)
				if err != nil || !strings.EqualFold(snap.Hostname, agentName) {
					continue
				}
				snapType := snap.Type
				if snapType == "" {
					snapType = "full"
				}
				snapshots = append(snapshots, snapshotJSON{
					ID: sid.String(), Time: snap.Time.Format(time.RFC3339), Hostname: snap.Hostname,
					Tags: snap.Tags, Paths: snap.Paths, FilesNew: snap.Summary.FilesNew,
					TotalFiles: snap.Summary.TotalFiles, TotalSize: snap.Summary.TotalSize,
					Duration: snap.Summary.TotalDuration, Type: snapType, RepoName: "default",
				})
			}
			sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Time > snapshots[j].Time })
			writeJSON(w, agentSnapshotsResponse{Agent: agentName, Repos: []string{"default"}, Snapshots: snapshots})
			return
		}
		writeJSON(w, agentSnapshotsResponse{Agent: agentName, Repos: []string{}, Snapshots: []snapshotJSON{}})
		return
	}

	repoFilter := r.URL.Query().Get("repo")
	typeFilter := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}

	reposSet := make(map[string]bool)
	var snapshots []snapshotJSON

	for _, mr := range d.repos.List() {
		if repoFilter != "" && mr.Name != repoFilter {
			continue
		}
		summaries, err := d.getSnapshotSummaries(mr)
		if err != nil {
			continue
		}
		for _, cs := range summaries {
			if !d.snapshotMatchesAgent(cs, agentName) {
				continue
			}
			bt := d.snapshotBackupType(cs)
			if typeFilter != "" && bt != typeFilter {
				continue
			}
			reposSet[mr.Name] = true
			snapType := cs.Type
			if snapType == "" {
				snapType = "full"
			}
			snapshots = append(snapshots, snapshotJSON{
				ID:             cs.ID,
				Time:           cs.Time.Format(time.RFC3339),
				Hostname:       cs.Hostname,
				Tags:           cs.Tags,
				Paths:          cs.Paths,
				FilesNew:       cs.Summary.FilesNew,
				TotalFiles:     cs.Summary.TotalFiles,
				TotalSize:      cs.Summary.TotalSize,
				Duration:       cs.Summary.TotalDuration,
				Type:           snapType,
				ParentSnapshot: cs.ParentSnapshot,
				RepoName:       mr.Name,
				BackupType:     bt,
			})
		}
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Time > snapshots[j].Time
	})
	if len(snapshots) > limit {
		snapshots = snapshots[:limit]
	}

	repos := make([]string, 0, len(reposSet))
	for name := range reposSet {
		repos = append(repos, name)
	}
	sort.Strings(repos)

	writeJSON(w, agentSnapshotsResponse{
		Agent:     agentName,
		Repos:     repos,
		Snapshots: snapshots,
	})
}
