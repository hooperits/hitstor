package server

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/tlsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type stubAgentServer struct {
	pb.UnimplementedAgentServiceServer
}

func (stubAgentServer) AgentInfo(context.Context, *pb.AgentInfoRequest) (*pb.AgentInfoResponse, error) {
	return &pb.AgentInfoResponse{Hostname: "stub"}, nil
}

// startStubAgent runs a TLS gRPC agent and returns its address and cert fingerprint.
func startStubAgent(t *testing.T) (addr, fingerprint string) {
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
	pb.RegisterAgentServiceServer(srv, stubAgentServer{})
	go srv.Serve(ln)
	t.Cleanup(srv.Stop)
	return ln.Addr().String(), tlsutil.Fingerprint(cert)
}

func TestDialAgentPinnedFirstUseRecords(t *testing.T) {
	addr, wantFP := startStubAgent(t)

	var mu sync.Mutex
	var recorded string
	client, err := DialAgentPinned(addr, "", func(fp string) {
		mu.Lock()
		recorded = fp
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.AgentInfoNoAuth(ctx); err != nil {
		t.Fatalf("AgentInfoNoAuth() error = %v, want success on first use", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if recorded != wantFP {
		t.Errorf("recorded fingerprint = %q, want %q", recorded, wantFP)
	}
}

func TestDialAgentPinnedMatchingPin(t *testing.T) {
	addr, fp := startStubAgent(t)
	client, err := DialAgentPinned(addr, fp, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.AgentInfoNoAuth(ctx); err != nil {
		t.Fatalf("AgentInfoNoAuth() with matching pin error = %v", err)
	}
}

func TestDialAgentPinnedWrongPinFails(t *testing.T) {
	addr, _ := startStubAgent(t)
	client, err := DialAgentPinned(addr, strings.Repeat("ab", 32), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = client.AgentInfoNoAuth(ctx)
	if err == nil {
		t.Fatal("AgentInfoNoAuth() with wrong pin succeeded, want TLS failure")
	}
	if s, ok := status.FromError(err); ok && !strings.Contains(s.Message(), "fingerprint") {
		t.Errorf("error = %v, want fingerprint mismatch", err)
	}
}
