package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/checker"
	"github.com/hooperits/hitstor/internal/pruner"
)

// operation tracks an async maintenance operation (check, prune).
type operation struct {
	ID       string
	Type     string // "check" or "prune"
	RepoName string
	Status   string // "running", "completed", "failed"
	mu       sync.Mutex
	messages []string
	Result   any
	done     chan struct{}
}

func (op *operation) addMessage(msg string) {
	op.mu.Lock()
	op.messages = append(op.messages, msg)
	op.mu.Unlock()
}

func (op *operation) getMessages(from int) []string {
	op.mu.Lock()
	defer op.mu.Unlock()
	if from >= len(op.messages) {
		return nil
	}
	msgs := make([]string, len(op.messages)-from)
	copy(msgs, op.messages[from:])
	return msgs
}

func newOperationID() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

type checkRequest struct {
	Deep  bool `json:"deep"`
	Force bool `json:"force"`
}

// handleAPICheck starts an integrity check on a repo.
func (d *Dashboard) handleAPICheck(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	mr, err := d.repos.Get(name)
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}

	var req checkRequest
	json.NewDecoder(r.Body).Decode(&req)

	// Check maintenance lock.
	if locked, reason := mr.MaintenanceLock.IsLocked(); locked {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{
			"error":  "repository is locked for maintenance",
			"reason": reason,
		})
		return
	}

	// Abort active sessions if force.
	if req.Force && d.srv != nil {
		d.srv.AbortSessionsByRepo(name)
	}

	// Acquire maintenance lock.
	if _, err := mr.MaintenanceLock.Acquire("integrity check"); err != nil {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"error": fmt.Sprintf("acquire lock: %v", err)})
		return
	}

	op := &operation{
		ID:       newOperationID(),
		Type:     "check",
		RepoName: name,
		Status:   "running",
		done:     make(chan struct{}),
	}

	d.opMu.Lock()
	d.operations[op.ID] = op
	d.opMu.Unlock()

	go func() {
		defer mr.MaintenanceLock.Release()
		defer close(op.done)

		d.checkTotal.Add(1)
		c := checker.New(mr.Repo)
		errors, err := c.Check(req.Deep, func(step string) {
			op.addMessage(step)
		})

		op.mu.Lock()
		if err != nil {
			op.Status = "failed"
			op.Result = map[string]any{"error": err.Error()}
			d.checkErrorTotal.Add(1)
		} else {
			op.Status = "completed"
			if len(errors) > 0 {
				d.checkErrorTotal.Add(1)
			}
			checkErrors := make([]map[string]string, 0, len(errors))
			for _, ce := range errors {
				checkErrors = append(checkErrors, map[string]string{
					"pack":    ce.Pack.String(),
					"blob":    ce.Blob.String(),
					"message": ce.Message,
				})
			}
			op.Result = map[string]any{
				"pass":   len(errors) == 0,
				"deep":   req.Deep,
				"errors": checkErrors,
			}
		}
		op.mu.Unlock()
	}()

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{
		"operation_id": op.ID,
		"type":         "check",
		"repo":         name,
	})
}

type pruneRequest struct {
	Force  bool `json:"force"`
	DryRun bool `json:"dry_run"`
}

// handleAPIPrune starts a prune operation on a repo.
func (d *Dashboard) handleAPIPrune(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	mr, err := d.repos.Get(name)
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}

	var req pruneRequest
	json.NewDecoder(r.Body).Decode(&req)

	if locked, reason := mr.MaintenanceLock.IsLocked(); locked {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{
			"error":  "repository is locked for maintenance",
			"reason": reason,
		})
		return
	}

	if req.Force && d.srv != nil {
		d.srv.AbortSessionsByRepo(name)
	}

	if _, err := mr.MaintenanceLock.Acquire("prune"); err != nil {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"error": fmt.Sprintf("acquire lock: %v", err)})
		return
	}

	op := &operation{
		ID:       newOperationID(),
		Type:     "prune",
		RepoName: name,
		Status:   "running",
		done:     make(chan struct{}),
	}

	d.opMu.Lock()
	d.operations[op.ID] = op
	d.opMu.Unlock()

	go func() {
		defer mr.MaintenanceLock.Release()
		defer close(op.done)

		d.pruneTotal.Add(1)
		p := pruner.New(mr.Repo)
		stats, err := p.Prune(pruner.Options{DryRun: req.DryRun}, func(step string) {
			op.addMessage(step)
		})

		op.mu.Lock()
		if err != nil {
			op.Status = "failed"
			op.Result = map[string]any{"error": err.Error()}
		} else {
			op.Status = "completed"
			d.invalidateSnapshotCache(name)
			d.pruneFreedBytes.Add(stats.BytesFreed)
			op.Result = map[string]any{
				"dry_run":         req.DryRun,
				"packs_deleted":   stats.PacksDeleted,
				"packs_repacked":  stats.PacksRepacked,
				"bytes_freed":     stats.BytesFreed,
				"indexes_deleted": stats.IndexesDeleted,
				"indexes_written": stats.IndexesWritten,
			}
		}
		op.mu.Unlock()
	}()

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{
		"operation_id": op.ID,
		"type":         "prune",
		"repo":         name,
	})
}

