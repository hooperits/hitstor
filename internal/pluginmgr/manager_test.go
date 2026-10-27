package pluginmgr

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginpkg"
)

type fixture struct {
	t     *testing.T
	key   ed25519.PrivateKey
	dir   string
	pkgs  string
	mgr   *Manager
	probe func(binary string) (plugin.WorkloadMetadata, error)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, key: key, dir: filepath.Join(t.TempDir(), "plugins"), pkgs: t.TempDir()}
	// The fake binary's content is "<id> <version>"; the default probe
	// reports exactly that, like a well-behaved plugin.
	f.probe = func(binary string) (plugin.WorkloadMetadata, error) {
		data, err := os.ReadFile(binary)
		if err != nil {
			return plugin.WorkloadMetadata{}, err
		}
		id, version, _ := strings.Cut(string(data), " ")
		return plugin.WorkloadMetadata{ID: id, Version: version, ProtocolVersion: 1, Capabilities: []string{"test"}}, nil
	}
	f.mgr = f.newManager("1.0.0", false)
	return f
}

func (f *fixture) newManager(agentVersion string, allowUnsigned bool) *Manager {
	return New(Options{
		Dir:           f.dir,
		Trust:         pluginpkg.TrustPolicy{Keys: []ed25519.PublicKey{f.key.Public().(ed25519.PublicKey)}, AllowUnsigned: allowUnsigned},
		AgentVersion:  agentVersion,
		GOOS:          "linux",
		GOARCH:        "amd64",
		SkipHardening: true,
		Probe: func(_ context.Context, binary string) (plugin.WorkloadMetadata, error) {
			return f.probe(binary)
		},
	})
}

type pkgSpec struct {
	id, version, platforms, minAgent string
	protocol                         int
	unsigned                         bool
	key                              ed25519.PrivateKey
}

// pkg writes a package and returns its path.
func (f *fixture) pkg(s pkgSpec) string {
	f.t.Helper()
	if s.id == "" {
		s.id = "example"
	}
	if s.platforms == "" {
		s.platforms = "linux/amd64"
	}
	if s.protocol == 0 {
		s.protocol = 1
	}
	manifest := fmt.Sprintf("id: %s\nname: Example\nversion: %s\ntype: workload\nprotocol_version: %d\nplatforms: [%s]\nentrypoint: bin/plugin\n",
		s.id, s.version, s.protocol, s.platforms)
	if s.minAgent != "" {
		manifest += "min_agent_version: " + s.minAgent + "\n"
	}
	key := f.key
	if s.key != nil {
		key = s.key
	}
	if s.unsigned {
		key = nil
	}
	data, err := pluginpkg.Build([]byte(manifest), map[string][]byte{"bin/plugin": []byte(s.id + " " + s.version)}, key)
	if err != nil {
		f.t.Fatal(err)
	}
	path := filepath.Join(f.pkgs, fmt.Sprintf("%s-%s.hpk", s.id, s.version))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		f.t.Fatal(err)
	}
	return path
}

func (f *fixture) install(path string, opts InstallOptions) (*InstalledPlugin, error) {
	return f.mgr.Install(context.Background(), path, opts)
}

// assertOnly checks that exactly the listed version directories exist for id
// and that no staging directory was left behind.
func (f *fixture) assertOnly(id string, versions ...string) {
	f.t.Helper()
	entries, _ := os.ReadDir(filepath.Join(f.dir, id))
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(versions, ",") {
		f.t.Fatalf("%s dirs = %v, want %v", id, got, versions)
	}
}

func TestInstallListGet(t *testing.T) {
	f := newFixture(t)
	p, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if p.Status != plugin.PluginStatusActive || p.Signer == "" || len(p.Capabilities) != 1 {
		t.Fatalf("entry = %+v", p)
	}
	list, err := f.mgr.List()
	if err != nil || len(list) != 1 || list[0].Version != "1.0.0" {
		t.Fatalf("List = %+v, %v", list, err)
	}
	bin, err := f.mgr.EntrypointPath("example")
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(bin); err != nil || string(data) != "example 1.0.0" {
		t.Fatalf("entrypoint %s: %q, %v", bin, data, err)
	}
	f.assertOnly("example", "1.0.0")
	if _, err := os.Stat(filepath.Join(f.dir, lockName)); !os.IsNotExist(err) {
		t.Fatal("lock file left behind")
	}
}

func TestInstallTwiceNeedsUpgrade(t *testing.T) {
	f := newFixture(t)
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.install(f.pkg(pkgSpec{version: "1.1.0"}), InstallOptions{}); !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("err = %v, want ErrAlreadyInstalled", err)
	}
}

