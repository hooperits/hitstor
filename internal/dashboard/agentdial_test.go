package dashboard

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strings"
	"testing"
	"time"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/tlsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type stubAgentTLSServer struct {
	pb.UnimplementedAgentServiceServer
}

func (stubAgentTLSServer) AgentInfo(context.Context, *pb.AgentInfoRequest) (*pb.AgentInfoResponse, error) {
	return &pb.AgentInfoResponse{Hostname: "stub"}, nil
}

// startStubAgentTLS runs a TLS gRPC agent and returns its address and cert fingerprint.
func startStubAgentTLS(t *testing.T) (addr, fingerprint string) {
	t.Helper()
	tlsCfg, certPEM, _, err := tlsutil.GenerateSelfSigned("stub-agent")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsCfg)))
	pb.RegisterAgentServiceServer(srv, stubAgentTLSServer{})
	go srv.Serve(ln)
	t.Cleanup(srv.Stop)
	return ln.Addr().String(), tlsutil.Fingerprint(cert)
}

func TestDialManagedAgentTOFU(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	addr, wantFP := startStubAgentTLS(t)
	if _, err := dash.registry.AddManagedAgent("pin-agent", addr, "token123"); err != nil {
		t.Fatal(err)
	}

	// First connection: no pin yet, fingerprint gets recorded.
	client, err := dash.dialManagedAgent("pin-agent", addr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.AgentInfoNoAuth(ctx); err != nil {
		t.Fatalf("first connection error = %v, want success", err)
	}
	client.Close()

	got := dash.registry.GetAgent("pin-agent").TLSFingerprint
	if got != wantFP {
		t.Fatalf("recorded fingerprint = %q, want %q", got, wantFP)
	}

	// Second connection with the pin in place still works.
	client, err = dash.dialManagedAgent("pin-agent", addr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.AgentInfoNoAuth(ctx); err != nil {
		t.Fatalf("pinned reconnection error = %v, want success", err)
	}
	client.Close()

	// A different certificate at the same address must be rejected.
	addr2, _ := startStubAgentTLS(t)
	client, err = dash.dialManagedAgent("pin-agent", addr2)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.AgentInfoNoAuth(ctx); err == nil {
		t.Fatal("connection to imposter agent succeeded, want fingerprint mismatch")
	} else if !strings.Contains(err.Error(), "fingerprint") {
		t.Errorf("error = %v, want fingerprint mismatch", err)
	}
}
