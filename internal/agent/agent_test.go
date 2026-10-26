package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/scanner"
	"github.com/hooperits/hitstor/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func testRepoManager(t *testing.T, repo *repository.Repository, be backend.Backend) *server.RepoManager {
	t.Helper()
	rm := server.NewRepoManager()
	rm.AddDirect("default", repo, be)
	return rm
}

// testSetup creates a temporary repository, starts an in-process gRPC server,
// and returns an agent Client, the repository, registry, and cleanup function.
func testSetup(t *testing.T) (*Client, *repository.Repository, *server.AgentRegistry, func()) {
	t.Helper()

	dir := t.TempDir()

	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}

	encKey := repo.Key().EncryptKey
	registry, err := server.LoadAgentRegistry(encKey, dir)
	if err != nil {
		t.Fatal(err)
	}

	srv := server.New(testRepoManager(t, repo, be), registry, server.Options{Version: "test-1.0"})

	lis := bufconn.Listen(bufSize)
	go func() {
		if err := srv.StartWithListener(lis); err != nil {
			// Ignore errors from server shutdown.
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}

	client := NewClientWithConn(conn)

	cleanup := func() {
		client.Close()
		srv.Stop()
		be.Close()
	}

	return client, repo, registry, cleanup
}

// createTestFiles creates a temporary directory with test files.
func createTestFiles(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestAgentPing(t *testing.T) {
	client, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	resp, err := client.Ping(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Version != "test-1.0" {
		t.Errorf("version = %q, want %q", resp.Version, "test-1.0")
	}
	if resp.RepoId == "" {
		t.Error("expected non-empty repo ID")
	}
}

func TestAgentAuthError(t *testing.T) {
	client, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	_, err := client.BeginSession(ctx, "bad-api-key", "host", []string{"/data"}, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", err)
	}
}

func TestAgentFullBackup(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Create test files.
	dataDir := createTestFiles(t, map[string][]byte{
		"file1.txt":        []byte("hello world, this is a test file with some content for backup"),
		"subdir/file2.txt": []byte("another file in a subdirectory with different content"),
	})

	// Run backup.
	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Hostname: "test-host",
		Tags:     []string{"test"},
		Parallel: 1,
	}

	ctx := context.Background()
	result, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if result.SnapshotID == "" {
		t.Error("expected non-empty snapshot ID")
	}
	if result.FilesNew == 0 {
		t.Error("expected at least one new file")
	}

	// Verify snapshot exists in repository.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapIDs) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapIDs))
	}

	snap, err := repo.LoadSnapshot(snapIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if snap.Hostname != "test-host" {
		t.Errorf("hostname = %q, want %q", snap.Hostname, "test-host")
	}
}

func TestAgentDeduplication(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Create test files.
	dataDir := createTestFiles(t, map[string][]byte{
		"data.txt": []byte("some data that will be backed up twice to test deduplication"),
	})

	ctx := context.Background()

	// First backup.
	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Hostname: "test-host",
		Parallel: 1,
	}
	result1, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result1.BytesAdded == 0 {
		t.Error("first backup should add bytes")
	}

	// Second backup — identical data should be fully deduplicated.
	result2, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result2.BytesAdded != 0 {
		t.Errorf("second backup BytesAdded = %d, want 0 (fully deduplicated)", result2.BytesAdded)
	}
}

func TestAgentExcludes(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Create test files.
	dataDir := createTestFiles(t, map[string][]byte{
		"keep.txt":   []byte("this file should be backed up"),
		"ignore.log": []byte("this log file should be excluded"),
		"also.txt":   []byte("this should also be backed up"),
	})

	ctx := context.Background()

	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Excludes: []string{"*.log"},
		Hostname: "test-host",
		Parallel: 1,
	}
	result, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Should have 2 new files (keep.txt and also.txt), not 3.
	if result.FilesNew != 2 {
		t.Errorf("FilesNew = %d, want 2 (*.log should be excluded)", result.FilesNew)
	}
}

func TestAgentSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires developer mode or admin on Windows")
	}

	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Create test files with symlink.
	dataDir := createTestFiles(t, map[string][]byte{
		"target.txt": []byte("the real file content"),
	})
	// Create symlink.
	if err := os.Symlink("target.txt", filepath.Join(dataDir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Hostname: "test-host",
		Parallel: 1,
	}
	result, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if result.SnapshotID == "" {
		t.Error("expected non-empty snapshot ID")
	}
	// At least 1 file should be new (target.txt).
	if result.FilesNew == 0 {
		t.Error("expected at least one new file")
	}
}