func TestUpgradeRequiresInstalled(t *testing.T) {
	f := newFixture(t)
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{Upgrade: true}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func TestUpgradeReplacesVersion(t *testing.T) {
	f := newFixture(t)
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	p, err := f.install(f.pkg(pkgSpec{version: "1.1.0"}), InstallOptions{Upgrade: true})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if p.Version != "1.1.0" {
		t.Fatalf("version = %s", p.Version)
	}
	f.assertOnly("example", "1.1.0")
}

func TestUpgradeSameAndOlder(t *testing.T) {
	f := newFixture(t)
	if _, err := f.install(f.pkg(pkgSpec{version: "2.0.0"}), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.install(f.pkg(pkgSpec{version: "2.0.0"}), InstallOptions{Upgrade: true}); !errors.Is(err, ErrSameVersion) {
		t.Fatalf("same: err = %v", err)
	}
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{Upgrade: true}); !errors.Is(err, ErrDowngrade) {
		t.Fatalf("older: err = %v", err)
	}
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{Upgrade: true, AllowDowngrade: true}); err != nil {
		t.Fatalf("allowed downgrade: %v", err)
	}
	f.assertOnly("example", "1.0.0")
}

func TestFailedUpgradeKeepsPreviousVersion(t *testing.T) {
	f := newFixture(t)
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	good := f.probe
	f.probe = func(binary string) (plugin.WorkloadMetadata, error) {
		return plugin.WorkloadMetadata{}, errors.New("crashed on start")
	}
	if _, err := f.install(f.pkg(pkgSpec{version: "1.1.0"}), InstallOptions{Upgrade: true}); err == nil || !strings.Contains(err.Error(), "crashed on start") {
		t.Fatalf("err = %v", err)
	}
	f.probe = good

	p, err := f.mgr.Get("example")
	if err != nil || p.Version != "1.0.0" {
		t.Fatalf("active = %+v, %v", p, err)
	}
	f.assertOnly("example", "1.0.0")
	bin, _ := f.mgr.EntrypointPath("example")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("previous binary gone: %v", err)
	}
}

func TestMetadataMustMatchManifest(t *testing.T) {
	f := newFixture(t)
	f.probe = func(string) (plugin.WorkloadMetadata, error) {
		return plugin.WorkloadMetadata{ID: "impostor", Version: "1.0.0", ProtocolVersion: 1}, nil
	}
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{}); err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("err = %v", err)
	}
	if list, _ := f.mgr.List(); len(list) != 0 {
		t.Fatalf("registered %+v", list)
	}
	f.assertOnly("example")
}

func TestTrustRules(t *testing.T) {
	f := newFixture(t)
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0", unsigned: true}), InstallOptions{}); !errors.Is(err, pluginpkg.ErrUnsigned) {
		t.Fatalf("unsigned: err = %v", err)
	}
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0", key: stranger}), InstallOptions{}); !errors.Is(err, pluginpkg.ErrUntrustedKey) {
		t.Fatalf("stranger: err = %v", err)
	}
	if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
		t.Fatal("rejected packages must not create the store")
	}

	f.mgr = f.newManager("1.0.0", true)
	p, err := f.install(f.pkg(pkgSpec{version: "1.0.0", unsigned: true}), InstallOptions{})
	if err != nil {
		t.Fatalf("allow unsigned: %v", err)
	}
	if p.Signer != "" {
		t.Fatalf("signer = %q", p.Signer)
	}
}

func TestCompatibilityRules(t *testing.T) {
	f := newFixture(t)
	cases := map[string]pkgSpec{
		"platform": {version: "1.0.0", platforms: "windows/amd64"},
		"protocol": {version: "1.0.0", protocol: 2},
		"agent":    {version: "1.0.0", minAgent: "9.0.0"},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.install(f.pkg(spec), InstallOptions{}); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	// Development agents skip the agent version check.
	f.mgr = f.newManager("dev", false)
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0", minAgent: "9.0.0"}), InstallOptions{}); err != nil {
		t.Fatalf("dev agent: %v", err)
	}
}

func TestRemove(t *testing.T) {
	f := newFixture(t)
	if err := f.mgr.Remove("example"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.install(f.pkg(pkgSpec{id: "other", version: "1.0.0"}), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := f.mgr.Remove("example"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "example")); !os.IsNotExist(err) {
		t.Fatal("files left behind")
	}
	list, _ := f.mgr.List()
	if len(list) != 1 || list[0].ID != "other" {
		t.Fatalf("List = %+v", list)
	}
}

func TestConcurrentOperationIsRefused(t *testing.T) {
	f := newFixture(t)
	if _, err := f.install(f.pkg(pkgSpec{version: "1.0.0"}), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	unlock, err := f.mgr.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := f.mgr.Remove("example"); !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
}

func TestListEmptyStore(t *testing.T) {
	f := newFixture(t)
	list, err := f.mgr.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if _, err := f.mgr.Get("x"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
}
