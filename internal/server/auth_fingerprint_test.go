package server

import (
	"strings"
	"testing"
)

func TestSetAgentFingerprint(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddAgent("web-01"); err != nil {
		t.Fatal(err)
	}

	fp := strings.Repeat("ab", 32)
	if err := reg.SetAgentFingerprint("web-01", fp); err != nil {
		t.Fatalf("SetAgentFingerprint() error = %v", err)
	}
	if got := reg.GetAgent("web-01").TLSFingerprint; got != fp {
		t.Errorf("TLSFingerprint = %q, want %q", got, fp)
	}

	// Persisted across reload.
	reg2, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reg2.GetAgent("web-01").TLSFingerprint; got != fp {
		t.Errorf("after reload TLSFingerprint = %q, want %q", got, fp)
	}
}

func TestReRegisterAgentClearsFingerprint(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddManagedAgent("web-01", "10.0.0.1:8422", "tok"); err != nil {
		t.Fatal(err)
	}
	if err := reg.SetAgentFingerprint("web-01", strings.Repeat("ab", 32)); err != nil {
		t.Fatal(err)
	}

	// Re-registration means the agent was reinstalled and has a new
	// certificate; the stale pin must be dropped so TOFU can re-record.
	if _, err := reg.ReRegisterAgent("web-01", "newtok"); err != nil {
		t.Fatal(err)
	}
	if got := reg.GetAgent("web-01").TLSFingerprint; got != "" {
		t.Errorf("TLSFingerprint after re-register = %q, want empty", got)
	}
}

func TestSetAgentFingerprintUnknownAgent(t *testing.T) {
	reg := NewEmptyRegistry()
	if err := reg.SetAgentFingerprint("ghost", "aa"); err == nil {
		t.Error("SetAgentFingerprint(unknown) error = nil, want error")
	}
}

func TestHandlerRegistrationConcurrentSafe(t *testing.T) {
	srv := New(nil, NewEmptyRegistry(), Options{})

	// Handlers may be swapped while server goroutines fire notifications;
	// this must be race-free (run with -race).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			srv.SetBackupFailedHandler(func(string, string, string, map[string]any) {})
			srv.SetThreatDetectedHandler(func(string, string, string, float64, map[string]any) {})
		}
	}()
	for i := 0; i < 100; i++ {
		srv.notifyBackupFailed("a", "h", "r", nil)
		srv.notifyThreatDetected("a", "h", "r", 0.5, nil)
	}
	<-done
}
