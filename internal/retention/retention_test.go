package retention

import (
	"fmt"
	"sort"
	"strings"
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

func forgottenIDs(r Result) string {
	var ids []string
	for _, s := range r.Forget {
		ids = append(ids, fmt.Sprintf("%d", s.ID[0]))
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func TestApply_ProtectsWorkloadChains(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return base.Add(time.Duration(n) * time.Hour) }
	wl := func(kind string) []string { return []string{"plugin:mssql", "kind:" + kind} }
	snapshots := []SnapshotInfo{
		snap(1, h(0), "sql01", nil, wl("full")),
		snap(2, h(1), "sql01", nil, wl("log")),
		snap(3, h(2), "sql01", nil, wl("differential")),
		snap(4, h(3), "sql01", nil, wl("log")),
		snap(5, h(4), "sql01", nil, wl("full")),
		snap(6, h(5), "sql01", nil, wl("log")),
		snap(7, h(6), "sql01", nil, wl("log")),
		// Another host's chain is independent.
		snap(8, h(0), "sql02", nil, wl("full")),
		snap(9, h(1), "sql02", nil, wl("log")),
		// Filesystem snapshots are untouched by chain rules.
		snap(10, h(0), "sql01", []string{"/etc"}, nil),
	}

	// Keeping only the newest log keeps its chain back to full #5.
	r, err := Apply(snapshots, Policy{KeepLast: 1}, "sql01", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := forgottenIDs(r); got != "1,10,2,3,4" {
		t.Fatalf("forgotten = %s, want 1,10,2,3,4", got)
	}

	// Keeping the differential (#3) keeps full #1 and the log between
	// them: snapshots without object tags keep their whole chain.
	r, _ = Apply(snapshots, Policy{KeepTags: []string{"kind:differential"}}, "sql01", "")
	if got := forgottenIDs(r); got != "10,4,5,6,7" {
		t.Fatalf("forgotten = %s, want 10,4,5,6,7", got)
	}

	// A log with no earlier full keeps everything older in its group.
	r, _ = Apply(snapshots[8:9], Policy{KeepLast: 1}, "", "")
	if len(r.Forget) != 0 {
		t.Fatalf("forgot %v", r.Forget)
	}
}

func TestApply_ProtectsChainsPerObject(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return base.Add(time.Duration(n) * time.Hour) }
	wl := func(kind string, objs ...string) []string {
		tags := []string{"plugin:mssql", "kind:" + kind}
		for _, o := range objs {
			tags = append(tags, "obj:"+o)
		}
		return tags
	}
	snapshots := []SnapshotInfo{
		snap(1, h(0), "sql01", nil, wl("full", "I/a")),                      // a's base
		snap(2, h(1), "sql01", nil, wl("full", "I/b")),                      // b's base, another job
		snap(3, h(2), "sql01", nil, append(wl("full", "I/a"), "copy-only")), // no base for a
		snap(4, h(3), "sql01", nil, wl("log", "I/b")),                       // b's chain
		snap(5, h(4), "sql01", nil, wl("differential", "I/a")),              // a's differential
		snap(6, h(5), "sql01", nil, wl("full", "I/c")),                      // unrelated
		snap(7, h(6), "sql01", nil, wl("log", "I/a", "I/b")),                // both
	}

	// Keeping only the newest log keeps what restores each database as of
	// it: both log chains (a's full #1, not the copy-only #3 nor a's
	// differential, which a log restore does not need; b's log #4 and full
	// #2) and c's newest backup, full #6.
	r, err := Apply(snapshots, Policy{KeepLast: 1}, "sql01", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := forgottenIDs(r); got != "3,5" {
		t.Fatalf("forgotten = %s, want 3,5", got)
	}

	// Keeping differentials keeps a's and its full; b and c have none.
	r, _ = Apply(snapshots, Policy{KeepTags: []string{"kind:differential"}}, "sql01", "")
	if got := forgottenIDs(r); got != "2,3,4,6,7" {
		t.Fatalf("forgotten = %s, want 2,3,4,6,7", got)
	}
}

func TestApply_KeepsDifferentialsOfDatabasesWithoutLogs(t *testing.T) {
	// An instance plan: A (FULL) takes logs, B (SIMPLE) does not. Keeping
	// the newest log backup per day must still keep B's differentials.
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return base.Add(time.Duration(n) * time.Hour) }
	tags := func(kind string, objs ...string) []string {
		t := []string{"plugin:mssql", "kind:" + kind}
		for _, o := range objs {
			t = append(t, "obj:"+o)
		}
		return t
	}
	snapshots := []SnapshotInfo{
		snap(1, h(0), "sql01", nil, tags("full", "I/A", "I/B")),
		snap(2, h(1), "sql01", nil, tags("log", "I/A")),
		snap(3, h(24), "sql01", nil, tags("differential", "I/A", "I/B")),
		snap(4, h(25), "sql01", nil, tags("log", "I/A")),
		snap(5, h(48), "sql01", nil, tags("differential", "I/A", "I/B")),
		snap(6, h(49), "sql01", nil, tags("log", "I/A")),
		snap(7, h(50), "sql01", nil, []string{"plugin:mssql", "kind:log", "objects:none"}),
	}
	r, err := Apply(snapshots, Policy{KeepDaily: 3}, "sql01", "")
	if err != nil {
		t.Fatal(err)
	}
	// Daily heads are #2, #4 and #7. B's state as of each (#1, #3, #5) is
	// kept; A's logs chain back to full #1.
	if got := forgottenIDs(r); got != "" {
		t.Fatalf("forgotten = %s, want none", got)
	}
	// With only the newest day kept: B's differential #5 and its full #1,
	// A's log chain #6, #4, #2 and #1; the logs need no differentials, but
	// #5 stays for B.
	r, _ = Apply(snapshots, Policy{KeepLast: 1}, "sql01", "")
	if got := forgottenIDs(r); got != "3" {
		t.Fatalf("forgotten = %s, want 3", got)
	}
}

func TestApply_CopyOnlyFullsAndLegacyBases(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return base.Add(time.Duration(n) * time.Hour) }

	// A plan of copy-only fulls only: each restores on its own, so older
	// ones go.
	var copies []SnapshotInfo
	for i := 1; i <= 5; i++ {
		copies = append(copies, snap(byte(i), h(24*i), "sql01", nil, []string{"plugin:mssql", "kind:full", "copy-only", "obj:I/a"}))
	}
	r, err := Apply(copies, Policy{KeepLast: 1}, "sql01", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := forgottenIDs(r); got != "1,2,3,4" {
		t.Fatalf("copy-only fulls: forgotten = %s, want 1,2,3,4", got)
	}

	// A snapshot without object tags may hold any object, so the first such
	// full ends a walk, as before tags. forget tags such snapshots from their
	// contents, so this only applies when that cannot be read.
	legacy := []SnapshotInfo{
		snap(1, h(0), "sql01", nil, []string{"plugin:mssql", "kind:full", "job:A"}),
		snap(2, h(1), "sql01", nil, []string{"plugin:mssql", "kind:full", "job:B"}),
		snap(3, h(2), "sql01", nil, []string{"plugin:mssql", "kind:differential", "obj:I/a", "job:A"}),
	}
	r, _ = Apply(legacy, Policy{KeepLast: 1}, "sql01", "")
	if got := forgottenIDs(r); got != "1" {
		t.Fatalf("legacy bases: forgotten = %s, want 1", got)
	}

	// Tagged logs taken after the upgrade but before the job's first tagged
	// full keep the job's untagged logs back to its newest untagged full,
	// not all history.
	var hist []SnapshotInfo
	for d := 0; d < 5; d++ {
		hist = append(hist,
			snap(byte(10*d+1), h(24*d), "sql01", nil, []string{"plugin:mssql", "kind:full", "job:A"}),
			snap(byte(10*d+2), h(24*d+1), "sql01", nil, []string{"plugin:mssql", "kind:log", "job:A"}))
	}
	hist = append(hist, snap(99, h(24*5), "sql01", nil, []string{"plugin:mssql", "kind:log", "obj:I/a", "job:A"}))
	r, _ = Apply(hist, Policy{KeepLast: 1}, "sql01", "")
	if len(r.Keep) != 3 { // #99, the day-4 log and full
		t.Fatalf("kept %d snapshots, want 3: %v", len(r.Keep), r.Keep)
	}
}

func TestApply_PolicyAppliesPerDatabase(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return base.Add(time.Duration(n) * time.Hour) }
	tags := func(kind string, objs ...string) []string {
		t := []string{"plugin:mssql", "kind:" + kind}
		for _, o := range objs {
			t = append(t, "obj:"+o)
		}
		return t
	}
	// X was dropped after day 1, or its backups fail since: later runs hold
	// only A. Each database keeps its own last backup and that backup's
	// chain; nothing else of the old days.
	snapshots := []SnapshotInfo{
		snap(1, h(0), "sql01", nil, tags("full", "I/A", "I/X")),
		snap(2, h(1), "sql01", nil, tags("log", "I/A", "I/X")),
		snap(3, h(24), "sql01", nil, tags("full", "I/A")),
		snap(4, h(25), "sql01", nil, tags("log", "I/A")),
		snap(5, h(48), "sql01", nil, tags("full", "I/A")),
		snap(6, h(49), "sql01", nil, tags("log", "I/A")),
	}
	r, err := Apply(snapshots, Policy{KeepLast: 1}, "sql01", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := forgottenIDs(r); got != "3,4" {
		t.Fatalf("forgotten = %s, want 3,4", got)
	}
}

func TestApply_SeparateJobsAndPartialFulls(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return base.Add(time.Duration(n) * time.Hour) }
	tags := func(job, kind string, objs ...string) []string {
		t := []string{"plugin:mssql", "kind:" + kind, "job:" + job}
		for _, o := range objs {
			t = append(t, "obj:"+o)
		}
		return t
	}
	// Fulls, differentials and logs from three jobs; X (SIMPLE) takes no
	// logs. The newest snapshots are logs, yet X keeps its differential.
	jobs := []SnapshotInfo{
		snap(1, h(0), "sql01", nil, tags("F", "full", "I/A", "I/X")),
		snap(2, h(24), "sql01", nil, tags("D", "differential", "I/A", "I/X")),
		snap(3, h(25), "sql01", nil, tags("L", "log", "I/A")),
		snap(4, h(26), "sql01", nil, tags("L", "log", "I/A")),
	}
	r, err := Apply(jobs, Policy{KeepLast: 1}, "sql01", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := forgottenIDs(r); got != "" {
		t.Fatalf("separate jobs: forgotten = %s, want none (X's differential and full, A's logs and full)", got)
	}

	// X's latest full failed (partial run): X keeps its earlier backups.
	partial := []SnapshotInfo{
		snap(1, h(0), "sql01", nil, tags("J", "full", "I/A", "I/X")),
		snap(2, h(24), "sql01", nil, tags("J", "differential", "I/A", "I/X")),
		snap(3, h(48), "sql01", nil, tags("J", "full", "I/A")),
		snap(4, h(49), "sql01", nil, tags("J", "log", "I/A")),
	}
	r, _ = Apply(partial, Policy{KeepLast: 1}, "sql01", "")
	if got := forgottenIDs(r); got != "" {
		t.Fatalf("partial full: forgotten = %s, want none", got)
	}
}
