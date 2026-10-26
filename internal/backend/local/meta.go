package local

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hooperits/hitstor/internal/backend"
)

const indexLatestName = "latest"

func (l *Local) indexLatestPath() string {
	return filepath.Join(l.basePath, "index", indexLatestName)
}

// ReadIndexLatest reads the consolidated index pointer from index/latest.
func (l *Local) ReadIndexLatest() (backend.ID, bool, error) {
	data, err := os.ReadFile(l.indexLatestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return backend.ID{}, false, nil
		}
		return backend.ID{}, false, err
	}
	hexID := strings.TrimSpace(string(data))
	if len(hexID) != 64 {
		return backend.ID{}, false, fmt.Errorf("invalid index latest length: %d", len(hexID))
	}
	raw, err := hex.DecodeString(hexID)
	if err != nil {
		return backend.ID{}, false, fmt.Errorf("decode index latest: %w", err)
	}
	var id backend.ID
	copy(id[:], raw)
	return id, true, nil
}

// WriteIndexLatest records the consolidated index ID in index/latest.
func (l *Local) WriteIndexLatest(id backend.ID) error {
	dir := filepath.Join(l.basePath, "index")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create index dir: %w", err)
	}
	tmp := filepath.Join(dir, ".latest-tmp")
	if err := os.WriteFile(tmp, []byte(id.String()+"\n"), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, l.indexLatestPath())
}
