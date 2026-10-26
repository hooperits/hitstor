package retention

import (
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
)

// testID returns a deterministic ID from a byte value.
func testID(b byte) backend.ID {
	var id backend.ID
	id[0] = b
	return id
}

// snap creates a SnapshotInfo with the given parameters.
func snap(id byte, t time.Time, host string, paths []string, tags []string) SnapshotInfo {
	return SnapshotInfo{
		ID:       testID(id),
		Time:     t,
		Hostname: host,
		Paths:    paths,
		Tags:     tags,
	}
}

func TestApply_EmptyPolicy(t *testing.T) {
	snapshots := []SnapshotInfo{snap(1, time.Now(), "host", nil, nil)}
	_, err := Apply(snapshots, Policy{}, "", "")
	if err == nil {
		t.Fatal("expected error for empty policy")
	}
}

func TestApply_KeepLast(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		keepLast   int
		wantKeep   int
		wantForget int
	}{
		{"keep-last-3-of-5", 5, 3, 3, 2},
		{"keep-last-5-of-3", 3, 5, 3, 0},
		{"keep-last-1-of-1", 1, 1, 1, 0},
		{"keep-last-1-of-10", 10, 1, 1, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var snapshots []SnapshotInfo
			base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for i := 0; i < tt.count; i++ {
				snapshots = append(snapshots, snap(byte(i), base.Add(time.Duration(i)*time.Hour), "host", nil, nil))
			}

			result, err := Apply(snapshots, Policy{KeepLast: tt.keepLast}, "", "")
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if len(result.Keep) != tt.wantKeep {
				t.Errorf("keep = %d, want %d", len(result.Keep), tt.wantKeep)
			}
			if len(result.Forget) != tt.wantForget {
				t.Errorf("forget = %d, want %d", len(result.Forget), tt.wantForget)
			}
		})
	}
}

