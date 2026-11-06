// Package pluginpkg reads, writes, signs, and verifies HitStor plugin packages
// (.hpk files).
//
// A package is a zip archive holding a plugin.yaml manifest, the plugin
// executable, optional documentation, and an optional detached ed25519
// SIGNATURE over a canonical digest of every other entry.
package pluginpkg

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Package file layout.
const (
	Extension     = ".hpk"
	ManifestName  = "plugin.yaml"
	SignatureName = "SIGNATURE"
)

// TypeWorkload is the only installable plugin type today.
const TypeWorkload = "workload"

var (
	idPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	versionPattern  = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	platformPattern = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+$`)
)

// Manifest is the plugin.yaml at the root of a package.
type Manifest struct {
	ID              string   `yaml:"id" json:"id"`
	Name            string   `yaml:"name" json:"name"`
	Version         string   `yaml:"version" json:"version"`
	Type            string   `yaml:"type" json:"type"`
	ProtocolVersion int      `yaml:"protocol_version" json:"protocol_version"`
	MinAgentVersion string   `yaml:"min_agent_version,omitempty" json:"min_agent_version,omitempty"`
	Platforms       []string `yaml:"platforms" json:"platforms"`
	Entrypoint      string   `yaml:"entrypoint" json:"entrypoint"`
	Publisher       string   `yaml:"publisher,omitempty" json:"publisher,omitempty"`
	Description     string   `yaml:"description,omitempty" json:"description,omitempty"`
}

// ParseManifest decodes and validates a plugin.yaml. Unknown fields are
// rejected so a typo cannot silently drop a constraint.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("pluginpkg: parse %s: %w", ManifestName, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks every manifest field.
func (m *Manifest) Validate() error {
	switch {
	case !idPattern.MatchString(m.ID):
		return fmt.Errorf("pluginpkg: invalid id %q (lowercase letters, digits and dashes)", m.ID)
	case strings.TrimSpace(m.Name) == "":
		return fmt.Errorf("pluginpkg: name is required")
	case !versionPattern.MatchString(m.Version):
		return fmt.Errorf("pluginpkg: invalid version %q (expected semver, e.g. 1.2.3)", m.Version)
	case m.Type != TypeWorkload:
		return fmt.Errorf("pluginpkg: unsupported type %q (want %q)", m.Type, TypeWorkload)
	case m.ProtocolVersion < 1:
		return fmt.Errorf("pluginpkg: protocol_version must be >= 1")
	case m.MinAgentVersion != "" && !versionPattern.MatchString(strings.TrimPrefix(m.MinAgentVersion, "v")):
		return fmt.Errorf("pluginpkg: invalid min_agent_version %q", m.MinAgentVersion)
	case len(m.Platforms) == 0:
		return fmt.Errorf("pluginpkg: at least one platform is required")
	}
	for _, p := range m.Platforms {
		if !platformPattern.MatchString(p) {
			return fmt.Errorf("pluginpkg: invalid platform %q (expected os/arch)", p)
		}
	}
	if err := validEntryName(m.Entrypoint); err != nil {
		return fmt.Errorf("pluginpkg: entrypoint: %w", err)
	}
	if m.Entrypoint == ManifestName || m.Entrypoint == SignatureName {
		return fmt.Errorf("pluginpkg: entrypoint cannot be %s", m.Entrypoint)
	}
	return nil
}

// SupportsPlatform reports whether the package targets goos/goarch.
func (m *Manifest) SupportsPlatform(goos, goarch string) bool {
	want := goos + "/" + goarch
	for _, p := range m.Platforms {
		if p == want {
			return true
		}
	}
	return false
}

// validEntryName accepts clean, relative, slash-separated paths that stay
// inside the package root.
func validEntryName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("empty path")
	case strings.Contains(name, `\`):
		return fmt.Errorf("%q: backslashes are not allowed", name)
	case strings.HasPrefix(name, "/") || (len(name) > 1 && name[1] == ':'):
		return fmt.Errorf("%q: absolute paths are not allowed", name)
	case path.Clean(name) != name:
		return fmt.Errorf("%q: path is not clean", name)
	case name == ".." || strings.HasPrefix(name, "../"):
		return fmt.Errorf("%q: path escapes the package", name)
	}
	return nil
}
