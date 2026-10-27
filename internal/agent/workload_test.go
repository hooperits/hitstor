package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/restorer"
)

// fakeSource plays a plugin: it hands each item to the handler in order.
type fakeSource struct {
	items []fakeItem
	err   error
}

type fakeItem struct {
	path string
	data []byte
	meta string
}

func (f *fakeSource) Backup(_ context.Context, _ plugin.BackupRequest, handle plugin.ItemHandler) ([]plugin.BackupItem, error) {
	var out []plugin.BackupItem
	for _, it := range f.items {
		if err := handle(it.path, bytes.NewReader(it.data)); err != nil {
			return nil, err
		}
		out = append(out, plugin.BackupItem{Path: it.path, Size: int64(len(it.data)), Metadata: json.RawMessage(it.meta)})
	}
	return out, f.err
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWorkloadBackupStoresItemsAndManifests(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, err := registry.AddAgent("db-agent")
	if err != nil {
		t.Fatal(err)
	}

	full := randomBytes(t, 3<<20)
	src := &fakeSource{items: []fakeItem{
		{path: "inst/db1/full-1.bak", data: full, meta: `{"lsn":"42"}`},
		{path: "inst/db2/full-1.bak", data: []byte("small"), meta: ""},
	}}
	b := &WorkloadBackup{
		Client: client, APIKey: apiKey, Hostname: "db-host",
		PluginID: "fakedb", PluginVersion: "1.2.3", Source: src,
		Request: plugin.BackupRequest{Selection: []string{"inst/db1", "inst/db2"}, Kind: "full"},
	}
	var progress []string
	b.SetProgress(func(p string, _ uint64, _ bool) { progress = append(progress, p) })

	res, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.SnapshotID == "" || res.FilesNew != 4 {
		t.Fatalf("result = %+v", res)
	}
	if len(progress) != 4 {
		t.Fatalf("progress = %v", progress)
	}

	snapIDs, err := repo.ListSnapshots()
	if err != nil || len(snapIDs) != 1 {
		t.Fatalf("snapshots = %v, %v", snapIDs, err)
	}
	snap, err := repo.LoadSnapshot(snapIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(snap.Paths, []string{"workload:fakedb"}) {
		t.Fatalf("paths = %v", snap.Paths)
	}
	for _, tag := range []string{"plugin:fakedb", "kind:full"} {
		if !slices.Contains(snap.Tags, tag) {
			t.Fatalf("tags = %v, missing %s", snap.Tags, tag)
		}
	}

	// Restoring the snapshot as plain files yields the items byte for byte.
	target := t.TempDir()
	if err := restorer.New(repo).Restore(snapIDs[0], target, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(target, "fakedb", "inst", "db1", "full-1.bak"))
	if err != nil || !bytes.Equal(got, full) {
		t.Fatalf("restored item differs (%v)", err)
	}

	raw, err := os.ReadFile(filepath.Join(target, "fakedb", "inst", "db1", "full-1.bak"+plugin.MetadataSuffix))
	if err != nil {
		t.Fatal(err)
	}
	var m plugin.ItemManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(full)
	var meta bytes.Buffer
	if err := json.Compact(&meta, m.Metadata); err != nil {
		t.Fatal(err)
	}
	if m.Plugin != "fakedb" || m.PluginVersion != "1.2.3" || m.Item != "inst/db1/full-1.bak" ||
		m.Kind != "full" || m.Size != int64(len(full)) || m.SHA256 != hex.EncodeToString(sum[:]) ||
		meta.String() != `{"lsn":"42"}` || m.CreatedAt.IsZero() {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestWorkloadBackupDeduplicatesAcrossRuns(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, _ := registry.AddAgent("db-agent")

	data := randomBytes(t, 4<<20)
	run := func(name string) *BackupResult {
		b := &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb", Source: &fakeSource{items: []fakeItem{{path: name, data: data}}}}
		res, err := b.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	first := run("full-1.bak")
	second := run("full-2.bak") // same bytes, new name: only the manifest is new
	if first.BytesAdded == 0 || second.BytesAdded*10 > first.BytesAdded {
		t.Fatalf("bytes added: first %d, second %d", first.BytesAdded, second.BytesAdded)
	}
}

func TestWorkloadBackupFailures(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, _ := registry.AddAgent("db-agent")

	empty := &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb", Source: &fakeSource{}}
	if _, err := empty.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "no items") {
		t.Fatalf("empty: err = %v", err)
	}

	failing := &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb",
		Source: &fakeSource{items: []fakeItem{{path: "a.bak", data: []byte("x")}}, err: errors.New("VDI timeout")}}
	if _, err := failing.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "VDI timeout") {
		t.Fatalf("failing: err = %v", err)
	}
}

func TestParentDirNodes(t *testing.T) {
	if len(parentDirNodes(nil)) != 0 {
		t.Fatal("dirs for no files")
	}
}
