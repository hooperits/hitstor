package dashboard

import (
	"testing"
)

func TestRestoreManagerStartAndGet(t *testing.T) {
	rm := NewRestoreManager()

	sess := rm.StartRestore("abc123", "agent1", "/tmp/test", nil)
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.Status != "running" {
		t.Fatalf("expected status 'running', got %q", sess.Status)
	}
	if sess.SnapshotID != "abc123" {
		t.Fatalf("expected snapshot ID 'abc123', got %q", sess.SnapshotID)
	}

	got := rm.GetSession(sess.ID)
	if got == nil {
		t.Fatal("GetSession returned nil")
	}
	if got.ID != sess.ID {
		t.Fatalf("expected ID %q, got %q", sess.ID, got.ID)
	}

	// Non-existent session.
	if rm.GetSession("nonexistent") != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}

func TestRestoreManagerHasActiveRestore(t *testing.T) {
	rm := NewRestoreManager()

	if rm.HasActiveRestore("agent1") {
		t.Fatal("expected no active restore before starting")
	}

	sess := rm.StartRestore("snap1", "agent1", "", nil)
	if !rm.HasActiveRestore("agent1") {
		t.Fatal("expected active restore for agent1")
	}
	if rm.HasActiveRestore("agent2") {
		t.Fatal("expected no active restore for agent2")
	}

	// Complete the restore.
	sess.mu.Lock()
	sess.Status = "completed"
	sess.mu.Unlock()

	if rm.HasActiveRestore("agent1") {
		t.Fatal("expected no active restore after completion")
	}
}

func TestRestoreManagerHasActiveRestoreFailed(t *testing.T) {
	rm := NewRestoreManager()

	sess := rm.StartRestore("snap1", "agent1", "", nil)
	if !rm.HasActiveRestore("agent1") {
		t.Fatal("expected active restore")
	}

	// Fail the restore.
	sess.mu.Lock()
	sess.Status = "failed"
	sess.mu.Unlock()

	if rm.HasActiveRestore("agent1") {
		t.Fatal("expected no active restore after failure")
	}
}

func TestRestoreManagerListSessions(t *testing.T) {
	rm := NewRestoreManager()

	rm.StartRestore("snap1", "agent1", "", nil)
	rm.StartRestore("snap2", "agent2", "/tmp", []string{"dir1"})

	sessions := rm.ListSessions()
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestRestoreSessionProgress(t *testing.T) {
	rm := NewRestoreManager()
	sess := rm.StartRestore("snap1", "agent1", "/tmp", nil)
	sess.FilesTotal = 100
	sess.BytesTotal = 1024 * 1024

	sess.FilesRestored.Store(50)
	sess.BytesWritten.Store(512 * 1024)

	sess.mu.Lock()
	sess.CurrentFile = "dir/file.txt"
	sess.mu.Unlock()

	p := sess.Progress()
	if p.FilesRestored != 50 {
		t.Fatalf("expected 50 files restored, got %d", p.FilesRestored)
	}
	if p.BytesWritten != 512*1024 {
		t.Fatalf("expected %d bytes, got %d", 512*1024, p.BytesWritten)
	}
	if p.CurrentFile != "dir/file.txt" {
		t.Fatalf("expected 'dir/file.txt', got %q", p.CurrentFile)
	}
	if p.Status != "running" {
		t.Fatalf("expected 'running', got %q", p.Status)
	}
}

func TestRestoreManagerConcurrentBlock(t *testing.T) {
	rm := NewRestoreManager()

	rm.StartRestore("snap1", "agent1", "", nil)

	// Second restore to same agent should be blocked.
	if !rm.HasActiveRestore("agent1") {
		t.Fatal("expected HasActiveRestore to return true for concurrent restore attempt")
	}

	// Different agent is fine.
	if rm.HasActiveRestore("agent2") {
		t.Fatal("expected HasActiveRestore to return false for different agent")
	}
}

func TestShouldProcess(t *testing.T) {
	tests := []struct {
		relPath  string
		includes []string
		want     bool
	}{
		{"dir/file.txt", nil, true},
		{"dir/file.txt", []string{}, true},
		{"dir/file.txt", []string{"dir"}, true},
		{"dir/file.txt", []string{"dir/file.txt"}, true},
		{"dir/file.txt", []string{"other"}, false},
		{"dir", []string{"dir/file.txt"}, true},
		{"dir/sub", []string{"dir"}, true},
	}

	for _, tt := range tests {
		got := shouldProcess(tt.relPath, tt.includes)
		if got != tt.want {
			t.Errorf("shouldProcess(%q, %v) = %v, want %v", tt.relPath, tt.includes, got, tt.want)
		}
	}
}
