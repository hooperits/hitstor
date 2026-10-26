package repository

import (
	"testing"
	"time"
)

func testSnapshot(t *testing.T) *Snapshot {
	t.Helper()

	var treeID ID
	for i := range treeID {
		treeID[i] = byte(i)
	}

	return &Snapshot{
		Time:     time.Now().UTC().Truncate(time.Second),
		Hostname: "testhost",
		Username: "testuser",
		Paths:    []string{"/home/testuser/data", "/etc/config"},
		Excludes: []string{"*.tmp", "cache/"},
		Tree:     treeID,
		Tags:     []string{"daily", "important"},
		Summary: SnapshotSummary{
			FilesNew:       10,
			FilesUnchanged: 200,
			DirsNew:        3,
			BytesAdded:     1024 * 1024,
			BytesProcessed: 5 * 1024 * 1024,
			TotalFiles:     210,
			TotalSize:      5 * 1024 * 1024,
			TotalDuration:  12.345,
		},
	}
}

func TestSnapshotEncodeDecode(t *testing.T) {
	key := testKey("snapshot-test-key")
	snap := testSnapshot(t)

	encoded, err := EncodeSnapshot(key, snap)
	if err != nil {
		t.Fatalf("EncodeSnapshot: %v", err)
	}

	decoded, err := DecodeSnapshot(key, encoded)
	if err != nil {
		t.Fatalf("DecodeSnapshot: %v", err)
	}

	// Verify all fields round-trip correctly.
	if decoded.Hostname != snap.Hostname {
		t.Errorf("Hostname: got %q, want %q", decoded.Hostname, snap.Hostname)
	}
	if decoded.Username != snap.Username {
		t.Errorf("Username: got %q, want %q", decoded.Username, snap.Username)
	}
	if len(decoded.Paths) != len(snap.Paths) {
		t.Fatalf("Paths length: got %d, want %d", len(decoded.Paths), len(snap.Paths))
	}
	for i, p := range snap.Paths {
		if decoded.Paths[i] != p {
			t.Errorf("Paths[%d]: got %q, want %q", i, decoded.Paths[i], p)
		}
	}
	if len(decoded.Excludes) != len(snap.Excludes) {
		t.Fatalf("Excludes length: got %d, want %d", len(decoded.Excludes), len(snap.Excludes))
	}
	for i, e := range snap.Excludes {
		if decoded.Excludes[i] != e {
			t.Errorf("Excludes[%d]: got %q, want %q", i, decoded.Excludes[i], e)
		}
	}
	if decoded.Tree != snap.Tree {
		t.Errorf("Tree: got %v, want %v", decoded.Tree, snap.Tree)
	}
	if len(decoded.Tags) != len(snap.Tags) {
		t.Fatalf("Tags length: got %d, want %d", len(decoded.Tags), len(snap.Tags))
	}
	for i, tag := range snap.Tags {
		if decoded.Tags[i] != tag {
			t.Errorf("Tags[%d]: got %q, want %q", i, decoded.Tags[i], tag)
		}
	}
}

func TestSnapshotTimePreserved(t *testing.T) {
	key := testKey("snapshot-time-key")
	now := time.Now().UTC().Truncate(time.Second)

	snap := &Snapshot{
		Time:     now,
		Hostname: "host",
		Username: "user",
		Paths:    []string{"/data"},
	}

	encoded, err := EncodeSnapshot(key, snap)
	if err != nil {
		t.Fatalf("EncodeSnapshot: %v", err)
	}

	decoded, err := DecodeSnapshot(key, encoded)
	if err != nil {
		t.Fatalf("DecodeSnapshot: %v", err)
	}

	if !decoded.Time.Equal(now) {
		t.Errorf("Time: got %v, want %v", decoded.Time, now)
	}

	// Verify the time zone is UTC.
	if decoded.Time.Location() != time.UTC {
		t.Errorf("Time location: got %v, want UTC", decoded.Time.Location())
	}
}

func TestSnapshotSummaryRoundTrip(t *testing.T) {
	key := testKey("snapshot-summary-key")
	snap := testSnapshot(t)

	encoded, err := EncodeSnapshot(key, snap)
	if err != nil {
		t.Fatalf("EncodeSnapshot: %v", err)
	}

	decoded, err := DecodeSnapshot(key, encoded)
	if err != nil {
		t.Fatalf("DecodeSnapshot: %v", err)
	}

	if decoded.Summary.FilesNew != snap.Summary.FilesNew {
		t.Errorf("FilesNew: got %d, want %d", decoded.Summary.FilesNew, snap.Summary.FilesNew)
	}
	if decoded.Summary.FilesUnchanged != snap.Summary.FilesUnchanged {
		t.Errorf("FilesUnchanged: got %d, want %d", decoded.Summary.FilesUnchanged, snap.Summary.FilesUnchanged)
	}
	if decoded.Summary.DirsNew != snap.Summary.DirsNew {
		t.Errorf("DirsNew: got %d, want %d", decoded.Summary.DirsNew, snap.Summary.DirsNew)
	}
	if decoded.Summary.BytesAdded != snap.Summary.BytesAdded {
		t.Errorf("BytesAdded: got %d, want %d", decoded.Summary.BytesAdded, snap.Summary.BytesAdded)
	}
	if decoded.Summary.BytesProcessed != snap.Summary.BytesProcessed {
		t.Errorf("BytesProcessed: got %d, want %d", decoded.Summary.BytesProcessed, snap.Summary.BytesProcessed)
	}
	if decoded.Summary.TotalFiles != snap.Summary.TotalFiles {
		t.Errorf("TotalFiles: got %d, want %d", decoded.Summary.TotalFiles, snap.Summary.TotalFiles)
	}
	if decoded.Summary.TotalSize != snap.Summary.TotalSize {
		t.Errorf("TotalSize: got %d, want %d", decoded.Summary.TotalSize, snap.Summary.TotalSize)
	}
	if decoded.Summary.TotalDuration != snap.Summary.TotalDuration {
		t.Errorf("TotalDuration: got %f, want %f", decoded.Summary.TotalDuration, snap.Summary.TotalDuration)
	}
}

func TestSnapshotDecryptWrongKey(t *testing.T) {
	key := testKey("snapshot-correct-key")
	snap := testSnapshot(t)

	encoded, err := EncodeSnapshot(key, snap)
	if err != nil {
		t.Fatalf("EncodeSnapshot: %v", err)
	}

	// Use a different key for decryption.
	wrongKey := testKey("snapshot-wrong-key")

	_, err = DecodeSnapshot(wrongKey, encoded)
	if err == nil {
		t.Fatal("DecodeSnapshot with wrong key: expected error, got nil")
	}
}
