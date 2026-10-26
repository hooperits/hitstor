package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/hook"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/sentinel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// testSetup creates a temporary repository, agent registry, and in-process
// gRPC server for testing. Returns the client connection and cleanup function.
// testRepoManager creates a RepoManager with a single test repo named "default".
func testRepoManager(t *testing.T, repo *repository.Repository, be backend.Backend) *RepoManager {
	t.Helper()
	rm := NewRepoManager()
	rm.mu.Lock()
	rm.repos["default"] = &ManagedRepo{
		Name:            "default",
		Repo:            repo,
		Backend:         be,
		MaintenanceLock: NewMaintenanceLock(),
	}
	rm.encryptKey = repo.Key().EncryptKey
	rm.hasKey = true
	rm.mu.Unlock()
	return rm
}

func testSetup(t *testing.T) (pb.BackupServiceClient, *repository.Repository, *AgentRegistry, func()) {
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
	registry, err := LoadAgentRegistry(encKey, dir)
	if err != nil {
		t.Fatal(err)
	}

	rm := testRepoManager(t, repo, be)
	srv := New(rm, registry, Options{Version: "test-1.0"})

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

	client := pb.NewBackupServiceClient(conn)

	cleanup := func() {
		conn.Close()
		srv.Stop()
		be.Close()
	}

	return client, repo, registry, cleanup
}

func TestPing(t *testing.T) {
	client, _, _, cleanup := testSetup(t)
	defer cleanup()

	resp, err := client.Ping(context.Background(), &pb.PingRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Version != "test-1.0" {
		t.Errorf("version = %q, want %q", resp.Version, "test-1.0")
	}
	if resp.RepoId == "" {
		t.Error("expected non-empty repo ID")
	}
	if len(resp.RepoId) != 64 {
		t.Errorf("repo ID length = %d, want 64", len(resp.RepoId))
	}
}

func TestBeginSession_Unauthenticated(t *testing.T) {
	client, _, _, cleanup := testSetup(t)
	defer cleanup()

	_, err := client.BeginSession(context.Background(), &pb.BeginSessionRequest{
		ApiKey:   "0000000000000000000000000000000000000000000000000000000000000000",
		Hostname: "test-host",
		Paths:    []string{"/data"},
	})
	if err == nil {
		t.Fatal("expected error for unauthenticated request")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}
}

func TestBeginSession_Authenticated(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.BeginSession(context.Background(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "test-host",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.SessionId == "" {
		t.Error("expected non-empty session ID")
	}
	if len(resp.IdKey) != 32 {
		t.Errorf("IDKey length = %d, want 32", len(resp.IdKey))
	}

	cfg := repo.Cfg()
	if resp.ChunkerMin != uint32(cfg.ChunkerParams.MinSize) {
		t.Errorf("chunker_min = %d, want %d", resp.ChunkerMin, cfg.ChunkerParams.MinSize)
	}
	if resp.ChunkerAvg != uint32(cfg.ChunkerParams.AvgSize) {
		t.Errorf("chunker_avg = %d, want %d", resp.ChunkerAvg, cfg.ChunkerParams.AvgSize)
	}
	if resp.ChunkerMax != uint32(cfg.ChunkerParams.MaxSize) {
		t.Errorf("chunker_max = %d, want %d", resp.ChunkerMax, cfg.ChunkerParams.MaxSize)
	}
}

// simulateAgentBackup runs a full backup cycle as a simulated agent.
// Creates test data, chunks it, computes blob IDs, sends to server.
func simulateAgentBackup(t *testing.T, client pb.BackupServiceClient, apiKey, hostname string, files map[string][]byte) string {
	t.Helper()
	ctx := context.Background()

	// 1. BeginSession
	beginResp, err := client.BeginSession(ctx, &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: hostname,
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatalf("BeginSession: %v", err)
	}

	var idKey [32]byte
	copy(idKey[:], beginResp.IdKey)

	// 2. For each file, "chunk" it (we treat each file as a single chunk for
	// simplicity), compute blob ID, compress, and track.
	type chunkInfo struct {
		blobID     [32]byte
		compressed []byte
		rawLen     int
	}

	var allChunks []chunkInfo
	var fileNodes []*pb.FileNode

	for name, data := range files {
		blobID := crypto.BlobID(idKey, data)

		compressed, err := compress.Compress(data)
		if err != nil {
			t.Fatalf("compress %q: %v", name, err)
		}

		allChunks = append(allChunks, chunkInfo{
			blobID:     blobID,
			compressed: compressed,
			rawLen:     len(data),
		})

		fileNodes = append(fileNodes, &pb.FileNode{
			Path:       name,
			Mode:       0644,
			MtimeNs:    1000000000,
			Size:       int64(len(data)),
			Uid:        1000,
			Gid:        1000,
			ContentIds: [][]byte{blobID[:]},
		})
	}

	// 3. CheckBlobs — ask server which blobs are needed.
	var blobIDs [][]byte
	for _, c := range allChunks {
		blobIDs = append(blobIDs, c.blobID[:])
	}

	checkResp, err := client.CheckBlobs(ctx, &pb.CheckBlobsRequest{
		SessionId: beginResp.SessionId,
		BlobIds:   blobIDs,
	})
	if err != nil {
		t.Fatalf("CheckBlobs: %v", err)
	}

	// Build a set of needed IDs.
	needed := make(map[[32]byte]bool)
	for _, idBytes := range checkResp.NeededIds {
		var bid [32]byte
		copy(bid[:], idBytes)
		needed[bid] = true
	}

	// 4. UploadChunks — stream needed chunks.
	stream, err := client.UploadChunks(ctx)
	if err != nil {
		t.Fatalf("UploadChunks: %v", err)
	}

	for _, c := range allChunks {
		if !needed[c.blobID] {
			continue
		}
		if err := stream.Send(&pb.UploadChunkMessage{
			SessionId:          beginResp.SessionId,
			BlobId:             c.blobID[:],
			CompressedData:     c.compressed,
			UncompressedLength: uint32(c.rawLen),
		}); err != nil {
			t.Fatalf("stream.Send: %v", err)
		}
	}

	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}

	// 5. FinalizeBackup
	// Add root directory node.
	fileNodes = append(fileNodes, &pb.FileNode{
		Path:  "",
		IsDir: true,
		Mode:  0755,
	})

	finalResp, err := client.FinalizeBackup(ctx, &pb.FinalizeBackupRequest{
		SessionId: beginResp.SessionId,
		Files:     fileNodes,
	})
	if err != nil {
		t.Fatalf("FinalizeBackup: %v", err)
	}

	if finalResp.SnapshotId == "" {
		t.Fatal("expected non-empty snapshot ID")
	}

	return finalResp.SnapshotId
}

func TestFullBackupCycle(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"hello.txt":     []byte("Hello, world!"),
		"data/test.bin": []byte("binary data content here"),
	}

	snapID := simulateAgentBackup(t, client, apiKey, "test-host", files)
	t.Logf("snapshot created: %s", snapID)

	// Verify snapshot exists in repo.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapIDs) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(snapIDs))
	}

	snap, err := repo.LoadSnapshot(snapIDs[0])
	if err != nil {
		t.Fatal(err)
	}

	if snap.Hostname != "test-host" {
		t.Errorf("hostname = %q, want %q", snap.Hostname, "test-host")
	}
	if len(snap.Paths) != 1 || snap.Paths[0] != "/data" {
		t.Errorf("paths = %v, want [/data]", snap.Paths)
	}
	if snap.Summary.TotalFiles != 2 {
		t.Errorf("total files = %d, want 2", snap.Summary.TotalFiles)
	}
	if snap.Summary.FilesNew != 2 {
		t.Errorf("files new = %d, want 2", snap.Summary.FilesNew)
	}
}

