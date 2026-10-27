//go:build integration

package pluginmgr

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hooperits/hitstor/internal/pluginpkg"
)

// TestIntegrationInstallRealPlugin builds the example plugin, packages and
// signs it, and installs it with the real go-plugin handshake.
func TestIntegrationInstallRealPlugin(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "hitstor-plugin-example")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "github.com/hooperits/hitstor/plugins/example")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build plugin: %v", err)
	}

	src, err := os.ReadFile(filepath.Join("..", "..", "plugins", "example", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := pluginpkg.ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := m.ForPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	binData, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data, err := pluginpkg.Build(manifest, map[string][]byte{pluginpkg.EntrypointFor(m.ID, runtime.GOOS): binData}, key)
	if err != nil {
		t.Fatal(err)
	}
	pkgPath := filepath.Join(tmp, pluginpkg.PackageName(m.ID, m.Version, runtime.GOOS, runtime.GOARCH))
	if err := os.WriteFile(pkgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := New(Options{
		Dir:           filepath.Join(tmp, "store"),
		Trust:         pluginpkg.TrustPolicy{Keys: []ed25519.PublicKey{pub}},
		AgentVersion:  "dev",
		SkipHardening: true,
	})
	p, err := mgr.Install(context.Background(), pkgPath, InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if p.ID != "example" || p.Version != m.Version || len(p.Capabilities) == 0 || p.Description == "" {
		t.Fatalf("installed = %+v", p)
	}
	if err := mgr.Remove("example"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}
