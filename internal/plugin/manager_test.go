package plugin

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/backend"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestFindPluginInDir(t *testing.T) {
	dir := t.TempDir()
	name := "hitstor-backend-test"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}

	got := findPlugin(name, dir)
	if got != path {
		t.Errorf("findPlugin = %q, want %q", got, path)
	}
}

func TestFindPluginNotFound(t *testing.T) {
	got := findPlugin("hitstor-nonexistent-plugin-xyz", "")
	if got != "" {
		t.Errorf("findPlugin = %q, want empty", got)
	}
}

func TestFindPluginDirIsDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "hitstor-backend-fake")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	got := findPlugin("hitstor-backend-fake", dir)
	if got != "" {
		t.Errorf("findPlugin returned %q for directory, want empty", got)
	}
}

func TestFindPluginEmptyDir(t *testing.T) {
	got := findPlugin("hitstor-nonexistent-xyz", "")
	if got != "" {
		t.Errorf("findPlugin = %q, want empty", got)
	}
}

func TestOpenStorageBackendNotFound(t *testing.T) {
	_, err := OpenStorageBackend("nonexistent-xyz", "", nil)
	if err == nil {
		t.Fatal("expected error for missing plugin")
	}
}

func TestOpenNotifierNotFound(t *testing.T) {
	_, err := OpenNotifier("nonexistent-xyz", "", nil)
	if err == nil {
		t.Fatal("expected error for missing plugin")
	}
}

func TestOpenHookNotFound(t *testing.T) {
	_, err := OpenHook("nonexistent-xyz", "", nil)
	if err == nil {
		t.Fatal("expected error for missing plugin")
	}
}

func TestOpenFilesystemScannerNotFound(t *testing.T) {
	_, err := OpenFilesystemScanner("nonexistent-xyz", "", nil)
	if err == nil {
		t.Fatal("expected error for missing plugin")
	}
}

func dialBufconn(lis *bufconn.Listener) grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
		return lis.DialContext(ctx)
	})
}

// TestStorageBackendGRPCServerClient tests the GRPCServer and GRPCClient
// factory methods via a real gRPC connection using bufconn.
func TestStorageBackendGRPCServerClient(t *testing.T) {
	p := &StorageBackendPlugin{Impl: nil}

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	if err := p.GRPCServer(nil, s); err != nil {
		t.Fatalf("GRPCServer: %v", err)
	}
	go s.Serve(lis)
	defer s.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		dialBufconn(lis),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	raw, err := p.GRPCClient(context.Background(), nil, conn)
	if err != nil {
		t.Fatalf("GRPCClient: %v", err)
	}
	if _, ok := raw.(*StorageBackendGRPCClient); !ok {
		t.Errorf("GRPCClient returned %T, want *StorageBackendGRPCClient", raw)
	}
}

// TestNotifierGRPCServerClient tests the NotifierPlugin factory methods.
func TestNotifierGRPCServerClient(t *testing.T) {
	p := &NotifierPlugin{Impl: nil}

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	if err := p.GRPCServer(nil, s); err != nil {
		t.Fatal(err)
	}
	go s.Serve(lis)
	defer s.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		dialBufconn(lis),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	raw, err := p.GRPCClient(context.Background(), nil, conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.(*NotifierGRPCClient); !ok {
		t.Errorf("GRPCClient returned %T, want *NotifierGRPCClient", raw)
	}
}

// TestHookGRPCServerClient tests the HookPlugin factory methods.
func TestHookGRPCServerClient(t *testing.T) {
	p := &HookPlugin{Impl: nil}

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	if err := p.GRPCServer(nil, s); err != nil {
		t.Fatal(err)
	}
	go s.Serve(lis)
	defer s.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		dialBufconn(lis),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	raw, err := p.GRPCClient(context.Background(), nil, conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.(*HookGRPCClient); !ok {
		t.Errorf("GRPCClient returned %T, want *HookGRPCClient", raw)
	}
}

// TestScannerGRPCServerClient tests the FilesystemScannerPlugin factory methods.
func TestScannerGRPCServerClient(t *testing.T) {
	p := &FilesystemScannerPlugin{Impl: nil}

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	if err := p.GRPCServer(nil, s); err != nil {
		t.Fatal(err)
	}
	go s.Serve(lis)
	defer s.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		dialBufconn(lis),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	raw, err := p.GRPCClient(context.Background(), nil, conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.(*FilesystemScannerGRPCClient); !ok {
		t.Errorf("GRPCClient returned %T, want *FilesystemScannerGRPCClient", raw)
	}
}

// TestStorageBackendCloseNilClient tests Close with nil pluginClient.
func TestStorageBackendCloseNilClient(t *testing.T) {
	c := &StorageBackendGRPCClient{client: nil, pluginClient: nil}
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// configurableMock is a minimal backend.Backend that also implements Configurable.
type configurableMock struct {
	settings map[string]string
}

func (m *configurableMock) Configure(s map[string]string) error {
	m.settings = s
	return nil
}

func (m *configurableMock) Save(_ context.Context, _ backend.FileType, _ backend.ID, _ io.Reader) error {
	return nil
}
func (m *configurableMock) Load(_ context.Context, _ backend.FileType, _ backend.ID) (io.ReadCloser, error) {
	return nil, nil
}
func (m *configurableMock) Stat(_ context.Context, _ backend.FileType, _ backend.ID) (backend.FileInfo, error) {
	return backend.FileInfo{}, nil
}
func (m *configurableMock) List(_ context.Context, _ backend.FileType) ([]backend.ID, error) {
	return nil, nil
}
func (m *configurableMock) Remove(_ context.Context, _ backend.FileType, _ backend.ID) error {
	return nil
}
func (m *configurableMock) Close() error     { return nil }
func (m *configurableMock) BasePath() string { return "" }

// TestStorageConfigureViaGRPC tests the Configure RPC through gRPC.
func TestStorageConfigureViaGRPC(t *testing.T) {
	mock := &configurableMock{}
	p := &StorageBackendPlugin{Impl: mock}

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	p.GRPCServer(nil, s)
	go s.Serve(lis)
	defer s.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		dialBufconn(lis),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := pb.NewStorageBackendClient(conn)
	_, err = client.Configure(context.Background(), &pb.ConfigureRequest{
		Settings: map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if mock.settings["key"] != "value" {
		t.Errorf("Configure didn't pass settings, got %v", mock.settings)
	}
}
