package server

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hooperits/hitstor/internal/tlsutil"
)

// LoadTLS pre-loads TLS credentials so they can be reused before Start() is called.
func (s *Server) LoadTLS() error {
	tlsCfg, err := s.loadTLS()
	if err != nil {
		return fmt.Errorf("server: TLS setup: %w", err)
	}
	s.tlsConfig = tlsCfg
	return nil
}

// DashboardTLSConfig returns the server's TLS configuration for reuse by the dashboard.
func (s *Server) DashboardTLSConfig() *tls.Config {
	return s.tlsConfig
}

// loadTLS loads or generates TLS credentials.
func (s *Server) loadTLS() (*tls.Config, error) {
	// Priority 1: explicit cert/key flags.
	if s.opts.TLSCert != "" && s.opts.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(s.opts.TLSCert, s.opts.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("load TLS keypair: %w", err)
		}
		slog.Info("TLS certificate loaded", "cert", s.opts.TLSCert)
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}

	// Priority 2: reuse existing auto-generated cert from repo dir.
	if basePath := s.repoBasePath(); basePath != "" {
		certPath := filepath.Join(basePath, "server.crt")
		keyPath := filepath.Join(basePath, "server.key")
		if _, err := os.Stat(certPath); err == nil {
			if _, err := os.Stat(keyPath); err == nil {
				cert, err := tls.LoadX509KeyPair(certPath, keyPath)
				if err == nil {
					slog.Info("TLS certificate loaded from repo", "cert", certPath)
					return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
				}
				slog.Warn("failed to load existing TLS cert, regenerating", "error", err)
			}
		}
	}

	// Priority 3: generate new self-signed certificate.
	return s.generateSelfSignedTLS()
}

// generateSelfSignedTLS creates a self-signed TLS certificate using the
// shared tlsutil package. Saves the cert/key to the repo directory if available.
func (s *Server) generateSelfSignedTLS() (*tls.Config, error) {
	tlsCfg, certPEM, keyPEM, err := tlsutil.GenerateSelfSigned("hitstor-server")
	if err != nil {
		return nil, err
	}

	// Try to save to repo directory for persistence across restarts.
	if basePath := s.repoBasePath(); basePath != "" {
		certPath := filepath.Join(basePath, "server.crt")
		keyPath := filepath.Join(basePath, "server.key")
		os.WriteFile(certPath, certPEM, 0644)
		os.WriteFile(keyPath, keyPEM, 0600)
		slog.Info("TLS certificate generated", "cert", certPath)
	} else {
		slog.Info("TLS certificate generated (in-memory)")
	}

	return tlsCfg, nil
}
