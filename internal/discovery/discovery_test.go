package discovery

import (
	"testing"
)

func TestListSubnets(t *testing.T) {
	subnets := ListSubnets()
	for _, s := range subnets {
		if s.Interface == "lo" || s.IP == "127.0.0.1" {
			t.Errorf("ListSubnets() returned loopback: %+v", s)
		}
		if s.CIDR == "" {
			t.Errorf("ListSubnets() returned empty CIDR for %s", s.Interface)
		}
		if s.Label == "" {
			t.Errorf("ListSubnets() returned empty Label for %s", s.Interface)
		}
	}
}

func TestEnumerateIPs(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		want    int
		wantErr bool
	}{
		{"slash 24", "192.168.1.0/24", 254, false},
		{"slash 30", "10.0.0.0/30", 2, false},
		{"slash 31", "10.0.0.0/31", 0, false},
		{"slash 32", "10.0.0.1/32", 0, false},
		{"slash 16", "172.16.0.0/16", 65534, false},
		{"too large slash 15", "172.16.0.0/15", 0, true},
		{"invalid cidr", "not-a-cidr", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, err := EnumerateIPs(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnumerateIPs(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(ips) != tt.want {
				t.Errorf("EnumerateIPs(%q) = %d IPs, want %d", tt.cidr, len(ips), tt.want)
			}
		})
	}
}

func TestEnumerateIPs_ExcludesNetworkAndBroadcast(t *testing.T) {
	ips, err := EnumerateIPs("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}

	for _, ip := range ips {
		if ip == "192.168.1.0" {
			t.Error("EnumerateIPs included network address 192.168.1.0")
		}
		if ip == "192.168.1.255" {
			t.Error("EnumerateIPs included broadcast address 192.168.1.255")
		}
	}

	if ips[0] != "192.168.1.1" {
		t.Errorf("first IP = %s, want 192.168.1.1", ips[0])
	}
	if ips[len(ips)-1] != "192.168.1.254" {
		t.Errorf("last IP = %s, want 192.168.1.254", ips[len(ips)-1])
	}
}

func TestProbeAgent_Unreachable(t *testing.T) {
	// RFC 5737 TEST-NET-1: guaranteed unreachable.
	agent := ProbeAgent("192.0.2.1", nil, nil)
	if agent != nil {
		t.Errorf("ProbeAgent(unreachable) = %+v, want nil", agent)
	}
}
