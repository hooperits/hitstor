// Package tlsutil provides shared TLS utilities for HitStor components.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// GenerateSelfSigned creates a self-signed TLS certificate with all local
// IP addresses in the SANs. Returns a tls.Config ready for use.
// The commonName parameter sets the certificate's CN field.
func GenerateSelfSigned(commonName string) (*tls.Config, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	// Discover all local IPs for certificate SANs.
	ips := []net.IP{net.ParseIP("127.0.0.1")}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				ips = append(ips, ipNet.IP)
			}
		}
	}

	dnsNames := []string{"localhost"}
	if hostname, err := os.Hostname(); err == nil && hostname != "localhost" {
		dnsNames = append(dnsNames, hostname)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  ips,
		DNSNames:     dnsNames,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse generated keypair: %w", err)
	}

	cfg := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	return cfg, certPEM, keyPEM, nil
}

// LoadOrGenerateSelfSigned reuses the certificate and key at certPath and
// keyPath, or generates a self-signed pair and saves it there. Peers pin the
// certificate's fingerprint on first contact, so it must survive restarts:
// a new certificate on every start would look like a man in the middle.
// If the pair cannot be saved, the generated one is used in memory.
func LoadOrGenerateSelfSigned(certPath, keyPath, commonName string) (*tls.Config, error) {
	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	} else if !os.IsNotExist(err) {
		slog.Warn("tlsutil: stored certificate unusable, generating a new one", "cert", certPath, "error", err)
	}
	cfg, certPEM, keyPEM, err := GenerateSelfSigned(commonName)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		slog.Warn("tlsutil: certificate not saved", "error", err)
		return cfg, nil
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		slog.Warn("tlsutil: certificate not saved", "error", err)
		return cfg, nil
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		slog.Warn("tlsutil: certificate not saved", "error", err)
		return cfg, nil
	}
	slog.Info("tlsutil: certificate generated", "cert", certPath)
	return cfg, nil
}
