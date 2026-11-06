package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
)

// fakeWorkload emits the items in items; misbehave selects protocol abuse.
type fakeWorkload struct {
	items     map[string][]byte // path -> data, emitted in order of order
	order     []string
	misbehave string
	gotReq    BackupRequest
}

func (f *fakeWorkload) Metadata(context.Context) (WorkloadMetadata, error) {
	return WorkloadMetadata{ID: "fake", Version: "1.0.0", ProtocolVersion: 1}, nil
}

func (f *fakeWorkload) Browse(_ context.Context, p string) ([]BrowseEntry, error) {
	if p == "" {
		return []BrowseEntry{{Path: "inst", Name: "inst", Kind: "instance", HasChildren: true}}, nil
	}
	return []BrowseEntry{{Path: p + "/db", Name: "db", Kind: "database", Selectable: true}}, nil
}

func (f *fakeWorkload) Backup(_ context.Context, req BackupRequest, sink ItemSink) error {
	f.gotReq = req
	switch f.misbehave {
	case "unfinished":
		_, err := sink.Item("a.bak")
		return err
	case "overlap":
		if _, err := sink.Item("a.bak"); err != nil {
			return err
		}
		_, err := sink.Item("b.bak")
		return err
	case "fail":
		w, _ := sink.Item("a.bak")
		w.Write([]byte("partial"))
		return errors.New("disk on fire")
	case "badmeta":
		w, _ := sink.Item("a.bak")
		return w.Finish([]byte("{not json"))
	case "escape":
		_, err := sink.Item("../evil")
		return err
	case "partial":
		// a finishes, b is aborted half way, c is excluded, then d fails hard.
		w, _ := sink.Item("a.bak")
		w.Write([]byte("aaaa"))
		w.Finish([]byte(`{"db":"a"}`))
		w, _ = sink.Item("b.bak")
		w.Write([]byte("partial"))
		if err := w.Abort(); err != nil {
			return err
		}
		if err := sink.Result(ObjectResult{Object: "inst/b", Status: ObjectFailed, Reason: "error", Message: "disk full"}); err != nil {
			return err
		}
		if err := sink.Result(ObjectResult{Object: "inst/c", Status: ObjectExcluded, Reason: "simple-recovery"}); err != nil {
			return err
		}
		return errors.New("instance went away")
	}
	for _, p := range f.order {
		w, err := sink.Item(p)
		if err != nil {
			return err
		}
		if _, err := w.Write(f.items[p]); err != nil {
			return err
		}
		if err := w.Finish([]byte(fmt.Sprintf(`{"size":%d}`, len(f.items[p])))); err != nil {
			return err
		}
	}
	return nil
}

// metaOnly implements Workload but none of the optional interfaces.
type metaOnly struct{}

func (metaOnly) Metadata(context.Context) (WorkloadMetadata, error) { return WorkloadMetadata{}, nil }

func setupWorkloadBufconn(t *testing.T, impl Workload) *WorkloadGRPCClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterWorkloadServer(srv, &workloadGRPCServer{impl: impl})
	go srv.Serve(lis)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Close()
		srv.Stop()
	})
	return &WorkloadGRPCClient{client: pb.NewWorkloadClient(conn)}
}

func TestWorkloadBackupStreamsItems(t *testing.T) {
	big := bytes.Repeat([]byte("0123456789abcdef"), 300_000) // 4.8 MB: several data messages
	impl := &fakeWorkload{
		items: map[string][]byte{"inst/db/full.bak": big, "inst/db/empty.bak": nil},
		order: []string{"inst/db/full.bak", "inst/db/empty.bak"},
	}
	c := setupWorkloadBufconn(t, impl)

	got := map[string][]byte{}
	out, err := c.Backup(context.Background(), BackupRequest{Selection: []string{"inst/db"}, Kind: "full", Options: map[string]string{"k": "v"}},
		func(p string, r io.Reader) error {
			data, err := io.ReadAll(r)
			got[p] = data
			return err
		})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if impl.gotReq.Kind != "full" || impl.gotReq.Selection[0] != "inst/db" || impl.gotReq.Options["k"] != "v" {
		t.Fatalf("plugin got %+v", impl.gotReq)
	}
	items := out.Items
	if len(items) != 2 || items[0].Path != "inst/db/full.bak" || items[0].Size != int64(len(big)) {
		t.Fatalf("items = %+v", items)
	}
	if string(items[0].Metadata) != fmt.Sprintf(`{"size":%d}`, len(big)) {
		t.Fatalf("metadata = %s", items[0].Metadata)
	}
	if !bytes.Equal(got["inst/db/full.bak"], big) || len(got["inst/db/empty.bak"]) != 0 {
		t.Fatal("item data mismatch")
	}
}

