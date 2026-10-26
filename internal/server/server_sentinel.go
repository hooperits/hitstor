package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/sentinel"
)

// SetThreatDetectedHandler sets the callback invoked on ransomware threat detections.
func (s *Server) SetThreatDetectedHandler(fn func(agentName, hostname, reason string, confidence float64, details map[string]any)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.onThreatDetected = fn
}

func (s *Server) notifyThreatDetected(agentName, hostname, reason string, confidence float64, details map[string]any) {
	s.handlersMu.RLock()
	fn := s.onThreatDetected
	s.handlersMu.RUnlock()
	if fn != nil {
		go fn(agentName, hostname, reason, confidence, details)
	}
}

func (s *Server) runSentinelCheck(sess *Session, resp *pb.FinalizeBackupResponse) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Gather historical baseline summaries for this host/agent.
	var history []sentinel.TelemetryData
	snapIDs, err := sess.repo.ListSnapshots()
	if err == nil {
		for _, sid := range snapIDs {
			// Skip the current snapshot itself
			if strings.HasPrefix(sid.String(), resp.SnapshotId) {
				continue
			}

			snap, err := sess.repo.LoadSnapshot(sid)
			if err != nil {
				continue
			}

			if snap.Hostname == sess.Hostname {
				history = append(history, sentinel.TelemetryData{
					Time:           snap.Time,
					FilesNew:       snap.Summary.FilesNew,
					FilesUnchanged: snap.Summary.FilesUnchanged,
					BytesAdded:     snap.Summary.BytesAdded,
					BytesProcessed: snap.Summary.BytesProcessed,
					Duration:       snap.Summary.TotalDuration,
				})
			}
		}
	}

	if len(history) > 10 {
		history = history[:10]
	}

	current := sentinel.TelemetryData{
		Time:            time.Now(),
		FilesNew:        resp.FilesNew,
		FilesUnchanged:  resp.FilesUnchanged,
		BytesAdded:      resp.BytesAdded,
		BytesProcessed:  resp.BytesProcessed,
		Duration:        resp.DurationSeconds,
		SuspiciousFiles: sess.suspiciousFiles,
	}

	// 2. Perform safety audit.
	analysis, err := s.sentinel.CheckSession(ctx, sess.AgentName, current, history)
	if err != nil {
		slog.Error("sentinel: safety check failed", "agent", sess.AgentName, "error", err)
		return
	}

	slog.Info("sentinel: safety check complete", "agent", sess.AgentName, "threat_detected", analysis.ThreatDetected, "confidence", analysis.Confidence)

	if analysis.ThreatDetected && analysis.Confidence >= s.opts.SentinelConfig.ConfidenceThreshold {
		slog.Warn("sentinel: ransomware threat detected! Initiating autonomous containment...", "agent", sess.AgentName, "confidence", analysis.Confidence)

		// A. Suspend agent credentials
		if err := s.registry.SuspendAgent(sess.AgentName); err != nil {
			slog.Error("sentinel: failed to suspend agent credentials", "agent", sess.AgentName, "error", err)
		} else {
			slog.Warn("sentinel: agent credentials successfully revoked/suspended", "agent", sess.AgentName)
		}

		// B. Lock repository storage
		if err := s.sentinel.LockGCSBucket(ctx); err != nil {
			slog.Error("sentinel: failed to lock GCS storage bucket", "error", err)
		}

		// C. Log alert event to the registry
		alertID := fmt.Sprintf("alert-%s-%d", resp.SnapshotId, time.Now().UnixNano())
		alert := SentinelAlert{
			ID:             alertID,
			AgentName:      sess.AgentName,
			Hostname:       sess.Hostname,
			Time:           time.Now(),
			SnapshotID:     resp.SnapshotId,
			ThreatDetected: true,
			Confidence:     analysis.Confidence,
			Reason:         analysis.Reason,
			Status:         "active",
		}
		if err := s.registry.AddAlert(alert); err != nil {
			slog.Error("sentinel: failed to write alert to registry", "error", err)
		}

		// D. Notify administrators
		s.notifyThreatDetected(sess.AgentName, sess.Hostname, analysis.Reason, analysis.Confidence, map[string]any{
			"snapshot_id":     resp.SnapshotId,
			"files_new":       resp.FilesNew,
			"files_unchanged": resp.FilesUnchanged,
			"bytes_added":     resp.BytesAdded,
			"bytes_processed": resp.BytesProcessed,
		})
	}
}
