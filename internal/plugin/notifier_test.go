package plugin

import (
	"context"
	"fmt"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/notifier"
)

// testNotifier records events for test assertions.
type testNotifier struct {
	mu       sync.Mutex
	events   []notifier.Event
	settings map[string]string
	failNext bool
}

var _ notifier.Notifier = (*testNotifier)(nil)

func (n *testNotifier) Notify(_ context.Context, event notifier.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.failNext {
		n.failNext = false
		return fmt.Errorf("injected error")
	}
	n.events = append(n.events, event)
	return nil
}

func (n *testNotifier) Close() error { return nil }

func (n *testNotifier) Configure(settings map[string]string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.settings = settings
	return nil
}

func (n *testNotifier) getEvents() []notifier.Event {
	n.mu.Lock()
	defer n.mu.Unlock()
	cp := make([]notifier.Event, len(n.events))
	copy(cp, n.events)
	return cp
}

func setupNotifierBufconn(t *testing.T, impl *testNotifier) (notifier.Notifier, func()) {
	t.Helper()

	maxMsgSize := math.MaxInt32
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(grpc.MaxRecvMsgSize(maxMsgSize), grpc.MaxSendMsgSize(maxMsgSize))
	pb.RegisterNotifierServer(srv, &notifierGRPCServer{impl: impl})
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

	client := &NotifierGRPCClient{client: pb.NewNotifierClient(conn)}
	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
		lis.Close()
	}
	return client, cleanup
}

func TestNotifierNotifySuccess(t *testing.T) {
	impl := &testNotifier{}
	n, cleanup := setupNotifierBufconn(t, impl)
	defer cleanup()

	evt := notifier.Event{
		EventType: notifier.EventBackupFailed,
		Server:    "hitstor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Agent:     "agent-1",
		Message:   "backup failed: connection lost",
	}

	if err := n.Notify(context.Background(), evt); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	events := impl.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.EventType != notifier.EventBackupFailed {
		t.Errorf("event type = %q, want %q", got.EventType, notifier.EventBackupFailed)
	}
	if got.Agent != "agent-1" {
		t.Errorf("agent = %q, want agent-1", got.Agent)
	}
	if got.Message != "backup failed: connection lost" {
		t.Errorf("message = %q", got.Message)
	}
}

func TestNotifierNotifyWithAllFields(t *testing.T) {
	impl := &testNotifier{}
	n, cleanup := setupNotifierBufconn(t, impl)
	defer cleanup()

	evt := notifier.Event{
		EventType: notifier.EventAgentStale,
		Server:    "hitstor",
		Timestamp: "2026-02-21T12:00:00Z",
		Agent:     "db-server",
		Hostname:  "db01.example.com",
		Message:   "no backup in 48h (threshold: 25h)",
		Details:   map[string]any{"last_backup": "2026-02-19T12:00:00Z", "threshold_hours": "25"},
	}

	if err := n.Notify(context.Background(), evt); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	events := impl.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.Hostname != "db01.example.com" {
		t.Errorf("hostname = %q, want db01.example.com", got.Hostname)
	}
	if got.Details == nil {
		t.Fatal("details should not be nil")
	}
	if got.Details["last_backup"] != "2026-02-19T12:00:00Z" {
		t.Errorf("details[last_backup] = %v", got.Details["last_backup"])
	}
}

func TestNotifierConfigureThenNotify(t *testing.T) {
	impl := &testNotifier{}
	n, cleanup := setupNotifierBufconn(t, impl)
	defer cleanup()

	// Configure via the gRPC client's underlying proto client.
	grpcClient := n.(*NotifierGRPCClient)
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

	// Notify should still work after Configure.
	evt := notifier.Event{
		EventType: notifier.EventBackupFailed,
		Server:    "hitstor",
		Agent:     "agent-1",
		Message:   "test",
	}
	if err := n.Notify(context.Background(), evt); err != nil {
		t.Fatalf("Notify after Configure: %v", err)
	}
}

func TestNotifierNotifyError(t *testing.T) {
	impl := &testNotifier{failNext: true}
	n, cleanup := setupNotifierBufconn(t, impl)
	defer cleanup()

	evt := notifier.Event{
		EventType: notifier.EventBackupFailed,
		Server:    "hitstor",
		Agent:     "agent-1",
		Message:   "test",
	}

	err := n.Notify(context.Background(), evt)
	if err == nil {
		t.Error("expected error from failing notifier")
	}
}

func TestNotifierClose(t *testing.T) {
	impl := &testNotifier{}
	n, cleanup := setupNotifierBufconn(t, impl)
	defer cleanup()

	// Close should not panic on a client without a pluginClient.
	if err := n.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
