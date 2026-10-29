package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/restorer"
	"github.com/hooperits/hitstor/internal/server"
)

// fakeSource plays a plugin: it hands each item to the handler in order.
type fakeSource struct {
	items   []fakeItem
	results []plugin.ObjectResult
	err     error
	gotReq  plugin.BackupRequest
}

type fakeItem struct {
	path string
	data []byte
	meta string
	// readErr makes reading the item fail; plugin.ErrItemAborted plays a
	// plugin that aborts it.
	readErr error
}

type failingReader struct {
	data []byte
	err  error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

func (f *fakeSource) Backup(_ context.Context, req plugin.BackupRequest, handle plugin.ItemHandler) (plugin.BackupOutcome, error) {
	f.gotReq = req
	out := plugin.BackupOutcome{Results: f.results}
	for _, it := range f.items {
		var r io.Reader = bytes.NewReader(it.data)
		if it.readErr != nil {
			r = &failingReader{data: it.data, err: it.readErr}
		}
		if err := handle(it.path, r); err != nil {
			if errors.Is(it.readErr, plugin.ErrItemAborted) {
				continue // the client skips an aborted item
			}
			return out, err
		}
		out.Items = append(out.Items, plugin.BackupItem{Path: it.path, Size: int64(len(it.data)), Metadata: json.RawMessage(it.meta)})
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

	// Nothing could be backed up: the error carries each object's reason.
	none := &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb", Source: &fakeSource{
		results: []plugin.ObjectResult{{Object: "inst/a", Status: plugin.ObjectFailed, Reason: "no-full"}},
		err:     errors.New("VDI timeout")}}
	_, err := none.Run(context.Background())
	var re *WorkloadRunError
	if !errors.As(err, &re) || len(re.Results) != 1 || !strings.Contains(err.Error(), "VDI timeout") || !strings.Contains(err.Error(), "inst/a (no-full") {
		t.Fatalf("none: err = %v", err)
	}
}

func TestWorkloadBackupKeepsFinishedItems(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, _ := registry.AddAgent("db-agent")

	// The plugin backed up one database, then failed on the next: the
	// database already counts the first as backed up, so it must be kept.
	src := &fakeSource{items: []fakeItem{{path: "inst/a/log-1.trn", data: []byte("log of a")}},
		results: []plugin.ObjectResult{{Object: "inst/c", Status: plugin.ObjectExcluded, Reason: "simple-recovery"}},
		err:     errors.New("inst/b: VDI timeout")}
	b := &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb", Source: src, ServerRecordsResults: true,
		Request: plugin.BackupRequest{Selection: []string{"inst"}, Kind: "log", Options: map[string]string{"copy_only": "1"}}}
	res, err := b.Run(context.Background())
	if err != nil || res.SnapshotID == "" {
		t.Fatalf("partial run: %+v, %v", res, err)
	}
	if !src.gotReq.ObjectResults {
		t.Fatal("the agent must ask the plugin for object results")
	}
	ids, _ := repo.ListSnapshots()
	snap, err := repo.LoadSnapshot(ids[0])
	if err != nil || !slices.Contains(snap.Tags, "copy-only") || !slices.Contains(snap.Tags, "kind:log") {
		t.Fatalf("snapshot tags = %v (%v)", snap.Tags, err)
	}

	// Every object excluded by design is a run with nothing to do, not a failure.
	excluded := &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb", Source: &fakeSource{
		results: []plugin.ObjectResult{{Object: "inst/c", Status: plugin.ObjectExcluded, Reason: "simple-recovery"}}},
		Request: plugin.BackupRequest{Selection: []string{"inst"}, Kind: "log"}}
	if _, err := excluded.Run(context.Background()); err != nil {
		t.Fatalf("all excluded: %v", err)
	}
}

func TestParentDirNodes(t *testing.T) {
	if len(parentDirNodes(nil)) != 0 {
		t.Fatal("dirs for no files")
	}
}

