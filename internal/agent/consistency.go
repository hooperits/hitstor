package agent

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// ConsistencyMethod identifies how a consistent read view was obtained.
type ConsistencyMethod string

const (
	ConsistencyNone  ConsistencyMethod = "none"
	ConsistencyVSS   ConsistencyMethod = "vss"
	ConsistencyLVM   ConsistencyMethod = "lvm"
	ConsistencyBtrfs ConsistencyMethod = "btrfs"
)

// ConsistencySession holds translated read paths for a consistent backup and
// resources that must be cleaned up when the backup finishes.
type ConsistencySession struct {
	Method        ConsistencyMethod
	OriginalPaths []string
	ReadPaths     []string // paths the scanner/walker should use
	// reverseMaps original absolute path prefix → snapshot absolute prefix.
	// Used only for diagnostics; metadata paths are derived from OriginalPaths.
	snapshotRoots []snapshotRoot
	cleanup       []func()
}

type snapshotRoot struct {
	// originalRoot is the filesystem root that was snapshotted (volume, LV mount, subvol).
	originalRoot string
	// snapshotRoot is where the consistent view is mounted/accessible.
	snapshotRoot string
}

// Close releases snapshot resources (unmount, delete LV/subvol/VSS). Safe to call multiple times.
func (s *ConsistencySession) Close() {
	if s == nil {
		return
	}
	// Reverse order (LIFO).
	for i := len(s.cleanup) - 1; i >= 0; i-- {
		if s.cleanup[i] != nil {
			s.cleanup[i]()
		}
	}
	s.cleanup = nil
}

// RelativePath returns the path that should be stored in snapshot metadata for
// a file found under ReadPaths, using the original backup roots so restores
// land on the live filesystem layout (not VSS/LVM device paths).
func (s *ConsistencySession) RelativePath(absReadPath string) string {
	if s == nil {
		return makeRelativePath(absReadPath, nil)
	}
	// Map snapshot read path back to the original absolute path first.
	orig := s.OriginalAbs(absReadPath)
	return makeRelativePath(orig, s.OriginalPaths)
}

// OriginalAbs maps a path under a snapshot root back to the live filesystem path.
func (s *ConsistencySession) OriginalAbs(absReadPath string) string {
	if s == nil || len(s.snapshotRoots) == 0 {
		return absReadPath
	}
	clean := filepath.Clean(absReadPath)
	for _, root := range s.snapshotRoots {
		snap := filepath.Clean(root.snapshotRoot)
		orig := filepath.Clean(root.originalRoot)
		if clean == snap {
			return orig
		}
		prefix := snap + string(filepath.Separator)
		if strings.HasPrefix(clean, prefix) {
			return filepath.Join(orig, clean[len(prefix):])
		}
	}
	return absReadPath
}

// PrepareConsistency creates a consistent read view of backupPaths when enabled.
// On failure it returns a none-session with the original paths and a non-nil error
// so callers can log and fall back to live reads.
//
// UseSnapshots is the job UseVSS flag: on Windows it means VSS; on Linux it means
// LVM/btrfs when available.
func PrepareConsistency(backupPaths []string, useSnapshots bool) (*ConsistencySession, error) {
	sess := &ConsistencySession{
		Method:        ConsistencyNone,
		OriginalPaths: append([]string(nil), backupPaths...),
		ReadPaths:     append([]string(nil), backupPaths...),
	}
	if !useSnapshots || len(backupPaths) == 0 {
		return sess, nil
	}
	return prepareConsistencyPlatform(sess, backupPaths)
}

// cmdRunner allows tests to stub external commands (PowerShell, lvs, btrfs, …).
type cmdRunner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
}

// realCmdRunner is the production runner.
type realCmdRunner struct{}

func (realCmdRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	return runCommand(name, args...)
}

// activeRunner is swapped in tests.
var activeRunner cmdRunner = realCmdRunner{}

func setCmdRunner(r cmdRunner) (restore func()) {
	prev := activeRunner
	activeRunner = r
	return func() { activeRunner = prev }
}

// translatePathUnderRoot rewrites absPath from under originalRoot to under snapshotRoot.
func translatePathUnderRoot(absPath, originalRoot, snapshotRoot string) string {
	clean := filepath.Clean(absPath)
	orig := filepath.Clean(originalRoot)
	snap := filepath.Clean(snapshotRoot)
	if clean == orig {
		return snap
	}
	prefix := orig + string(filepath.Separator)
	if strings.HasPrefix(clean, prefix) {
		return filepath.Join(snap, clean[len(prefix):])
	}
	return absPath
}

// normalizeVolumeRoot ensures Windows volume roots end with a separator.
func normalizeVolumeRoot(vol string) string {
	if vol == "" {
		return ""
	}
	vol = strings.ToUpper(vol)
	if !strings.HasSuffix(vol, `\`) && !strings.HasSuffix(vol, `/`) {
		vol += `\`
	}
	return vol
}

func logConsistency(method ConsistencyMethod, paths []string, detail string) {
	slog.Info("consistency snapshot ready",
		"method", method,
		"paths", paths,
		"detail", detail,
	)
}

func fmtJoined(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		if e != nil {
			parts = append(parts, e.Error())
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}
