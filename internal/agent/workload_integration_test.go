//go:build integration

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/restorer"
)

// TestIntegrationWorkloadBackupRealPlugin runs the example plugin binary over
// go-plugin as the source of a workload backup into a live server.
func TestIntegrationWorkloadBackupRealPlugin(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "hitstor-plugin-example")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "github.com/hooperits/hitstor/plugins/example")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build plugin: %v", err)
	}
	c, err := plugin.OpenWorkload(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Configure(context.Background(), map[string]string{"size_bytes": "5000000"}); err != nil {
		t.Fatal(err)
	}

	client, repo, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, _ := registry.AddAgent("db-agent")

	b := &WorkloadBackup{
		Client: client, APIKey: apiKey, PluginID: "example", PluginVersion: "0.1.0", Source: c,
		Request: plugin.BackupRequest{Selection: []string{"demo/alpha", "demo/beta"}, Kind: "full"},
	}
	res, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FilesNew != 4 || res.BytesProcessed < 10_000_000 {
		t.Fatalf("result = %+v", res)
	}

	snaps, _ := repo.ListSnapshots()
	target := t.TempDir()
	if err := restorer.New(repo).Restore(snaps[0], target, nil); err != nil {
		t.Fatal(err)
	}
	items, _ := filepath.Glob(filepath.Join(target, "example", "demo", "*", "full-*.dat"))
	if len(items) != 2 {
		t.Fatalf("restored items = %v", items)
	}
	for _, it := range items {
		fi, err := os.Stat(it)
		if err != nil || fi.Size() != 5_000_000 {
			t.Fatalf("%s: %v %v", it, fi, err)
		}
		if _, err := os.Stat(it + plugin.MetadataSuffix); err != nil {
			t.Fatalf("manifest missing: %v", err)
		}
	}
}
