package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TiB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{1000000, "1,000,000"},
		{1234567890, "1,234,567,890"},
	}

	for _, tt := range tests {
		got := FormatNumber(tt.input)
		if got != tt.want {
			t.Errorf("FormatNumber(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"abcd", "abcd"},
		{"12345678", "12345678"},
		{"123456789abcdef0", "12345678"},
	}

	for _, tt := range tests {
		got := truncateID(tt.input)
		if got != tt.want {
			t.Errorf("truncateID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBackupComplete_Text(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)

	p.BackupComplete("abcdef1234567890", 10, 200, 5, 1024*1024, 2*1024*1024, 1.5)

	out := buf.String()
	if !strings.Contains(out, "abcdef12") {
		t.Errorf("expected truncated snapshot ID, got: %s", out)
	}
	if !strings.Contains(out, "10") {
		t.Errorf("expected files new count, got: %s", out)
	}
	if !strings.Contains(out, "200") {
		t.Errorf("expected files unchanged count, got: %s", out)
	}
}

func TestBackupComplete_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, true, false)

	p.BackupComplete("abcdef1234567890", 10, 200, 5, 1024, 2048, 1.5)

	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if data["snapshot_id"] != "abcdef12" {
		t.Errorf("snapshot_id = %v, want abcdef12", data["snapshot_id"])
	}
	if data["files_new"] != float64(10) {
		t.Errorf("files_new = %v, want 10", data["files_new"])
	}
}

func TestRestoreComplete_Text(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)

	p.RestoreComplete(42, 1024*1024, 0.5)

	out := buf.String()
	if !strings.Contains(out, "42") {
		t.Errorf("expected files count, got: %s", out)
	}
	if !strings.Contains(out, "1.0 MiB") {
		t.Errorf("expected bytes formatted, got: %s", out)
	}
}

func TestRestoreComplete_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, true, false)

	p.RestoreComplete(42, 1024, 0.5)

	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if data["files_restored"] != float64(42) {
		t.Errorf("files_restored = %v, want 42", data["files_restored"])
	}
}

func TestInitComplete_Text(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)

	p.InitComplete("abcdef1234567890", "/tmp/repo")

	out := buf.String()
	if !strings.Contains(out, "abcdef12") {
		t.Errorf("expected truncated repo ID, got: %s", out)
	}
	if !strings.Contains(out, "/tmp/repo") {
		t.Errorf("expected location, got: %s", out)
	}
}

func TestInitComplete_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, true, false)

	p.InitComplete("abcdef1234567890", "/tmp/repo")

	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if data["repository_id"] != "abcdef12" {
		t.Errorf("repository_id = %v, want abcdef12", data["repository_id"])
	}
	if data["location"] != "/tmp/repo" {
		t.Errorf("location = %v, want /tmp/repo", data["location"])
	}
}

func TestStartBackup_Quiet(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, true)

	p.StartBackup()

	if buf.Len() != 0 {
		t.Errorf("expected no output in quiet mode, got: %s", buf.String())
	}
}

func TestStartBackup_Normal(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)

	p.StartBackup()

	if !strings.Contains(buf.String(), "scanning") {
		t.Errorf("expected scanning message, got: %s", buf.String())
	}
}

func TestForgetComplete_Text(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)

	p.ForgetComplete(3, 7)

	out := buf.String()
	if !strings.Contains(out, "3 kept") {
		t.Errorf("expected kept count, got: %s", out)
	}
	if !strings.Contains(out, "7 forgotten") {
		t.Errorf("expected forgotten count, got: %s", out)
	}
}

func TestForgetComplete_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, true, false)

	p.ForgetComplete(3, 7)

	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if data["snapshots_kept"] != float64(3) {
		t.Errorf("snapshots_kept = %v, want 3", data["snapshots_kept"])
	}
	if data["snapshots_forgotten"] != float64(7) {
		t.Errorf("snapshots_forgotten = %v, want 7", data["snapshots_forgotten"])
	}
}

func TestPruneComplete_Text(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)

	p.PruneComplete(5, 2, 1024*1024*100, 1024*1024*10, 3, 1)

	out := buf.String()
	if !strings.Contains(out, "5 deleted") {
		t.Errorf("expected packs deleted, got: %s", out)
	}
	if !strings.Contains(out, "2 repacked") {
		t.Errorf("expected packs repacked, got: %s", out)
	}
}

func TestPruneComplete_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, true, false)

	p.PruneComplete(5, 2, 1024, 512, 3, 1)

	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if data["packs_deleted"] != float64(5) {
		t.Errorf("packs_deleted = %v, want 5", data["packs_deleted"])
	}
	if data["indexes_written"] != float64(1) {
		t.Errorf("indexes_written = %v, want 1", data["indexes_written"])
	}
}

// --- ProgressBar tests ---

func TestProgressBar_Suppressed_Quiet(t *testing.T) {
	var buf bytes.Buffer
	bar := NewProgressBar(&buf, true) // suppress=true
	bar.Start(100, 1024)
	bar.Add(512)
	time.Sleep(200 * time.Millisecond)
	bar.Finish()

	if buf.Len() != 0 {
		t.Errorf("expected no output when suppressed, got: %q", buf.String())
	}
}

func TestProgressBar_Suppressed_NonTerminal(t *testing.T) {
	// A bytes.Buffer is not a terminal, so bar should be inactive.
	var buf bytes.Buffer
	bar := NewProgressBar(&buf, false)
	if bar.Active() {
		t.Fatal("expected bar to be inactive for non-terminal writer")
	}

	bar.Start(100, 1024)
	bar.Add(512)
	bar.Finish()

	if buf.Len() != 0 {
		t.Errorf("expected no output for non-terminal, got: %q", buf.String())
	}
}

