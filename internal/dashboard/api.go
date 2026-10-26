package dashboard

import (
	"context"
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

type snapshotJSON struct {
	ID             string   `json:"id"`
	Time           string   `json:"time"`
	Hostname       string   `json:"hostname"`
	Tags           []string `json:"tags,omitempty"`
	Paths          []string `json:"paths"`
	FilesNew       uint64   `json:"files_new"`
	TotalFiles     uint64   `json:"total_files"`
	TotalSize      uint64   `json:"total_size"`
	Duration       float64  `json:"duration"`
	Type           string   `json:"type"`
	ParentSnapshot string   `json:"parent_snapshot,omitempty"`
	RepoName       string   `json:"repo_name,omitempty"`
	BackupType     string   `json:"backup_type,omitempty"`
}

type statsJSON struct {
	Snapshots      int    `json:"snapshots"`
	Packs          int    `json:"packs"`
	Indexes        int    `json:"indexes"`
	TotalBytes     uint64 `json:"total_bytes"`
	Agents         int    `json:"agents"`
	ActiveSessions int    `json:"active_sessions"`
}

// FormatDuration formats a duration as human-readable text (e.g., "2h 15m", "3m 45s").
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func (d *Dashboard) handleAPISnapshots(w http.ResponseWriter, r *http.Request) {
	hostFilter := r.URL.Query().Get("host")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	var result []snapshotJSON
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			snapIDs, err := mr.Repo.ListSnapshots()
			if err != nil {
				continue
			}
			for _, sid := range snapIDs {
				snap, err := mr.Repo.LoadSnapshot(sid)
				if err != nil {
					continue
				}
				if hostFilter != "" && snap.Hostname != hostFilter {
					continue
				}
				result = append(result, snapshotJSON{
					ID:         sid.String(),
					Time:       snap.Time.Format(time.RFC3339),
					Hostname:   snap.Hostname,
					Tags:       snap.Tags,
					Paths:      snap.Paths,
					FilesNew:   snap.Summary.FilesNew,
					TotalFiles: snap.Summary.TotalFiles,
					TotalSize:  snap.Summary.TotalSize,
					Duration:   snap.Summary.TotalDuration,
					RepoName:   mr.Name,
				})
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time > result[j].Time
	})

	if len(result) > limit {
		result = result[:limit]
	}

	writeJSON(w, result)
}

func (d *Dashboard) getFileSizes(ctx context.Context, be backend.Backend, repoName string, ft backend.FileType, ids []backend.ID) uint64 {
	var total uint64
	var missingIDs []backend.ID

	d.sizeCacheMu.Lock()
	if d.sizeCache == nil {
		d.sizeCache = make(map[string]int64)
	}
	for _, id := range ids {
		key := fmt.Sprintf("%s/%d/%s", repoName, ft, id.String())
		if size, ok := d.sizeCache[key]; ok {
			total += uint64(size)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}
	d.sizeCacheMu.Unlock()

	newSizes := make(map[backend.ID]int64)
	for _, id := range missingIDs {
		fi, err := be.Stat(ctx, ft, id)
		if err == nil {
			newSizes[id] = fi.Size
		}
	}

	d.sizeCacheMu.Lock()
	defer d.sizeCacheMu.Unlock()

	for id, size := range newSizes {
		key := fmt.Sprintf("%s/%d/%s", repoName, ft, id.String())
		d.sizeCache[key] = size
		total += uint64(size)
	}

	currentKeys := make(map[string]bool, len(ids))
	for _, id := range ids {
		key := fmt.Sprintf("%s/%d/%s", repoName, ft, id.String())
		currentKeys[key] = true
	}

	prefix := fmt.Sprintf("%s/%d/", repoName, ft)
	for key := range d.sizeCache {
		if strings.HasPrefix(key, prefix) && !currentKeys[key] {
			delete(d.sizeCache, key)
		}
	}

	return total
}

func (d *Dashboard) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var snapIDs []repository.ID
	var packIDs, indexIDs []backend.ID
	var totalBytes uint64

	if d.repo != nil {
		snapIDs, _ = d.repo.ListSnapshots()
	}
	if d.be != nil {
		packIDs, _ = d.be.List(ctx, backend.DataFile)
		indexIDs, _ = d.be.List(ctx, backend.IndexFile)

		totalBytes += d.getFileSizes(ctx, d.be, "default", backend.DataFile, packIDs)
		totalBytes += d.getFileSizes(ctx, d.be, "default", backend.IndexFile, indexIDs)
		totalBytes += d.getFileSizes(ctx, d.be, "default", backend.SnapshotFile, snapIDs)
	}

	agents := d.registry.ListAgents()
	activeSessions := 0
	if d.activeSessionsFn != nil {
		activeSessions = d.activeSessionsFn()
	}

	writeJSON(w, statsJSON{
		Snapshots:      len(snapIDs),
		Packs:          len(packIDs),
		Indexes:        len(indexIDs),
		TotalBytes:     totalBytes,
		Agents:         len(agents),
		ActiveSessions: activeSessions,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

// --- Backup Jobs API ---