func TestDeduplication(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"file1.txt": []byte("deduplicated content"),
	}

	// First backup.
	simulateAgentBackup(t, client, apiKey, "host-1", files)

	// Second backup of the same data — should dedup.
	ctx := context.Background()

	beginResp, err := client.BeginSession(ctx, &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "host-1",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var idKey [32]byte
	copy(idKey[:], beginResp.IdKey)

	data := []byte("deduplicated content")
	blobID := crypto.BlobID(idKey, data)

	checkResp, err := client.CheckBlobs(ctx, &pb.CheckBlobsRequest{
		SessionId: beginResp.SessionId,
		BlobIds:   [][]byte{blobID[:]},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Server should say zero blobs needed (all deduplicated).
	if len(checkResp.NeededIds) != 0 {
		t.Errorf("needed IDs = %d, want 0 (should be fully deduplicated)", len(checkResp.NeededIds))
	}

	// Still finalize to create a second snapshot.
	stream, err := client.UploadChunks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatal(err)
	}

	finalResp, err := client.FinalizeBackup(ctx, &pb.FinalizeBackupRequest{
		SessionId: beginResp.SessionId,
		Files: []*pb.FileNode{
			{Path: "", IsDir: true, Mode: 0755},
			{Path: "file1.txt", Mode: 0644, MtimeNs: 1000000000, Size: int64(len(data)), Uid: 1000, Gid: 1000, ContentIds: [][]byte{blobID[:]}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if finalResp.FilesUnchanged != 1 {
		t.Errorf("files unchanged = %d, want 1", finalResp.FilesUnchanged)
	}
	if finalResp.BytesAdded != 0 {
		t.Errorf("bytes added = %d, want 0", finalResp.BytesAdded)
	}

	// Two snapshots should exist.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapIDs) != 2 {
		t.Errorf("snapshot count = %d, want 2", len(snapIDs))
	}
}

func TestCrossAgentDeduplication(t *testing.T) {
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

	sharedContent := []byte("shared content between agents")

	// Agent 1 backs up.
	simulateAgentBackup(t, client, apiKey1, "host-1", map[string][]byte{
		"shared.txt": sharedContent,
	})

	// Agent 2 backs up the same content.
	ctx := context.Background()
	beginResp, err := client.BeginSession(ctx, &pb.BeginSessionRequest{
		ApiKey:   apiKey2,
		Hostname: "host-2",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var idKey [32]byte
	copy(idKey[:], beginResp.IdKey)
	blobID := crypto.BlobID(idKey, sharedContent)

	checkResp, err := client.CheckBlobs(ctx, &pb.CheckBlobsRequest{
		SessionId: beginResp.SessionId,
		BlobIds:   [][]byte{blobID[:]},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cross-agent dedup: server already has this blob.
	if len(checkResp.NeededIds) != 0 {
		t.Errorf("needed IDs = %d, want 0 (cross-agent dedup)", len(checkResp.NeededIds))
	}
}

func TestConcurrentSessions(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey1, err := registry.AddAgent("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	apiKey2, err := registry.AddAgent("agent-2")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				errs[0] = fmt.Errorf("panic: %v", r)
			}
		}()
		simulateAgentBackup(t, client, apiKey1, "host-1", map[string][]byte{
			"agent1-file.txt": []byte("data from agent 1"),
		})
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				errs[1] = fmt.Errorf("panic: %v", r)
			}
		}()
		simulateAgentBackup(t, client, apiKey2, "host-2", map[string][]byte{
			"agent2-file.txt": []byte("data from agent 2"),
		})
	}()

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("agent %d error: %v", i+1, err)
		}
	}

	// Both snapshots should exist.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapIDs) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snapIDs))
	}

	// Verify each snapshot has distinct hostname.
	hostnames := make(map[string]bool)
	for _, id := range snapIDs {
		snap, err := repo.LoadSnapshot(id)
		if err != nil {
			t.Fatal(err)
		}
		hostnames[snap.Hostname] = true
	}
	if !hostnames["host-1"] || !hostnames["host-2"] {
		t.Errorf("expected both host-1 and host-2, got: %v", hostnames)
	}
}

// testSetupWithServer is like testSetup but also returns the *Server so that
// callers can test Server-level methods (Counters, ActiveSessionCount, etc.).
func testSetupWithServer(t *testing.T) (pb.BackupServiceClient, *Server, *repository.Repository, *AgentRegistry, func()) {
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
	registry, err := LoadAgentRegistry(encKey, dir)
	if err != nil {
		t.Fatal(err)
	}

	rm := testRepoManager(t, repo, be)
	srv := New(rm, registry, Options{Version: "test-1.0"})

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

	client := pb.NewBackupServiceClient(conn)

	cleanup := func() {
		conn.Close()
		srv.Stop()
		be.Close()
	}

	return client, srv, repo, registry, cleanup
}

func TestActiveSessionCountEmpty(t *testing.T) {
	_, srv, _, _, cleanup := testSetupWithServer(t)
	defer cleanup()

	if n := srv.ActiveSessionCount(); n != 0 {
		t.Errorf("ActiveSessionCount = %d, want 0", n)
	}
}

func TestCountersInitial(t *testing.T) {
	_, srv, _, _, cleanup := testSetupWithServer(t)
	defer cleanup()

	completed, failed, bytesRecv, chunksRecv := srv.Counters()
	if completed != 0 || failed != 0 || bytesRecv != 0 || chunksRecv != 0 {
		t.Errorf("initial counters = (%d,%d,%d,%d), want all zeros",
			completed, failed, bytesRecv, chunksRecv)
	}
}

func TestCountersAfterBackup(t *testing.T) {
	client, srv, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("counter-agent")
	if err != nil {
		t.Fatal(err)
	}

	simulateAgentBackup(t, client, apiKey, "counter-host", map[string][]byte{
		"file.txt": []byte("counter data"),
	})

	completed, failed, bytesRecv, chunksRecv := srv.Counters()
	if completed != 1 {
		t.Errorf("completed = %d, want 1", completed)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}
	if bytesRecv == 0 {
		t.Error("bytesRecv should be > 0")
	}
	if chunksRecv == 0 {
		t.Error("chunksRecv should be > 0")
	}
}

func TestSetBackupFailedHandler(t *testing.T) {
	_, srv, _, _, cleanup := testSetupWithServer(t)
	defer cleanup()

	var mu sync.Mutex
	var gotAgent, gotHost, gotReason string

	srv.SetBackupFailedHandler(func(agentName, hostname, reason string, details map[string]any) {
		mu.Lock()
		gotAgent = agentName
		gotHost = hostname
		gotReason = reason
		mu.Unlock()
	})

	// Call notifyBackupFailed directly.
	srv.notifyBackupFailed("test-agent", "test-host", "disk full", nil)

	// Wait briefly for the goroutine to execute.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if gotAgent != "test-agent" {
		t.Errorf("agent = %q, want %q", gotAgent, "test-agent")
	}
	if gotHost != "test-host" {
		t.Errorf("hostname = %q, want %q", gotHost, "test-host")
	}
	if gotReason != "disk full" {
		t.Errorf("reason = %q, want %q", gotReason, "disk full")
	}
}