func TestProgressBar_FormatDuration(t *testing.T) {
	tests := []struct {
		secs float64
		want string
	}{
		{0, "0s"},
		{30, "30s"},
		{59, "59s"},
		{60, "1m00s"},
		{90, "1m30s"},
		{3661, "61m01s"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.secs)
		if got != tt.want {
			t.Errorf("formatDuration(%.0f) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}

func TestProgressBar_IsTerminal(t *testing.T) {
	var buf bytes.Buffer
	if isTerminal(&buf) {
		t.Error("expected bytes.Buffer to not be a terminal")
	}
}

func TestProgressBar_Render(t *testing.T) {
	// Test the render method directly by creating a bar with a buffer writer
	// and forcing active=true for testing.
	var buf bytes.Buffer
	bar := &ProgressBar{
		totalFiles: 100,
		totalBytes: 10240,
		startTime:  time.Now().Add(-10 * time.Second),
		w:          &buf,
		active:     true,
		barWidth:   20,
	}

	// Simulate 50% progress.
	bar.doneFiles = 50
	bar.doneBytes = 5120

	bar.render()

	out := buf.String()
	if !strings.Contains(out, "50%") {
		t.Errorf("expected 50%% in output, got: %q", out)
	}
	if !strings.Contains(out, "50/100") {
		t.Errorf("expected 50/100 files in output, got: %q", out)
	}
	if !strings.Contains(out, "/s") {
		t.Errorf("expected throughput in output, got: %q", out)
	}
	if !strings.Contains(out, "ETA") {
		t.Errorf("expected ETA in output, got: %q", out)
	}
	// Bar should have 10 '#' and 10 '-' for 50%.
	if !strings.Contains(out, "##########----------") {
		t.Errorf("expected half-filled bar, got: %q", out)
	}
}

func TestScanComplete_Normal(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)
	p.ScanComplete(42, 5)
	out := buf.String()
	if !strings.Contains(out, "42") {
		t.Errorf("expected file count, got: %s", out)
	}
	if !strings.Contains(out, "5") {
		t.Errorf("expected dir count, got: %s", out)
	}
}

func TestScanComplete_Quiet(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, true)
	p.ScanComplete(42, 5)
	if buf.Len() != 0 {
		t.Errorf("expected no output in quiet mode, got: %s", buf.String())
	}
}

func TestScanComplete_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, true, false)
	p.ScanComplete(42, 5)
	if buf.Len() != 0 {
		t.Errorf("expected no output in JSON mode for scan complete, got: %s", buf.String())
	}
}

func TestFileProcessed_Normal(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)
	p.FileProcessed("/data/file.txt", 1024, true)
	out := buf.String()
	if !strings.Contains(out, "new") {
		t.Errorf("expected 'new' status, got: %s", out)
	}
	if !strings.Contains(out, "/data/file.txt") {
		t.Errorf("expected path, got: %s", out)
	}
}

func TestFileProcessed_Unchanged(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)
	p.FileProcessed("/data/old.txt", 512, false)
	out := buf.String()
	if !strings.Contains(out, "unchanged") {
		t.Errorf("expected 'unchanged' status, got: %s", out)
	}
}

func TestFileProcessed_Quiet(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, true)
	p.FileProcessed("/data/file.txt", 1024, true)
	if buf.Len() != 0 {
		t.Errorf("expected no output in quiet mode, got: %s", buf.String())
	}
}

func TestAgentBackupComplete_Text(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)
	p.AgentBackupComplete("abcdef1234567890", 5, 100, 2048, 4096, 2.5)
	out := buf.String()
	if !strings.Contains(out, "abcdef12") {
		t.Errorf("expected truncated snapshot ID, got: %s", out)
	}
	if !strings.Contains(out, "remote") {
		t.Errorf("expected 'remote' label, got: %s", out)
	}
}

func TestAgentBackupComplete_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, true, false)
	p.AgentBackupComplete("abcdef1234567890", 5, 100, 2048, 4096, 2.5)
	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["snapshot_id"] != "abcdef12" {
		t.Errorf("snapshot_id = %v, want abcdef12", data["snapshot_id"])
	}
	if data["files_new"] != float64(5) {
		t.Errorf("files_new = %v, want 5", data["files_new"])
	}
}

func TestRestoreStart_Normal(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)
	p.RestoreStart("abcdef1234567890", "/tmp/restore")
	out := buf.String()
	if !strings.Contains(out, "abcdef12") {
		t.Errorf("expected snapshot ID, got: %s", out)
	}
	if !strings.Contains(out, "/tmp/restore") {
		t.Errorf("expected target path, got: %s", out)
	}
}

func TestRestoreStart_Quiet(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, true)
	p.RestoreStart("abcdef1234567890", "/tmp/restore")
	if buf.Len() != 0 {
		t.Errorf("expected no output in quiet mode, got: %s", buf.String())
	}
}

func TestFileRestored_Normal(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, false)
	p.FileRestored("/restored/file.txt", 2048)
	out := buf.String()
	if !strings.Contains(out, "restored") {
		t.Errorf("expected 'restored' label, got: %s", out)
	}
	if !strings.Contains(out, "/restored/file.txt") {
		t.Errorf("expected path, got: %s", out)
	}
}

func TestFileRestored_Quiet(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, false, true)
	p.FileRestored("/restored/file.txt", 2048)
	if buf.Len() != 0 {
		t.Errorf("expected no output in quiet mode, got: %s", buf.String())
	}
}
