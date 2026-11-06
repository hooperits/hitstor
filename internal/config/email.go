package config

import "fmt"

// EmailConfig is the outgoing mail server the dashboard uses for account
// emails: invitations and password resets. Secrets are encrypted at rest
// with the repository key, like the integrations'.
type EmailConfig struct {
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Security string `yaml:"security,omitempty"` // none, starttls, tls
	Auth     string `yaml:"auth,omitempty"`     // none, password, microsoft365

	Username          string `yaml:"username,omitempty"`
	PasswordEncrypted string `yaml:"password_encrypted,omitempty"`

	TenantID              string `yaml:"tenant_id,omitempty"`
	ClientID              string `yaml:"client_id,omitempty"`
	ClientSecretEncrypted string `yaml:"client_secret_encrypted,omitempty"`

	From          string `yaml:"from,omitempty"`
	FromName      string `yaml:"from_name,omitempty"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify,omitempty"`

	// DashboardURL is the dashboard's address as people reach it, the base
	// of the links in emails (never taken from a request's Host header).
	DashboardURL string `yaml:"dashboard_url,omitempty"`
}

// Configured reports whether account emails can be sent.
func (e EmailConfig) Configured() bool {
	return e.Host != "" && e.From != "" && e.DashboardURL != ""
}

// EmailSecrets are an email configuration's decrypted secrets.
type EmailSecrets struct {
	Password     string
	ClientSecret string
}

// ResolveEmailSecrets decrypts the email password and client secret.
func ResolveEmailSecrets(e EmailConfig, encKey [32]byte, hasKey bool) (EmailSecrets, error) {
	var out EmailSecrets
	for _, s := range []struct {
		enc string
		dst *string
	}{{e.PasswordEncrypted, &out.Password}, {e.ClientSecretEncrypted, &out.ClientSecret}} {
		if s.enc == "" {
			continue
		}
		if !hasKey {
			return out, fmt.Errorf("config: cannot decrypt email secrets: no encryption key")
		}
		v, err := DecryptCredential(encKey, s.enc)
		if err != nil {
			return out, err
		}
		*s.dst = v
	}
	return out, nil
}
