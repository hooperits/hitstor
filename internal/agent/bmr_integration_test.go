package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// dialAgentServer runs srv over bufconn and returns a server-side AgentClient
// pointed at it, exercising the same code path the dashboard uses.
func dialAgentServer(t *testing.T, srv *AgentServer) *server.AgentClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := grpc.NewServer()
	pb.RegisterAgentServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.Stop)

	client, err := server.DialAgentWith("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial agent: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// fakeReaRAgent returns an AgentServer whose builds are served by a stub ReaR
// that writes body as the ISO.
func fakeReaRAgent(t *testing.T, body []byte) *AgentServer {
	t.Helper()

	root := t.TempDir()
	outDir := filepath.Join(root, "media")
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(root, "seed.bin")
	if err := os.WriteFile(seed, body, 0o600); err != nil {
		t.Fatal(err)
	}
	fakeRear := filepath.Join(root, "rear")
	script := "#!/bin/sh\necho 'Creating ISO image'\ncp " + seed + " " +
		filepath.Join(outDir, "rear-host.iso") + "\n"
	if err := os.WriteFile(fakeRear, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	srv := NewAgentServer(AgentServerOptions{Token: testAgentToken, Version: "test"})
	srv.Media = MediaOptions{
		BuildDir:   outDir,
		ConfigPath: filepath.Join(root, "local.conf"),
		HookPath:   filepath.Join(root, "recover.sh"),
		ReaRPath:   fakeRear,
		SkipCheck:  true,
	}
	return srv
}

func TestServerCachesMediaBuiltOnAnAgent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}

	body := []byte(strings.Repeat("HITSTOR", 500_000))
	client := dialAgentServer(t, fakeReaRAgent(t, body))
	cache := server.NewMediaCache(t.TempDir())

	var progress []string
	var lastBytes int64
	info, err := client.BuildRecoveryMedia(context.Background(), cache, "web01",
		server.BuildRecoveryMediaRequest{
			Token:      testAgentToken,
			ServerAddr: "srv:8420",
			OnProgress: func(line string) { progress = append(progress, line) },
			OnBytes:    func(n int64) { lastBytes = n },
		})
	if err != nil {
		t.Fatalf("BuildRecoveryMedia: %v", err)
	}

	if info.SizeBytes != int64(len(body)) {
		t.Errorf("SizeBytes = %d, want %d", info.SizeBytes, len(body))
	}
	if lastBytes != int64(len(body)) {
		t.Errorf("OnBytes last reported %d, want %d", lastBytes, len(body))
	}
	if !strings.Contains(strings.Join(progress, "\n"), "Creating ISO image") {
		t.Errorf("ReaR output did not reach the server: %v", progress)
	}

	cached, err := cache.Get("web01")
	if err != nil {
		t.Fatalf("Get after build: %v", err)
	}
	got, err := os.ReadFile(cached.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(body) {
		t.Errorf("cached image is %d bytes, want %d", len(got), len(body))
	}
}

func TestServerLeavesGoodMediaInPlaceWhenARebuildFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}

	cache := server.NewMediaCache(t.TempDir())

	good := dialAgentServer(t, fakeReaRAgent(t, []byte("good-image")))
	if _, err := good.BuildRecoveryMedia(context.Background(), cache, "web01",
		server.BuildRecoveryMediaRequest{Token: testAgentToken}); err != nil {
		t.Fatalf("first build: %v", err)
	}

	// A rebuild is most likely to fail exactly when the host is in trouble —
	// which is when the operator most needs the media they already have.
	root := t.TempDir()
	failing := filepath.Join(root, "rear")
	if err := os.WriteFile(failing,
		[]byte("#!/bin/sh\necho 'ERROR: disk layout unreadable' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := NewAgentServer(AgentServerOptions{Token: testAgentToken, Version: "test"})
	broken.Media = MediaOptions{
		BuildDir:   filepath.Join(root, "media"),
		ConfigPath: filepath.Join(root, "local.conf"),
		HookPath:   filepath.Join(root, "recover.sh"),
		ReaRPath:   failing,
		SkipCheck:  true,
	}

	_, err := dialAgentServer(t, broken).BuildRecoveryMedia(context.Background(), cache, "web01",
		server.BuildRecoveryMediaRequest{Token: testAgentToken})
	if err == nil {
		t.Fatal("a failing rebuild reported success")
	}
	if !strings.Contains(err.Error(), "disk layout unreadable") {
		t.Errorf("error does not carry ReaR's diagnosis: %v", err)
	}

	cached, err := cache.Get("web01")
	if err != nil {
		t.Fatalf("previous media lost after a failed rebuild: %v", err)
	}
	data, _ := os.ReadFile(cached.Path)
	if string(data) != "good-image" {
		t.Errorf("cached image = %q; the failed rebuild clobbered it", data)
	}
}

func TestServerRefusesTruncatedMedia(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}

	// A transfer cut off partway has died mid-stream. Publishing what arrived
	// would hand an operator an ISO that boots to nothing, so the cache must
	// keep nothing at all.
	body := []byte(strings.Repeat("HITSTOR", 500_000))
	client := dialAgentServer(t, fakeReaRAgent(t, body))
	cache := server.NewMediaCache(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := client.BuildRecoveryMedia(ctx, cache, "web01",
		server.BuildRecoveryMediaRequest{
			Token: testAgentToken,
			// Drop the connection as soon as image bytes start arriving.
			OnBytes: func(int64) { cancel() },
		})
	if err == nil {
		t.Fatal("a truncated transfer was accepted")
	}
	if _, err := cache.Get("web01"); err == nil {
		t.Error("truncated image was published to the cache")
	}
}
