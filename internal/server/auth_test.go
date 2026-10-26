package server

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testEncryptKey(t *testing.T) [32]byte {
	t.Helper()
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestAddAgent(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	apiKey, err := reg.AddAgent("web-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(apiKey) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("API key length = %d, want 64", len(apiKey))
	}

	agents := reg.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(agents))
	}
	if agents[0].Name != "web-01" {
		t.Errorf("agent name = %q, want %q", agents[0].Name, "web-01")
	}
	if agents[0].LastBackup != nil {
		t.Error("expected nil LastBackup for new agent")
	}

	// File exists on disk.
	if _, err := os.Stat(filepath.Join(dir, "agents.json")); err != nil {
		t.Errorf("agents.json not created: %v", err)
	}
}

func TestAddAgent_Duplicate(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := reg.AddAgent("web-01"); err != nil {
		t.Fatal(err)
	}

	_, err = reg.AddAgent("web-01")
	if err == nil {
		t.Fatal("expected error for duplicate agent")
	}
}

func TestRemoveAgent(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := reg.AddAgent("web-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddAgent("db-01"); err != nil {
		t.Fatal(err)
	}

	if err := reg.RemoveAgent("web-01", true); err != nil {
		t.Fatal(err)
	}

	agents := reg.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(agents))
	}
	if agents[0].Name != "db-01" {
		t.Errorf("remaining agent = %q, want %q", agents[0].Name, "db-01")
	}
}

func TestRemoveAgent_NotFound(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	err = reg.RemoveAgent("nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestVerifyKey_Valid(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	apiKey, err := reg.AddAgent("web-01")
	if err != nil {
		t.Fatal(err)
	}

	name, ok := reg.VerifyKey(apiKey)
	if !ok {
		t.Fatal("expected valid key")
	}
	if name != "web-01" {
		t.Errorf("agent name = %q, want %q", name, "web-01")
	}
}

func TestVerifyKey_Invalid(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := reg.AddAgent("web-01"); err != nil {
		t.Fatal(err)
	}

	_, ok := reg.VerifyKey("0000000000000000000000000000000000000000000000000000000000000000")
	if ok {
		t.Fatal("expected invalid key to be rejected")
	}

	_, ok = reg.VerifyKey("not-hex")
	if ok {
		t.Fatal("expected non-hex key to be rejected")
	}

	_, ok = reg.VerifyKey("abcd")
	if ok {
		t.Fatal("expected short key to be rejected")
	}
}

func TestVerifyKey_Revoked(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	apiKey, err := reg.AddAgent("web-01")
	if err != nil {
		t.Fatal(err)
	}

	// Key works before removal.
	if _, ok := reg.VerifyKey(apiKey); !ok {
		t.Fatal("expected key to work before removal")
	}

	if err := reg.RemoveAgent("web-01", true); err != nil {
		t.Fatal(err)
	}

	// Key fails after removal.
	_, ok := reg.VerifyKey(apiKey)
	if ok {
		t.Fatal("expected revoked key to be rejected")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	// Create registry with agents.
	reg1, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	apiKey1, err := reg1.AddAgent("web-01")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg1.AddAgent("db-01"); err != nil {
		t.Fatal(err)
	}

	// Load from disk with same key.
	reg2, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	agents := reg2.ListAgents()
	if len(agents) != 2 {
		t.Fatalf("agents count = %d, want 2", len(agents))
	}

	// Verify key works after reload.
	name, ok := reg2.VerifyKey(apiKey1)
	if !ok {
		t.Fatal("expected key to work after reload")
	}
	if name != "web-01" {
		t.Errorf("agent name = %q, want %q", name, "web-01")
	}
}

func TestLoadAgentRegistry_WrongKey(t *testing.T) {
	dir := t.TempDir()
	key1 := testEncryptKey(t)
	key2 := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key1, dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := reg.AddAgent("web-01"); err != nil {
		t.Fatal(err)
	}

	// Load with wrong key should fail.
	_, err = LoadAgentRegistry(key2, dir)
	if err == nil {
		t.Fatal("expected error with wrong key")
	}
}

func TestLoadAgentRegistry_Empty(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	agents := reg.ListAgents()
	if len(agents) != 0 {
		t.Errorf("agents count = %d, want 0", len(agents))
	}
}

func TestUpdateLastBackup(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := reg.AddAgent("web-01"); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second)
	if err := reg.UpdateLastBackup("web-01", now); err != nil {
		t.Fatal(err)
	}

	// Reload and verify.
	reg2, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	agents := reg2.ListAgents()
	if agents[0].LastBackup == nil {
		t.Fatal("expected non-nil LastBackup")
	}
	if !agents[0].LastBackup.Truncate(time.Second).Equal(now) {
		t.Errorf("LastBackup = %v, want %v", agents[0].LastBackup, now)
	}
}

func TestVerifyKey_Suspended(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	apiKey, err := reg.AddAgent("web-01")
	if err != nil {
		t.Fatal(err)
	}

	// Key works initially.
	if _, ok := reg.VerifyKey(apiKey); !ok {
		t.Fatal("expected key to work before suspension")
	}

	// Suspend the agent.
	if err := reg.SuspendAgent("web-01"); err != nil {
		t.Fatal(err)
	}

	// Key fails after suspension.
	if _, ok := reg.VerifyKey(apiKey); ok {
		t.Fatal("expected suspended agent to be rejected")
	}

	// Unsuspend the agent.
	if err := reg.UnsuspendAgent("web-01"); err != nil {
		t.Fatal(err)
	}

	// Key works again.
	if _, ok := reg.VerifyKey(apiKey); !ok {
		t.Fatal("expected key to work after unsuspension")
	}
}

func TestSentinelAlertsRegistry(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	alert := SentinelAlert{
		ID:             "alert-1",
		AgentName:      "web-01",
		Hostname:       "web-server-1",
		Time:           time.Now().Truncate(time.Second),
		SnapshotID:     "snap-1",
		ThreatDetected: true,
		Confidence:     0.9,
		Reason:         "Ransomware telemetry match",
		Status:         "active",
	}

	if err := reg.AddAlert(alert); err != nil {
		t.Fatal(err)
	}

	alerts := reg.ListAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Reason != "Ransomware telemetry match" {
		t.Errorf("got reason %q, want %q", alerts[0].Reason, "Ransomware telemetry match")
	}

	// Resolve the alert.
	if err := reg.ResolveAlert("alert-1"); err != nil {
		t.Fatal(err)
	}

	// Reload and verify status is resolved.
	reg2, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}

	alerts = reg2.ListAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Status != "resolved" {
		t.Errorf("got status %q, want %q", alerts[0].Status, "resolved")
	}
}
