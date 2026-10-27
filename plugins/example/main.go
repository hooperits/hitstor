// hitstor-plugin-example is a reference workload plugin. It only answers
// GetMetadata, which is enough to exercise packaging, signing, and the agent's
// plugin manager end to end, and serves as a template for real plugins.
//
// Its id and version come from the embedded plugin.yaml, so the binary and
// the package manifest cannot drift apart.
//
// Usage:
//
//	hitstor-pluginkit pack --source plugins/example/plugin.yaml \
//	    --binary bin/hitstor-plugin-example --os linux --arch amd64 --out dist
//	hitstor-agent plugin install dist/hitstor-plugin-example-0.1.0-linux-amd64.hpk
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginpkg"
)

//go:embed plugin.yaml
var sourceManifest []byte

type example struct {
	manifest *pluginpkg.Manifest
}

func (e *example) Metadata(context.Context) (hitstorplugin.WorkloadMetadata, error) {
	return hitstorplugin.WorkloadMetadata{
		ID:              e.manifest.ID,
		Name:            e.manifest.Name,
		Version:         e.manifest.Version,
		ProtocolVersion: hitstorplugin.WorkloadProtocolVersion,
		Description:     e.manifest.Description,
		Capabilities:    []string{"example.noop"},
	}, nil
}

func main() {
	m, err := pluginpkg.ParseSource(sourceManifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	hitstorplugin.ServeWorkload(&example{manifest: m})
}
