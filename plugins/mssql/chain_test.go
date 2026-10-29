package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

func TestCheckChain(t *testing.T) {
	at := time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC)
	ours := &historyEntry{Type: "D", Device: "HitStor_92c72bfac6dd19495f17cace", Finish: at, User: "hitstor_backup"}
	native := &historyEntry{Type: "D", Device: `E:\BackupBD\db.bak`, Finish: at, User: `NT AUTHORITY\SYSTEM`}
	nativeLog := &historyEntry{Type: "L", Device: `E:\BackupBD\db.trn`, Finish: at, User: "sa"}
	oursLog := &historyEntry{Type: "L", Device: "HitStor_aa", Finish: at, User: "hitstor_backup"}

	tests := []struct {
		name, kind string
		s          chainState
		want       string // reason, "" for none
	}{
		{"full needs nothing", "full", chainState{}, ""},
		{"diff without full", "differential", chainState{}, reasonNoFull},
		{"diff on HitStor full", "differential", chainState{DiffBaseLSN: "100", Base: ours}, ""},
		{"diff on native full", "differential", chainState{DiffBaseLSN: "100", Base: native}, reasonExternalFull},
		{"diff base not in history", "differential", chainState{DiffBaseLSN: "100"}, reasonUnknownBase},
		{"log without chain", "log", chainState{RecoveryModel: "FULL"}, reasonNoFull},
		{"log after HitStor full", "log", chainState{LastLogLSN: "100", Chain: ours}, ""},
		{"log after HitStor log", "log", chainState{LastLogLSN: "200", Chain: oursLog}, ""},
		{"log after native log", "log", chainState{LastLogLSN: "200", Chain: nativeLog}, reasonExternalLog},
		{"log chain started by native full", "log", chainState{LastLogLSN: "100", Chain: native}, reasonExternalFull},
		{"log chain not in history", "log", chainState{LastLogLSN: "100"}, reasonUnknownBase},
		// A native full after HitStor's does not move the log chain, so
		// HitStor's log backups still restore from HitStor's full.
		{"diff base moved by native full, log still fine", "log", chainState{DiffBaseLSN: "300", Base: native, LastLogLSN: "200", Chain: oursLog}, ""},
	}
	for _, tt := range tests {
		got := checkChain(tt.kind, "MSSQLSERVER/db", tt.s, nil)
		if (got == nil) != (tt.want == "") || (got != nil && got.Reason != tt.want) {
			t.Errorf("%s: got %v, want %q", tt.name, got, tt.want)
		}
	}
}

func TestFullRequiredError(t *testing.T) {
	at := time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC)
	err := fullRequiredError{
		{Object: "MSSQLSERVER/a", Reason: reasonNoFull},
		*checkChain("differential", "MSSQLSERVER/b", chainState{DiffBaseLSN: "1",
			Base: &historyEntry{Type: "D", Device: `E:\b.bak`, Finish: at, User: "sa"}}, nil),
	}
	want := `full backup required: MSSQLSERVER/a (no-full); MSSQLSERVER/b (external-full: full of 2026-10-06 20:00 UTC by sa to E:\b.bak)`
	if err.Error() != want {
		t.Errorf("got  %s\nwant %s", err.Error(), want)
	}
	if !strings.HasPrefix(err.Error(), fullRequiredPrefix) {
		t.Error("missing prefix")
	}
}

