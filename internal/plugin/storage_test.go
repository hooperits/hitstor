package plugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"math"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
)

const bufSize = 1024 * 1024

func testID(s string) backend.ID {
	return sha256.Sum256([]byte(s))
}

// setupBufconn creates a local backend wrapped in grpcServer+grpcClient
// connected via bufconn. Returns the client-side backend.Backend.
func setupBufconn(t *testing.T) (backend.Backend, func()) {
	t.Helper()
	dir := t.TempDir()
	localBe, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Match go-plugin's message size limits for large payload tests.
	maxMsgSize := math.MaxInt32

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(grpc.MaxRecvMsgSize(maxMsgSize), grpc.MaxSendMsgSize(maxMsgSize))
	pb.RegisterStorageBackendServer(srv, &storageGRPCServer{impl: localBe})
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

	be := &StorageBackendGRPCClient{client: pb.NewStorageBackendClient(conn)}
	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
		lis.Close()
		localBe.Close()
	}
	return be, cleanup
}

func TestPluginSaveThenLoad(t *testing.T) {
	be, cleanup := setupBufconn(t)
	defer cleanup()

	ctx := context.Background()
	id := testID("test-save-load")
	data := []byte("hello plugin world")

	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rc, err := be.Load(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Load = %q, want %q", got, data)
	}
}

func TestPluginStat(t *testing.T) {
	be, cleanup := setupBufconn(t)
	defer cleanup()

	ctx := context.Background()
	id := testID("test-stat")
	data := []byte("stat test data")

	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := be.Stat(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Size != int64(len(data)) {
		t.Errorf("Stat size = %d, want %d", fi.Size, len(data))
	}
}

func TestPluginList(t *testing.T) {
	be, cleanup := setupBufconn(t)
	defer cleanup()

	ctx := context.Background()
	ids := []backend.ID{
		testID("list-1"),
		testID("list-2"),
		testID("list-3"),
	}

	for _, id := range ids {
		if err := be.Save(ctx, backend.IndexFile, id, bytes.NewReader([]byte("data"))); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, err := be.List(ctx, backend.IndexFile)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("List count = %d, want 3", len(got))
	}

	idSet := make(map[backend.ID]bool)
	for _, id := range got {
		idSet[id] = true
	}
	for _, id := range ids {
		if !idSet[id] {
			t.Errorf("List missing ID %s", id)
		}
	}
}

func TestPluginRemove(t *testing.T) {
	be, cleanup := setupBufconn(t)
	defer cleanup()

	ctx := context.Background()
	id := testID("test-remove")

	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader([]byte("remove me"))); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := be.Remove(ctx, backend.DataFile, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err := be.Load(ctx, backend.DataFile, id)
	if err == nil {
		t.Error("Load after Remove should fail")
	}
}

func TestPluginLoadNonExistent(t *testing.T) {
	be, cleanup := setupBufconn(t)
	defer cleanup()

	ctx := context.Background()
	id := testID("nonexistent")

	_, err := be.Load(ctx, backend.DataFile, id)
	if err == nil {
		t.Error("Load nonexistent should fail")
	}
}

func TestPluginAllFileTypes(t *testing.T) {
	be, cleanup := setupBufconn(t)
	defer cleanup()

	ctx := context.Background()
	fileTypes := []backend.FileType{
		backend.DataFile,
		backend.IndexFile,
		backend.SnapshotFile,
		backend.KeyFile,
		backend.LockFile,
	}

	for _, ft := range fileTypes {
		id := testID("filetype-" + ft.String())
		data := []byte("data for " + ft.String())

		if err := be.Save(ctx, ft, id, bytes.NewReader(data)); err != nil {
			t.Fatalf("Save(%s): %v", ft, err)
		}

		rc, err := be.Load(ctx, ft, id)
		if err != nil {
			t.Fatalf("Load(%s): %v", ft, err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("ReadAll(%s): %v", ft, err)
		}
		if !bytes.Equal(got, data) {
			t.Errorf("Load(%s) = %q, want %q", ft, got, data)
		}
	}
}

func TestPluginLargePayload(t *testing.T) {
	be, cleanup := setupBufconn(t)
	defer cleanup()

	ctx := context.Background()
	id := testID("large-payload")

	// 10 MiB payload.
	data := make([]byte, 10*1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rc, err := be.Load(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("Load returned different data for 10MiB payload")
	}
}

func TestBytesToID(t *testing.T) {
	// 32-byte input.
	input := make([]byte, 32)
	for i := range input {
		input[i] = byte(i)
	}
	id := bytesToID(input)
	for i := range id {
		if id[i] != byte(i) {
			t.Fatalf("bytesToID mismatch at index %d: got %d, want %d", i, id[i], i)
		}
	}

	// Short input — should zero-pad.
	short := []byte{0xAA, 0xBB}
	id = bytesToID(short)
	if id[0] != 0xAA || id[1] != 0xBB {
		t.Error("bytesToID short input: first bytes wrong")
	}
	for i := 2; i < 32; i++ {
		if id[i] != 0 {
			t.Errorf("bytesToID short input: byte %d = %d, want 0", i, id[i])
		}
	}

	// Empty input — should be zero ID.
	id = bytesToID(nil)
	for i := range id {
		if id[i] != 0 {
			t.Errorf("bytesToID nil: byte %d = %d, want 0", i, id[i])
		}
	}
}
