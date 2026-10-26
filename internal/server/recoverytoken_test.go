package server

import (
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/repository"
)

// tokenTestRegistry returns a persisted registry holding one agent.
func tokenTestRegistry(t *testing.T) *AgentRegistry {
	t.Helper()

	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatalf("backend: %v", err)
	}
	t.Cleanup(func() { be.Close() })

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	reg, err := LoadAgentRegistry(repo.Key().EncryptKey, dir)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if _, err := reg.AddAgent("web01"); err != nil {
		t.Fatalf("add agent: %v", err)
	}
	return reg
}

func TestMintRecoveryTokenReturnsASecretStoredOnlyAsAHash(t *testing.T) {
	reg := tokenTestRegistry(t)

	token, err := reg.MintRecoveryToken("web01", 0)
	if err != nil {
		t.Fatalf("MintRecoveryToken: %v", err)
	}
	if len(token) < 32 {
		t.Errorf("token is only %d characters; too little entropy for a network credential", len(token))
	}

	// The plaintext is shown once and never persisted, exactly as agent API
	// keys are handled: a registry file that leaks must not hand over a way
	// into a recovery.
	for _, rec := range reg.ListRecoveryTokens() {
		if strings.Contains(rec.Agent+rec.ID, token) {
			t.Error("the token's plaintext appears in the stored record")
		}
	}
}

func TestMintRecoveryTokenRejectsAnUnknownAgent(t *testing.T) {
	reg := tokenTestRegistry(t)

	if _, err := reg.MintRecoveryToken("nope", 0); err == nil {
		t.Fatal("minted a recovery token for an agent that does not exist")
	}
}

func TestConsumeRecoveryTokenAcceptsItExactlyOnce(t *testing.T) {
	reg := tokenTestRegistry(t)

	token, err := reg.MintRecoveryToken("web01", 0)
	if err != nil {
		t.Fatal(err)
	}

	agent, err := reg.ConsumeRecoveryToken(token, "web01")
	if err != nil {
		t.Fatalf("first use: %v", err)
	}
	if agent != "web01" {
		t.Errorf("agent = %q, want %q", agent, "web01")
	}

	// Single use: a token replayed from a captured recovery must not start a
	// second one.
	if _, err := reg.ConsumeRecoveryToken(token, "web01"); err == nil {
		t.Fatal("a consumed token was accepted a second time")
	}
}

func TestConsumeRecoveryTokenRejectsAMismatchedHost(t *testing.T) {
	reg := tokenTestRegistry(t)

	token, err := reg.MintRecoveryToken("web01", 0)
	if err != nil {
		t.Fatal(err)
	}

	// Host-bound: a token minted for one machine must not recover another
	// machine's data onto whatever hardware presents it.
	if _, err := reg.ConsumeRecoveryToken(token, "db02"); err == nil {
		t.Fatal("a token minted for web01 recovered db02")
	}
	// The rejection must not burn the token: the legitimate host still needs it.
	if _, err := reg.ConsumeRecoveryToken(token, "web01"); err != nil {
		t.Fatalf("the rightful host was locked out by a mismatched attempt: %v", err)
	}
}

func TestConsumeRecoveryTokenRejectsAnExpiredToken(t *testing.T) {
	reg := tokenTestRegistry(t)

	token, err := reg.MintRecoveryToken("web01", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)

	// A token that still works a year after its ISO was built, sitting on a
	// shelf, is the risk a long-lived key would create.
	if _, err := reg.ConsumeRecoveryToken(token, "web01"); err == nil {
		t.Fatal("an expired token was accepted")
	}
}

func TestMintRecoveryTokenDefaultTTLIsADay(t *testing.T) {
	reg := tokenTestRegistry(t)

	if _, err := reg.MintRecoveryToken("web01", 0); err != nil {
		t.Fatal(err)
	}

	tokens := reg.ListRecoveryTokens()
	if len(tokens) != 1 {
		t.Fatalf("got %d tokens, want 1", len(tokens))
	}
	ttl := time.Until(tokens[0].ExpiresAt)
	if ttl < 23*time.Hour || ttl > 25*time.Hour {
		t.Errorf("default TTL is %v, want about 24h", ttl)
	}
}