type forgetRequest struct {
	SnapshotIDs []string `json:"snapshot_ids"`
	Prune       bool     `json:"prune"`
}

type forgetFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// handleAPIForget removes one or more snapshots from a repo, optionally
// pruning unreferenced data afterward. Unlike check/prune, this runs
// synchronously since forgetting snapshot metadata is fast; only the
// optional prune step can take noticeable time.
func (d *Dashboard) handleAPIForget(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	mr, err := d.repos.Get(name)
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}

	var req forgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.SnapshotIDs) == 0 {
		http.Error(w, "snapshot_ids is required", http.StatusBadRequest)
		return
	}

	ids := make([]backend.ID, 0, len(req.SnapshotIDs))
	for _, idStr := range req.SnapshotIDs {
		var id backend.ID
		b, err := hex.DecodeString(idStr)
		if err != nil || len(b) != len(id) {
			http.Error(w, fmt.Sprintf("invalid snapshot id %q", idStr), http.StatusBadRequest)
			return
		}
		copy(id[:], b)
		ids = append(ids, id)
	}

	if locked, reason := mr.MaintenanceLock.IsLocked(); locked {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{
			"error":  "repository is locked for maintenance",
			"reason": reason,
		})
		return
	}

	if _, err := mr.MaintenanceLock.Acquire("forget"); err != nil {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"error": fmt.Sprintf("acquire lock: %v", err)})
		return
	}
	defer mr.MaintenanceLock.Release()

	forgotten := 0
	var failed []forgetFailure
	for _, id := range ids {
		if err := mr.Repo.RemoveSnapshot(id); err != nil {
			failed = append(failed, forgetFailure{ID: id.String(), Error: err.Error()})
			continue
		}
		forgotten++
	}

	result := map[string]any{
		"forgotten": forgotten,
		"failed":    failed,
	}

	if req.Prune && forgotten > 0 {
		d.pruneTotal.Add(1)
		p := pruner.New(mr.Repo)
		stats, err := p.Prune(pruner.Options{}, func(string) {})
		if err != nil {
			result["prune_error"] = err.Error()
		} else {
			d.pruneFreedBytes.Add(stats.BytesFreed)
			result["prune"] = map[string]any{
				"packs_deleted":   stats.PacksDeleted,
				"packs_repacked":  stats.PacksRepacked,
				"bytes_freed":     stats.BytesFreed,
				"indexes_deleted": stats.IndexesDeleted,
				"indexes_written": stats.IndexesWritten,
			}
		}
	}

	d.invalidateSnapshotCache(name)
	writeJSON(w, result)
}

// handleAPIOperationEvents streams SSE events for an operation.
func (d *Dashboard) handleAPIOperationEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	d.opMu.RLock()
	op, ok := d.operations[id]
	d.opMu.RUnlock()

	if !ok {
		http.Error(w, "operation not found", http.StatusNotFound)
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

	cursor := 0
	ctx := r.Context()

	for {
		msgs := op.getMessages(cursor)
		for _, msg := range msgs {
			fmt.Fprintf(w, "data: %s\n\n", msg)
			cursor++
		}
		if len(msgs) > 0 {
			flusher.Flush()
		}

		// Check if done.
		select {
		case <-op.done:
			// Drain remaining messages.
			msgs = op.getMessages(cursor)
			for _, msg := range msgs {
				fmt.Fprintf(w, "data: %s\n\n", msg)
			}

			// Send result.
			op.mu.Lock()
			result := op.Result
			status := op.Status
			op.mu.Unlock()

			resultJSON, _ := json.Marshal(map[string]any{
				"status": status,
				"result": result,
			})
			fmt.Fprintf(w, "event: complete\ndata: %s\n\n", resultJSON)
			flusher.Flush()
			return
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			// Poll interval.
		}
	}
}

// --- Agent Management API ---