func TestNotifyBackupFailedNilHandler(t *testing.T) {
	_, srv, _, _, cleanup := testSetupWithServer(t)
	defer cleanup()

	// Should not panic with nil handler.
	srv.notifyBackupFailed("agent", "host", "reason", nil)
}

// mockHook records pre/post-backup calls for testing.
type mockHook struct {
	preCalls  []hook.HookEvent
	postCalls []hook.HookEvent
	preErr    error
	mu        sync.Mutex
}

func (h *mockHook) PreBackup(_ context.Context, evt hook.HookEvent) error {
	h.mu.Lock()
	h.preCalls = append(h.preCalls, evt)
	h.mu.Unlock()
	return h.preErr
}

func (h *mockHook) PostBackup(_ context.Context, evt hook.HookEvent) error {
	h.mu.Lock()
	h.postCalls = append(h.postCalls, evt)
	h.mu.Unlock()
	return nil
}

func (h *mockHook) Close() error { return nil }

func TestSetHooksFireDuringBackup(t *testing.T) {
	client, srv, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()

	mh := &mockHook{}
	srv.SetHooks([]hook.Hook{mh})

	apiKey, err := registry.AddAgent("hook-agent")
	if err != nil {
		t.Fatal(err)
	}

	simulateAgentBackup(t, client, apiKey, "hook-host", map[string][]byte{
		"hook-file.txt": []byte("hook test data"),
	})

	mh.mu.Lock()
	defer mh.mu.Unlock()

	if len(mh.preCalls) != 1 {
		t.Fatalf("pre-backup hook called %d times, want 1", len(mh.preCalls))
	}
	if mh.preCalls[0].Agent != "hook-agent" {
		t.Errorf("pre-backup agent = %q, want %q", mh.preCalls[0].Agent, "hook-agent")
	}
	if len(mh.postCalls) != 1 {
		t.Fatalf("post-backup hook called %d times, want 1", len(mh.postCalls))
	}
	if mh.postCalls[0].SnapshotID == "" {
		t.Error("post-backup hook should have snapshot ID set")
	}
}

