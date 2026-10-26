package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/hooperits/hitstor/internal/config"
)

// DefaultRecoveryTokenTTL bounds how long a minted recovery token stays
// usable.
//
// A day is long enough to cover an outage that starts overnight and is dealt
// with the next morning, and short enough that a token which leaked is dead
// before anyone finds it. Callers wanting a tighter window pass their own.
const DefaultRecoveryTokenTTL = 24 * time.Hour

// recoveryTokenBytes is the token's entropy. Same size as an agent API key.
const recoveryTokenBytes = 32

// RecoveryToken authorizes one bare-metal recovery for one host.
//
// It exists because the rescue media deliberately carries no credentials: an
// ISO on a shelf must not be a way into the repository. The token is minted
// when an operator prepares a recovery, typed in at the rescue prompt, and
// dead immediately afterwards.
//
// Stored hashed, exactly as agent API keys are: a registry file that leaks
// must not hand over a live recovery.
type RecoveryToken struct {
	// ID names the token for revocation without revealing it.
	ID string `json:"id"`
	// Agent is the only host this token may recover.
	Agent     string    `json:"agent"`
	Hash      [32]byte  `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	// UsedAt records when the token was consumed. Spent tokens are kept
	// rather than deleted so a second attempt can be told apart from an
	// unknown token in the audit log.
	UsedAt *time.Time `json:"used_at,omitempty"`
}

// Live reports whether the token can still be used.
func (t RecoveryToken) Live(now time.Time) bool {
	return t.UsedAt == nil && now.Before(t.ExpiresAt)
}

// hashRecoveryToken derives the stored form of a token. Domain-separated from
// hashAPIKey so a value valid as one can never be valid as the other.
func hashRecoveryToken(token []byte) [32]byte {
	mac := hmac.New(sha256.New, token)
	mac.Write([]byte("hitstor-recovery-token"))
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// MintRecoveryToken issues a single-use, host-bound recovery token and returns
// its plaintext, which is shown once and never stored.
//
// A ttl of zero uses DefaultRecoveryTokenTTL.
func (r *AgentRegistry) MintRecoveryToken(agent string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = DefaultRecoveryTokenTTL
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.getAgentLocked(agent) == nil {
		return "", fmt.Errorf("server: agent %q not found", agent)
	}

	var secret [recoveryTokenBytes]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", fmt.Errorf("server: generate recovery token: %w", err)
	}
	var idBytes [8]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return "", fmt.Errorf("server: generate recovery token id: %w", err)
	}

	now := time.Now()
	// Drop tokens nobody can use any more, so the registry does not grow a
	// tail of dead records over the life of a server.
	r.pruneRecoveryTokensLocked(now)

	r.recoveryTokens = append(r.recoveryTokens, RecoveryToken{
		ID:        hex.EncodeToString(idBytes[:]),
		Agent:     agent,
		Hash:      hashRecoveryToken(secret[:]),
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	})

	if err := r.save(); err != nil {
		r.recoveryTokens = r.recoveryTokens[:len(r.recoveryTokens)-1]
		return "", err
	}
	return hex.EncodeToString(secret[:]), nil
}

// ConsumeRecoveryToken validates a token for the named host and spends it.
//
// Consumption happens under the registry's write lock, so two rescue hosts
// racing on the same token cannot both be admitted.
func (r *AgentRegistry) ConsumeRecoveryToken(token, agent string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name, err := r.consumeRecoveryTokenLocked(token, agent)
	if err != nil {
		return "", err
	}
	if err := r.save(); err != nil {
		r.unspendRecoveryTokenLocked(name, token)
		return "", err
	}
	return name, nil
}

// consumeRecoveryTokenLocked validates and spends a token in memory, leaving
// persistence to the caller so an announce can record the rescue host's
// address in the same write. Callers must hold the write lock.
func (r *AgentRegistry) consumeRecoveryTokenLocked(token, agent string) (string, error) {
	secret, err := hex.DecodeString(token)
	if err != nil || len(secret) != recoveryTokenBytes {
		return "", fmt.Errorf("server: invalid recovery token")
	}
	hash := hashRecoveryToken(secret)

	now := time.Now()
	for i := range r.recoveryTokens {
		t := &r.recoveryTokens[i]
		if !hmac.Equal(t.Hash[:], hash[:]) {
			continue
		}
		if t.UsedAt != nil {
			return "", fmt.Errorf("server: recovery token has already been used")
		}
		if !now.Before(t.ExpiresAt) {
			return "", fmt.Errorf("server: recovery token expired at %s",
				t.ExpiresAt.Format(time.RFC3339))
		}
		if t.Agent != agent {
			// Deliberately not spent: the rightful host still needs it, and
			// burning it here would turn a wrong guess into a denial of
			// service against the recovery it was minted for.
			return "", fmt.Errorf(
				"server: recovery token was issued for %q, not %q", t.Agent, agent)
		}

		t.UsedAt = &now
		return t.Agent, nil
	}

	return "", fmt.Errorf("server: unknown recovery token")
}

// unspendRecoveryTokenLocked reverses an in-memory consumption when the write
// that would have made it durable failed. Without it, a transient disk error
// would silently destroy the one token an operator has mid-outage.
func (r *AgentRegistry) unspendRecoveryTokenLocked(agent, token string) {
	secret, err := hex.DecodeString(token)
	if err != nil {
		return
	}
	hash := hashRecoveryToken(secret)
	for i := range r.recoveryTokens {
		if hmac.Equal(r.recoveryTokens[i].Hash[:], hash[:]) {
			r.recoveryTokens[i].UsedAt = nil
			return
		}
	}
}

// ListRecoveryTokens returns the tokens that can still be used, so a UI never
// invites an operator to rely on one that will fail.
func (r *AgentRegistry) ListRecoveryTokens() []RecoveryToken {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	var out []RecoveryToken
	for _, t := range r.recoveryTokens {
		if t.Live(now) {
			out = append(out, t)
		}
	}
	return out
}

// RevokeRecoveryToken kills an unused token, for an operator who realises one
// has leaked before anyone uses it.
func (r *AgentRegistry) RevokeRecoveryToken(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.recoveryTokens {
		if r.recoveryTokens[i].ID != id {
			continue
		}
		r.recoveryTokens = append(r.recoveryTokens[:i], r.recoveryTokens[i+1:]...)
		return r.save()
	}
	return fmt.Errorf("server: recovery token %q not found", id)
}

// pruneRecoveryTokensLocked drops tokens that can no longer be used. The
// caller must hold the write lock.
func (r *AgentRegistry) pruneRecoveryTokensLocked(now time.Time) {
	kept := r.recoveryTokens[:0]
	for _, t := range r.recoveryTokens {
		// Spent tokens are held briefly so a replay reports "already used"
		// rather than "unknown", which is the more actionable message.
		if t.Live(now) || now.Sub(t.ExpiresAt) < 7*24*time.Hour {
			kept = append(kept, t)
		}
	}
	r.recoveryTokens = kept
}

// BeginRecovery admits a rescue host: it spends the token and records where
// the host can be reached, so the existing server→agent restore push can dial
// it back.
//
// The address comes from the announce rather than from anything on file. A
// machine that just booted off rescue media has whatever address DHCP handed
// it, which nobody told the server about.
// RecoveryAnnounce is what a rescue host reports when it calls home.
type RecoveryAnnounce struct {
	// Token is the single-use recovery token an operator minted.
	Token string
	// Agent is the registered agent this media recovers.
	Agent string
	// Addr is where the server can dial the rescue system back.
	Addr string
	// AgentToken is the token the rescue system's own gRPC listener will
	// accept, so the dial-back can authenticate without the server
	// disclosing, or the rescue host guessing, the original agent's token.
	AgentToken string
}

func (r *AgentRegistry) BeginRecovery(token, agent, addr string) (string, error) {
	return r.BeginRecoveryAnnounce(RecoveryAnnounce{Token: token, Agent: agent, Addr: addr})
}

// BeginRecoveryAnnounce is the full form of BeginRecovery, carrying the rescue
// system's own agent token.
func (r *AgentRegistry) BeginRecoveryAnnounce(a RecoveryAnnounce) (string, error) {
	token, agent, addr := a.Token, a.Agent, a.Addr
	r.mu.Lock()
	defer r.mu.Unlock()

	// Spending the token and recording the address are one write. Split
	// across two, a failure in between would burn the operator's only token
	// while admitting nobody.
	name, err := r.consumeRecoveryTokenLocked(token, agent)
	if err != nil {
		return "", err
	}

	rec := r.mutableAgentLocked(name)
	if rec == nil {
		r.unspendRecoveryTokenLocked(name, token)
		return "", fmt.Errorf("server: agent %q not found", name)
	}
	prevAddr, prevPin := rec.AgentAddr, rec.TLSFingerprint
	prevRecoveryToken := rec.RecoveryTokenEncrypted

	if a.AgentToken != "" {
		encrypted, err := config.EncryptCredential(r.encryptKey, a.AgentToken)
		if err != nil {
			r.unspendRecoveryTokenLocked(name, token)
			return "", fmt.Errorf("server: store recovery agent token: %w", err)
		}
		rec.RecoveryTokenEncrypted = encrypted
	}

	rec.AgentAddr = addr
	rec.Recovering = true
	// The rescue system generates its own self-signed certificate, so the pin
	// recorded from the original host can never match it. Leaving it in place
	// would fail the dial-back handshake on every bare-metal recovery. A new
	// pin is recorded on first connection, as it was originally.
	rec.TLSFingerprint = ""

	if err := r.save(); err != nil {
		rec.AgentAddr, rec.TLSFingerprint, rec.Recovering = prevAddr, prevPin, false
		rec.RecoveryTokenEncrypted = prevRecoveryToken
		r.unspendRecoveryTokenLocked(name, token)
		return "", err
	}
	return name, nil
}

// EndRecovery clears the recovering flag once the restore reaches a terminal
// state, or once the real post-reboot agent registers normally.
func (r *AgentRegistry) EndRecovery(agent string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec := r.mutableAgentLocked(agent)
	if rec == nil {
		return fmt.Errorf("server: agent %q not found", agent)
	}
	if !rec.Recovering {
		return nil
	}
	rec.Recovering = false
	// The rescue system's token dies with the rescue system. Keeping it would
	// leave a credential on file for a machine that no longer exists.
	rec.RecoveryTokenEncrypted = ""
	// The pin recorded from the rescue system does not describe the restored
	// host, which generates a fresh certificate when it boots.
	rec.TLSFingerprint = ""
	return r.save()
}