func TestConsumeRecoveryTokenRejectsGarbage(t *testing.T) {
	reg := tokenTestRegistry(t)
	if _, err := reg.MintRecoveryToken("web01", 0); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"", "not-hex", strings.Repeat("a", 64)} {
		if _, err := reg.ConsumeRecoveryToken(bad, "web01"); err == nil {
			t.Errorf("accepted %q as a recovery token", bad)
		}
	}
}

func TestRevokeRecoveryTokenStopsAnUnusedToken(t *testing.T) {
	reg := tokenTestRegistry(t)

	token, err := reg.MintRecoveryToken("web01", 0)
	if err != nil {
		t.Fatal(err)
	}
	list := reg.ListRecoveryTokens()
	if len(list) != 1 {
		t.Fatalf("got %d tokens, want 1", len(list))
	}

	// An operator who realises a token leaked must be able to kill it before
	// anyone uses it.
	if err := reg.RevokeRecoveryToken(list[0].ID); err != nil {
		t.Fatalf("RevokeRecoveryToken: %v", err)
	}
	if _, err := reg.ConsumeRecoveryToken(token, "web01"); err == nil {
		t.Fatal("a revoked token was accepted")
	}
	if got := len(reg.ListRecoveryTokens()); got != 0 {
		t.Errorf("%d tokens listed after revoke, want 0", got)
	}
}

