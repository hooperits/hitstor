package dashboard

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginmgr"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/server"
)

// objectStore is an in-memory agent.PluginObjectStore for plugin "example".
type objectStore struct{ saved map[string]map[string]string }

func (s *objectStore) Get(id string) (*pluginmgr.InstalledPlugin, error) {
	return &pluginmgr.InstalledPlugin{ID: id, Fields: []plugin.FieldSpec{
		{Key: "auth", Scope: plugin.FieldScopeObject, Type: "choice", ObjectKinds: []string{"instance"}},
		{Key: "password", Scope: plugin.FieldScopeObject, Type: plugin.FieldTypePassword, ObjectKinds: []string{"instance"}},
	}}, nil
}

func (s *objectStore) ObjectConfigs(string) (map[string]map[string]string, error) {
	return s.saved, nil
}

func (s *objectStore) SetObjectConfig(_, path string, values map[string]string) error {
	if values == nil {
		delete(s.saved, path)
	} else {
		s.saved[path] = values
	}
	return nil
}

func TestAPIObjectConfig(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	if _, err := dash.registry.AddManagedAgent("db-agent", "bufnet:1", wlAgentToken); err != nil {
		t.Fatal(err)
	}
	dash.registry.SetAgentPlugins("db-agent", []server.AgentPlugin{{ID: "example", Status: "active"}})
	store := &objectStore{saved: map[string]map[string]string{}}
	agentSrv := agent.NewAgentServer(agent.AgentServerOptions{Token: wlAgentToken})
	agentSrv.OpenWorkload = func(string, map[string]string) (agent.WorkloadPlugin, func(), error) {
		return &fakeDBPlugin{}, func() {}, nil
	}
	agentSrv.PluginObjects = store
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
	do := func(method, url, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		dash.Handler().ServeHTTP(rec, req)
		return rec
	}
	const url = "/api/agents/db-agent/plugins/example/object-config"

	rec := do("PUT", url, `{"path":"demo","values":{"auth":"sql","password":"s3cret"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if store.saved["demo"]["password"] != "s3cret" {
		t.Fatalf("agent store = %v", store.saved)
	}
	rec = do("GET", url+"?path=demo", "")
	var got objectConfigJSON
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil || !got.Configured || got.Values["auth"] != "sql" ||
		got.Values["password"] != "" || len(got.SecretsSet) != 1 {
		t.Fatalf("get: %+v %v", got, err)
	}
	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Fatal("secret returned to the dashboard")
	}
	// The browse result carries the configured state.
	rec = do("GET", "/api/agents/db-agent/plugins/example/browse", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("browse: %d %s", rec.Code, rec.Body)
	}
	if rec = do("PUT", url, `{"path":"demo","values":{"nope":"x"}}`); rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "rpc error") {
		t.Fatalf("unknown field: %d %s", rec.Code, rec.Body)
	}
	if rec = do("PUT", url, `{"path":"demo","remove":true}`); rec.Code != http.StatusOK || len(store.saved) != 0 {
		t.Fatalf("remove: %d %s %v", rec.Code, rec.Body, store.saved)
	}
	if rec = do("PUT", "/api/agents/db-agent/plugins/other/object-config", `{"path":"demo"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unreported plugin: %d", rec.Code)
	}
	// The server keeps nothing: no job, no registry entry holds the secret.
	if data, _ := json.Marshal(dash.registry.ListJobs()); strings.Contains(string(data), "s3cret") {
		t.Fatal("secret stored on the server")
	}
}
