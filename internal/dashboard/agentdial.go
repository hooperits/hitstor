package dashboard

import (
	"log/slog"

	"github.com/hooperits/hitstor/internal/server"
)

// dialManagedAgent dials a registered agent with trust-on-first-use TLS
// certificate pinning: the agent's certificate fingerprint is recorded in the
// registry on first connection and enforced on every later one, so a swapped
// certificate (possible MITM) fails the handshake. Unregistered probes
// (discovery, initial registration) use server.DialAgent directly.
func (d *Dashboard) dialManagedAgent(name, addr string) (*server.AgentClient, error) {
	return dialPinnedAgent(d.registry, name, addr)
}

// dialPinnedAgent is the registry-parameterized form of dialManagedAgent for
// callers that hold a registry reference directly (e.g. the restore manager).
func dialPinnedAgent(reg *server.AgentRegistry, name, addr string) (*server.AgentClient, error) {
	pin := ""
	if a := reg.GetAgent(name); a != nil {
		pin = a.TLSFingerprint
	}
	return server.DialAgentPinned(addr, pin, func(fp string) {
		if err := reg.SetAgentFingerprint(name, fp); err != nil {
			slog.Warn("record agent tls fingerprint", "agent", name, "error", err)
		}
	})
}
