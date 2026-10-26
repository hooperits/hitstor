// Package discovery provides network scanning for HitStor agents.
package discovery

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hooperits/hitstor/internal/server"
)

// SubnetInfo describes a network interface and its IPv4 subnet.
type SubnetInfo struct {
	Interface string `json:"interface"`
	IP        string `json:"ip"`
	CIDR      string `json:"cidr"`
	Label     string `json:"label"`
}

// DiscoveredAgent represents an agent found during a network scan.
type DiscoveredAgent struct {
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	Hostname   string `json:"hostname"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Version    string `json:"version"`
	Registered bool   `json:"registered"`
	AgentName  string `json:"agent_name,omitempty"`
}

// ListSubnets enumerates the server's network interfaces and returns
// their IPv4 subnets, excluding loopback and link-local addresses.
func ListSubnets() []SubnetInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		slog.Warn("discovery: list interfaces", "error", err)
		return nil
	}

	var subnets []SubnetInfo
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			// Skip link-local (169.254.0.0/16).
			if ip4[0] == 169 && ip4[1] == 254 {
				continue
			}

			// Compute network address for CIDR display.
			network := ip4.Mask(ipNet.Mask)
			ones, _ := ipNet.Mask.Size()
			cidr := fmt.Sprintf("%s/%d", network, ones)

			subnets = append(subnets, SubnetInfo{
				Interface: iface.Name,
				IP:        ip4.String(),
				CIDR:      cidr,
				Label:     fmt.Sprintf("%s — %s", iface.Name, cidr),
			})
		}
	}
	return subnets
}

// EnumerateIPs returns all host IPs in a CIDR range, excluding the
// network and broadcast addresses. Rejects subnets larger than /16.
func EnumerateIPs(cidr string) ([]string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("discovery: parse CIDR: %w", err)
	}

	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("discovery: IPv6 not supported")
	}
	if ones < 16 {
		return nil, fmt.Errorf("discovery: subnet too large (max /16, got /%d)", ones)
	}

	// Total host count (excluding network + broadcast).
	hostBits := uint(bits - ones)
	totalHosts := (1 << hostBits) - 2
	if totalHosts <= 0 {
		return nil, nil
	}

	networkIP := binary.BigEndian.Uint32(ipNet.IP.To4())
	ips := make([]string, 0, totalHosts)
	for i := 1; i <= totalHosts; i++ {
		hostIP := networkIP + uint32(i)
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, hostIP)
		ips = append(ips, ip.String())
	}
	return ips, nil
}

// ProbeAgent attempts to connect to an agent at the given IP address
// on port 8422 and retrieve its metadata via AgentInfo (unauthenticated).
// Returns nil if the host is unreachable or not a HitStor agent.
func ProbeAgent(ip string, registered map[string]bool, registeredNames map[string]string) *DiscoveredAgent {
	addr := fmt.Sprintf("%s:8422", ip)

	// Fast TCP check before full gRPC dial.
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil
	}
	conn.Close()

	// gRPC probe for agent metadata.
	client, err := server.DialAgent(addr)
	if err != nil {
		return nil
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.AgentInfoNoAuth(ctx)
	if err != nil {
		return nil
	}

	agent := &DiscoveredAgent{
		IP:       ip,
		Port:     8422,
		Hostname: resp.Hostname,
		OS:       resp.Os,
		Arch:     resp.Arch,
		Version:  resp.Version,
	}

	if registered[ip] {
		agent.Registered = true
		agent.AgentName = registeredNames[ip]
	}

	return agent
}

// ScanSubnet scans all hosts in a CIDR range for HitStor agents.
// Discovered agents are reported incrementally via the onFound callback.
// Progress is reported via onProgress. The scan respects context cancellation
// and enforces a 30-second overall timeout.
func ScanSubnet(ctx context.Context, cidr string, registered map[string]bool, registeredNames map[string]string, onFound func(*DiscoveredAgent), onProgress func(probed, total int)) error {
	ips, err := EnumerateIPs(cidr)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	total := len(ips)
	var probed atomic.Int32
	sem := make(chan struct{}, 32)
	var wg sync.WaitGroup

	slog.Info("discovery: scan started", "cidr", cidr, "hosts", total)

	for _, ip := range ips {
		select {
		case <-ctx.Done():
			slog.Info("discovery: scan cancelled", "probed", probed.Load(), "total", total)
			wg.Wait()
			return nil
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()

			agent := ProbeAgent(ip, registered, registeredNames)
			p := int(probed.Add(1))

			if agent != nil {
				slog.Debug("discovery: agent found", "ip", ip, "hostname", agent.Hostname)
				onFound(agent)
			}

			if p%32 == 0 || p == total {
				onProgress(p, total)
			}
		}(ip)
	}

	wg.Wait()
	finalProbed := int(probed.Load())
	onProgress(finalProbed, total)
	slog.Info("discovery: scan complete", "probed", finalProbed, "total", total)
	return nil
}
