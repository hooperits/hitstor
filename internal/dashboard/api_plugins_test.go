package dashboard

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/hooperits/hitstor/internal/pluginmgr"
	"github.com/hooperits/hitstor/internal/pluginpkg"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/server"
)

func testPackage(t *testing.T, key ed25519.PrivateKey, version string) []byte {
	t.Helper()
	manifest := "id: example\nname: Example\nversion: " + version + "\ntype: workload\nprotocol_version: 1\nplatforms: [windows/amd64]\nentrypoint: bin/hitstor-plugin-example.exe\n"
	data, err := pluginpkg.Build([]byte(manifest), map[string][]byte{"bin/hitstor-plugin-example.exe": []byte("binary")}, key)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fakeInstaller stands in for the agent's plugin manager.
type fakeInstaller struct {
	got     []byte
	upgrade bool
	removed string
	fail    error
}

func (f *fakeInstaller) Install(_ context.Context, path string, o pluginmgr.InstallOptions) (*pluginmgr.InstalledPlugin, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	f.got, _ = os.ReadFile(path)
	f.upgrade = o.Upgrade
	pkg, err := pluginpkg.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &pluginmgr.InstalledPlugin{ID: pkg.Manifest.ID, Name: pkg.Manifest.Name, Version: pkg.Manifest.Version, Status: "active", Signer: pkg.Signer()}, nil
}

func (f *fakeInstaller) Remove(id string) error { f.removed = id; return nil }

func TestPluginPackagesAndRemoteInstall(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	dash.pluginPackages = t.TempDir()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	dash.cfg.PluginTrustedKeys = []string{pluginpkg.EncodePublicKey(pub)}
	h := dash.Handler()
	do := func(method, url string, body []byte) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, url, bytes.NewReader(body)))
		return rec
	}

	// Upload: garbage, an untrusted signer, then a trusted package.
	if rec := do("POST", "/api/plugin-packages", []byte("not a zip")); rec.Code != http.StatusBadRequest {
		t.Fatalf("garbage: %d", rec.Code)
	}
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	if rec := do("POST", "/api/plugin-packages", testPackage(t, stranger, "0.1.0")); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "untrusted") {
		t.Fatalf("untrusted: %d %s", rec.Code, rec.Body)
	}
	pkgData := testPackage(t, key, "0.1.0")
	rec := do("POST", "/api/plugin-packages", pkgData)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	var up pluginPackageJSON
	json.NewDecoder(rec.Body).Decode(&up)
	if up.File != "hitstor-plugin-example-0.1.0-windows-amd64.hpk" || up.ID != "example" || up.Official {
		t.Fatalf("uploaded = %+v", up)
	}
	var list []pluginPackageJSON
	json.NewDecoder(do("GET", "/api/plugin-packages", nil).Body).Decode(&list)
	if len(list) != 1 || list[0].Version != "0.1.0" {
		t.Fatalf("list = %+v", list)
	}

	// A managed agent that accepts installs, over bufconn.
	if _, err := dash.registry.AddManagedAgent("win01", "bufnet:1", wlAgentToken); err != nil {
		t.Fatal(err)
	}
	inst := &fakeInstaller{}
	agentSrv := agent.NewAgentServer(agent.AgentServerOptions{Token: wlAgentToken})
	agentSrv.Plugins = inst
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

	rec = do("POST", "/api/agents/win01/plugins", []byte(`{"package":"`+up.File+`","upgrade":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("install: %d %s", rec.Code, rec.Body)
	}
	if !bytes.Equal(inst.got, pkgData) || !inst.upgrade {
		t.Fatalf("agent got %d bytes (upgrade %v)", len(inst.got), inst.upgrade)
	}
	if ps := dash.registry.GetAgent("win01").Plugins; len(ps) != 1 || ps[0].ID != "example" || ps[0].Version != "0.1.0" {
		t.Fatalf("inventory = %+v", ps)
	}

	// The agent's refusal reaches the operator.
	inst.fail = errors.New("pluginmgr: does not support linux/amd64")
	if rec := do("POST", "/api/agents/win01/plugins", []byte(`{"package":"`+up.File+`"}`)); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "does not support") {
		t.Fatalf("refused: %d %s", rec.Code, rec.Body)
	}

	// A plugin a job still uses cannot be removed.
	if err := dash.registry.AddJob(server.BackupJob{ID: "j1", Name: "db", AgentName: "win01", RepoName: "default",
		JobType: server.JobTypeWorkload, Plugin: "example", Selection: []string{"demo/alpha"}}); err != nil {
		t.Fatal(err)
	}
	if rec := do("DELETE", "/api/agents/win01/plugins/example", nil); rec.Code != http.StatusConflict {
		t.Fatalf("remove with job: %d", rec.Code)
	}
	dash.registry.RemoveJob("j1")
	if rec := do("DELETE", "/api/agents/win01/plugins/example", nil); rec.Code != http.StatusNoContent || inst.removed != "example" {
		t.Fatalf("remove: %d (removed %q)", rec.Code, inst.removed)
	}
	if ps := dash.registry.GetAgent("win01").Plugins; len(ps) != 0 {
		t.Fatalf("inventory after remove = %+v", ps)
	}

	// Package deletion only accepts bare .hpk names.
	for _, bad := range []string{"..%2Fconfig.yaml", "x.zip"} {
		if rec := do("DELETE", "/api/plugin-packages/"+bad, nil); rec.Code != http.StatusBadRequest {
			t.Fatalf("delete %q: %d", bad, rec.Code)
		}
	}
	if rec := do("DELETE", "/api/plugin-packages/"+up.File, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
}