func TestWorkloadBackupHandlerMayStopEarly(t *testing.T) {
	impl := &fakeWorkload{items: map[string][]byte{"a.bak": []byte("aaaa"), "b.bak": []byte("bb")}, order: []string{"a.bak", "b.bak"}}
	c := setupWorkloadBufconn(t, impl)
	out, err := c.Backup(context.Background(), BackupRequest{}, func(string, io.Reader) error { return nil })
	items := out.Items
	if err != nil {
		t.Fatal(err)
	}
	// Unread data is drained, so sizes and the next item stay correct.
	if len(items) != 2 || items[0].Size != 4 || items[1].Size != 2 {
		t.Fatalf("items = %+v", items)
	}
}

func TestWorkloadBackupErrors(t *testing.T) {
	cases := map[string]string{
		"unfinished": "not finished",
		"overlap":    "before",
		"fail":       "disk on fire",
		"badmeta":    "not valid JSON",
		"escape":     "not clean",
	}
	for mode, want := range cases {
		t.Run(mode, func(t *testing.T) {
			c := setupWorkloadBufconn(t, &fakeWorkload{misbehave: mode})
			_, err := c.Backup(context.Background(), BackupRequest{}, func(_ string, r io.Reader) error {
				_, err := io.ReadAll(r)
				return err
			})
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestWorkloadBackupHandlerErrorAborts(t *testing.T) {
	impl := &fakeWorkload{items: map[string][]byte{"a.bak": []byte("x")}, order: []string{"a.bak"}}
	c := setupWorkloadBufconn(t, impl)
	_, err := c.Backup(context.Background(), BackupRequest{}, func(string, io.Reader) error { return errors.New("upload failed") })
	if err == nil || !strings.Contains(err.Error(), "upload failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkloadBackupObjectResults(t *testing.T) {
	impl := &fakeWorkload{misbehave: "partial"}
	c := setupWorkloadBufconn(t, impl)
	known := []KnownItem{{Item: "inst/a/full.bak", Metadata: json.RawMessage(`{"type":"D"}`)}}
	var stored []string
	out, err := c.Backup(context.Background(), BackupRequest{Selection: []string{"inst"}, Kind: "log", Known: known, ObjectResults: true},
		func(p string, r io.Reader) error {
			data, err := io.ReadAll(r)
			if err != nil {
				return err // an aborted item is not kept
			}
			stored = append(stored, p+"="+string(data))
			return nil
		})
	// The hard failure comes back, together with what finished before it.
	if err == nil || !strings.Contains(err.Error(), "instance went away") {
		t.Fatalf("err = %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Path != "a.bak" || strings.Join(stored, ",") != "a.bak=aaaa" {
		t.Fatalf("items = %+v, stored = %v", out.Items, stored)
	}
	if len(out.Results) != 2 || out.Results[0].Status != ObjectFailed || out.Results[1].Reason != "simple-recovery" {
		t.Fatalf("results = %+v", out.Results)
	}
	if len(impl.gotReq.Known) != 1 || string(impl.gotReq.Known[0].Metadata) != `{"type":"D"}` || !impl.gotReq.ObjectResults {
		t.Fatalf("request = %+v", impl.gotReq)
	}
}

func TestWorkloadBackupResultsNeedHostSupport(t *testing.T) {
	// A plugin may not report results or abort items to a host that did
	// not ask for them: older hosts would not understand.
	c := setupWorkloadBufconn(t, &fakeWorkload{misbehave: "partial"})
	_, err := c.Backup(context.Background(), BackupRequest{}, func(_ string, r io.Reader) error { _, err := io.ReadAll(r); return err })
	if err == nil || !strings.Contains(err.Error(), "does not accept aborted items") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkloadBrowse(t *testing.T) {
	c := setupWorkloadBufconn(t, &fakeWorkload{})
	top, err := c.Browse(context.Background(), "")
	if err != nil || len(top) != 1 || top[0].Kind != "instance" || !top[0].HasChildren {
		t.Fatalf("top = %+v, %v", top, err)
	}
	dbs, err := c.Browse(context.Background(), "inst")
	if err != nil || len(dbs) != 1 || dbs[0].Path != "inst/db" || !dbs[0].Selectable {
		t.Fatalf("dbs = %+v, %v", dbs, err)
	}
}

func TestWorkloadOptionalRPCsUnsupported(t *testing.T) {
	c := setupWorkloadBufconn(t, metaOnly{})
	if _, err := c.Browse(context.Background(), ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Browse err = %v", err)
	}
	if _, err := c.Backup(context.Background(), BackupRequest{}, nil); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Backup err = %v", err)
	}
}

func TestValidItemPath(t *testing.T) {
	for _, ok := range []string{"a.bak", "inst/db/full-1.bak"} {
		if err := ValidItemPath(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/abs", "C:/x", `a\b`, "../x", "a/../../x", "./a", "a" + MetadataSuffix} {
		if err := ValidItemPath(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