func TestAgentCrossAgentDedup(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey1, err := registry.AddAgent("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	apiKey2, err := registry.AddAgent("agent-2")
	if err != nil {
		t.Fatal(err)
	}

	sharedContent := []byte("shared data content that both agents back up identically for cross-agent dedup")

	// Agent 1 backup.
	dir1 := createTestFiles(t, map[string][]byte{"shared.txt": sharedContent})
	ctx := context.Background()

	b1 := &Backup{
		Client:   client,
		APIKey:   apiKey1,
		Paths:    []string{dir1},
		Hostname: "host-1",
		Parallel: 1,
	}
	result1, err := b1.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result1.BytesAdded == 0 {
		t.Error("agent-1 should add bytes")
	}

	// Agent 2 backup — same content, different agent.
	dir2 := createTestFiles(t, map[string][]byte{"shared.txt": sharedContent})

	b2 := &Backup{
		Client:   client,
		APIKey:   apiKey2,
		Paths:    []string{dir2},
		Hostname: "host-2",
		Parallel: 1,
	}
	result2, err := b2.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result2.BytesAdded != 0 {
		t.Errorf("agent-2 BytesAdded = %d, want 0 (cross-agent dedup)", result2.BytesAdded)
	}
}

func TestAgentEmptyDir(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Create a directory with an empty subdirectory.
	dataDir := createTestFiles(t, map[string][]byte{
		"file.txt": []byte("a file alongside an empty dir"),
	})
	if err := os.MkdirAll(filepath.Join(dataDir, "empty-subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Hostname: "test-host",
		Parallel: 1,
	}
	result, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if result.SnapshotID == "" {
		t.Error("expected non-empty snapshot ID")
	}
}

func TestCheckBlobsBatching(t *testing.T) {
	// Verify that CheckBlobs handles batching correctly with >1000 IDs.
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	beginResp, err := client.BeginSession(ctx, apiKey, "host", []string{"/data"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Generate 2500 fake blob IDs (more than checkBlobsBatchSize=1000).
	ids := make([][32]byte, 2500)
	for i := range ids {
		ids[i][0] = byte(i >> 8)
		ids[i][1] = byte(i)
	}

	needed, err := client.CheckBlobs(ctx, beginResp.SessionId, ids)
	if err != nil {
		t.Fatal(err)
	}

	// All IDs should be needed since repo is empty.
	if len(needed) != 2500 {
		t.Errorf("needed = %d, want 2500", len(needed))
	}
}

func TestAgentParallelWorkers(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Create many small files to exercise parallel processing.
	files := make(map[string][]byte)
	for i := 0; i < 20; i++ {
		name := filepath.Join("batch", fmt.Sprintf("file-%03d.txt", i))
		files[name] = []byte(fmt.Sprintf("content for file number %d to test parallel workers", i))
	}
	dataDir := createTestFiles(t, files)

	ctx := context.Background()

	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Hostname: "test-host",
		Parallel: 4, // Use multiple workers.
	}
	result, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesNew != 20 {
		t.Errorf("FilesNew = %d, want 20", result.FilesNew)
	}
	if result.SnapshotID == "" {
		t.Error("expected non-empty snapshot ID")
	}
}

func TestAgentWithProgressCallbacks(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	dataDir := createTestFiles(t, map[string][]byte{
		"a.txt": []byte("file a content for progress callback test"),
		"b.txt": []byte("file b content for progress callback test"),
	})

	ctx := context.Background()

	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Hostname: "test-host",
		Parallel: 1,
	}

	var mu sync.Mutex
	var progressCount int
	b.SetProgress(func(path string, size uint64, isNew bool) {
		mu.Lock()
		progressCount++
		mu.Unlock()
	})

	var scanFiles, scanDirs int
	b.SetScanComplete(func(files, dirs int, totalBytes uint64) {
		scanFiles = files
		scanDirs = dirs
	})

	result, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if result.SnapshotID == "" {
		t.Error("expected non-empty snapshot ID")
	}

	mu.Lock()
	defer mu.Unlock()
	if progressCount != 2 {
		t.Errorf("progress called %d times, want 2", progressCount)
	}
	if scanFiles != 2 {
		t.Errorf("scanComplete files = %d, want 2", scanFiles)
	}
	// scanDirs may be 0 if files are directly under root (fs.Walk skips root).
	_ = scanDirs
}

// mockScanner implements scanner.Scanner for testing the plugin scanner path.
type mockScanner struct {
	paths    []string
	excludes []string
}

func (m *mockScanner) Scan(paths []string, excludes []string) (<-chan scanner.ScanResult, error) {
	m.paths = paths
	m.excludes = excludes
	ch := make(chan scanner.ScanResult)
	go func() {
		defer close(ch)
		for _, p := range paths {
			_ = fs.Walk(p, excludes, func(path string, node fs.Node) error {
				ch <- scanner.ScanResult{Path: path, Node: node}
				return nil
			})
		}
	}()
	return ch, nil
}

func (m *mockScanner) Close() error { return nil }

func TestAgentWithPluginScanner(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	dataDir := createTestFiles(t, map[string][]byte{
		"scanned.txt": []byte("file found by plugin scanner"),
	})

	ctx := context.Background()

	sc := &mockScanner{}

	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Hostname: "test-host",
		Parallel: 1,
		Scanner:  sc,
	}

	result, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if result.SnapshotID == "" {
		t.Error("expected non-empty snapshot ID")
	}
	if result.FilesNew == 0 {
		t.Error("expected at least one new file from scanner")
	}
}

func TestAgentAutoHostname(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	dataDir := createTestFiles(t, map[string][]byte{
		"file.txt": []byte("auto hostname test content"),
	})

	ctx := context.Background()

	// Empty hostname triggers os.Hostname() auto-detection.
	b := &Backup{
		Client:   client,
		APIKey:   apiKey,
		Paths:    []string{dataDir},
		Hostname: "",
		Parallel: 1,
	}

	result, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if result.SnapshotID == "" {
		t.Error("expected non-empty snapshot ID")
	}
}

func TestNewClientBadCACert(t *testing.T) {
	_, err := NewClient(ClientOptions{
		ServerAddr: "localhost:1234",
		CACertPath: "/nonexistent/ca.pem",
	})
	if err == nil {
		t.Error("expected error for non-existent CA cert")
	}
}

func TestNewClientNoCACert(t *testing.T) {
	c, err := NewClient(ClientOptions{
		ServerAddr: "localhost:1234",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.Close()
}
