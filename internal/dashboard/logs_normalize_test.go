package dashboard

import (
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/logstore"
)

func TestParseSlogTextLine(t *testing.T) {
	line := `time=2026-06-26T21:20:52.408Z level=WARN msg="M365 backup completed with partial mail collection"`
	e, ok := parseSlogTextLine(line)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if e.Level != "warn" {
		t.Fatalf("level = %q", e.Level)
	}
	if !stringsContains(e.Message, "partial mail collection") {
		t.Fatalf("msg = %q", e.Message)
	}
	if e.Time.UTC().Format(time.RFC3339) != "2026-06-26T21:20:52Z" {
		t.Fatalf("time = %s", e.Time)
	}
}

func TestDedupeLogEntries(t *testing.T) {
	ts := time.Date(2026, 6, 26, 21, 0, 0, 0, time.UTC)
	a := logstore.Entry{Time: ts, Level: "info", Source: "server", Message: "http: TLS handshake error from 1.2.3.4: remote error: tls: unknown certificate"}
	b := logstore.Entry{Time: ts, Level: "info", Source: "system", Message: `time=2026-06-26T21:00:00Z level=INFO msg="http: TLS handshake error from 1.2.3.4: remote error: tls: unknown certificate"`}
	c := logstore.Entry{Time: ts, Level: "warn", Source: "server", Message: "M365 backup completed with partial mail collection"}
	d := logstore.Entry{Time: ts, Level: "info", Source: "server", Message: "http: TLS handshake error from 192.168.56.1:53682: EOF"}

	out := dedupeLogEntries([]logstore.Entry{a, b, c, d})
	if len(out) != 1 {
		t.Fatalf("deduped len = %d, want 1 (TLS noise removed)", len(out))
	}
	if out[0].Message != c.Message {
		t.Fatalf("got %q", out[0].Message)
	}
}

func TestDedupePrefersServerOverSystem(t *testing.T) {
	ts := time.Date(2026, 6, 26, 21, 0, 0, 0, time.UTC)
	system := logstore.Entry{Time: ts, Level: "info", Source: "system", Message: `time=2026-06-26T21:00:00Z level=WARN msg="M365 mail: no folders in scope"`}
	server := logstore.Entry{Time: ts, Level: "warn", Source: "server", Message: "M365 mail: no folders in scope"}

	out := dedupeLogEntries([]logstore.Entry{system, server})
	if len(out) != 1 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].Source != "server" {
		t.Fatalf("source = %q", out[0].Source)
	}
	if out[0].Level != "warn" {
		t.Fatalf("level = %q", out[0].Level)
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
