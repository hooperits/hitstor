package dashboard

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hooperits/hitstor/internal/config"
)

const (
	defaultTLSDir    = "/etc/hitstor/tls"
	maxTLSCertUpload = 256 * 1024
	maxTLSKeyUpload  = 64 * 1024
)

// TLSStatusView describes the active TLS certificate for the dashboard API.
type TLSStatusView struct {
	Source    string   `json:"source"` // custom, repo, self_signed, unknown
	CertPath  string   `json:"cert_path,omitempty"`
	KeyPath   string   `json:"key_path,omitempty"`
	Subject   string   `json:"subject,omitempty"`
	Issuer    string   `json:"issuer,omitempty"`
	NotBefore string   `json:"not_before,omitempty"`
	NotAfter  string   `json:"not_after,omitempty"`
	DaysLeft  int      `json:"days_left,omitempty"`
	SANs      []string `json:"sans,omitempty"`
	Expired   bool     `json:"expired"`
}

func (d *Dashboard) resolveTLSCertPaths() (certPath, keyPath, source string) {
	cfg, err := d.requireConfig()
	if err == nil {
		if cfg.ServeTLSCert != "" && cfg.ServeTLSKey != "" {
			return cfg.ServeTLSCert, cfg.ServeTLSKey, "custom"
		}
	}

	if d.repos != nil {
		for _, mr := range d.repos.List() {
			if bp := localRepoBasePath(mr.Backend); bp != "" {
				cert := filepath.Join(bp, "server.crt")
				key := filepath.Join(bp, "server.key")
				if fileExists(cert) && fileExists(key) {
					return cert, key, "repo"
				}
			}
		}
	}
	return "", "", "self_signed"
}

func localRepoBasePath(be interface{}) string {
	type basePather interface {
		BasePath() string
	}
	if bp, ok := be.(basePather); ok {
		return bp.BasePath()
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (d *Dashboard) buildTLSStatus() TLSStatusView {
	certPath, keyPath, source := d.resolveTLSCertPaths()
	view := TLSStatusView{
		Source:   source,
		CertPath: certPath,
		KeyPath:  keyPath,
	}
	if certPath == "" {
		return view
	}

	pemData, err := os.ReadFile(certPath)
	if err != nil {
		view.Source = "unknown"
		return view
	}
	cert, err := parseCertificatePEM(pemData)
	if err != nil {
		view.Source = "unknown"
		return view
	}

	view.Subject = cert.Subject.String()
	view.Issuer = cert.Issuer.String()
	view.NotBefore = cert.NotBefore.UTC().Format(time.RFC3339)
	view.NotAfter = cert.NotAfter.UTC().Format(time.RFC3339)
	view.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
	view.Expired = time.Now().After(cert.NotAfter)
	view.SANs = collectSANs(cert)
	return view
}

func parseCertificatePEM(pemData []byte) (*x509.Certificate, error) {
	var block *pem.Block
	rest := pemData
	for len(rest) > 0 {
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		return x509.ParseCertificate(block.Bytes)
	}
	return nil, fmt.Errorf("no certificate found in PEM data")
}

func collectSANs(cert *x509.Certificate) []string {
	sans := append([]string(nil), cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		sans = append(sans, ip.String())
	}
	return sans
}

func (d *Dashboard) handleAPITLSStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, d.buildTLSStatus())
}

func (d *Dashboard) handleAPITLSUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxTLSCertUpload + maxTLSKeyUpload); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	certFile, certHeader, err := r.FormFile("cert")
	if err != nil {
		http.Error(w, "certificate file required", http.StatusBadRequest)
		return
	}
	defer certFile.Close()

	keyFile, keyHeader, err := r.FormFile("key")
	if err != nil {
		http.Error(w, "private key file required", http.StatusBadRequest)
		return
	}
	defer keyFile.Close()

	if certHeader.Size > maxTLSCertUpload || keyHeader.Size > maxTLSKeyUpload {
		http.Error(w, "uploaded file too large", http.StatusBadRequest)
		return
	}

	certPEM, err := io.ReadAll(io.LimitReader(certFile, maxTLSCertUpload+1))
	if err != nil {
		http.Error(w, "read certificate failed", http.StatusBadRequest)
		return
	}
	keyPEM, err := io.ReadAll(io.LimitReader(keyFile, maxTLSKeyUpload+1))
	if err != nil {
		http.Error(w, "read private key failed", http.StatusBadRequest)
		return
	}

	if _, err := parseCertificatePEM(certPEM); err != nil {
		http.Error(w, fmt.Sprintf("invalid certificate: %v", err), http.StatusBadRequest)
		return
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		http.Error(w, fmt.Sprintf("certificate and key do not match: %v", err), http.StatusBadRequest)
		return
	}

	tlsDir := defaultTLSDir
	if err := os.MkdirAll(tlsDir, 0700); err != nil {
		http.Error(w, fmt.Sprintf("create tls dir: %v", err), http.StatusInternalServerError)
		return
	}

	certPath := filepath.Join(tlsDir, "server.crt")
	keyPath := filepath.Join(tlsDir, "server.key")
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		http.Error(w, fmt.Sprintf("write certificate: %v", err), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		os.Remove(certPath)
		http.Error(w, fmt.Sprintf("write private key: %v", err), http.StatusInternalServerError)
		return
	}

	cfg, err := d.requireConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.ServeTLSCert = certPath
	cfg.ServeTLSKey = keyPath
	if cfg.Path == "" {
		cfg.Path = config.DefaultPath()
	}
	cfg.Exists = true
	if err := config.Save(*cfg); err != nil {
		http.Error(w, fmt.Sprintf("save config: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, settingsUpdateResponse{OK: true, RestartRequired: true})
}
