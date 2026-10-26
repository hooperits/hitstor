package agent

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const testAgentToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// startBMRAgent runs an AgentServer over bufconn and returns a client for it.
func startBMRAgent(t *testing.T, srv *AgentServer) pb.AgentServiceClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := grpc.NewServer()
	pb.RegisterAgentServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return pb.NewAgentServiceClient(conn)
}

func newBMRAgentServer(t *testing.T) *AgentServer {
	t.Helper()
	return NewAgentServer(AgentServerOptions{Token: testAgentToken, Version: "test"})
}

func TestCheckRecoveryMediaRejectsABadToken(t *testing.T) {
	client := startBMRAgent(t, newBMRAgentServer(t))

	_, err := client.CheckRecoveryMedia(context.Background(),
		&pb.CheckRecoveryMediaRequest{Token: "wrong"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated (err: %v)", status.Code(err), err)
	}
}

func TestCheckRecoveryMediaReportsReadiness(t *testing.T) {
	client := startBMRAgent(t, newBMRAgentServer(t))

	resp, err := client.CheckRecoveryMedia(context.Background(),
		&pb.CheckRecoveryMediaRequest{Token: testAgentToken})
	if err != nil {
		t.Fatalf("CheckRecoveryMedia: %v", err)
	}

	if resp.Platform != runtime.GOOS {
		t.Errorf("Platform = %q, want %q", resp.Platform, runtime.GOOS)
	}
	if resp.CheckedAtUnix == 0 {
		t.Error("CheckedAtUnix is zero; the dashboard shows readiness age from it")
	}
	if runtime.GOOS == "linux" && len(resp.Tools) == 0 {
		t.Error("no tools reported on a Linux host")
	}
	// This test host has no ReaR, so the report must say so rather than
	// claiming readiness.
	if resp.Ready && !resp.RearInstalled {
		t.Error("reported ready without ReaR installed")
	}
}

func TestBuildRecoveryMediaRejectsABadToken(t *testing.T) {
	client := startBMRAgent(t, newBMRAgentServer(t))

	stream, err := client.BuildRecoveryMedia(context.Background(),
		&pb.BuildRecoveryMediaRequest{Token: "wrong"})
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated (err: %v)", status.Code(err), err)
	}
}

func TestBuildRecoveryMediaStreamsProgressThenTheImage(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}

	root := t.TempDir()
	outDir := filepath.Join(root, "media")
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		t.Fatal(err)
	}
	// A payload larger than one chunk, to prove the stream reassembles.
	isoBody := []byte(strings.Repeat("HITSTOR-ISO", 200_000))
	seed := filepath.Join(root, "seed.bin")
	if err := os.WriteFile(seed, isoBody, 0o600); err != nil {
		t.Fatal(err)
	}
	fakeRear := filepath.Join(root, "rear")
	script := "#!/bin/sh\necho 'Creating ISO image'\ncp " + seed + " " +
		filepath.Join(outDir, "rear-host.iso") + "\n"
	if err := os.WriteFile(fakeRear, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	srv := newBMRAgentServer(t)
	srv.Media = MediaOptions{
		BuildDir:   outDir,
		ConfigPath: filepath.Join(root, "local.conf"),
		HookPath:   filepath.Join(root, "recover.sh"),
		ReaRPath:   fakeRear,
		SkipCheck:  true,
	}

	client := startBMRAgent(t, srv)
	stream, err := client.BuildRecoveryMedia(context.Background(),
		&pb.BuildRecoveryMediaRequest{Token: testAgentToken, ServerAddr: "srv:8420"})
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	var (
		progress []string
		body     []byte
		complete *pb.MediaComplete
	)
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "EOF") {
				break
			}
			t.Fatalf("recv: %v", err)
		}
		switch p := msg.Payload.(type) {
		case *pb.MediaChunk_Progress:
			progress = append(progress, p.Progress)
		case *pb.MediaChunk_Data:
			body = append(body, p.Data...)
		case *pb.MediaChunk_Complete:
			complete = p.Complete
		}
	}

	if complete == nil {
		t.Fatal("stream ended without a completion message")
	}
	if !bytes.Equal(body, isoBody) {
		t.Errorf("received %d ISO bytes, want %d identical bytes", len(body), len(isoBody))
	}
	if complete.SizeBytes != uint64(len(isoBody)) {
		t.Errorf("SizeBytes = %d, want %d", complete.SizeBytes, len(isoBody))
	}
	if complete.Filename != "rear-host.iso" {
		t.Errorf("Filename = %q, want %q", complete.Filename, "rear-host.iso")
	}
	if complete.BuiltAtUnix == 0 {
		t.Error("BuiltAtUnix is zero; media age is shown from it")
	}
	if !strings.Contains(strings.Join(progress, "\n"), "Creating ISO image") {
		t.Errorf("ReaR output not forwarded as progress: %v", progress)
	}
}

func TestBuildRecoveryMediaReportsAFailedBuildAsAnError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}

	root := t.TempDir()
	fakeRear := filepath.Join(root, "rear")
	if err := os.WriteFile(fakeRear,
		[]byte("#!/bin/sh\necho 'ERROR: no bootloader found' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	srv := newBMRAgentServer(t)
	srv.Media = MediaOptions{
		BuildDir:   filepath.Join(root, "media"),
		ConfigPath: filepath.Join(root, "local.conf"),
		HookPath:   filepath.Join(root, "recover.sh"),
		ReaRPath:   fakeRear,
		SkipCheck:  true,
	}

	client := startBMRAgent(t, srv)
	stream, err := client.BuildRecoveryMedia(context.Background(),
		&pb.BuildRecoveryMediaRequest{Token: testAgentToken})
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	var recvErr error
	var sawComplete bool
	for {
		msg, err := stream.Recv()
		if err != nil {
			recvErr = err
			break
		}
		if msg.GetComplete() != nil {
			sawComplete = true
		}
	}
	if sawComplete {
		t.Error("a failed build reported completion")
	}
	if recvErr == nil || status.Code(recvErr) == codes.OK {
		t.Fatalf("failed build did not surface as a stream error: %v", recvErr)
	}
	// The server writes this straight into the dashboard; "exit status 1"
	// alone sends an operator digging through logs on a host they may lose.
	if !strings.Contains(recvErr.Error(), "no bootloader found") {
		t.Errorf("error does not carry ReaR's own diagnosis: %v", recvErr)
	}
}
