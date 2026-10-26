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
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/scanner"
)

// testScanner returns predefined results for test assertions.
type testScanner struct {
	mu       sync.Mutex
	results  []scanner.ScanResult
	settings map[string]string
	failScan bool
}

var _ scanner.Scanner = (*testScanner)(nil)

func (s *testScanner) Scan(paths []string, excludes []string) (<-chan scanner.ScanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failScan {
		return nil, fmt.Errorf("injected scan error")
	}

	ch := make(chan scanner.ScanResult)
	results := make([]scanner.ScanResult, len(s.results))
	copy(results, s.results)
	go func() {
		defer close(ch)
		for _, r := range results {
			ch <- r
		}
	}()
	return ch, nil
}

func (s *testScanner) Close() error { return nil }

func (s *testScanner) Configure(settings map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return nil
}

func setupScannerBufconn(t *testing.T, impl *testScanner) (scanner.Scanner, func()) {
	t.Helper()

	maxMsgSize := math.MaxInt32
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(grpc.MaxRecvMsgSize(maxMsgSize), grpc.MaxSendMsgSize(maxMsgSize))
	pb.RegisterFilesystemScannerServer(srv, &scannerGRPCServer{impl: impl})
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

	client := &FilesystemScannerGRPCClient{client: pb.NewFilesystemScannerClient(conn)}
	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
		lis.Close()
	}
	return client, cleanup
}

func TestScannerScanSuccess(t *testing.T) {
	impl := &testScanner{
		results: []scanner.ScanResult{
			{Path: "/data/file1.txt", Node: fs.Node{Name: "file1.txt", Type: fs.NodeTypeFile, Size: 100, Mode: 0644}},
			{Path: "/data/subdir", Node: fs.Node{Name: "subdir", Type: fs.NodeTypeDir, Mode: 0755}},
			{Path: "/data/subdir/file2.txt", Node: fs.Node{Name: "file2.txt", Type: fs.NodeTypeFile, Size: 200, Mode: 0644}},
		},
	}
	sc, cleanup := setupScannerBufconn(t, impl)
	defer cleanup()

	ch, err := sc.Scan([]string{"/data"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var results []scanner.ScanResult
	for r := range ch {
		results = append(results, r)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].Path != "/data/file1.txt" {
		t.Errorf("results[0].Path = %q, want /data/file1.txt", results[0].Path)
	}
	if results[0].Node.Name != "file1.txt" {
		t.Errorf("results[0].Node.Name = %q, want file1.txt", results[0].Node.Name)
	}
	if results[0].Node.Type != fs.NodeTypeFile {
		t.Errorf("results[0].Node.Type = %q, want file", results[0].Node.Type)
	}
	if results[0].Node.Size != 100 {
		t.Errorf("results[0].Node.Size = %d, want 100", results[0].Node.Size)
	}
	if results[1].Node.Type != fs.NodeTypeDir {
		t.Errorf("results[1].Node.Type = %q, want dir", results[1].Node.Type)
	}
}

func TestScannerScanWithAllFields(t *testing.T) {
	impl := &testScanner{
		results: []scanner.ScanResult{
			{Path: "/link", Node: fs.Node{
				Name:       "link",
				Type:       fs.NodeTypeSymlink,
				Mode:       0777,
				UID:        1000,
				GID:        1000,
				MtimeNs:    1700000000000000000,
				AtimeNs:    1700000001000000000,
				CtimeNs:    1700000002000000000,
				Size:       0,
				Inode:      12345,
				Links:      1,
				LinkTarget: "/target",
			}},
		},
	}
	sc, cleanup := setupScannerBufconn(t, impl)
	defer cleanup()

	ch, err := sc.Scan([]string{"/"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var results []scanner.ScanResult
	for r := range ch {
		results = append(results, r)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	n := results[0].Node
	if n.UID != 1000 {
		t.Errorf("UID = %d, want 1000", n.UID)
	}
	if n.GID != 1000 {
		t.Errorf("GID = %d, want 1000", n.GID)
	}
	if n.Inode != 12345 {
		t.Errorf("Inode = %d, want 12345", n.Inode)
	}
	if n.LinkTarget != "/target" {
		t.Errorf("LinkTarget = %q, want /target", n.LinkTarget)
	}
	if n.MtimeNs != 1700000000000000000 {
		t.Errorf("MtimeNs = %d, want 1700000000000000000", n.MtimeNs)
	}
}

func TestScannerScanError(t *testing.T) {
	impl := &testScanner{
		results: []scanner.ScanResult{
			{Path: "/data/good.txt", Node: fs.Node{Name: "good.txt", Type: fs.NodeTypeFile}},
			{Err: fmt.Errorf("permission denied: /data/secret")},
		},
	}
	sc, cleanup := setupScannerBufconn(t, impl)
	defer cleanup()

	ch, err := sc.Scan([]string{"/data"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var results []scanner.ScanResult
	for r := range ch {
		results = append(results, r)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("results[0] should not have error, got %v", results[0].Err)
	}
	if results[1].Err == nil {
		t.Fatal("results[1] should have error")
	}
	if results[1].Err.Error() != "permission denied: /data/secret" {
		t.Errorf("results[1].Err = %q, want permission denied message", results[1].Err)
	}
}

func TestScannerConfigureThenScan(t *testing.T) {
	impl := &testScanner{
		results: []scanner.ScanResult{
			{Path: "/x", Node: fs.Node{Name: "x", Type: fs.NodeTypeFile}},
		},
	}
	sc, cleanup := setupScannerBufconn(t, impl)
	defer cleanup()

	// Configure via the raw gRPC client.
	grpcClient := sc.(*FilesystemScannerGRPCClient)
	_, err := grpcClient.client.Configure(context.Background(), &pb.ConfigureRequest{
		Settings: map[string]string{"mode": "fast"},
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	impl.mu.Lock()
	if impl.settings["mode"] != "fast" {
		t.Errorf("settings[mode] = %q, want fast", impl.settings["mode"])
	}
	impl.mu.Unlock()

	ch, err := sc.Scan([]string{"/"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var count int
	for range ch {
		count++
	}
	if count != 1 {
		t.Errorf("got %d results, want 1", count)
	}
}

func TestScannerClose(t *testing.T) {
	impl := &testScanner{}
	sc, cleanup := setupScannerBufconn(t, impl)
	defer cleanup()

	if err := sc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
