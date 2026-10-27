package dashboard

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/hooperits/hitstor/internal/archiver"
	"github.com/hooperits/hitstor/internal/plugin"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/server"
)

const wlAgentToken = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

// fakeDBPlugin plays a database plugin inside the test agent.
type fakeDBPlugin struct {
	plan     plugin.PlanRestoreRequest
	options  map[string]string
	restored map[string][]byte
}

func (f *fakeDBPlugin) Browse(context.Context, string) ([]plugin.BrowseEntry, error) {
	return []plugin.BrowseEntry{{Path: "demo/alpha", Name: "alpha", Selectable: true}}, nil
}

// PlanRestore restores the newest full at or before the target.
func (f *fakeDBPlugin) PlanRestore(_ context.Context, req plugin.PlanRestoreRequest) ([]string, error) {
	f.plan = req
	var best *plugin.RestoreCandidate
	for i, c := range req.Candidates {
		if (req.Target.IsZero() || !c.CreatedAt.After(req.Target)) && (best == nil || c.CreatedAt.After(best.CreatedAt)) {
			best = &req.Candidates[i]
		}
	}
	return []string{best.Ref}, nil
}

func (f *fakeDBPlugin) Restore(_ context.Context, req plugin.RestoreRequest, items plugin.RestoreItems) (string, error) {
	f.options = req.Options
	f.restored = map[string][]byte{}
	for {
		it, err := items.Next()
		if err == io.EOF {
			return "restored " + req.Object, nil
		}
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(it.Data)
		if err != nil {
			return "", err
		}
		f.restored[it.Item] = data
	}
}

// writeItem lays out an item and its manifest as an agent would store them.
func writeItem(t *testing.T, root, item string, data []byte, created time.Time) {
	t.Helper()
	dst := filepath.Join(root, filepath.FromSlash(item))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	m, _ := json.Marshal(plugin.ItemManifest{
		Plugin: "example", Item: item, Kind: "full", Size: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), CreatedAt: created, Metadata: json.RawMessage(`{"n":1}`),
	})
	if err := os.WriteFile(dst+plugin.MetadataSuffix, m, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWorkloadRestoreEndToEnd(t *testing.T) {
	dash, rm, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	mr, err := rm.Get("default")
	if err != nil {
		t.Fatal(err)
	}

	// Two backups of demo/alpha, an hour apart, as the agent stores them:
	// <plugin>/<item> plus the manifest, in snapshots tagged plugin:example.
	t0 := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
	older, newer := make([]byte, 2<<20), make([]byte, 3<<20)
	rand.Read(older)
	rand.Read(newer)
	for i, data := range [][]byte{older, newer} {
		src := filepath.Join(t.TempDir(), "example")
		item := "demo/alpha/full-" + string(rune('1'+i)) + ".dat"
		writeItem(t, src, item, data, t0.Add(time.Duration(i)*time.Hour))
		if _, _, err := archiver.New(mr.Repo).Backup([]string{src}, nil, "db-host", []string{"plugin:example", "db-agent"}); err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
	}

	// A managed agent running the fake plugin, reached over bufconn.
	if _, err := dash.registry.AddManagedAgent("db-agent", "bufnet:1", wlAgentToken); err != nil {
		t.Fatal(err)
	}
	dash.registry.SetAgentPlugins("db-agent", []server.AgentPlugin{{ID: "example", Status: "active"}})
	fake := &fakeDBPlugin{}
	agentSrv := agent.NewAgentServer(agent.AgentServerOptions{Token: wlAgentToken})
	agentSrv.OpenWorkload = func(string, map[string]string) (agent.WorkloadPlugin, func(), error) {
		return fake, func() {}, nil
	}
	lis := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	pb.RegisterAgentServiceServer(grpcSrv, agentSrv)
	go grpcSrv.Serve(lis)
	defer grpcSrv.Stop()
	dash.dialAgent = func(string, string) (*server.AgentClient, error) {
		return server.DialAgentWith("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	h := dash.Handler()
	do := func(method, url, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Restore points.
	rec := do("GET", "/api/workload/points?agent=db-agent&plugin=example&object=demo/alpha", "")
	var points []workloadPoint
	if err := json.NewDecoder(rec.Body).Decode(&points); err != nil || len(points) != 2 {
		t.Fatalf("points: %d %v (%s)", len(points), err, rec.Body)
	}
	if points[0].Item != "demo/alpha/full-1.dat" || !points[0].CreatedAt.Equal(t0) || points[0].Size != int64(len(older)) {
		t.Fatalf("points[0] = %+v", points[0])
	}

	// Browse through the agent.
	rec = do("GET", "/api/agents/db-agent/plugins/example/browse", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "demo/alpha") {
		t.Fatalf("browse: %d %s", rec.Code, rec.Body)
	}

	// Point-in-time restore to between the two backups picks the older one.
	target := t0.Add(30 * time.Minute).Format(time.RFC3339)
	rec = do("POST", "/api/workload/restores", `{"agent_name":"db-agent","plugin":"example","object":"demo/alpha",
		"target":"`+target+`","options":{"new_name":"alpha_copy"}}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	var sess workloadRestoreJSON
	json.NewDecoder(rec.Body).Decode(&sess)

	deadline := time.Now().Add(10 * time.Second)
	for sess.Status != "completed" && sess.Status != "failed" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		json.NewDecoder(do("GET", "/api/workload/restores/"+sess.ID, "").Body).Decode(&sess)
	}
	if sess.Status != "completed" {
		t.Fatalf("session = %+v", sess)
	}
	if sess.Message != "restored demo/alpha" || len(sess.Plan) != 1 || sess.Plan[0] != "demo/alpha/full-1.dat" ||
		sess.BytesSent != int64(len(older)) || sess.BytesTotal != int64(len(older)) {
		t.Fatalf("session = %+v", sess)
	}
	if !bytes.Equal(fake.restored["demo/alpha/full-1.dat"], older) || fake.options["new_name"] != "alpha_copy" {
		t.Fatalf("plugin restored %d items, options %v", len(fake.restored), fake.options)
	}
	if string(fake.plan.Candidates[0].Metadata) != `{"n":1}` {
		t.Fatalf("candidate metadata = %s", fake.plan.Candidates[0].Metadata)
	}
}

func TestWorkloadRestoreValidation(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	if _, err := dash.registry.AddManagedAgent("db-agent", "x:1", wlAgentToken); err != nil {
		t.Fatal(err)
	}
	h := dash.Handler()
	post := func(body string) int {
		req := httptest.NewRequest("POST", "/api/workload/restores", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := post(`{"agent_name":"db-agent","plugin":"example"}`); c != http.StatusBadRequest {
		t.Fatalf("missing object: %d", c)
	}
	if c := post(`{"agent_name":"db-agent","plugin":"example","object":"demo/alpha"}`); c != http.StatusBadRequest {
		t.Fatalf("plugin not reported: %d", c)
	}
	dash.registry.SetAgentPlugins("db-agent", []server.AgentPlugin{{ID: "example", Status: "active"}})
	if c := post(`{"agent_name":"db-agent","plugin":"example","object":"demo/alpha"}`); c != http.StatusNotFound {
		t.Fatalf("no backups: %d", c)
	}
}
