package main

import (
	"strings"
	"testing"
	"time"
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
		got := checkChain(tt.kind, "MSSQLSERVER/db", tt.s)
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
			Base: &historyEntry{Type: "D", Device: `E:\b.bak`, Finish: at, User: "sa"}}),
	}
	want := `full backup required: MSSQLSERVER/a (no-full); MSSQLSERVER/b (external-full: full of 2026-10-06 20:00 UTC by sa to E:\b.bak)`
	if err.Error() != want {
		t.Errorf("got  %s\nwant %s", err.Error(), want)
	}
	if !strings.HasPrefix(err.Error(), fullRequiredPrefix) {
		t.Error("missing prefix")
	}
}
