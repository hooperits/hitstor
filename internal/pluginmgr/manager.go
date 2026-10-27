// Package pluginmgr installs, upgrades, lists, and removes workload plugin
// packages on an agent.
//
// Layout under the store directory:
//
//	installed-plugins.json      registry of installed plugins
//	.lock                       held during install/upgrade/remove
//	<id>/<version>/             unpacked package of the active version
//	<id>/.staging-*             transient, during install
//
// An install verifies the package, unpacks it into a staging directory,
// launches the plugin once to confirm it matches its manifest, and only then
// makes it the active version. Any failure leaves the previous version active.
package pluginmgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginpkg"
)

const (
	registryName = "installed-plugins.json"
	lockName     = ".lock"
	stagingPref  = ".staging-"

	// probeTimeout bounds the GetMetadata handshake run at install time.
	probeTimeout = 60 * time.Second
)

// Errors returned by the manager.
var (
	ErrNotInstalled     = errors.New("pluginmgr: plugin is not installed")
	ErrAlreadyInstalled = errors.New("pluginmgr: plugin is already installed (use upgrade)")
	ErrSameVersion      = errors.New("pluginmgr: this version is already installed")
	ErrDowngrade        = errors.New("pluginmgr: package is older than the installed version (use --allow-downgrade)")
	ErrBusy             = errors.New("pluginmgr: another plugin operation is in progress")
)

// InstalledPlugin is one entry of installed-plugins.json.
type InstalledPlugin struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Version         string    `json:"version"`
	Description     string    `json:"description,omitempty"`
	Status          string    `json:"status"`
	ProtocolVersion int       `json:"protocol_version"`
	Capabilities    []string  `json:"capabilities,omitempty"`
	Signer          string    `json:"signer,omitempty"` // base64 public key; empty when unsigned
	Entrypoint      string    `json:"entrypoint"`       // relative to the version directory
	InstalledAt     time.Time `json:"installed_at"`
}

type registryFile struct {
	Version int               `json:"version"`
	Plugins []InstalledPlugin `json:"plugins"`
}

// ProbeFunc launches a plugin binary and returns the metadata it reports.
type ProbeFunc func(ctx context.Context, binaryPath string) (plugin.WorkloadMetadata, error)

// Options configures a Manager.
type Options struct {
	// Dir is the plugin store directory.
	Dir string
	// Trust decides which packages may be installed.
	Trust pluginpkg.TrustPolicy
	// AgentVersion is checked against a package's min_agent_version.
	// "dev" or empty skips the check.
	AgentVersion string
	// GOOS and GOARCH default to the running platform.
	GOOS, GOARCH string
	// Probe defaults to launching the plugin over go-plugin.
	Probe ProbeFunc
	// SkipHardening leaves the store's permissions alone. Tests only.
	SkipHardening bool
}

// Manager manages the plugin store of one agent.
type Manager struct {
	opts Options
}

// New returns a manager for the store at opts.Dir. The directory is created
// on first install, not here, so read-only commands work without privileges.
func New(opts Options) *Manager {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	if opts.Probe == nil {
		opts.Probe = probeWorkload
	}
	return &Manager{opts: opts}
}

// Dir returns the store directory.
func (m *Manager) Dir() string { return m.opts.Dir }

// List returns the installed plugins sorted by id.
func (m *Manager) List() ([]InstalledPlugin, error) {
	reg, err := m.load()
	if err != nil {
		return nil, err
	}
	return reg.Plugins, nil
}

// Get returns one installed plugin.
func (m *Manager) Get(id string) (*InstalledPlugin, error) {
	reg, err := m.load()
	if err != nil {
		return nil, err
	}
	if p := find(reg, id); p != nil {
		return p, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotInstalled, id)
}

