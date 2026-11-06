package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fingerprintOf(t *testing.T, cfg *tls.Config) string {
	t.Helper()
	cert, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return Fingerprint(cert)
}

func TestLoadOrGenerateSelfSignedPersists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent")
	crt, key := filepath.Join(dir, "agent.crt"), filepath.Join(dir, "agent.key")

	first, err := LoadOrGenerateSelfSigned(crt, key, "hitstor-agent")
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrGenerateSelfSigned(crt, key, "hitstor-agent")
	if err != nil {
		t.Fatal(err)
	}
	if fingerprintOf(t, first) != fingerprintOf(t, second) {
		t.Fatal("certificate changed across restarts: pinned peers would reject the agent")
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(key); fi.Mode().Perm() != 0o600 {
			t.Fatalf("key mode = %v, want 0600", fi.Mode().Perm())
		}
	}
}

func TestLoadOrGenerateSelfSignedReplacesDamagedPair(t *testing.T) {
	dir := t.TempDir()
	crt, key := filepath.Join(dir, "agent.crt"), filepath.Join(dir, "agent.key")
	os.WriteFile(crt, []byte("garbage"), 0o644)
	os.WriteFile(key, []byte("garbage"), 0o600)
	cfg, err := LoadOrGenerateSelfSigned(crt, key, "hitstor-agent")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := LoadOrGenerateSelfSigned(crt, key, "hitstor-agent")
	if fingerprintOf(t, cfg) != fingerprintOf(t, again) {
		t.Fatal("regenerated certificate was not saved")
	}
}

func TestLoadOrGenerateSelfSignedUnwritableDir(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a non-root Unix user")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	cfg, err := LoadOrGenerateSelfSigned(filepath.Join(dir, "sub", "a.crt"), filepath.Join(dir, "sub", "a.key"), "hitstor-agent")
	if err != nil || len(cfg.Certificates) != 1 {
		t.Fatalf("cfg = %v, err = %v; want an in-memory certificate", cfg, err)
	}
}
