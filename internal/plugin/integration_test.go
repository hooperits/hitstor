//go:build integration

package plugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/hook"
	"github.com/hooperits/hitstor/internal/notifier"
	"github.com/hooperits/hitstor/internal/scanner"
)

func TestIntegrationPluginSubprocess(t *testing.T) {
	// Build the memory plugin binary.
	tmpBin := t.TempDir()
	pluginPath := filepath.Join(tmpBin, "hitstor-backend-memory")

	build := exec.Command("go", "build", "-o", pluginPath,
		"github.com/hooperits/hitstor/cmd/hitstor-backend-memory")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build plugin: %v", err)
	}

	// Launch via OpenStorageBackend.
	be, err := OpenStorageBackend("memory", tmpBin, nil)
	if err != nil {
		t.Fatalf("open plugin: %v", err)
	}
	defer be.Close()

	ctx := context.Background()
	id := sha256.Sum256([]byte("integration-test"))
	data := []byte("integration test data payload")

	// Save.
	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load.
	rc, err := be.Load(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Load = %q, want %q", got, data)
	}

	// Stat.
	fi, err := be.Stat(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Size != int64(len(data)) {
		t.Errorf("Stat size = %d, want %d", fi.Size, len(data))
	}

	// List.
	ids, err := be.List(ctx, backend.DataFile)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 1 || ids[0] != id {
		t.Errorf("List = %v, want [%s]", ids, backend.ID(id))
	}

	// Remove.
	if err := be.Remove(ctx, backend.DataFile, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err = be.Load(ctx, backend.DataFile, id)
	if err == nil {
		t.Error("Load after Remove should fail")
	}
}

func TestIntegrationNotifierPlugin(t *testing.T) {
	// Build the log notifier plugin binary.
	tmpBin := t.TempDir()
	pluginPath := filepath.Join(tmpBin, "hitstor-notifier-log")

	build := exec.Command("go", "build", "-o", pluginPath,
		"github.com/hooperits/hitstor/cmd/hitstor-notifier-log")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build plugin: %v", err)
	}

	// Launch via OpenNotifier.
	n, err := OpenNotifier("log", tmpBin, map[string]string{"level": "debug"})
	if err != nil {
		t.Fatalf("open notifier plugin: %v", err)
	}
	defer n.Close()

	ctx := context.Background()

	// Send backup_failed event.
	evt := notifier.Event{
		EventType: notifier.EventBackupFailed,
		Server:    "hitstor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Agent:     "integration-agent",
		Hostname:  "int-host",
		Message:   "integration test: backup failed",
		Details:   map[string]any{"session_id": "int-123"},
	}
	if err := n.Notify(ctx, evt); err != nil {
		t.Fatalf("Notify backup_failed: %v", err)
	}

	// Send agent_stale event.
	evt2 := notifier.Event{
		EventType: notifier.EventAgentStale,
		Server:    "hitstor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Agent:     "integration-agent",
		Message:   "integration test: agent stale",
	}
	if err := n.Notify(ctx, evt2); err != nil {
		t.Fatalf("Notify agent_stale: %v", err)
	}
}

func TestIntegrationHookPlugin(t *testing.T) {
	// Build the log hook plugin binary.
	tmpBin := t.TempDir()
	pluginPath := filepath.Join(tmpBin, "hitstor-hook-log")

	build := exec.Command("go", "build", "-o", pluginPath,
		"github.com/hooperits/hitstor/cmd/hitstor-hook-log")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build plugin: %v", err)
	}

	// Launch via OpenHook.
	h, err := OpenHook("log", tmpBin, map[string]string{"level": "debug"})
	if err != nil {
		t.Fatalf("open hook plugin: %v", err)
	}
	defer h.Close()

	ctx := context.Background()

	// Send pre-backup event.
	preEvt := hook.HookEvent{
		Agent:    "integration-agent",
		Hostname: "int-host",
		Paths:    []string{"/data", "/logs"},
		Excludes: []string{"*.tmp"},
		Tags:     []string{"daily"},
	}
	if err := h.PreBackup(ctx, preEvt); err != nil {
		t.Fatalf("PreBackup: %v", err)
	}

	// Send post-backup event.
	postEvt := hook.HookEvent{
		Agent:      "integration-agent",
		Hostname:   "int-host",
		Paths:      []string{"/data", "/logs"},
		SnapshotID: "snap-int-456",
		Details:    map[string]string{"files_new": "100"},
	}
	if err := h.PostBackup(ctx, postEvt); err != nil {
		t.Fatalf("PostBackup: %v", err)
	}
}

func TestIntegrationScannerPlugin(t *testing.T) {
	// Build the builtin scanner plugin binary.
	tmpBin := t.TempDir()
	pluginPath := filepath.Join(tmpBin, "hitstor-scanner-builtin")

	build := exec.Command("go", "build", "-o", pluginPath,
		"github.com/hooperits/hitstor/cmd/hitstor-scanner-builtin")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build plugin: %v", err)
	}

	// Launch via OpenFilesystemScanner.
	sc, err := OpenFilesystemScanner("builtin", tmpBin, nil)
	if err != nil {
		t.Fatalf("open scanner plugin: %v", err)
	}
	defer sc.Close()

	// Create a temp directory with some files to scan.
	scanDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(scanDir, "file1.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(scanDir, "sub")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file2.txt"), []byte("world!"), 0644); err != nil {
		t.Fatal(err)
	}

	// Scan the temp directory.
	ch, err := sc.Scan([]string{scanDir}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var results []scanner.ScanResult
	for r := range ch {
		if r.Err != nil {
			t.Errorf("unexpected scan error: %v", r.Err)
			continue
		}
		results = append(results, r)
	}

	// Expect at least: scanDir itself, file1.txt, sub/, sub/file2.txt
	if len(results) < 3 {
		t.Fatalf("got %d results, want at least 3 (file1.txt, sub/, sub/file2.txt)", len(results))
	}

	// Verify we got a file and a directory.
	var gotFile, gotDir bool
	for _, r := range results {
		if r.Node.Type == fs.NodeTypeFile && r.Node.Size > 0 {
			gotFile = true
		}
		if r.Node.Type == fs.NodeTypeDir {
			gotDir = true
		}
	}
	if !gotFile {
		t.Error("expected at least one file result")
	}
	if !gotDir {
		t.Error("expected at least one directory result")
	}
}