func TestListRecoveryTokensHidesSpentAndExpiredOnes(t *testing.T) {
	reg := tokenTestRegistry(t)

	spent, _ := reg.MintRecoveryToken("web01", 0)
	if _, err := reg.ConsumeRecoveryToken(spent, "web01"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.MintRecoveryToken("web01", time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := reg.MintRecoveryToken("web01", 0); err != nil {
		t.Fatal(err)
	}

	// The list drives a UI showing what is currently usable; a spent or
	// expired token in it invites an operator to rely on one that will fail.
	live := reg.ListRecoveryTokens()
	if len(live) != 1 {
		t.Fatalf("got %d live tokens, want 1: %+v", len(live), live)
	}
	if live[0].Agent != "web01" {
		t.Errorf("Agent = %q, want %q", live[0].Agent, "web01")
	}
}

func TestRecoveryTokensSurviveAReload(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()
	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	key := repo.Key().EncryptKey

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddAgent("web01"); err != nil {
		t.Fatal(err)
	}
	token, err := reg.MintRecoveryToken("web01", 0)
	if err != nil {
		t.Fatal(err)
	}

	// A token is minted before an outage and used during one, quite possibly
	// after the server has been restarted too.
	reloaded, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, err := reloaded.ConsumeRecoveryToken(token, "web01"); err != nil {
		t.Fatalf("token did not survive a restart: %v", err)
	}
	// And the consumption must survive too, or a restart resets single-use.
	again, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := again.ConsumeRecoveryToken(token, "web01"); err == nil {
		t.Fatal("a restart resurrected a spent token")
	}
}

func TestBeginRecoveryRecordsTheRescueHostAddress(t *testing.T) {
	reg := tokenTestRegistry(t)

	token, err := reg.MintRecoveryToken("web01", 0)
	if err != nil {
		t.Fatal(err)
	}

	// A rescue host's address is whatever DHCP handed it, so the server
	// learns it from the announce rather than from anything on file.
	agent, err := reg.BeginRecovery(token, "web01", "10.1.2.3:8422")
	if err != nil {
		t.Fatalf("BeginRecovery: %v", err)
	}
	if agent != "web01" {
		t.Errorf("agent = %q, want %q", agent, "web01")
	}

	rec := reg.GetAgent("web01")
	if rec.AgentAddr != "10.1.2.3:8422" {
		t.Errorf("AgentAddr = %q, want the announced address", rec.AgentAddr)
	}
	if !rec.Recovering {
		t.Error("Recovering flag not set; the dashboard cannot tell a rescue host from a live one")
	}
}

func TestBeginRecoveryClearsAStalePinSoTheRescueAgentCanBeDialed(t *testing.T) {
	reg := tokenTestRegistry(t)
	if err := reg.SetAgentFingerprint("web01", "old-host-fingerprint"); err != nil {
		t.Fatal(err)
	}

	token, _ := reg.MintRecoveryToken("web01", 0)
	if _, err := reg.BeginRecovery(token, "web01", "10.1.2.3:8422"); err != nil {
		t.Fatal(err)
	}

	// The rescue system generates its own self-signed certificate, so the pin
	// recorded from the original host can never match. Keeping it would make
	// every bare-metal recovery fail its TLS handshake.
	if got := reg.GetAgent("web01").TLSFingerprint; got != "" {
		t.Errorf("TLSFingerprint = %q, want it cleared for the rescue host", got)
	}
}

func TestEndRecoveryClearsTheFlag(t *testing.T) {
	reg := tokenTestRegistry(t)
	token, _ := reg.MintRecoveryToken("web01", 0)
	if _, err := reg.BeginRecovery(token, "web01", "10.1.2.3:8422"); err != nil {
		t.Fatal(err)
	}

	if err := reg.EndRecovery("web01"); err != nil {
		t.Fatalf("EndRecovery: %v", err)
	}
	if reg.GetAgent("web01").Recovering {
		t.Error("Recovering still set after the restore reached a terminal state")
	}
}

func TestRecoveryUsesTheRescueSystemsOwnAgentToken(t *testing.T) {
	reg := tokenTestRegistry(t)
	// Give the agent a normal managed token, as dashboard registration does.
	if _, err := reg.AddManagedAgent("db02", "10.0.0.9:8422", "original-agent-token"); err != nil {
		t.Fatalf("add managed agent: %v", err)
	}

	token, err := reg.MintRecoveryToken("db02", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.BeginRecoveryAnnounce(RecoveryAnnounce{
		Token:      token,
		Agent:      "db02",
		Addr:       "10.1.2.3:8422",
		AgentToken: "rescue-system-token",
	}); err != nil {
		t.Fatalf("BeginRecoveryAnnounce: %v", err)
	}

	// The dial-back must authenticate against the rescue system, which knows
	// only the token it generated itself.
	got, err := reg.DecryptToken("db02")
	if err != nil {
		t.Fatalf("DecryptToken: %v", err)
	}
	if got != "rescue-system-token" {
		t.Errorf("DecryptToken = %q, want the rescue system's token", got)
	}

	// And the real agent's token must survive untouched, for when the
	// restored OS boots and takes over again.
	if err := reg.EndRecovery("db02"); err != nil {
		t.Fatal(err)
	}
	got, err = reg.DecryptToken("db02")
	if err != nil {
		t.Fatalf("DecryptToken after recovery: %v", err)
	}
	if got != "original-agent-token" {
		t.Errorf("DecryptToken = %q, want the original agent token back", got)
	}
}

func TestEndRecoveryDropsTheRescueSystemsCredentials(t *testing.T) {
	reg := tokenTestRegistry(t)
	if _, err := reg.AddManagedAgent("db02", "10.0.0.9:8422", "original-agent-token"); err != nil {
		t.Fatal(err)
	}
	token, _ := reg.MintRecoveryToken("db02", 0)
	if _, err := reg.BeginRecoveryAnnounce(RecoveryAnnounce{
		Token: token, Agent: "db02", Addr: "10.1.2.3:8422", AgentToken: "rescue-system-token",
	}); err != nil {
		t.Fatal(err)
	}

	if err := reg.EndRecovery("db02"); err != nil {
		t.Fatal(err)
	}

	rec := reg.GetAgent("db02")
	// A credential on file for a machine that no longer exists is a liability
	// with no use.
	if rec.RecoveryTokenEncrypted != "" {
		t.Error("the rescue system's token outlived the rescue system")
	}
	// The restored host generates a fresh certificate when it boots, so the
	// pin taken from the rescue system would lock it out.
	if rec.TLSFingerprint != "" {
		t.Error("the rescue system's certificate pin was kept")
	}
}
