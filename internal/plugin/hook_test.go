package plugin

import (
	"context"
	"fmt"
	"math"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/hook"
)

// testHook records events for test assertions.
type testHook struct {
	mu          sync.Mutex
	preEvents   []hook.HookEvent
	postEvents  []hook.HookEvent
	settings    map[string]string
	failPreNext bool
}

var _ hook.Hook = (*testHook)(nil)

func (h *testHook) PreBackup(_ context.Context, event hook.HookEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failPreNext {
		h.failPreNext = false
		return fmt.Errorf("injected pre-backup error")
	}
	h.preEvents = append(h.preEvents, event)
	return nil
}

func (h *testHook) PostBackup(_ context.Context, event hook.HookEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.postEvents = append(h.postEvents, event)
	return nil
}

func (h *testHook) Close() error { return nil }

func (h *testHook) Configure(settings map[string]string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.settings = settings
	return nil
}

func (h *testHook) getPreEvents() []hook.HookEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]hook.HookEvent, len(h.preEvents))
	copy(cp, h.preEvents)
	return cp
}

func (h *testHook) getPostEvents() []hook.HookEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]hook.HookEvent, len(h.postEvents))
	copy(cp, h.postEvents)
	return cp
}

func setupHookBufconn(t *testing.T, impl *testHook) (hook.Hook, func()) {
	t.Helper()

	maxMsgSize := math.MaxInt32
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(grpc.MaxRecvMsgSize(maxMsgSize), grpc.MaxSendMsgSize(maxMsgSize))
	pb.RegisterHookServer(srv, &hookGRPCServer{impl: impl})
	go srv.Serve(lis)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMsgSize),
			grpc.MaxCallSendMsgSize(maxMsgSize),
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	client := &HookGRPCClient{client: pb.NewHookClient(conn)}
	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
		lis.Close()
	}
	return client, cleanup
}

func TestHookPreBackupSuccess(t *testing.T) {
	impl := &testHook{}
	h, cleanup := setupHookBufconn(t, impl)
	defer cleanup()

	evt := hook.HookEvent{
		Agent:    "agent-1",
		Hostname: "host-1",
		Paths:    []string{"/data", "/logs"},
		Excludes: []string{"*.tmp"},
		Tags:     []string{"daily"},
	}

	if err := h.PreBackup(context.Background(), evt); err != nil {
		t.Fatalf("PreBackup: %v", err)
	}

	events := impl.getPreEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 pre event, got %d", len(events))
	}
	got := events[0]
	if got.Agent != "agent-1" {
		t.Errorf("agent = %q, want agent-1", got.Agent)
	}
	if got.Hostname != "host-1" {
		t.Errorf("hostname = %q, want host-1", got.Hostname)
	}
	if len(got.Paths) != 2 || got.Paths[0] != "/data" || got.Paths[1] != "/logs" {
		t.Errorf("paths = %v, want [/data /logs]", got.Paths)
	}
	if len(got.Excludes) != 1 || got.Excludes[0] != "*.tmp" {
		t.Errorf("excludes = %v, want [*.tmp]", got.Excludes)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "daily" {
		t.Errorf("tags = %v, want [daily]", got.Tags)
	}
}

func TestHookPostBackupSuccess(t *testing.T) {
	impl := &testHook{}
	h, cleanup := setupHookBufconn(t, impl)
	defer cleanup()

	evt := hook.HookEvent{
		Agent:      "agent-1",
		Hostname:   "host-1",
		Paths:      []string{"/data"},
		SnapshotID: "abc123",
		Details:    map[string]string{"files_new": "42", "bytes_added": "1024"},
	}

	if err := h.PostBackup(context.Background(), evt); err != nil {
		t.Fatalf("PostBackup: %v", err)
	}

	events := impl.getPostEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 post event, got %d", len(events))
	}
	got := events[0]
	if got.SnapshotID != "abc123" {
		t.Errorf("snapshot_id = %q, want abc123", got.SnapshotID)
	}
	if got.Details == nil {
		t.Fatal("details should not be nil")
	}
	if got.Details["files_new"] != "42" {
		t.Errorf("details[files_new] = %v, want 42", got.Details["files_new"])
	}
	if got.Details["bytes_added"] != "1024" {
		t.Errorf("details[bytes_added] = %v, want 1024", got.Details["bytes_added"])
	}
}

func TestHookPreBackupError(t *testing.T) {
	impl := &testHook{failPreNext: true}
	h, cleanup := setupHookBufconn(t, impl)
	defer cleanup()

	evt := hook.HookEvent{
		Agent: "agent-1",
		Paths: []string{"/data"},
	}

	err := h.PreBackup(context.Background(), evt)
	if err == nil {
		t.Error("expected error from failing pre-backup hook")
	}
}

func TestHookConfigureThenHook(t *testing.T) {
	impl := &testHook{}
	h, cleanup := setupHookBufconn(t, impl)
	defer cleanup()

	// Configure via the gRPC client's underlying proto client.
	grpcClient := h.(*HookGRPCClient)
	_, err := grpcClient.client.Configure(context.Background(), &pb.ConfigureRequest{
		Settings: map[string]string{"level": "debug"},
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	impl.mu.Lock()
	if impl.settings["level"] != "debug" {
		t.Errorf("settings[level] = %q, want debug", impl.settings["level"])
	}
	impl.mu.Unlock()

	// PreBackup should still work after Configure.
	evt := hook.HookEvent{
		Agent: "agent-1",
		Paths: []string{"/data"},
	}
	if err := h.PreBackup(context.Background(), evt); err != nil {
		t.Fatalf("PreBackup after Configure: %v", err)
	}
}

func TestHookClose(t *testing.T) {
	impl := &testHook{}
	h, cleanup := setupHookBufconn(t, impl)
	defer cleanup()

	// Close should not panic on a client without a pluginClient.
	if err := h.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
