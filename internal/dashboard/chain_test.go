package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/i18n"
)

func chainItem(t *testing.T, snap string, at time.Time, m chainMeta) workloadPoint {
	m.Finish, m.DatabaseGUID = at, "g1"
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return workloadPoint{SnapshotID: snap, Item: "MSSQLSERVER/db/x.bak", CreatedAt: at, Size: 1 << 20, Metadata: raw}
}

func TestBuildChains(t *testing.T) {
	t0 := time.Date(2026, 10, 29, 11, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	items := []workloadPoint{
		chainItem(t, "log0", at(-30), chainMeta{Type: "L", FirstLSN: "10", LastLSN: "50"}), // before any full
		chainItem(t, "full1", at(0), chainMeta{Type: "D", FirstLSN: "90", LastLSN: "110", CheckpointLSN: "100"}),
		chainItem(t, "log1", at(10), chainMeta{Type: "L", FirstLSN: "100", LastLSN: "200"}),
		chainItem(t, "diff1", at(20), chainMeta{Type: "I", DifferentialBaseLSN: "100", FirstLSN: "240", LastLSN: "250"}),
		chainItem(t, "log2", at(30), chainMeta{Type: "L", FirstLSN: "200", LastLSN: "300"}),
		chainItem(t, "copy", at(35), chainMeta{Type: "D", CopyOnly: true, CheckpointLSN: "305", FirstLSN: "305", LastLSN: "306"}),
		chainItem(t, "full2", at(40), chainMeta{Type: "D", FirstLSN: "310", LastLSN: "330", CheckpointLSN: "320"}),
		chainItem(t, "log3", at(50), chainMeta{Type: "L", FirstLSN: "300", LastLSN: "400"}),
		chainItem(t, "log4", at(60), chainMeta{Type: "L", FirstLSN: "450", LastLSN: "500"}), // gap 400→450
		chainItem(t, "diffX", at(70), chainMeta{Type: "I", DifferentialBaseLSN: "999", FirstLSN: "510", LastLSN: "520"}),
	}
	got := buildChains(i18n.Get("en"), "MSSQLSERVER/db", items)

	if len(got.Chains) != 3 {
		t.Fatalf("chains = %d, want 3 (full2, copy-only, full1)", len(got.Chains))
	}
	cur, copyOnly, first := got.Chains[0], got.Chains[1], got.Chains[2]
	if cur.Full.Snapshot != "full2" || !cur.Current || !cur.Broken || len(cur.Logs) != 2 || !cur.Logs[0].Gap || cur.Logs[1].Gap {
		t.Errorf("current chain = %+v", cur)
	}
	if !copyOnly.Full.CopyOnly || len(copyOnly.Diffs)+len(copyOnly.Logs) != 0 {
		t.Errorf("copy-only full took backups: %+v", copyOnly)
	}
	if first.Full.Snapshot != "full1" || len(first.Diffs) != 1 || first.Diffs[0].Snapshot != "diff1" ||
		len(first.Logs) != 2 || first.Broken || first.Current {
		t.Errorf("first chain = %+v", first)
	}
	if len(got.Orphans) != 2 || got.Orphans[0].Snapshot != "diffX" || got.Orphans[1].Snapshot != "log0" {
		t.Errorf("orphans = %+v", got.Orphans)
	}
}

// A chain of only a full backup still lists its (empty) differentials and
// logs: the page reads them as arrays.
func TestChainWithOnlyAFullHasEmptyLists(t *testing.T) {
	items := []workloadPoint{chainItem(t, "full", time.Now(), chainMeta{Type: "D", FirstLSN: "1", LastLSN: "2", CheckpointLSN: "1"})}
	raw, err := json.Marshal(buildChains(i18n.Get("en"), "MSSQLSERVER/db", items))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Chains []map[string]json.RawMessage `json:"chains"`
	}
	json.Unmarshal(raw, &got)
	if len(got.Chains) != 1 || string(got.Chains[0]["differentials"]) != "[]" || string(got.Chains[0]["logs"]) != "[]" {
		t.Errorf("chain JSON = %s", raw)
	}
}

func TestParseFullRequired(t *testing.T) {
	c := i18n.Get("en")
	msg := `MSSQLSERVER/db: full backup required: MSSQLSERVER/a (no-full); MSSQLSERVER/b (external-full: full of 2026-10-06 20:00 UTC by NT AUTHORITY\SYSTEM to E:\BackupBD\b.bak)`
	got := parseFullRequired(c, "differential", msg)
	if len(got) != 2 || got[0].Object != "MSSQLSERVER/a" || got[0].Reason != "no-full" ||
		got[1].Reason != "external-full" || got[1].By != `NT AUTHORITY\SYSTEM` || got[1].Where != `E:\BackupBD\b.bak` || got[1].When != "2026-10-06 20:00 UTC" {
		t.Fatalf("got %+v", got)
	}
	if parseFullRequired(c, "log", "login failed") != nil {
		t.Fatal("other errors are not full-required")
	}
}