// EntrypointPath returns the absolute path of an installed plugin's binary.
func (m *Manager) EntrypointPath(id string) (string, error) {
	p, err := m.Get(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(m.versionDir(p.ID, p.Version), filepath.FromSlash(p.Entrypoint)), nil
}

// InstallOptions selects install versus upgrade semantics.
type InstallOptions struct {
	// Upgrade requires the plugin to be installed already and replaces it.
	Upgrade bool
	// AllowDowngrade lets Upgrade install an older version.
	AllowDowngrade bool
}

// Install verifies and installs the package at pkgPath.
func (m *Manager) Install(ctx context.Context, pkgPath string, opts InstallOptions) (*InstalledPlugin, error) {
	pkg, err := pluginpkg.ReadFile(pkgPath)
	if err != nil {
		return nil, err
	}
	if err := m.check(pkg); err != nil {
		return nil, err
	}
	man := pkg.Manifest

	if err := m.ensureStore(); err != nil {
		return nil, err
	}
	unlock, err := m.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	reg, err := m.load()
	if err != nil {
		return nil, err
	}
	prev := find(reg, man.ID)
	switch {
	case prev == nil && opts.Upgrade:
		return nil, fmt.Errorf("%w: %s", ErrNotInstalled, man.ID)
	case prev != nil && !opts.Upgrade:
		return nil, fmt.Errorf("%w: %s %s", ErrAlreadyInstalled, prev.ID, prev.Version)
	case prev != nil && prev.Version == man.Version:
		return nil, fmt.Errorf("%w: %s %s", ErrSameVersion, prev.ID, prev.Version)
	case prev != nil && plugin.CompareVersions(man.Version, prev.Version) < 0 && !opts.AllowDowngrade:
		return nil, fmt.Errorf("%w: %s %s < %s", ErrDowngrade, man.ID, man.Version, prev.Version)
	}

	pluginDir := filepath.Join(m.opts.Dir, man.ID)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return nil, fmt.Errorf("pluginmgr: %w", err)
	}
	staging := filepath.Join(pluginDir, stagingPref+man.Version+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := pkg.Extract(staging); err != nil {
		os.RemoveAll(staging)
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			os.RemoveAll(staging)
		}
	}()

	meta, err := m.probe(ctx, filepath.Join(staging, filepath.FromSlash(man.Entrypoint)))
	if err != nil {
		return nil, err
	}
	if meta.ID != man.ID || meta.Version != man.Version || int(meta.ProtocolVersion) != man.ProtocolVersion {
		return nil, fmt.Errorf("pluginmgr: plugin reports %s %s (protocol %d) but its manifest says %s %s (protocol %d)",
			meta.ID, meta.Version, meta.ProtocolVersion, man.ID, man.Version, man.ProtocolVersion)
	}

	final := m.versionDir(man.ID, man.Version)
	// A directory left behind by an interrupted operation is never the
	// active version (that case was refused above), so it is safe to drop.
	if err := os.RemoveAll(final); err != nil {
		return nil, fmt.Errorf("pluginmgr: %w", err)
	}
	if err := os.Rename(staging, final); err != nil {
		return nil, fmt.Errorf("pluginmgr: activate %s: %w", man.ID, err)
	}
	committed = true

	entry := InstalledPlugin{
		ID:              man.ID,
		Name:            man.Name,
		Version:         man.Version,
		Description:     meta.Description,
		Status:          plugin.PluginStatusActive,
		ProtocolVersion: man.ProtocolVersion,
		Capabilities:    meta.Capabilities,
		Signer:          pkg.Signer(),
		Entrypoint:      man.Entrypoint,
		InstalledAt:     time.Now().UTC(),
	}
	upsert(reg, entry)
	if err := m.save(reg); err != nil {
		// Without the registry update the old version is still the one in
		// use; drop the new directory so the two stay consistent.
		os.RemoveAll(final)
		return nil, err
	}

	if prev != nil {
		if err := os.RemoveAll(m.versionDir(prev.ID, prev.Version)); err != nil {
			slog.Warn("pluginmgr: remove previous version", "plugin", prev.ID, "version", prev.Version, "error", err)
		}
	}
	slog.Info("plugin installed", "plugin", entry.ID, "version", entry.Version, "signed", entry.Signer != "")
	return &entry, nil
}

// Remove uninstalls a plugin. Snapshots it produced stay in the repository.
func (m *Manager) Remove(id string) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()

	reg, err := m.load()
	if err != nil {
		return err
	}
	if find(reg, id) == nil {
		return fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	kept := reg.Plugins[:0]
	for _, p := range reg.Plugins {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	reg.Plugins = kept
	// Unregister first: a directory without a registry entry is inert, an
	// entry without its directory is a broken plugin.
	if err := m.save(reg); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(m.opts.Dir, id)); err != nil {
		return fmt.Errorf("pluginmgr: remove files of %s: %w", id, err)
	}
	slog.Info("plugin removed", "plugin", id)
	return nil
}

