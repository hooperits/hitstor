// hitstor-plugin-example is a reference workload plugin. It exposes a small
// fixed object tree and backs up synthetic, deterministic data, which is
// enough to exercise packaging, signing, the agent's plugin manager, and the
// workload backup pipeline end to end. It also serves as a template for real
// plugins.
//
// Its id and version come from the embedded plugin.yaml, so the binary and
// the package manifest cannot drift apart.
//
// Settings (Configure):
//
//	size_bytes   bytes of data per item (default 1048576)
//
// Usage:
//
//	hitstor-pluginkit pack --source plugins/example/plugin.yaml \
//	    --binary bin/hitstor-plugin-example --os linux --arch amd64 --out dist
//	hitstor-agent plugin install dist/hitstor-plugin-example-0.1.0-linux-amd64.hpk
//	hitstor-agent plugin browse example
//	hitstor-agent plugin test-backup example --select demo/alpha --out /tmp/items
package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginpkg"
)

//go:embed plugin.yaml
var sourceManifest []byte

// objects is the fixed selection tree: one container with two objects.
var objects = []string{"demo/alpha", "demo/beta"}

type example struct {
	manifest *pluginpkg.Manifest
	size     int64
}

func (e *example) Metadata(context.Context) (hitstorplugin.WorkloadMetadata, error) {
	return hitstorplugin.WorkloadMetadata{
		ID:              e.manifest.ID,
		Name:            e.manifest.Name,
		Version:         e.manifest.Version,
		ProtocolVersion: hitstorplugin.WorkloadProtocolVersion,
		Description:     e.manifest.Description,
		Capabilities:    []string{"browse", "backup.full"},
	}, nil
}

func (e *example) Configure(settings map[string]string) error {
	if v, ok := settings["size_bytes"]; ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("size_bytes: invalid value %q", v)
		}
		e.size = n
	}
	return nil
}

func (e *example) Browse(_ context.Context, p string) ([]hitstorplugin.BrowseEntry, error) {
	if p == "" {
		return []hitstorplugin.BrowseEntry{{Path: "demo", Name: "demo", Kind: "container", HasChildren: true}}, nil
	}
	if p != "demo" {
		return nil, fmt.Errorf("unknown object %q", p)
	}
	var out []hitstorplugin.BrowseEntry
	for _, o := range objects {
		out = append(out, hitstorplugin.BrowseEntry{Path: o, Name: strings.TrimPrefix(o, "demo/"), Kind: "object", Selectable: true})
	}
	return out, nil
}

func (e *example) Backup(ctx context.Context, req hitstorplugin.BackupRequest, sink hitstorplugin.ItemSink) error {
	kind := req.Kind
	if kind == "" {
		kind = "full"
	}
	if kind != "full" {
		return fmt.Errorf("backup kind %q not supported (only full)", kind)
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, sel := range req.Selection {
		if !isObject(sel) {
			return fmt.Errorf("unknown object %q", sel)
		}
		w, err := sink.Item(fmt.Sprintf("%s/%s-%s.dat", sel, kind, stamp))
		if err != nil {
			return err
		}
		// Same object, same bytes: consecutive backups deduplicate.
		src := rand.NewChaCha8(sha256.Sum256([]byte(sel)))
		if _, err := io.CopyN(w, src, e.size); err != nil {
			return err
		}
		meta, _ := json.Marshal(map[string]any{"object": sel, "kind": kind, "size": e.size})
		if err := w.Finish(meta); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func isObject(p string) bool {
	for _, o := range objects {
		if o == p {
			return true
		}
	}
	return false
}

func main() {
	m, err := pluginpkg.ParseSource(sourceManifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	hitstorplugin.ServeWorkload(&example{manifest: m, size: 1 << 20})
}
