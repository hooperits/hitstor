package agent

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginmgr"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

// memObjects is an in-memory PluginObjectStore for plugin "fakedb".
type memObjects struct {
	saved map[string]map[string]string
}

func (m *memObjects) Get(id string) (*pluginmgr.InstalledPlugin, error) {
	if id != "fakedb" {
		return nil, pluginmgr.ErrNotInstalled
	}
	return &pluginmgr.InstalledPlugin{ID: id, Fields: []plugin.FieldSpec{
		{Key: "auth", Scope: plugin.FieldScopeObject, Type: "choice", ObjectKinds: []string{"instance"}},
		{Key: "username", Scope: plugin.FieldScopeObject, Type: "string", ObjectKinds: []string{"instance"}},
		{Key: "password", Scope: plugin.FieldScopeObject, Type: plugin.FieldTypePassword, ObjectKinds: []string{"instance"}},
		{Key: "copy_only", Scope: plugin.FieldScopeBackup, Type: "bool"},
	}}, nil
}

func (m *memObjects) ObjectConfigs(string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for k, v := range m.saved {
		out[k] = v
	}
	return out, nil
}

func (m *memObjects) SetObjectConfig(_, path string, values map[string]string) error {
	if values == nil {
		delete(m.saved, path)
	} else {
		m.saved[path] = values
	}
	return nil
}

// testedWorkload fails Browse unless the saved password is "good".
type testedWorkload struct {
	recordingWorkload
	store *memObjects
}

func (w *testedWorkload) Browse(ctx context.Context, p string) ([]plugin.BrowseEntry, error) {
	if p == "" {
		return []plugin.BrowseEntry{{Path: "inst", Name: "inst", Kind: "instance", HasChildren: true}, {Path: "other", Kind: "instance"}}, nil
	}
	if w.store.saved[p]["password"] != "good" {
		return nil, errors.New("login failed")
	}
	return w.recordingWorkload.Browse(ctx, p)
}

func startObjectsAgent(t *testing.T) (pb.AgentServiceClient, *memObjects) {
	t.Helper()
	store := &memObjects{saved: map[string]map[string]string{}}
	w := &testedWorkload{store: store}
	srv := NewAgentServer(AgentServerOptions{Token: testAgentToken, Version: "test"})
	srv.OpenWorkload = func(string, map[string]string) (WorkloadPlugin, func(), error) { return w, func() {}, nil }
	srv.PluginObjects = store
	return startBMRAgent(t, srv), store
}

func TestWorkloadObjectConfig(t *testing.T) {
	client, store := startObjectsAgent(t)
	ctx := context.Background()
	set := func(values map[string]string) error {
		_, err := client.SetWorkloadObjectConfig(ctx, &pb.SetWorkloadObjectConfigRequest{
			Token: testAgentToken, Plugin: "fakedb", Path: "inst", Values: values,
		})
		return err
	}

	// A failing test leaves nothing saved.
	if err := set(map[string]string{"auth": "sql", "username": "u", "password": "bad"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("bad password: %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved after failure: %v", store.saved)
	}
	if err := set(map[string]string{"auth": "sql", "username": "u", "password": "good"}); err != nil {
		t.Fatal(err)
	}
	// An empty password keeps the saved one; a failing change restores the
	// previous settings.
	if err := set(map[string]string{"auth": "sql", "username": "u2", "password": ""}); err != nil {
		t.Fatal(err)
	}
	if err := set(map[string]string{"auth": "sql", "username": "u3", "password": "bad"}); err == nil {
		t.Fatal("bad password accepted")
	}
	if got := store.saved["inst"]; got["username"] != "u2" || got["password"] != "good" {
		t.Fatalf("saved = %v", got)
	}
	if err := set(map[string]string{"copy_only": "true"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("non-object field: %v", err)
	}

	// Reading back never returns the secret.
	got, err := client.GetWorkloadObjectConfig(ctx, &pb.GetWorkloadObjectConfigRequest{Token: testAgentToken, Plugin: "fakedb", Path: "inst"})
	if err != nil || !got.Configured || got.Values["username"] != "u2" || got.Values["password"] != "" ||
		len(got.SecretsSet) != 1 || got.SecretsSet[0] != "password" {
		t.Fatalf("get = %v, %v", got, err)
	}

	// Browse marks configurable and configured objects.
	br, err := client.BrowseWorkload(ctx, &pb.BrowseWorkloadRequest{Token: testAgentToken, Plugin: "fakedb"})
	if err != nil || !br.Entries[0].Configurable || !br.Entries[0].Configured || br.Entries[1].Configured {
		t.Fatalf("browse = %v, %v", br, err)
	}

	if _, err := client.SetWorkloadObjectConfig(ctx, &pb.SetWorkloadObjectConfigRequest{
		Token: testAgentToken, Plugin: "fakedb", Path: "inst", Remove: true,
	}); err != nil || len(store.saved) != 0 {
		t.Fatalf("remove: %v, %v", err, store.saved)
	}
	if _, err := client.GetWorkloadObjectConfig(ctx, &pb.GetWorkloadObjectConfigRequest{Token: "wrong", Plugin: "fakedb"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bad token: %v", err)
	}
}
