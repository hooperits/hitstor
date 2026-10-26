package agent

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/hooperits/hitstor/internal/tlsutil"
)

// serverPinTLS returns a client tls.Config that pins the server's certificate
// on first use, mirroring the server→agent connection. The fingerprint is
// persisted at pinPath; an empty pinPath keeps the pin for this process only.
//
// When the presented certificate does not match the pin but is valid for
// serverAddr under the system roots (for example after the operator installs
// a CA-issued certificate), it is accepted and the pin is updated.
func serverPinTLS(serverAddr, pinPath string) (*tls.Config, error) {
	pin, err := readServerPin(pinPath)
	if err != nil {
		return nil, err
	}
	record := func(fp string) {
		if pinPath == "" {
			return
		}
		if err := writeServerPin(pinPath, fp); err != nil {
			slog.Warn("agent: could not persist server certificate fingerprint", "path", pinPath, "error", err)
			return
		}
		slog.Info("agent: pinned server certificate", "fingerprint", fp, "path", pinPath)
	}

	cfg := tlsutil.PinnedClientTLS(pin, record)
	verifyPin := cfg.VerifyPeerCertificate
	cfg.VerifyPeerCertificate = func(rawCerts [][]byte, chains [][]*x509.Certificate) error {
		pinErr := verifyPin(rawCerts, chains)
		if pinErr == nil {
			return nil
		}
		if fp, ok := publiclyTrusted(rawCerts, serverAddr); ok {
			record(fp)
			return nil
		}
		if pinPath != "" {
			return fmt.Errorf("%w; if the server certificate was replaced on purpose, delete %s to pin the new one", pinErr, pinPath)
		}
		return pinErr
	}
	return cfg, nil
}

// publiclyTrusted reports whether rawCerts form a chain valid for serverAddr
// under the system roots, returning the leaf fingerprint when it does.
func publiclyTrusted(rawCerts [][]byte, serverAddr string) (string, bool) {
	if len(rawCerts) == 0 {
		return "", false
	}
	certs := make([]*x509.Certificate, 0, len(rawCerts))
	for _, raw := range rawCerts {
		c, err := x509.ParseCertificate(raw)
		if err != nil {
			return "", false
		}
		certs = append(certs, c)
	}
	host := serverAddr
	if h, _, err := net.SplitHostPort(serverAddr); err == nil {
		host = h
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	if _, err := certs[0].Verify(x509.VerifyOptions{DNSName: host, Intermediates: inter}); err != nil {
		return "", false
	}
	return tlsutil.Fingerprint(certs[0]), true
}

func readServerPin(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("agent: read server pin %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func writeServerPin(path, fingerprint string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(fingerprint+"\n"), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