func TestPreBackupHookRejectsSession(t *testing.T) {
	client, srv, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()

	mh := &mockHook{preErr: errors.New("access denied")}
	srv.SetHooks([]hook.Hook{mh})

	apiKey, err := registry.AddAgent("rejected-agent")
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.BeginSession(context.Background(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "rejected-host",
		Paths:    []string{"/data"},
	})
	if err == nil {
		t.Fatal("expected error when pre-backup hook rejects")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status, got: %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("status code = %v, want FailedPrecondition", st.Code())
	}
}

func TestGetScheduleEnabled(t *testing.T) {
	client, _, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("schedule-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Create a backup job with schedule.
	now := time.Now()
	if err := registry.AddJob(BackupJob{
		ID:              "deadbeef01234567",
		Name:            "test-job",
		AgentName:       "schedule-agent",
		RepoName:        "default",
		Paths:           []string{"/tmp"},
		ScheduleEnabled: true,
		ScheduleHours:   6,
		LastBackup:      &now,
		CreatedAt:       now,
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := client.GetSchedule(context.Background(), &pb.GetScheduleRequest{
		ApiKey: apiKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check legacy fields (populated from first enabled job).
	if !resp.Enabled {
		t.Error("expected schedule enabled")
	}
	if resp.IntervalHours != 6 {
		t.Errorf("interval = %d, want 6", resp.IntervalHours)
	}
	if resp.NextRunUnix == 0 {
		t.Error("expected non-zero next_run_unix")
	}

	// Check new jobs field.
	if len(resp.Jobs) != 1 {
		t.Fatalf("jobs count = %d, want 1", len(resp.Jobs))
	}
	if resp.Jobs[0].JobId != "deadbeef01234567" {
		t.Errorf("job_id = %q, want deadbeef01234567", resp.Jobs[0].JobId)
	}
	if resp.Jobs[0].IntervalHours != 6 {
		t.Errorf("job interval = %d, want 6", resp.Jobs[0].IntervalHours)
	}
}

func TestGetScheduleDisabled(t *testing.T) {
	client, _, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("nosched-agent")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.GetSchedule(context.Background(), &pb.GetScheduleRequest{
		ApiKey: apiKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Enabled {
		t.Error("expected schedule disabled")
	}
	if resp.NextRunUnix != 0 {
		t.Errorf("next_run_unix = %d, want 0", resp.NextRunUnix)
	}
}

func TestGetScheduleInvalidKey(t *testing.T) {
	client, _, _, _, cleanup := testSetupWithServer(t)
	defer cleanup()

	_, err := client.GetSchedule(context.Background(), &pb.GetScheduleRequest{
		ApiKey: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want Unauthenticated", st.Code())
	}
}

func TestGenerateSelfSignedTLS(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(testRepoManager(t, repo, be), nil, Options{})
	cfg, err := srv.generateSelfSignedTLS()
	if err != nil {
		t.Fatalf("generateSelfSignedTLS: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("certificates count = %d, want 1", len(cfg.Certificates))
	}
}

func TestLoadTLSAutoGenerate(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}

	// No TLS cert/key paths set — should auto-generate.
	srv := New(testRepoManager(t, repo, be), nil, Options{})
	cfg, err := srv.loadTLS()
	if err != nil {
		t.Fatalf("loadTLS (auto-generate): %v", err)
	}
	if cfg == nil || len(cfg.Certificates) == 0 {
		t.Error("expected auto-generated TLS certificate")
	}
}

func TestLoadTLSWithCertFiles(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}

	// First generate certs so we have valid files.
	srv := New(testRepoManager(t, repo, be), nil, Options{})
	srv.generateSelfSignedTLS()

	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	// Now test loadTLS with explicit cert paths.
	srv2 := New(testRepoManager(t, repo, be), nil, Options{TLSCert: certPath, TLSKey: keyPath})
	cfg, err := srv2.loadTLS()
	if err != nil {
		t.Fatalf("loadTLS (with files): %v", err)
	}
	if cfg == nil || len(cfg.Certificates) == 0 {
		t.Error("expected loaded TLS certificate")
	}
}

func TestLoadTLSInvalidPaths(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(testRepoManager(t, repo, be), nil, Options{
		TLSCert: "/nonexistent/cert.pem",
		TLSKey:  "/nonexistent/key.pem",
	})
	_, err = srv.loadTLS()
	if err == nil {
		t.Error("expected error for invalid TLS cert/key paths")
	}
}

func TestRepoBasePath(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(testRepoManager(t, repo, be), nil, Options{})
	basePath := srv.repoBasePath()
	if basePath != dir {
		t.Errorf("repoBasePath() = %q, want %q", basePath, dir)
	}
}

func TestGeneratedCertIncludesHostIPs(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(testRepoManager(t, repo, be), nil, Options{})
	srv.generateSelfSignedTLS()

	// Read the persisted cert and parse it.
	certPEM, err := os.ReadFile(filepath.Join(dir, "server.crt"))
	if err != nil {
		t.Fatalf("read generated cert: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	// Must include 127.0.0.1.
	hasLoopback := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			hasLoopback = true
		}
	}
	if !hasLoopback {
		t.Error("certificate missing 127.0.0.1 in SANs")
	}

	// Must include localhost in DNS names.
	hasLocalhost := false
	for _, dns := range cert.DNSNames {
		if dns == "localhost" {
			hasLocalhost = true
		}
	}
	if !hasLocalhost {
		t.Error("certificate missing localhost in DNS SANs")
	}

	// Should have more than just loopback (at least one interface IP).
	if len(cert.IPAddresses) < 2 {
		t.Logf("warning: only %d IP(s) in cert SANs (may be OK in minimal environments)", len(cert.IPAddresses))
	}
}

func TestLoadTLSReusesExisting(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}

	// First call generates and saves the cert.
	srv := New(testRepoManager(t, repo, be), nil, Options{})
	srv.generateSelfSignedTLS()

	certPath := filepath.Join(dir, "server.crt")
	original, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}

	// Second call to loadTLS should reuse the existing cert (not regenerate).
	srv2 := New(testRepoManager(t, repo, be), nil, Options{})
	cfg, err := srv2.loadTLS()
	if err != nil {
		t.Fatalf("loadTLS (reuse): %v", err)
	}
	if cfg == nil || len(cfg.Certificates) == 0 {
		t.Fatal("expected loaded TLS certificate")
	}

	// Cert file should not have changed.
	after, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert after loadTLS: %v", err)
	}
	if string(original) != string(after) {
		t.Error("loadTLS regenerated cert instead of reusing existing one")
	}
}

func TestCheckBlobsInvalidLength(t *testing.T) {
	client, _, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("checkblob-agent")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	beginResp, err := client.BeginSession(ctx, &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "checkblob-host",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Send blob IDs with invalid length (not 32 bytes) — should be skipped.
	resp, err := client.CheckBlobs(ctx, &pb.CheckBlobsRequest{
		SessionId: beginResp.SessionId,
		BlobIds:   [][]byte{{1, 2, 3}}, // too short
	})
	if err != nil {
		t.Fatalf("CheckBlobs: %v", err)
	}
	// Invalid-length IDs are skipped, so nothing returned as needed.
	if len(resp.NeededIds) != 0 {
		t.Errorf("NeededIds = %d, want 0 for invalid-length blob ID", len(resp.NeededIds))
	}
}

func TestCheckBlobsInvalidSession(t *testing.T) {
	client, _, _, _, cleanup := testSetupWithServer(t)
	defer cleanup()

	_, err := client.CheckBlobs(context.Background(), &pb.CheckBlobsRequest{
		SessionId: "nonexistent-session-id",
		BlobIds:   [][]byte{{1, 2, 3}},
	})
	if err == nil {
		t.Fatal("expected error for invalid session ID")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("status code = %v, want NotFound", st.Code())
	}
}

func TestFinalizeInvalidSession(t *testing.T) {
	client, _, _, _, cleanup := testSetupWithServer(t)
	defer cleanup()

	_, err := client.FinalizeBackup(context.Background(), &pb.FinalizeBackupRequest{
		SessionId: "nonexistent-session-id",
	})
	if err == nil {
		t.Fatal("expected error for invalid session ID")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("status code = %v, want NotFound", st.Code())
	}
}

func TestBeginSessionRoutesToAgentRepo(t *testing.T) {
	// Create two repos.
	dir1 := filepath.Join(t.TempDir(), "repo1")
	dir2 := filepath.Join(t.TempDir(), "repo2")
	os.MkdirAll(dir1, 0755)
	os.MkdirAll(dir2, 0755)

	be1, _ := local.New(dir1)
	be2, _ := local.New(dir2)
	repo1, _ := repository.Init(be1, "pw1")
	repo2, _ := repository.Init(be2, "pw2")

	rm := NewRepoManager()
	rm.AddDirect("repo1", repo1, be1)
	rm.AddDirect("repo2", repo2, be2)

	registry, _ := LoadAgentRegistry(repo1.Key().EncryptKey, dir1)
	apiKey, _ := registry.AddAgent("agent-a")

	// Assign agent to repo2.
	registry.UpdateRepoName("agent-a", "repo2")

	srv := New(rm, registry, Options{Version: "test"})
	lis := bufconn.Listen(bufSize)
	go srv.StartWithListener(lis)

	conn, _ := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	defer func() {
		conn.Close()
		srv.Stop()
		rm.CloseAll()
	}()

	client := pb.NewBackupServiceClient(conn)
	resp, err := client.BeginSession(context.Background(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "test-host",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatalf("BeginSession: %v", err)
	}
	if resp.SessionId == "" {
		t.Error("expected non-empty session ID")
	}

	// Verify session got repo2's chunker params (different repo ID).
	repo2Cfg := repo2.Cfg()
	if resp.ChunkerMin != uint32(repo2Cfg.ChunkerParams.MinSize) {
		t.Errorf("chunker_min = %d, want %d", resp.ChunkerMin, repo2Cfg.ChunkerParams.MinSize)
	}
}

func TestBeginSessionBlockedByMaintenanceLock(t *testing.T) {
	dir := t.TempDir()
	be, _ := local.New(dir)
	repo, _ := repository.Init(be, "test-pw")

	rm := NewRepoManager()
	rm.AddDirect("default", repo, be)

	mr, _ := rm.Get("default")
	mr.MaintenanceLock.Acquire("check")

	registry, _ := LoadAgentRegistry(repo.Key().EncryptKey, dir)
	apiKey, _ := registry.AddAgent("test-agent")

	srv := New(rm, registry, Options{Version: "test"})
	lis := bufconn.Listen(bufSize)
	go srv.StartWithListener(lis)

	conn, _ := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	defer func() {
		conn.Close()
		srv.Stop()
		rm.CloseAll()
	}()

	client := pb.NewBackupServiceClient(conn)
	_, err := client.BeginSession(context.Background(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "test-host",
		Paths:    []string{"/data"},
	})
	if err == nil {
		t.Fatal("expected error when maintenance lock held")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status, got: %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("status code = %v, want Unavailable", st.Code())
	}
}

func TestSessionByJobID(t *testing.T) {
	client, srv, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("job-agent")
	if err != nil {
		t.Fatal(err)
	}

	// No sessions yet.
	if s := srv.SessionByJobID("job-123"); s != nil {
		t.Error("expected nil for non-existent job")
	}

	// Create a job and begin session with it.
	registry.AddJob(BackupJob{
		ID:        "job-123",
		Name:      "test-job",
		AgentName: "job-agent",
		RepoName:  "Reository1",
		Paths:     []string{"/data"},
	})

	resp, err := client.BeginSession(t.Context(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "test-host",
		Paths:    []string{"/data"},
		JobId:    "job-123",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should find the session now.
	s := srv.SessionByJobID("job-123")
	if s == nil {
		t.Fatal("expected session for job-123")
	}
	if s.ID != resp.SessionId {
		t.Errorf("session ID = %q, want %q", s.ID, resp.SessionId)
	}

	// Different job ID should not find it.
	if s := srv.SessionByJobID("job-999"); s != nil {
		t.Error("expected nil for wrong job ID")
	}
}

func TestCancelSessionByJobID(t *testing.T) {
	client, srv, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("cancel-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Cancel non-existent job returns false.
	if srv.CancelSessionByJobID("no-such-job") {
		t.Error("expected false for non-existent job")
	}

	registry.AddJob(BackupJob{
		ID:        "cancel-job",
		Name:      "cancel-test",
		AgentName: "cancel-agent",
		RepoName:  "Reository1",
		Paths:     []string{"/tmp"},
	})

	_, err = client.BeginSession(t.Context(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "cancel-host",
		Paths:    []string{"/tmp"},
		JobId:    "cancel-job",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Session should exist.
	if srv.SessionByJobID("cancel-job") == nil {
		t.Fatal("expected session before cancel")
	}

	// Cancel should succeed.
	if !srv.CancelSessionByJobID("cancel-job") {
		t.Error("expected true for existing session")
	}

	// Session should be gone.
	if srv.SessionByJobID("cancel-job") != nil {
		t.Error("expected nil after cancel")
	}

	// Cancel again should return false (idempotent).
	if srv.CancelSessionByJobID("cancel-job") {
		t.Error("expected false for already-cancelled session")
	}
}

func TestServerSentinelThreatDetection(t *testing.T) {
	os.Setenv("HITSTOR_TEST", "true")
	defer os.Unsetenv("HITSTOR_TEST")

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
	registry, err := LoadAgentRegistry(encKey, dir)
	if err != nil {
		t.Fatal(err)
	}

	apiKey, err := registry.AddAgent("threat-agent")
	if err != nil {
		t.Fatal(err)
	}

	rm := testRepoManager(t, repo, be)

	// Create server with SentinelConfig enabled.
	srv := New(rm, registry, Options{
		Version: "test-1.0",
		SentinelConfig: sentinel.Config{
			Enabled:             true,
			ConfidenceThreshold: 0.8,
			RetentionDays:       30,
		},
	})

	// Setup threat alert listener. The handler runs on a server goroutine,
	// so the flag and reason need synchronized access.
	var alertFired atomic.Bool
	var threatReason atomic.Value
	srv.SetThreatDetectedHandler(func(agentName, hostname, reason string, confidence float64, details map[string]any) {
		threatReason.Store(reason)
		alertFired.Store(true)
	})

	lis := bufconn.Listen(bufSize)
	go func() {
		srv.StartWithListener(lis)
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
	defer func() {
		conn.Close()
		srv.Stop()
		rm.CloseAll()
	}()

	client := pb.NewBackupServiceClient(conn)

	// 1. Begin Session.
	beginResp, err := client.BeginSession(context.Background(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "threat-host",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatalf("BeginSession: %v", err)
	}

	// 2. Upload Chunks to make them "new".
	stream, err := client.UploadChunks(context.Background())
	if err != nil {
		t.Fatalf("UploadChunks: %v", err)
	}

	var contentIDs [][]byte
	for i := 0; i < 11; i++ {
		var bid [32]byte
		bid[0] = byte(i + 1)
		contentIDs = append(contentIDs, bid[:])

		if err := stream.Send(&pb.UploadChunkMessage{
			SessionId:          beginResp.SessionId,
			BlobId:             bid[:],
			CompressedData:     []byte("some encrypted ransomware data"),
			UncompressedLength: 30,
		}); err != nil {
			t.Fatalf("stream.Send: %v", err)
		}
	}
	_, err = stream.CloseAndRecv()
	if err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}

	// 3. Finalize Session with Ransomware-like telemetry.
	var files []*pb.FileNode
	for i := 0; i < 11; i++ {
		files = append(files, &pb.FileNode{
			Path:       fmt.Sprintf("/data/file%d.txt", i+1),
			Size:       30,
			ContentIds: [][]byte{contentIDs[i]},
		})
	}

	_, err = client.FinalizeBackup(context.Background(), &pb.FinalizeBackupRequest{
		SessionId: beginResp.SessionId,
		Files:     files,
	})
	if err != nil {
		t.Fatalf("FinalizeBackup: %v", err)
	}

	// 3. The Sentinel run check is asynchronous. Wait up to 1 second.
	deadline := time.Now().Add(1 * time.Second)
	var agentRecord *AgentRecord
	for time.Now().Before(deadline) {
		agentRecord = registry.GetAgent("threat-agent")
		if agentRecord != nil && agentRecord.Suspended && alertFired.Load() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if agentRecord == nil {
		t.Fatal("expected threat-agent record to exist")
	}
	if !agentRecord.Suspended {
		t.Error("expected threat-agent to be suspended by Sentinel, but it was not")
	}
	if !alertFired.Load() {
		t.Error("expected threat detection alert callback to be fired, but it was not")
	}
	if reason, _ := threatReason.Load().(string); reason == "" {
		t.Error("expected threat reason to be populated")
	}

	// 4. Verify that subsequent requests from the agent are rejected (since suspended).
	_, err = client.BeginSession(context.Background(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "threat-host",
		Paths:    []string{"/data"},
	})
	if err == nil {
		t.Fatal("expected subsequent BeginSession from suspended agent to be rejected, but it succeeded")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated status for suspended agent, got %v", st.Code())
	}
}

func plantStaleRepositoryLock(t *testing.T, be backend.Backend) {
	t.Helper()
	ctx := context.Background()
	stale := repository.Lock{
		Time:      time.Now().Add(-48 * time.Hour),
		Hostname:  "crashed-host",
		PID:       999999,
		Exclusive: true,
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	lockID := repository.ID(sha256.Sum256(data))
	if err := be.Save(ctx, backend.LockFile, lockID, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}

func TestBeginSessionBreaksStaleRepositoryLock(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	plantStaleRepositoryLock(t, be)

	registry, err := LoadAgentRegistry(repo.Key().EncryptKey, dir)
	if err != nil {
		t.Fatal(err)
	}
	apiKey, err := registry.AddAgent("stale-lock-agent")
	if err != nil {
		t.Fatal(err)
	}

	rm := testRepoManager(t, repo, be)
	srv := New(rm, registry, Options{Version: "test-1.0"})
	lis := bufconn.Listen(bufSize)
	go func() {
		_ = srv.StartWithListener(lis)
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
	defer func() {
		conn.Close()
		srv.Stop()
		be.Close()
	}()

	client := pb.NewBackupServiceClient(conn)
	resp, err := client.BeginSession(context.Background(), &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "stale-lock-host",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatalf("BeginSession: %v", err)
	}
	if resp.SessionId == "" {
		t.Fatal("expected non-empty session ID")
	}
}

func TestFinalizeBackupUpdatesAgentLastBackupForJob(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("job-backup-agent")
	if err != nil {
		t.Fatal(err)
	}

	oldBackup := time.Now().Add(-30 * 24 * time.Hour)
	if err := registry.UpdateLastBackup("job-backup-agent", oldBackup); err != nil {
		t.Fatal(err)
	}

	if err := registry.AddJob(BackupJob{
		ID:        "job-backup-1",
		Name:      "job backup test",
		AgentName: "job-backup-agent",
		RepoName:  "default",
		Paths:     []string{"/data"},
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	beginResp, err := client.BeginSession(ctx, &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: "job-backup-host",
		Paths:    []string{"/data"},
		JobId:    "job-backup-1",
	})
	if err != nil {
		t.Fatalf("BeginSession: %v", err)
	}

	_, err = client.FinalizeBackup(ctx, &pb.FinalizeBackupRequest{
		SessionId: beginResp.SessionId,
		Files:     []*pb.FileNode{},
	})
	if err != nil {
		t.Fatalf("FinalizeBackup: %v", err)
	}

	agent := registry.GetAgent("job-backup-agent")
	if agent == nil || agent.LastBackup == nil {
		t.Fatal("expected agent last backup to be set")
	}
	if !agent.LastBackup.After(oldBackup) {
		t.Fatalf("agent LastBackup = %v, want after %v", agent.LastBackup, oldBackup)
	}

	job := registry.GetJob("job-backup-1")
	if job == nil || job.LastBackup == nil {
		t.Fatal("expected job last backup to be set")
	}
	if !job.LastBackup.After(oldBackup) {
		t.Fatalf("job LastBackup = %v, want after %v", job.LastBackup, oldBackup)
	}
}
