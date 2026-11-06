package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hooperits/hitstor/internal/plugin"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

// recordingWorkload is an in-process WorkloadPlugin.
type recordingWorkload struct {
	settings map[string]string
	plan     plugin.PlanRestoreRequest
	req      plugin.RestoreRequest
	restored map[string][]byte
	released bool
}

func (w *recordingWorkload) Browse(_ context.Context, p string) ([]plugin.BrowseEntry, error) {
	return []plugin.BrowseEntry{{Path: p + "/db", Name: "db", Kind: "database", Selectable: true}}, nil
}

func (w *recordingWorkload) PlanRestore(_ context.Context, req plugin.PlanRestoreRequest) ([]string, error) {
	w.plan = req
	refs := make([]string, 0, len(req.Candidates))
	for _, c := range req.Candidates {
		refs = append(refs, c.Ref)
	}
	return refs, nil
}

func (w *recordingWorkload) Restore(_ context.Context, req plugin.RestoreRequest, items plugin.RestoreItems) (string, error) {
	w.req = req
	w.restored = map[string][]byte{}
	for {
		it, err := items.Next()
		if err == io.EOF {
			return "ok", nil
		}
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(it.Data)
		if err != nil {
			return "", err
		}
		w.restored[it.Ref] = data
	}
}

func startWorkloadAgent(t *testing.T, w *recordingWorkload) pb.AgentServiceClient {
	t.Helper()
	srv := NewAgentServer(AgentServerOptions{Token: testAgentToken, Version: "test"})
	srv.OpenWorkload = func(id string, settings map[string]string) (WorkloadPlugin, func(), error) {
		if id != "fakedb" {
			return nil, nil, errors.New("plugin is not installed")
		}
		w.settings = settings
		return w, func() { w.released = true }, nil
	}
	return startBMRAgent(t, srv)
}

func TestWorkloadRPCsNeedOpenerAndToken(t *testing.T) {
	client := startBMRAgent(t, NewAgentServer(AgentServerOptions{Token: testAgentToken}))
	_, err := client.BrowseWorkload(context.Background(), &pb.BrowseWorkloadRequest{Token: testAgentToken, Plugin: "fakedb"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("no opener: %v", err)
	}
	client = startWorkloadAgent(t, &recordingWorkload{})
	_, err = client.BrowseWorkload(context.Background(), &pb.BrowseWorkloadRequest{Token: "wrong", Plugin: "fakedb"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bad token: %v", err)
	}
	_, err = client.BrowseWorkload(context.Background(), &pb.BrowseWorkloadRequest{Token: testAgentToken, Plugin: "missing"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("missing plugin: %v", err)
	}
}

func TestBrowseAndPlanWorkload(t *testing.T) {
	w := &recordingWorkload{}
	client := startWorkloadAgent(t, w)
	br, err := client.BrowseWorkload(context.Background(), &pb.BrowseWorkloadRequest{
		Token: testAgentToken, Plugin: "fakedb", Path: "inst", Settings: map[string]string{"instance": "X"},
	})
	if err != nil || len(br.Entries) != 1 || br.Entries[0].Path != "inst/db" || !br.Entries[0].Selectable {
		t.Fatalf("browse = %v, %v", br, err)
	}
	if w.settings["instance"] != "X" || !w.released {
		t.Fatalf("settings %v released %v", w.settings, w.released)
	}

	plan, err := client.PlanWorkloadRestore(context.Background(), &pb.PlanWorkloadRestoreRequest{
		Token: testAgentToken, Plugin: "fakedb", Object: "inst/db", TargetUnixNanos: 1_700_000_000_000_000_000,
		Candidates: []*pb.WorkloadCandidate{{Ref: "a", Item: "inst/db/full.bak", Kind: "full", CreatedUnixNanos: 1_600_000_000_000_000_000}},
	})
	if err != nil || len(plan.Refs) != 1 || plan.Refs[0] != "a" {
		t.Fatalf("plan = %v, %v", plan, err)
	}
	if w.plan.Object != "inst/db" || w.plan.Target.UnixNano() != 1_700_000_000_000_000_000 || w.plan.Candidates[0].Kind != "full" {
		t.Fatalf("plugin got %+v", w.plan)
	}
}

func sendRestore(t *testing.T, client pb.AgentServiceClient, items map[string][]byte, order []string, corrupt string) (*pb.WorkloadRestoreResponse, error) {
	t.Helper()
	stream, err := client.RestoreWorkload(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&pb.WorkloadRestoreMessage{Payload: &pb.WorkloadRestoreMessage_Begin{Begin: &pb.WorkloadRestoreBegin{
		Token: testAgentToken, Plugin: "fakedb", Object: "inst/db", ItemCount: int32(len(order)),
		Options: map[string]string{"new_name": "db_copy"},
	}}})
	for _, ref := range order {
		data := items[ref]
		sum := sha256.Sum256(data)
		stream.Send(&pb.WorkloadRestoreMessage{Payload: &pb.WorkloadRestoreMessage_Start{Start: &pb.WorkloadItemStart{
			Ref: ref, Item: "inst/db/" + ref, Size: int64(len(data)), Sha256: hex.EncodeToString(sum[:]),
		}}})
		if ref == corrupt {
			data = append([]byte("X"), data[1:]...)
		}
		for len(data) > 0 {
			n := min(len(data), 1<<20)
			if err := stream.Send(&pb.WorkloadRestoreMessage{Payload: &pb.WorkloadRestoreMessage_Data{Data: data[:n]}}); err != nil {
				break
			}
			data = data[n:]
		}
		stream.Send(&pb.WorkloadRestoreMessage{Payload: &pb.WorkloadRestoreMessage_End{End: &pb.WorkloadItemEnd{}}})
	}
	return stream.CloseAndRecv()
}

func TestRestoreWorkloadPipesVerifiedItems(t *testing.T) {
	w := &recordingWorkload{}
	client := startWorkloadAgent(t, w)
	items := map[string][]byte{"full.bak": bytes.Repeat([]byte("f"), 3<<20), "log.trn": []byte("log")}
	resp, err := sendRestore(t, client, items, []string{"full.bak", "log.trn"}, "")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if resp.Message != "ok" || !bytes.Equal(w.restored["full.bak"], items["full.bak"]) || string(w.restored["log.trn"]) != "log" {
		t.Fatalf("resp %v, restored %d items", resp, len(w.restored))
	}
	if w.req.Options["new_name"] != "db_copy" || w.req.ItemCount != 2 {
		t.Fatalf("req = %+v", w.req)
	}
}

func TestRestoreWorkloadRejectsCorruptItem(t *testing.T) {
	client := startWorkloadAgent(t, &recordingWorkload{})
	items := map[string][]byte{"full.bak": bytes.Repeat([]byte("f"), 1<<20)}
	_, err := sendRestore(t, client, items, []string{"full.bak"}, "full.bak")
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("err = %v", err)
	}
}
