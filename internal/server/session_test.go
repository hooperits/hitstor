package server

import (
	"testing"
	"time"
)

func TestSessionProgressTracking(t *testing.T) {
	// Create a session with nil repo — we only test progress fields.
	s := &Session{
		ID:           "test-session",
		AgentName:    "test-agent",
		Hostname:     "test-host",
		known:        make(map[[32]byte]bool),
		newIDs:       make(map[[32]byte]bool),
		startTime:    time.Now(),
		lastActivity: time.Now(),
		phase:        "uploading",
	}

	// Verify initial state.
	bytes, files, filesNew, phase, _ := s.Progress()
	if bytes != 0 || files != 0 || filesNew != 0 {
		t.Errorf("initial counters: got bytes=%d files=%d filesNew=%d, want 0/0/0", bytes, files, filesNew)
	}
	if phase != "uploading" {
		t.Errorf("initial phase: got %q, want %q", phase, "uploading")
	}

	// Simulate adding chunks by directly manipulating counters (no repo needed).
	s.mu.Lock()
	s.bytesAdded = 1024
	s.filesProcessed = 5
	s.filesNew = 3
	s.lastActivity = time.Now()
	s.mu.Unlock()

	bytes, files, filesNew, phase, lastAct := s.Progress()
	if bytes != 1024 {
		t.Errorf("bytesAdded: got %d, want 1024", bytes)
	}
	if files != 5 {
		t.Errorf("filesProcessed: got %d, want 5", files)
	}
	if filesNew != 3 {
		t.Errorf("filesNew: got %d, want 3", filesNew)
	}
	if phase != "uploading" {
		t.Errorf("phase: got %q, want %q", phase, "uploading")
	}
	if time.Since(lastAct) > time.Second {
		t.Errorf("lastActivity too old: %v", lastAct)
	}

	// Test SetPhase.
	s.SetPhase("finalizing")
	_, _, _, phase, _ = s.Progress()
	if phase != "finalizing" {
		t.Errorf("after SetPhase: got %q, want %q", phase, "finalizing")
	}
}

func TestSessionToInfo(t *testing.T) {
	now := time.Now()
	s := &Session{
		ID:         "sess-123",
		AgentName:  "agent-1",
		Hostname:   "host-1",
		RepoName:   "repo-1",
		JobID:      "job-456",
		BackupType: "incremental",
		known:      make(map[[32]byte]bool),
		newIDs:     make(map[[32]byte]bool),
		startTime:  now,
		phase:      "uploading",
	}
	s.lastActivity = now
	s.bytesAdded = 2048
	s.filesProcessed = 10
	s.filesNew = 4

	info := sessionToInfo(s)
	if info.ID != "sess-123" {
		t.Errorf("ID: got %q, want %q", info.ID, "sess-123")
	}
	if info.JobID != "job-456" {
		t.Errorf("JobID: got %q, want %q", info.JobID, "job-456")
	}
	if info.BytesAdded != 2048 {
		t.Errorf("BytesAdded: got %d, want 2048", info.BytesAdded)
	}
	if info.FilesProcessed != 10 {
		t.Errorf("FilesProcessed: got %d, want 10", info.FilesProcessed)
	}
	if info.Phase != "uploading" {
		t.Errorf("Phase: got %q, want %q", info.Phase, "uploading")
	}
}
