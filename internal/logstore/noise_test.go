package logstore

import (
	"testing"
	"time"
)

func TestIsNoiseTLSUnknownCert(t *testing.T) {
	e := Entry{Message: "http: TLS handshake error from 1.2.3.4: remote error: tls: unknown certificate"}
	if !IsNoise(e) {
		t.Fatal("expected noise")
	}
}

func TestIsNoiseTLSHandshakeEOF(t *testing.T) {
	e := Entry{Message: "http: TLS handshake error from 192.168.56.1:53682: EOF"}
	if !IsNoise(e) {
		t.Fatal("expected EOF handshake noise")
	}
}

func TestIsNoiseRealError(t *testing.T) {
	e := Entry{Message: "M365 collection failed: token expired"}
	if IsNoise(e) {
		t.Fatal("expected real error to remain")
	}
}

func TestParseSlogTextLine(t *testing.T) {
	line := `time=2026-06-26T21:20:52.408Z level=WARN msg="M365 backup completed with partial mail collection"`
	got, ok := ParseSlogTextLine(line)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if got.Level != "warn" {
		t.Fatalf("level = %q", got.Level)
	}
	if got.Time.UTC().Format(time.RFC3339) != "2026-06-26T21:20:52Z" {
		t.Fatalf("time = %s", got.Time)
	}
}
