package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

var t0 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

func at(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }

func cand(ref string, m itemMeta) hitstorplugin.RestoreCandidate {
	if m.DatabaseGUID == "" {
		m.DatabaseGUID = "db-guid-1"
	}
	raw, _ := json.Marshal(m)
	return hitstorplugin.RestoreCandidate{Ref: ref, Metadata: raw, CreatedAt: m.Finish}
}

// A realistic chain. LSNs grow with time; each log starts where the
// previous one ended.
//
//	full1  h0  ckpt 100 last 110
//	log1   h1  100..200
//	diff1  h2  base 100, last 250
//	log2   h3  200..300
//	full2  h4  ckpt 320 last 330   (new differential base)
//	log3   h5  300..400
//	log4   h6  400..500
func chain() []hitstorplugin.RestoreCandidate {
	return []hitstorplugin.RestoreCandidate{
		cand("full1", itemMeta{Type: typeFull, CheckpointLSN: "100", FirstLSN: "90", LastLSN: "110", Finish: at(0)}),
		cand("log1", itemMeta{Type: typeLog, FirstLSN: "100", LastLSN: "200", Finish: at(1)}),
		cand("diff1", itemMeta{Type: typeDifferential, DifferentialBaseLSN: "100", FirstLSN: "240", LastLSN: "250", CheckpointLSN: "245", Finish: at(2)}),
		cand("log2", itemMeta{Type: typeLog, FirstLSN: "200", LastLSN: "300", Finish: at(3)}),
		cand("full2", itemMeta{Type: typeFull, CheckpointLSN: "320", FirstLSN: "310", LastLSN: "330", Finish: at(4)}),
		cand("log3", itemMeta{Type: typeLog, FirstLSN: "300", LastLSN: "400", Finish: at(5)}),
		cand("log4", itemMeta{Type: typeLog, FirstLSN: "400", LastLSN: "500", Finish: at(6)}),
	}
}

func mustPlan(t *testing.T, target time.Time, cands []hitstorplugin.RestoreCandidate) string {
	t.Helper()
	refs, err := planRestore(target, cands)
	if err != nil {
		t.Fatalf("planRestore(%v): %v", target, err)
	}
	return strings.Join(refs, ",")
}

func TestPlanLatestUsesNewestFullAndFollowingLogs(t *testing.T) {
	if got := mustPlan(t, time.Time{}, chain()); got != "full2,log3,log4" {
		t.Fatalf("plan = %s", got)
	}
}

func TestPlanPointInTime(t *testing.T) {
	cases := map[string]struct {
		target time.Time
		want   string
	}{
		"right after full1":        {at(0).Add(10 * time.Minute), "full1,log1"},
		"after diff, before log2":  {at(2).Add(30 * time.Minute), "full1,diff1,log2"},
		"exactly at diff finish":   {at(2), "full1,diff1"},
		"between full2 and log3":   {at(4).Add(30 * time.Minute), "full2,log3"},
		"inside the last log":      {at(5).Add(30 * time.Minute), "full2,log3,log4"},
		"after everything":         {at(9), "full2,log3,log4"},
		"exactly at a log finish":  {at(5), "full2,log3"},
		"exactly at full2 finish":  {at(4), "full2"},
		"before diff, inside log1": {at(1).Add(-time.Minute), "full1,log1"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := mustPlan(t, c.target, chain()); got != c.want {
				t.Fatalf("plan = %s, want %s", got, c.want)
			}
		})
	}
}

func TestPlanFullOnlySimpleRecovery(t *testing.T) {
	cands := []hitstorplugin.RestoreCandidate{
		cand("f1", itemMeta{Type: typeFull, CheckpointLSN: "10", LastLSN: "20", Finish: at(0)}),
		cand("f2", itemMeta{Type: typeFull, CheckpointLSN: "30", LastLSN: "40", Finish: at(24)}),
		cand("d2", itemMeta{Type: typeDifferential, DifferentialBaseLSN: "30", LastLSN: "60", Finish: at(30)}),
	}
	if got := mustPlan(t, time.Time{}, cands); got != "f2,d2" {
		t.Fatalf("latest = %s", got)
	}
	if got := mustPlan(t, at(12), cands); got != "f1" {
		t.Fatalf("at h12 = %s", got)
	}
}

func TestPlanBrokenLogChainStopsAtGap(t *testing.T) {
	cands := []hitstorplugin.RestoreCandidate{
		cand("full", itemMeta{Type: typeFull, CheckpointLSN: "100", LastLSN: "110", Finish: at(0)}),
		cand("log1", itemMeta{Type: typeLog, FirstLSN: "100", LastLSN: "200", Finish: at(1)}),
		// log covering 200..300 is missing
		cand("log3", itemMeta{Type: typeLog, FirstLSN: "300", LastLSN: "400", Finish: at(3)}),
	}
	if got := mustPlan(t, time.Time{}, cands); got != "full,log1" {
		t.Fatalf("plan = %s", got)
	}
}

func TestPlanCopyOnlyFullIsNotADifferentialBase(t *testing.T) {
	cands := []hitstorplugin.RestoreCandidate{
		cand("full", itemMeta{Type: typeFull, CheckpointLSN: "100", LastLSN: "110", Finish: at(0)}),
		cand("copy", itemMeta{Type: typeFull, CopyOnly: true, CheckpointLSN: "150", LastLSN: "160", Finish: at(1)}),
		cand("diff", itemMeta{Type: typeDifferential, DifferentialBaseLSN: "100", LastLSN: "200", Finish: at(2)}),
	}
	// The copy-only full is newest at h1; the diff is based on "full".
	if got := mustPlan(t, at(1), cands); got != "copy" {
		t.Fatalf("at h1 = %s", got)
	}
	// Latest: copy is the newest full, and the diff does not apply to it.
	if got := mustPlan(t, time.Time{}, cands); got != "copy" {
		t.Fatalf("latest = %s", got)
	}
}

func TestPlanIgnoresOtherDatabasesAndForeignItems(t *testing.T) {
	cands := []hitstorplugin.RestoreCandidate{
		cand("full", itemMeta{Type: typeFull, CheckpointLSN: "100", LastLSN: "110", Finish: at(0)}),
		// Same name, recreated database: different GUID.
		cand("other-log", itemMeta{Type: typeLog, DatabaseGUID: "db-guid-2", FirstLSN: "100", LastLSN: "200", Finish: at(1)}),
		{Ref: "junk", Metadata: []byte(`{"object":"demo"}`)},
		{Ref: "nometa"},
	}
	if got := mustPlan(t, time.Time{}, cands); got != "full" {
		t.Fatalf("plan = %s", got)
	}
}

func TestPlanNoFull(t *testing.T) {
	cands := []hitstorplugin.RestoreCandidate{
		cand("log", itemMeta{Type: typeLog, FirstLSN: "1", LastLSN: "2", Finish: at(1)}),
	}
	if _, err := planRestore(time.Time{}, cands); err == nil {
		t.Fatal("planned without a full backup")
	}
	if _, err := planRestore(at(-1), chain()); err == nil || !strings.Contains(err.Error(), "at or before") {
		t.Fatalf("err = %v", err)
	}
}