func TestApply_KeepDaily(t *testing.T) {
	// 10 snapshots across 5 days (2 per day), keep-daily 3.
	// Should keep the most recent snapshot per day for the 3 most recent days.
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(2, time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(3, time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(4, time.Date(2026, 1, 2, 14, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(5, time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(6, time.Date(2026, 1, 3, 14, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(7, time.Date(2026, 1, 4, 10, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(8, time.Date(2026, 1, 4, 14, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(9, time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(10, time.Date(2026, 1, 5, 14, 0, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepDaily: 3}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// 3 days * 1 snapshot each = 3 kept.
	if len(result.Keep) != 3 {
		t.Errorf("keep = %d, want 3", len(result.Keep))
	}
	if len(result.Forget) != 7 {
		t.Errorf("forget = %d, want 7", len(result.Forget))
	}

	// Verify the kept ones are the most recent per day (IDs 10, 8, 6).
	keptIDs := make(map[byte]bool)
	for _, s := range result.Keep {
		keptIDs[s.ID[0]] = true
	}
	for _, id := range []byte{10, 8, 6} {
		if !keptIDs[id] {
			t.Errorf("expected snapshot %d to be kept", id)
		}
	}
}

func TestApply_KeepWeekly(t *testing.T) {
	// Snapshots across 4 weeks, keep-weekly 2.
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC), "host", nil, nil),  // Week 2
		snap(2, time.Date(2026, 1, 12, 12, 0, 0, 0, time.UTC), "host", nil, nil), // Week 3
		snap(3, time.Date(2026, 1, 19, 12, 0, 0, 0, time.UTC), "host", nil, nil), // Week 4
		snap(4, time.Date(2026, 1, 26, 12, 0, 0, 0, time.UTC), "host", nil, nil), // Week 5
	}

	result, err := Apply(snapshots, Policy{KeepWeekly: 2}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
	if len(result.Forget) != 2 {
		t.Errorf("forget = %d, want 2", len(result.Forget))
	}

	// Most recent 2 weeks: IDs 4 and 3.
	keptIDs := make(map[byte]bool)
	for _, s := range result.Keep {
		keptIDs[s.ID[0]] = true
	}
	if !keptIDs[4] || !keptIDs[3] {
		t.Errorf("expected snapshots 4 and 3 to be kept, got %v", keptIDs)
	}
}

func TestApply_KeepMonthly(t *testing.T) {
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2025, 10, 15, 12, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(2, time.Date(2025, 11, 15, 12, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(3, time.Date(2025, 12, 15, 12, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(4, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepMonthly: 2}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
}

func TestApply_KeepYearly(t *testing.T) {
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(2, time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(3, time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepYearly: 2}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
	if len(result.Forget) != 1 {
		t.Errorf("forget = %d, want 1", len(result.Forget))
	}
}

func TestApply_KeepHourly(t *testing.T) {
	// 6 snapshots across 3 hours (2 per hour), keep-hourly 2.
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(2, time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC), "host", nil, nil),
		snap(3, time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(4, time.Date(2026, 1, 1, 11, 30, 0, 0, time.UTC), "host", nil, nil),
		snap(5, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(6, time.Date(2026, 1, 1, 12, 30, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepHourly: 2}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
	if len(result.Forget) != 4 {
		t.Errorf("forget = %d, want 4", len(result.Forget))
	}
}

func TestApply_KeepTag(t *testing.T) {
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "host", nil, []string{"important"}),
		snap(2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(3, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), "host", nil, []string{"important", "weekly"}),
		snap(4, time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepTags: []string{"important"}}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// IDs 1 and 3 have the "important" tag.
	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
	if len(result.Forget) != 2 {
		t.Errorf("forget = %d, want 2", len(result.Forget))
	}
}

func TestApply_UnionSemantics(t *testing.T) {
	// keep-last 1 + keep-daily 2. These should be unioned.
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(2, time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(3, time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepLast: 1, KeepDaily: 2}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// keep-last 1 keeps ID 3; keep-daily 2 keeps IDs 3 and 2. Union = 2 kept.
	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
	if len(result.Forget) != 1 {
		t.Errorf("forget = %d, want 1", len(result.Forget))
	}
}

func TestApply_HostFilter(t *testing.T) {
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "hostA", nil, nil),
		snap(2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "hostA", nil, nil),
		snap(3, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), "hostA", nil, nil),
		snap(4, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "hostB", nil, nil),
		snap(5, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "hostB", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepLast: 1}, "hostA", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// hostA: keep 1 (most recent = ID 3), forget 2 (IDs 1, 2).
	// hostB: out of scope, both kept (IDs 4, 5).
	// Total: keep 3, forget 2.
	if len(result.Keep) != 3 {
		t.Errorf("keep = %d, want 3", len(result.Keep))
	}
	if len(result.Forget) != 2 {
		t.Errorf("forget = %d, want 2", len(result.Forget))
	}
}

func TestApply_PathFilter(t *testing.T) {
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "host", []string{"/data"}, nil),
		snap(2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "host", []string{"/data"}, nil),
		snap(3, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), "host", []string{"/home"}, nil),
	}

	result, err := Apply(snapshots, Policy{KeepLast: 1}, "", "/data")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// /data: keep 1 (ID 2), forget 1 (ID 1).
	// /home: out of scope, kept (ID 3).
	// Total: keep 2, forget 1.
	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
	if len(result.Forget) != 1 {
		t.Errorf("forget = %d, want 1", len(result.Forget))
	}
}

func TestApply_AllKept(t *testing.T) {
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepLast: 10}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
	if len(result.Forget) != 0 {
		t.Errorf("forget = %d, want 0", len(result.Forget))
	}
}

func TestApply_EmptySnapshotList(t *testing.T) {
	result, err := Apply(nil, Policy{KeepLast: 1}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Keep) != 0 {
		t.Errorf("keep = %d, want 0", len(result.Keep))
	}
	if len(result.Forget) != 0 {
		t.Errorf("forget = %d, want 0", len(result.Forget))
	}
}

func TestApply_SingleSnapshot(t *testing.T) {
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepLast: 1}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Keep) != 1 {
		t.Errorf("keep = %d, want 1", len(result.Keep))
	}
	if len(result.Forget) != 0 {
		t.Errorf("forget = %d, want 0", len(result.Forget))
	}
}

func TestApply_CombinedTagAndLast(t *testing.T) {
	// keep-last 1 + keep-tag "pin". Snapshot 1 is tagged, snapshot 3 is newest.
	// Both should be kept.
	snapshots := []SnapshotInfo{
		snap(1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "host", nil, []string{"pin"}),
		snap(2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "host", nil, nil),
		snap(3, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), "host", nil, nil),
	}

	result, err := Apply(snapshots, Policy{KeepLast: 1, KeepTags: []string{"pin"}}, "", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Keep) != 2 {
		t.Errorf("keep = %d, want 2", len(result.Keep))
	}
	if len(result.Forget) != 1 {
		t.Errorf("forget = %d, want 1", len(result.Forget))
	}
}
