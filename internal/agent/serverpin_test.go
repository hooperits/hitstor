package agent

import (
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/tlsutil"
)

// startPinTestServer starts a TLS listener with a fresh self-signed cert and
// returns its address.
func startPinTestServer(t *testing.T) string {
	t.Helper()
	cfg, _, _, err := tlsutil.GenerateSelfSigned("pin-test-server")
	if err != nil {
		t.Fatalf("GenerateSelfSigned() error = %v", err)
	}
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
			if tc, ok := conn.(*tls.Conn); ok {
				_ = tc.Handshake()
			}
			conn.Close()
		}
	}()
	return ln.Addr().String()
}

func pinHandshake(t *testing.T, addr string, cfg *tls.Config) error {
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

func TestServerPinTLS(t *testing.T) {
	tests := []struct {
		name    string
		seedPin func(t *testing.T, addr string, path string) // nil: no pin file
		wantErr string
	}{
		{name: "first use records pin"},
		{
			name: "matching pin accepted",
			seedPin: func(t *testing.T, addr, path string) {
				cfg, err := serverPinTLS(addr, path)
				if err != nil {
					t.Fatal(err)
				}
				if err := pinHandshake(t, addr, cfg); err != nil {
					t.Fatalf("seed handshake: %v", err)
				}
			},
		},
		{
			name: "mismatched pin rejected with hint",
			seedPin: func(t *testing.T, _, path string) {
				if err := os.WriteFile(path, []byte(strings.Repeat("ab", 32)+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "delete",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := startPinTestServer(t)
			path := filepath.Join(t.TempDir(), "server-fingerprint")
			if tt.seedPin != nil {
				tt.seedPin(t, addr, path)
			}
			before, _ := os.ReadFile(path)

			cfg, err := serverPinTLS(addr, path)
			if err != nil {
				t.Fatalf("serverPinTLS() error = %v", err)
			}
			err = pinHandshake(t, addr, cfg)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("handshake error = %v, want error containing %q", err, tt.wantErr)
				}
				after, _ := os.ReadFile(path)
				if string(after) != string(before) {
					t.Errorf("pin file changed on rejected handshake: %q -> %q", before, after)
				}
				return
			}
			if err != nil {
				t.Fatalf("handshake error = %v, want success", err)
			}
			pin, _ := os.ReadFile(path)
			if got := strings.TrimSpace(string(pin)); len(got) != 64 {
				t.Errorf("pin file = %q, want 64-char fingerprint", got)
			}
		})
	}
}

func TestServerPinTLSNoPathKeepsPinInMemory(t *testing.T) {
	addr := startPinTestServer(t)
	cfg, err := serverPinTLS(addr, "")
	if err != nil {
		t.Fatalf("serverPinTLS() error = %v", err)
	}
	if err := pinHandshake(t, addr, cfg); err != nil {
		t.Fatalf("handshake error = %v, want success", err)
	}
}
