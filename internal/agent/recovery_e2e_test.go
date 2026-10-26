package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// rescueSystem is a recovery-mode agent running over bufconn, standing in for
// a machine booted off recovery media.
type rescueSystem struct {
	recovery *Recovery
	client   pb.AgentServiceClient
}

// startRescueSystem wires a Recovery to an AgentServer the way the recover
// command does, and returns a client for the server's dial-back.
func startRescueSystem(t *testing.T, opts RecoveryOptions) *rescueSystem {
	t.Helper()

	rec, err := NewRecovery(opts)
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	srv := NewAgentServer(AgentServerOptions{Token: rec.AgentToken(), Version: "test"})
	srv.OnRestoreFinished = rec.RestoreFinished

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
		t.Fatalf("dial rescue system: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return &rescueSystem{recovery: rec, client: pb.NewAgentServiceClient(conn)}
}

// pushRestore plays the server's side: one directory and one file, into the
// rescue system's target.
func pushRestore(t *testing.T, rs *rescueSystem, token, target, name, body string) {
	t.Helper()

	stream, err := rs.client.RestoreData(context.Background())
	if err != nil {
		t.Fatalf("open restore stream: %v", err)
	}
	send := func(msg *pb.RestoreDataRequest) {
		t.Helper()
		if err := stream.Send(msg); err != nil {
			t.Fatalf("send restore message: %v", err)
		}
	}
	now := time.Now().UnixNano()

	send(&pb.RestoreDataRequest{Payload: &pb.RestoreDataRequest_Begin{Begin: &pb.RestoreBegin{
		Token:      token,
		TargetPath: target,
		SnapshotId: "deadbeef",
		FilesTotal: 1,
	}}})
	send(&pb.RestoreDataRequest{Payload: &pb.RestoreDataRequest_FileHeader{
		FileHeader: &pb.RestoreFileHeader{
			Path: name, Mode: 0o644, Size: uint64(len(body)),
			MtimeNs: now, AtimeNs: now,
		}}})
	send(&pb.RestoreDataRequest{Payload: &pb.RestoreDataRequest_FileChunk{
		FileChunk: &pb.RestoreFileChunk{Data: []byte(body)}}})
	send(&pb.RestoreDataRequest{Payload: &pb.RestoreDataRequest_End{End: &pb.RestoreEnd{}}})

	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("close restore stream: %v", err)
	}
}

func TestRecoveryModeAnnouncesThenExitsWhenTheRestoreLands(t *testing.T) {
	target := t.TempDir()
	announcer := &recordingAnnouncer{}

	rs := startRescueSystem(t, RecoveryOptions{
		AgentName:  "web01",
		Token:      "operator-recovery-token",
		TargetPath: target,
		ListenPort: 8422,
		Announcer:  announcer,
		Timeout:    10 * time.Second,
	})

	done := make(chan RestoreOutcome, 1)
	errCh := make(chan error, 1)
	go func() {
		outcome, err := rs.recovery.Run(context.Background())
		if err != nil {
			errCh <- err
			return
		}
		done <- outcome
	}()

	// The server learns where to dial back only from the announce; wait for
	// it before playing the server's part.
	deadline := time.Now().Add(2 * time.Second)
	for announcer.announcement().AgentToken == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	announced := announcer.announcement()
	if announced.AgentToken == "" {
		t.Fatal("the rescue system never announced itself")
	}

	// Everything from here is the ordinary restore push, unchanged.
	pushRestore(t, rs, announced.AgentToken, target, "etc/hostname", "web01\n")

	select {
	case err := <-errCh:
		t.Fatalf("recovery failed: %v", err)
	case outcome := <-done:
		if outcome.FilesRestored != 1 {
			t.Errorf("FilesRestored = %d, want 1", outcome.FilesRestored)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not exit after the restore completed")
	}

	// And the data actually landed under ReaR's mountpoint, not in the
	// rescue system's own ramdisk.
	got, err := os.ReadFile(filepath.Join(target, "etc", "hostname"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != "web01\n" {
		t.Errorf("restored content = %q, want %q", got, "web01\n")
	}
}

func TestRecoveryModeRejectsARestorePushedWithTheWrongToken(t *testing.T) {
	target := t.TempDir()
	rs := startRescueSystem(t, RecoveryOptions{
		AgentName:  "web01",
		Token:      "operator-recovery-token",
		TargetPath: target,
		ListenPort: 8422,
		Announcer:  &recordingAnnouncer{},
		Timeout:    time.Second,
	})

	stream, err := rs.client.RestoreData(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The rescue system's listener is reachable on whatever network it booted
	// onto; only the announced token may drive it.
	if err := stream.Send(&pb.RestoreDataRequest{
		Payload: &pb.RestoreDataRequest_Begin{Begin: &pb.RestoreBegin{
			Token: "not-the-announced-token", TargetPath: target,
		}},
	}); err != nil {
		t.Fatalf("send begin: %v", err)
	}
	if _, err := stream.CloseAndRecv(); err == nil {
		t.Fatal("a restore with the wrong token was accepted")
	}
}
