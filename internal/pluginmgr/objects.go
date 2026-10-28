package pluginmgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// objectsName is the file, under a plugin's directory, holding the settings
// saved for its objects (fields of scope "object"), such as the credentials
// of each database instance. It survives upgrades and goes with Remove. The
// content is sealed with the platform's secret protection (DPAPI on
// Windows) and the store directory is readable only by the agent's account
// and administrators.
const objectsName = "objects.dat"

type objectsFile struct {
	Version int                          `json:"version"`
	Objects map[string]map[string]string `json:"objects"`
}

// ObjectConfigs returns the settings saved for the plugin's objects, keyed
// by object path. Values include secrets.
func (m *Manager) ObjectConfigs(id string) (map[string]map[string]string, error) {
	f, err := m.loadObjects(id)
	if err != nil {
		return nil, err
	}
	return f.Objects, nil
}

// SetObjectConfig saves the settings of one object, replacing what was
// saved; nil values remove them. The plugin must be installed.
func (m *Manager) SetObjectConfig(id, path string, values map[string]string) error {
	if path == "" {
		return errors.New("pluginmgr: object path is empty")
	}
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
	f, err := m.loadObjects(id)
	if err != nil {
		return err
	}
	if values == nil {
		delete(f.Objects, path)
	} else {
		f.Objects[path] = values
	}
	return m.saveObjects(id, f)
}

func (m *Manager) loadObjects(id string) (*objectsFile, error) {
	f := &objectsFile{Version: 1, Objects: map[string]map[string]string{}}
	sealed, err := os.ReadFile(filepath.Join(m.opts.Dir, id, objectsName))
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pluginmgr: read object settings of %s: %w", id, err)
	}
	data, err := unseal(sealed)
	if err != nil {
		return nil, fmt.Errorf("pluginmgr: unseal object settings of %s: %w", id, err)
	}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("pluginmgr: parse object settings of %s: %w", id, err)
	}
	if f.Objects == nil {
		f.Objects = map[string]map[string]string{}
	}
	return f, nil
}

func (m *Manager) saveObjects(id string, f *objectsFile) error {
	f.Version = 1
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	sealed, err := seal(data)
	if err != nil {
		return fmt.Errorf("pluginmgr: seal object settings of %s: %w", id, err)
	}
	path := filepath.Join(m.opts.Dir, id, objectsName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return fmt.Errorf("pluginmgr: write object settings of %s: %w", id, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("pluginmgr: write object settings of %s: %w", id, err)
	}
	return nil
}