func TestWorkloadPlanRunsEndToEnd(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, _ := registry.AddAgent("sql01")
	if err := registry.AddJob(server.BackupJob{ID: "j1", Name: "db", AgentName: "sql01", RepoName: "default",
		JobType: server.JobTypeWorkload, Plugin: "fakedb", Selection: []string{"inst"},
		Plan: []server.PlanEntry{{Kind: "full"}, {Kind: "log"}}}); err != nil {
		t.Fatal(err)
	}
	run := func(kind string, src *fakeSource) error {
		b := &WorkloadBackup{Client: client, APIKey: apiKey, JobID: "j1", Tags: []string{"job:j1"}, PluginID: "fakedb", Source: src,
			ServerRecordsResults: true, Request: plugin.BackupRequest{Selection: []string{"inst"}, Kind: kind}}
		_, err := b.Run(context.Background())
		return err
	}

	// A full of a and b; c is excluded by design, d fails: the run is partial.
	full := &fakeSource{
		items: []fakeItem{
			{path: "inst/a/full-1.bak", data: []byte("a full"), meta: `{"type":"D","checkpoint_lsn":"100"}`},
			{path: "inst/b/full-1.bak", data: []byte("b full"), meta: `{"type":"D","checkpoint_lsn":"500"}`},
		},
		results: []plugin.ObjectResult{
			{Object: "inst/c", Status: plugin.ObjectExcluded, Reason: "simple-recovery"},
			{Object: "inst/d", Status: plugin.ObjectFailed, Reason: "not-online", Message: "database is OFFLINE"},
		}}
	if err := run("full", full); err != nil {
		t.Fatal(err)
	}
	if full.gotReq.KnownProvided {
		t.Fatal("a full backup needs no list of known backups")
	}
	st := registry.GetJob("j1").KindRuns["full"]
	if st.LastBackup == nil || !st.Partial || len(st.Objects) != 2 || st.Objects[1].Reason != "not-online" {
		t.Fatalf("full run = %+v", st)
	}
	ids, _ := repo.ListSnapshots()
	snap, _ := repo.LoadSnapshot(ids[0])
	if !slices.Contains(snap.Tags, "obj:inst/a") || !slices.Contains(snap.Tags, "obj:inst/b") || slices.Contains(snap.Tags, "obj:inst/c") {
		t.Fatalf("snapshot tags = %v", snap.Tags)
	}

	// The log backup gets the items the server holds, to check the chain.
	logSrc := &fakeSource{results: []plugin.ObjectResult{{Object: "inst/a", Status: plugin.ObjectFailed, Reason: "no-full"}}}
	err := run("log", logSrc)
	var re *WorkloadRunError
	if !errors.As(err, &re) {
		t.Fatalf("log run: %v", err)
	}
	if !logSrc.gotReq.KnownProvided || len(logSrc.gotReq.Known) != 2 {
		t.Fatalf("known = %v %+v", logSrc.gotReq.KnownProvided, logSrc.gotReq.Known)
	}
	// The agent's caller reports it; the server keeps each object's reason.
	if err := client.ReportJobFailure(context.Background(), apiKey, "j1", err); err != nil {
		t.Fatal(err)
	}
	if st := registry.GetJob("j1").KindRuns["log"]; st.LastErrorAt == nil || len(st.Objects) != 1 || st.Objects[0].Reason != "no-full" {
		t.Fatalf("log run = %+v", st)
	}
}

func TestWorkloadBackupStoreFailureKeepsNothing(t *testing.T) {
	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, _ := registry.AddAgent("db-agent")

	// a finished, b was aborted by the plugin (fine), c could not be read
	// (the host's own failure: chunks of finished items may be lost too).
	src := &fakeSource{items: []fakeItem{
		{path: "inst/a/log.trn", data: []byte("a")},
		{path: "inst/b/log.trn", data: []byte("b"), readErr: plugin.ErrItemAborted},
		{path: "inst/c/log.trn", data: []byte("c"), readErr: errors.New("connection reset")},
	}}
	b := &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb", Source: src, ServerRecordsResults: true}
	_, err := b.Run(context.Background())
	var re *WorkloadRunError
	if !errors.As(err, &re) || !strings.Contains(err.Error(), "store backup") {
		t.Fatalf("err = %v", err)
	}
	if ids, _ := repo.ListSnapshots(); len(ids) != 0 {
		t.Fatalf("a snapshot was kept: %v", ids)
	}

	// An aborted item alone is not a host failure: the others are kept.
	src = &fakeSource{items: []fakeItem{
		{path: "inst/a/log.trn", data: []byte("a")},
		{path: "inst/b/log.trn", data: []byte("b"), readErr: plugin.ErrItemAborted},
	}}
	b = &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb", Source: src, ServerRecordsResults: true}
	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("aborted item: %v", err)
	}
}

func TestWorkloadBackupOlderServerGetsNoResults(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, _ := registry.AddAgent("db-agent")
	src := &fakeSource{items: []fakeItem{{path: "inst/a/full.bak", data: []byte("a")}}}
	b := &WorkloadBackup{Client: client, APIKey: apiKey, PluginID: "fakedb", Source: src}
	if _, err := b.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if src.gotReq.ObjectResults {
		t.Fatal("a server that cannot record per-object results must not get partial runs")
	}
}