func TestCheckChainAgainstKnownBackups(t *testing.T) {
	at := time.Date(2026, 10, 29, 16, 1, 36, 0, time.UTC)
	meta := func(m itemMeta) hitstorplugin.KnownItem {
		m.DatabaseGUID = "a4b9200c-bab2-4325-ba6e-7da5d026bb10"
		raw, _ := json.Marshal(m)
		return hitstorplugin.KnownItem{Item: "MSSQLSERVER/db/x.bak", Metadata: raw}
	}
	held := newKnownBackups(hitstorplugin.BackupRequest{KnownProvided: true, Known: []hitstorplugin.KnownItem{
		meta(itemMeta{Type: typeFull, FirstLSN: "100", CheckpointLSN: "100", LastLSN: "110"}),
		meta(itemMeta{Type: typeLog, FirstLSN: "100", LastLSN: "200"}),
		meta(itemMeta{Type: typeFull, CopyOnly: true, CheckpointLSN: "300", FirstLSN: "300", LastLSN: "310"}),
	}})
	ours := &historyEntry{Type: "L", Device: "HitStor_aa", Finish: at, User: "hitstor_backup"}
	guid := "A4B9200C-BAB2-4325-BA6E-7DA5D026BB10"
	tests := []struct {
		name, kind string
		s          chainState
		want       string
	}{
		{"diff on a held full", "differential", chainState{DatabaseGUID: guid, DiffBaseLSN: "100"}, ""},
		{"diff on a copy-only full is not a base", "differential", chainState{DatabaseGUID: guid, DiffBaseLSN: "300"}, reasonUnknownBase},
		{"log after a held log", "log", chainState{DatabaseGUID: guid, LastLogLSN: "200"}, ""},
		{"log right after the held full", "log", chainState{DatabaseGUID: guid, LastLogLSN: "100"}, ""},
		// HitStor's device, but HitStor never stored it (a run that failed
		// before this fix): the chain has a gap.
		{"log after a HitStor log HitStor does not hold", "log", chainState{DatabaseGUID: guid, LastLogLSN: "250", Chain: ours}, reasonNotHeld},
		{"another database with the same LSNs", "log", chainState{DatabaseGUID: "other", LastLogLSN: "200", Chain: ours}, reasonNotHeld},
	}
	for _, tt := range tests {
		got := checkChain(tt.kind, "MSSQLSERVER/db", tt.s, held)
		if (got == nil) != (tt.want == "") || (got != nil && got.Reason != tt.want) {
			t.Errorf("%s: got %v, want %q", tt.name, got, tt.want)
		}
	}
	// A host that sent its list, even empty, is believed.
	if got := checkChain("log", "MSSQLSERVER/db", chainState{DatabaseGUID: guid, LastLogLSN: "200", Chain: ours},
		newKnownBackups(hitstorplugin.BackupRequest{KnownProvided: true})); got == nil || got.Reason != reasonNotHeld {
		t.Errorf("empty list: got %v", got)
	}
	if newKnownBackups(hitstorplugin.BackupRequest{}) != nil {
		t.Error("a host without a list falls back to msdb")
	}
}

func TestLogChainRestartsAfterAGap(t *testing.T) {
	guid := "A4B9200C-BAB2-4325-BA6E-7DA5D026BB10"
	known := func(object string, ms ...itemMeta) knownBackups {
		var items []hitstorplugin.KnownItem
		for _, m := range ms {
			m.DatabaseGUID = guid
			raw, _ := json.Marshal(m)
			items = append(items, hitstorplugin.KnownItem{Item: object + "/x", Metadata: raw})
		}
		return newKnownBackups(hitstorplugin.BackupRequest{KnownProvided: true, Known: items})
	}
	ours := &historyEntry{Type: "L", Device: "HitStor_lost", User: "hitstor_backup"}
	// The log ending at 500 was taken but never stored: a gap. A full does
	// not move the chain's start (still 500), but the next log covers a
	// full taken after it, so that full is what the gap asks for.
	gap := chainState{DatabaseGUID: guid, LastLogLSN: "500", Chain: ours}
	before := itemMeta{Type: typeFull, FirstLSN: "90", CheckpointLSN: "100", LastLSN: "110"}
	after := itemMeta{Type: typeFull, FirstLSN: "590", CheckpointLSN: "600", LastLSN: "610"}
	if got := checkChain("log", "I/db", gap, known("I/db", before)); got == nil || got.Reason != reasonNotHeld {
		t.Fatalf("gap: got %v", got)
	}
	if got := checkChain("log", "I/db", gap, known("I/db", before, after)); got != nil {
		t.Fatalf("full after the gap: got %v", got)
	}
	// A differential does not: retention would not keep it for the logs.
	diff := itemMeta{Type: typeDifferential, DifferentialBaseLSN: "100", LastLSN: "650"}
	if got := checkChain("log", "I/db", gap, known("I/db", before, diff)); got == nil {
		t.Fatal("a differential after the gap restarted the chain")
	}
	// Held under another name (renamed database): retention keeps chains
	// by name, so it does not count.
	if got := checkChain("log", "I/db2", gap, known("I/db", before, after)); got == nil {
		t.Fatal("backups of the old name")
	}
	if got := checkChain("log", "I/DB", gap, known("I/db", before, after)); got == nil {
		t.Fatal("backups of a name that differs in case")
	}
	// Without a list from the host, msdb's record of a later HitStor full
	// does the same.
	gap.Restart = &historyEntry{Type: "D", Device: "HitStor_full"}
	if got := checkChain("log", "I/db", gap, nil); got != nil {
		t.Fatalf("msdb restart: got %v", got)
	}
}
