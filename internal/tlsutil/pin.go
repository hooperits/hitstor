package tlsutil

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
)

// Fingerprint returns the lowercase hex SHA-256 digest of the certificate's
// DER encoding, used to pin self-signed peer certificates.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// PinnedClientTLS returns a client tls.Config implementing trust-on-first-use
// pinning for self-signed peer certificates.
//
// When pin is non-empty the peer's leaf certificate fingerprint must match it
// or the handshake fails. When pin is empty the certificate is accepted and
// its fingerprint is passed to record (if non-nil) so the caller can persist
// it for future connections.
//
// Chain verification stays disabled (the peers use self-signed certificates);
// the fingerprint check in VerifyPeerCertificate replaces it.
func PinnedClientTLS(pin string, record func(fingerprint string)) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("tlsutil: peer presented no certificate")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("tlsutil: parse peer certificate: %w", err)
			}
			fp := Fingerprint(cert)
			if pin != "" {
				if subtle.ConstantTimeCompare([]byte(fp), []byte(pin)) != 1 {
					return fmt.Errorf("tlsutil: peer certificate fingerprint %s does not match pinned %s (possible MITM or reinstalled peer)", fp, pin)
				}
				return nil
			}
			if record != nil {
				record(fp)
			}
			return nil
		},
	}
}
