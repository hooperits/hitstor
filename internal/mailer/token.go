package mailer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// exchangeScope is the token audience Exchange Online's SMTP accepts.
const exchangeScope = "https://outlook.office365.com/.default"

// tokenSource gets app-only access tokens from the Microsoft identity
// platform (client credentials grant) and keeps one until shortly before
// it expires.
type tokenSource struct {
	url, clientID, secret string
	client                *http.Client

	mu      sync.Mutex
	value   string
	expires time.Time
}

func newTokenSource(tenant, clientID, secret, tokenURL string) *tokenSource {
	if tokenURL == "" {
		tokenURL = "https://login.microsoftonline.com/" + url.PathEscape(tenant) + "/oauth2/v2.0/token"
	}
	return &tokenSource{url: tokenURL, clientID: clientID, secret: secret, client: &http.Client{Timeout: 20 * time.Second}}
}

func (s *tokenSource) reset() {
	s.mu.Lock()
	s.value = ""
	s.mu.Unlock()
}

func (s *tokenSource) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value != "" && time.Until(s.expires) > time.Minute {
		return s.value, nil
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {s.clientID},
		"client_secret": {s.secret},
		"scope":         {exchangeScope},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get a Microsoft 365 token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &out); err != nil || resp.StatusCode != http.StatusOK || out.AccessToken == "" {
		msg := out.Description
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		if i := strings.IndexAny(msg, "\r\n"); i > 0 {
			msg = msg[:i] // the first line names the problem (AADSTS code)
		}
		return "", fmt.Errorf("get a Microsoft 365 token: %s: %s", resp.Status, msg)
	}
	s.value, s.expires = out.AccessToken, time.Now().Add(time.Duration(out.ExpiresIn)*time.Second)
	return s.value, nil
}
