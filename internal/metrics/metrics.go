package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

const (
	contentType    = "text/plain; version=0.0.4; charset=utf-8"
	staleThreshold = 25 * time.Hour
)

// Metrics collects and serves Prometheus-compatible metrics for the HitStor server.
type Metrics struct {
	repo               *repository.Repository
	registry           *server.AgentRegistry
	be                 backend.Backend
	activeSessionsFn   func() int
	countersFn         func() (completed, failed, bytesRecv, chunksRecv uint64)
	maintenanceCountFn func() (checks, checkErrors, prunes, pruneFreedBytes uint64)
}

// New creates a Metrics instance.
func New(
	repo *repository.Repository,
	registry *server.AgentRegistry,
	be backend.Backend,
	activeSessionsFn func() int,
	countersFn func() (completed, failed, bytesRecv, chunksRecv uint64),
) *Metrics {
	return &Metrics{
		repo:             repo,
		registry:         registry,
		be:               be,
		activeSessionsFn: activeSessionsFn,
		countersFn:       countersFn,
	}
}

// SetMaintenanceCounters registers a callback for dashboard maintenance counters.
func (m *Metrics) SetMaintenanceCounters(fn func() (checks, checkErrors, prunes, pruneFreedBytes uint64)) {
	m.maintenanceCountFn = fn
}

// Handler returns an http.HandlerFunc that writes Prometheus text exposition format.
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		m.writeMetrics(w)
	}
}

func (m *Metrics) writeMetrics(w io.Writer) {
	ctx := context.Background()

	// --- Gauges: repository objects ---
	snapIDs, _ := m.repo.ListSnapshots()
	writeGauge(w, "hitstor_snapshots_total", "Total snapshots in repository.", uint64(len(snapIDs)))

	packIDs, _ := m.be.List(ctx, backend.DataFile)
	writeGauge(w, "hitstor_packs_total", "Total pack files in repository.", uint64(len(packIDs)))

	indexIDs, _ := m.be.List(ctx, backend.IndexFile)
	writeGauge(w, "hitstor_indexes_total", "Total index files in repository.", uint64(len(indexIDs)))

	// --- Gauge: repository size ---
	var totalBytes uint64
	for _, id := range packIDs {
		fi, err := m.be.Stat(ctx, backend.DataFile, id)
		if err == nil {
			totalBytes += uint64(fi.Size)
		}
	}
	for _, id := range indexIDs {
		fi, err := m.be.Stat(ctx, backend.IndexFile, id)
		if err == nil {
			totalBytes += uint64(fi.Size)
		}
	}
	for _, id := range snapIDs {
		fi, err := m.be.Stat(ctx, backend.SnapshotFile, id)
		if err == nil {
			totalBytes += uint64(fi.Size)
		}
	}
	writeGauge(w, "hitstor_repository_size_bytes", "Total repository size in bytes.", totalBytes)

	// --- Gauge: active sessions ---
	writeGauge(w, "hitstor_active_sessions", "Current active backup sessions.", uint64(m.activeSessionsFn()))

	// --- Gauges: agents ---
	agents := m.registry.ListAgents()
	writeGauge(w, "hitstor_registered_agents", "Total registered backup agents.", uint64(len(agents)))

	now := time.Now()
	var okCount, staleCount, neverCount uint64
	for _, a := range agents {
		if a.LastBackup == nil {
			neverCount++
		} else if now.Sub(*a.LastBackup) > staleThreshold {
			staleCount++
		} else {
			okCount++
		}
	}
	writeGauge(w, "hitstor_agents_ok", "Agents with a recent backup (within 25h).", okCount)
	writeGauge(w, "hitstor_agents_stale", "Agents with a stale backup (over 25h).", staleCount)
	writeGauge(w, "hitstor_agents_never", "Agents that have never backed up.", neverCount)

	// --- Counters (from gRPC server) ---
	completed, failed, bytesRecv, chunksRecv := m.countersFn()
	writeCounter(w, "hitstor_backups_completed_total", "Cumulative successful backups.", completed)
	writeCounter(w, "hitstor_backups_failed_total", "Cumulative failed or aborted backups.", failed)
	writeCounter(w, "hitstor_bytes_received_total", "Cumulative bytes received from agents.", bytesRecv)
	writeCounter(w, "hitstor_chunks_received_total", "Cumulative chunks received from agents.", chunksRecv)

	// --- Counters (from dashboard maintenance operations) ---
	if m.maintenanceCountFn != nil {
		checks, checkErrors, prunes, pruneFreedBytes := m.maintenanceCountFn()
		writeCounter(w, "hitstor_checks_total", "Cumulative integrity checks performed.", checks)
		writeCounter(w, "hitstor_check_errors_total", "Cumulative integrity check errors.", checkErrors)
		writeCounter(w, "hitstor_prunes_total", "Cumulative prune operations performed.", prunes)
		writeCounter(w, "hitstor_prune_freed_bytes_total", "Cumulative bytes freed by prune operations.", pruneFreedBytes)
	}

	// --- Per-agent: last backup timestamp ---
	fmt.Fprintf(w, "# HELP hitstor_agent_last_backup_timestamp_seconds Unix timestamp of agent's last successful backup.\n")
	fmt.Fprintf(w, "# TYPE hitstor_agent_last_backup_timestamp_seconds gauge\n")
	for _, a := range agents {
		var ts float64
		if a.LastBackup != nil {
			ts = float64(a.LastBackup.Unix())
		}
		fmt.Fprintf(w, "hitstor_agent_last_backup_timestamp_seconds{agent=%q} %g\n", a.Name, ts)
	}

	// --- Per-agent: schedule info ---
	fmt.Fprintf(w, "# HELP hitstor_agent_schedule_enabled Whether scheduled backups are enabled (1=yes, 0=no).\n")
	fmt.Fprintf(w, "# TYPE hitstor_agent_schedule_enabled gauge\n")
	for _, a := range agents {
		var val int
		if a.ScheduleEnabled {
			val = 1
		}
		fmt.Fprintf(w, "hitstor_agent_schedule_enabled{agent=%q} %d\n", a.Name, val)
	}

	fmt.Fprintf(w, "# HELP hitstor_agent_schedule_interval_hours Configured backup interval in hours.\n")
	fmt.Fprintf(w, "# TYPE hitstor_agent_schedule_interval_hours gauge\n")
	for _, a := range agents {
		fmt.Fprintf(w, "hitstor_agent_schedule_interval_hours{agent=%q} %d\n", a.Name, a.ScheduleHours)
	}
}

func writeGauge(w io.Writer, name, help string, value uint64) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s gauge\n", name)
	fmt.Fprintf(w, "%s %d\n", name, value)
}

func writeCounter(w io.Writer, name, help string, value uint64) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s counter\n", name)
	fmt.Fprintf(w, "%s %d\n", name, value)
}
