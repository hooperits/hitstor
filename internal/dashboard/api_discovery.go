package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"log/slog"
	"strings"

	"github.com/hooperits/hitstor/internal/discovery"
)

// handleAPIDiscoverSubnets returns the server's network interfaces and subnets.
func (d *Dashboard) handleAPIDiscoverSubnets(w http.ResponseWriter, r *http.Request) {
	subnets := discovery.ListSubnets()
	if subnets == nil {
		subnets = []discovery.SubnetInfo{}
	}
	writeJSON(w, subnets)
}

// handleAPIDiscoverScan starts an asynchronous network scan for agents.
func (d *Dashboard) handleAPIDiscoverScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CIDR string `json:"cidr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.CIDR == "" {
		http.Error(w, "cidr is required", http.StatusBadRequest)
		return
	}

	// Validate CIDR and size.
	if _, err := discovery.EnumerateIPs(req.CIDR); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build registered agents map keyed by IP.
	registered := make(map[string]bool)
	registeredNames := make(map[string]string)
	if d.registry != nil {
		for _, a := range d.registry.ListAgents() {
			if a.AgentAddr != "" {
				ip := a.AgentAddr
				if idx := strings.Index(ip, ":"); idx > 0 {
					ip = ip[:idx]
				}
				registered[ip] = true
				registeredNames[ip] = a.Name
			}
		}
	}

	// Create operation for SSE streaming.
	op := &operation{
		ID:     newOperationID(),
		Type:   "discovery",
		Status: "running",
		done:   make(chan struct{}),
	}
	d.opMu.Lock()
	d.operations[op.ID] = op
	d.opMu.Unlock()

	go func() {
		defer close(op.done)

		var found int
		startTime := time.Now()

		onFound := func(agent *discovery.DiscoveredAgent) {
			found++
			data, _ := json.Marshal(map[string]any{
				"type":       "agent",
				"ip":         agent.IP,
				"hostname":   agent.Hostname,
				"os":         agent.OS,
				"arch":       agent.Arch,
				"version":    agent.Version,
				"registered": agent.Registered,
				"agent_name": agent.AgentName,
			})
			op.addMessage(string(data))
		}

		onProgress := func(probed, total int) {
			data, _ := json.Marshal(map[string]any{
				"type":   "progress",
				"probed": probed,
				"total":  total,
				"found":  found,
			})
			op.addMessage(string(data))
		}

		err := discovery.ScanSubnet(context.Background(), req.CIDR, registered, registeredNames, onFound, onProgress)

		op.mu.Lock()
		if err != nil {
			op.Status = "failed"
			op.Result = map[string]any{
				"error":        err.Error(),
				"agents_found": found,
			}
		} else {
			op.Status = "completed"
			op.Result = map[string]any{
				"total_probed": len(registered), // approximate
				"agents_found": found,
				"duration_ms":  time.Since(startTime).Milliseconds(),
			}
		}
		op.mu.Unlock()

		slog.Info("discovery: scan operation finished", "id", op.ID, "found", found)
	}()

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{"operation_id": op.ID})
}

// handleAPIDiscoverProbe probes a single agent by IP address.
func (d *Dashboard) handleAPIDiscoverProbe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.IP == "" {
		http.Error(w, "ip is required", http.StatusBadRequest)
		return
	}

	// Build registered map.
	registered := make(map[string]bool)
	registeredNames := make(map[string]string)
	if d.registry != nil {
		for _, a := range d.registry.ListAgents() {
			if a.AgentAddr != "" {
				ip := a.AgentAddr
				if idx := strings.Index(ip, ":"); idx > 0 {
					ip = ip[:idx]
				}
				registered[ip] = true
				registeredNames[ip] = a.Name
			}
		}
	}

	agent := discovery.ProbeAgent(req.IP, registered, registeredNames)
	if agent == nil {
		http.Error(w, fmt.Sprintf("Agent not reachable at %s:8422 — verify the agent service is running and the address is correct", req.IP), http.StatusUnprocessableEntity)
		return
	}

	writeJSON(w, agent)
}
