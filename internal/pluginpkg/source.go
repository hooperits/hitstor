package pluginpkg

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseSource decodes the plugin.yaml kept next to a plugin's source code.
// A source manifest omits platforms and entrypoint: packaging fills them in
// for each target, so one source manifest yields one package per platform.
// Plugin binaries embed it to report their own id and version.
func ParseSource(data []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("pluginpkg: parse source manifest: %w", err)
	}
	if len(m.Platforms) > 0 || m.Entrypoint != "" {
		return nil, fmt.Errorf("pluginpkg: source manifest must not set platforms or entrypoint")
	}
	return &m, nil
}

// EntrypointFor returns the conventional entrypoint of plugin id on goos.
func EntrypointFor(id, goos string) string {
	name := "bin/hitstor-plugin-" + id
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// PackageName returns the conventional package file name.
func PackageName(id, version, goos, goarch string) string {
	return fmt.Sprintf("hitstor-plugin-%s-%s-%s-%s%s", id, version, goos, goarch, Extension)
}

// ForPlatform returns the package manifest for one target, validated.
func (m Manifest) ForPlatform(goos, goarch string) ([]byte, error) {
	m.Platforms = []string{goos + "/" + goarch}
	m.Entrypoint = EntrypointFor(m.ID, goos)
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return yaml.Marshal(&m)
}