// check validates a package against trust, platform, and version rules.
func (m *Manager) check(pkg *pluginpkg.Package) error {
	if err := pkg.Verify(m.opts.Trust); err != nil {
		return err
	}
	man := pkg.Manifest
	if !man.SupportsPlatform(m.opts.GOOS, m.opts.GOARCH) {
		return fmt.Errorf("pluginmgr: %s %s does not support %s/%s (supports %v)",
			man.ID, man.Version, m.opts.GOOS, m.opts.GOARCH, man.Platforms)
	}
	if man.ProtocolVersion != plugin.WorkloadProtocolVersion {
		return fmt.Errorf("pluginmgr: %s %s speaks workload protocol %d; this agent speaks %d",
			man.ID, man.Version, man.ProtocolVersion, plugin.WorkloadProtocolVersion)
	}
	v := m.opts.AgentVersion
	if man.MinAgentVersion != "" && v != "" && v != "dev" && plugin.CompareVersions(v, man.MinAgentVersion) < 0 {
		return fmt.Errorf("pluginmgr: %s %s requires agent %s or newer (running %s)",
			man.ID, man.Version, man.MinAgentVersion, v)
	}
	return nil
}

func (m *Manager) probe(ctx context.Context, binary string) (plugin.WorkloadMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	meta, err := m.opts.Probe(ctx, binary)
	if err != nil {
		return plugin.WorkloadMetadata{}, fmt.Errorf("pluginmgr: plugin failed to start: %w", err)
	}
	return meta, nil
}

func probeWorkload(ctx context.Context, binaryPath string) (plugin.WorkloadMetadata, error) {
	c, err := plugin.OpenWorkload(binaryPath)
	if err != nil {
		return plugin.WorkloadMetadata{}, err
	}
	defer c.Close()
	return c.Metadata(ctx)
}

func (m *Manager) versionDir(id, version string) string {
	return filepath.Join(m.opts.Dir, id, version)
}

func (m *Manager) ensureStore() error {
	if err := os.MkdirAll(m.opts.Dir, 0o700); err != nil {
		return fmt.Errorf("pluginmgr: create store: %w", err)
	}
	if m.opts.SkipHardening {
		return nil
	}
	if err := hardenDir(m.opts.Dir); err != nil {
		return fmt.Errorf("pluginmgr: restrict permissions of %s: %w", m.opts.Dir, err)
	}
	return nil
}

// lock takes the store-wide lock file. It is not reentrant.
func (m *Manager) lock() (func(), error) {
	path := filepath.Join(m.opts.Dir, lockName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w (if no other operation is running, delete %s)", ErrBusy, path)
		}
		if errors.Is(err, os.ErrNotExist) {
			// No store yet: nothing to protect.
			return func() {}, nil
		}
		return nil, fmt.Errorf("pluginmgr: lock: %w", err)
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(path) }, nil
}

func (m *Manager) load() (*registryFile, error) {
	data, err := os.ReadFile(filepath.Join(m.opts.Dir, registryName))
	if errors.Is(err, os.ErrNotExist) {
		return &registryFile{Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pluginmgr: read registry: %w", err)
	}
	var reg registryFile
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("pluginmgr: parse registry: %w", err)
	}
	sort.Slice(reg.Plugins, func(i, j int) bool { return reg.Plugins[i].ID < reg.Plugins[j].ID })
	return &reg, nil
}

// save writes the registry atomically.
func (m *Manager) save(reg *registryFile) error {
	reg.Version = 1
	sort.Slice(reg.Plugins, func(i, j int) bool { return reg.Plugins[i].ID < reg.Plugins[j].ID })
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(m.opts.Dir, registryName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("pluginmgr: write registry: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("pluginmgr: write registry: %w", err)
	}
	return nil
}

func find(reg *registryFile, id string) *InstalledPlugin {
	for i := range reg.Plugins {
		if reg.Plugins[i].ID == id {
			p := reg.Plugins[i]
			return &p
		}
	}
	return nil
}

func upsert(reg *registryFile, entry InstalledPlugin) {
	for i := range reg.Plugins {
		if reg.Plugins[i].ID == entry.ID {
			reg.Plugins[i] = entry
			return
		}
	}
	reg.Plugins = append(reg.Plugins, entry)
}
