package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strings"
	"sync"
	"testing"
)

// startTLSServer starts a TLS listener with a self-signed cert and returns its
// address plus the cert's expected fingerprint.
func startTLSServer(t *testing.T) (addr, fingerprint string) {
	t.Helper()
	cfg, certPEM, _, err := GenerateSelfSigned("test-server")
	if err != nil {
		t.Fatalf("GenerateSelfSigned() error = %v", err)
	}
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	fingerprint = Fingerprint(cert)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("tls.Listen() error = %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Drive the handshake from the server side, then close.
			if tc, ok := conn.(*tls.Conn); ok {
				_ = tc.Handshake()
			}
			conn.Close()
		}
	}()
	return ln.Addr().String(), fingerprint
}

func dialWith(t *testing.T, addr string, cfg *tls.Config) error {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("net.Dial() error = %v", err)
	}
	defer conn.Close()
	tlsConn := tls.Client(conn, cfg)
	defer tlsConn.Close()
	return tlsConn.Handshake()
}

func TestFingerprintFormat(t *testing.T) {
	_, fp := startTLSServer(t)
	if len(fp) != 64 || strings.ToLower(fp) != fp {
		t.Errorf("Fingerprint() = %q, want 64 lowercase hex chars", fp)
	}
}

func TestPinnedClientTLSFirstUseRecords(t *testing.T) {
	addr, wantFP := startTLSServer(t)

	var mu sync.Mutex
	var recorded string
	cfg := PinnedClientTLS("", func(fp string) {
		mu.Lock()
		recorded = fp
		mu.Unlock()
	})

	if err := dialWith(t, addr, cfg); err != nil {
		t.Fatalf("first-use handshake error = %v, want success", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if recorded != wantFP {
		t.Errorf("recorded fingerprint = %q, want %q", recorded, wantFP)
	}
}

func TestPinnedClientTLSMatchingPin(t *testing.T) {
	addr, fp := startTLSServer(t)
	cfg := PinnedClientTLS(fp, nil)
	if err := dialWith(t, addr, cfg); err != nil {
		t.Fatalf("handshake with matching pin error = %v, want success", err)
	}
}

func TestPinnedClientTLSWrongPinRejects(t *testing.T) {
	addr, _ := startTLSServer(t)
	wrong := strings.Repeat("ab", 32)
	cfg := PinnedClientTLS(wrong, nil)
	err := dialWith(t, addr, cfg)
	if err == nil {
		t.Fatal("handshake with wrong pin succeeded, want failure")
	}
	if !strings.Contains(err.Error(), "fingerprint") {
		t.Errorf("error = %v, want fingerprint mismatch error", err)
	}
}
