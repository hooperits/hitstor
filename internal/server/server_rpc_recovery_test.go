package server

import (
	"context"
	"net"
	"strings"
	"testing"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// recoveryTestServer returns a server whose registry holds one agent.
func recoveryTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{registry: tokenTestRegistry(t)}
}

// peerCtx attaches a client address, as a real gRPC call would.
func peerCtx(addr string) context.Context {
	tcp, _ := net.ResolveTCPAddr("tcp", addr)
	return peer.NewContext(context.Background(), &peer.Peer{Addr: tcp})
}

func TestAnnounceForRecoveryAdmitsARescueHost(t *testing.T) {
	srv := recoveryTestServer(t)
	token, err := srv.registry.MintRecoveryToken("web01", 0)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := srv.AnnounceForRecovery(peerCtx("10.1.2.3:54321"), &pb.AnnounceForRecoveryRequest{
		RecoveryToken: token,
		AgentName:     "web01",
		AgentAddr:     "10.1.2.3:8422",
		Hostname:      "web01",
		TargetPath:    "/mnt/local",
	})
	if err != nil {
		t.Fatalf("AnnounceForRecovery: %v", err)
	}
	if resp.AgentName != "web01" {
		t.Errorf("AgentName = %q, want %q", resp.AgentName, "web01")
	}
	// An operator is standing at a console with no dashboard in front of
	// them; the response has to say what happens next.
	if resp.Message == "" {
		t.Error("no next step returned to the console")
	}

	rec := srv.registry.GetAgent("web01")
	if rec.AgentAddr != "10.1.2.3:8422" {
		t.Errorf("AgentAddr = %q, want the announced address", rec.AgentAddr)
	}
	if !rec.Recovering {
		t.Error("Recovering not set")
	}
}

func TestAnnounceForRecoveryFillsInTheHostFromTheConnection(t *testing.T) {
	srv := recoveryTestServer(t)
	token, _ := srv.registry.MintRecoveryToken("web01", 0)

	// A rescue system knows the port it is listening on but not which of its
	// addresses the server can actually reach. The connection it just opened
	// is proof of one that works.
	if _, err := srv.AnnounceForRecovery(peerCtx("192.168.7.44:51000"),
		&pb.AnnounceForRecoveryRequest{
			RecoveryToken: token,
			AgentName:     "web01",
			AgentAddr:     ":8422",
		}); err != nil {
		t.Fatalf("AnnounceForRecovery: %v", err)
	}

	if got := srv.registry.GetAgent("web01").AgentAddr; got != "192.168.7.44:8422" {
		t.Errorf("AgentAddr = %q, want the peer address with the announced port", got)
	}
}

func TestAnnounceForRecoveryRefusesABadToken(t *testing.T) {
	srv := recoveryTestServer(t)
	good, _ := srv.registry.MintRecoveryToken("web01", 0)

	tests := map[string]*pb.AnnounceForRecoveryRequest{
		"unknown token": {
			RecoveryToken: strings.Repeat("ab", 32), AgentName: "web01", AgentAddr: "10.0.0.1:8422",
		},
		"wrong host": {
			RecoveryToken: good, AgentName: "db02", AgentAddr: "10.0.0.1:8422",
		},
		"malformed token": {
			RecoveryToken: "nonsense", AgentName: "web01", AgentAddr: "10.0.0.1:8422",
		},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := srv.AnnounceForRecovery(peerCtx("10.0.0.1:5000"), req)
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("code = %v, want PermissionDenied (err: %v)", status.Code(err), err)
			}
		})
	}

	// None of those attempts may have burned the real token.
	if _, err := srv.AnnounceForRecovery(peerCtx("10.0.0.1:5000"),
		&pb.AnnounceForRecoveryRequest{
			RecoveryToken: good, AgentName: "web01", AgentAddr: "10.0.0.1:8422",
		}); err != nil {
		t.Fatalf("the legitimate announce was locked out by failed attempts: %v", err)
	}
}

func TestAnnounceForRecoveryIsSingleUse(t *testing.T) {
	srv := recoveryTestServer(t)
	token, _ := srv.registry.MintRecoveryToken("web01", 0)
	req := &pb.AnnounceForRecoveryRequest{
		RecoveryToken: token, AgentName: "web01", AgentAddr: "10.0.0.1:8422",
	}

	if _, err := srv.AnnounceForRecovery(peerCtx("10.0.0.1:5000"), req); err != nil {
		t.Fatal(err)
	}
	// Replaying a captured announce must not redirect the next restore to
	// whatever hardware presents it.
	if _, err := srv.AnnounceForRecovery(peerCtx("10.6.6.6:5000"), req); err == nil {
		t.Fatal("a replayed announce was accepted")
	}
	if got := srv.registry.GetAgent("web01").AgentAddr; got != "10.0.0.1:8422" {
		t.Errorf("AgentAddr = %q; a replay moved the restore target", got)
	}
}

func TestAnnounceForRecoveryRejectsUnusableInput(t *testing.T) {
	srv := recoveryTestServer(t)
	token, _ := srv.registry.MintRecoveryToken("web01", 0)

	tests := map[string]*pb.AnnounceForRecoveryRequest{
		"no agent name": {RecoveryToken: token, AgentAddr: "10.0.0.1:8422"},
		"no address":    {RecoveryToken: token, AgentName: "web01"},
		"no port":       {RecoveryToken: token, AgentName: "web01", AgentAddr: "10.0.0.1"},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := srv.AnnounceForRecovery(peerCtx("10.0.0.1:5000"), req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
			}
		})
	}
}
