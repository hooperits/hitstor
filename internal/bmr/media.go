package bmr

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// tailLines is how much of ReaR's output an error carries back. Enough to name
// the actual failure, bounded so a chatty build cannot balloon an error string.
const tailLines = 30

// Media describes a built recovery image.
type Media struct {
	ISOPath   string    `json:"iso_path"`
	SizeBytes int64     `json:"size_bytes"`
	BuiltAt   time.Time `json:"built_at"`
}

// BuildOptions controls one `rear mkrescue` run.
type BuildOptions struct {
	// Config is the ReaR configuration to write before building.
	Config ConfigOptions
	// ConfigPath is where that configuration is written. Defaults to
	// /etc/rear/local.conf.
	ConfigPath string

	// ReaRPath overrides the `rear` binary. Empty resolves it from PATH.
	ReaRPath string

	// SkipCheck bypasses the readiness gate. Only for tests and for an
	// operator who has deliberately overridden a blocker.
	SkipCheck bool

	// OnProgress receives ReaR's stdout and stderr line by line as the build
	// runs. A build takes minutes; without this a caller cannot distinguish a
	// slow build from a hung one.
	//
	// Calls are serialized even though stdout and stderr are read
	// concurrently, so a callback need not synchronize itself.
	OnProgress func(line string)
}

// NotReadyError reports that the host cannot build recovery media, carrying
// the full readiness report so a caller can show every gap at once rather than
// making the operator fix them one run at a time.
type NotReadyError struct {
	Readiness Readiness
}

func (e *NotReadyError) Error() string {
	return fmt.Sprintf("bmr: host is not ready to build recovery media: %s",
		strings.Join(e.Readiness.Blockers, "; "))
}

// WriteConfigFiles writes the ReaR configuration and the EXTERNAL_RESTORE hook
// to disk, creating parent directories as needed.
//
// Both files are regenerated on every media build, so writing over an existing
// pair is the normal case rather than a conflict.
func WriteConfigFiles(configPath string, opts ConfigOptions) error {
	opts = opts.withDefaults()

	conf, err := RenderConfig(opts)
	if err != nil {
		return err
	}
	hook, err := RenderHookScript(opts)
	if err != nil {
		return err
	}
	if configPath == "" {
		configPath = DefaultReaRConfig
	}

	// The config names the repository server; the hook is executed by root.
	// Neither should be world-writable.
	if err := writeFile(configPath, []byte(conf), 0o640); err != nil {
		return fmt.Errorf("bmr: write ReaR config: %w", err)
	}
	if err := writeFile(opts.HookScript, []byte(hook), 0o750); err != nil {
		return fmt.Errorf("bmr: write recovery hook: %w", err)
	}
	return nil
}

// writeFile creates parents and writes path with the given mode, replacing any
// existing file (including its mode, which os.WriteFile leaves alone).
func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// BuildMedia writes the ReaR configuration and runs `rear mkrescue`, returning
// the ISO it produced.
//
// The build runs unbounded by design: `rear mkrescue` legitimately takes many
// minutes on a large host. Callers bound it through ctx.
func BuildMedia(ctx context.Context, opts BuildOptions) (Media, error) {
	cfg := opts.Config.withDefaults()

	if !opts.SkipCheck {
		if readiness := Check(ctx); !readiness.Ready {
			return Media{}, &NotReadyError{Readiness: readiness}
		}
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o750); err != nil {
		return Media{}, fmt.Errorf("bmr: create media output directory: %w", err)
	}
	if err := WriteConfigFiles(opts.ConfigPath, cfg); err != nil {
		return Media{}, err
	}

	// Anything already in the output directory is a previous build. Note what
	// was there so a stale image cannot be mistaken for a fresh one.
	before, err := isoSnapshot(cfg.OutputDir)
	if err != nil {
		return Media{}, err
	}

	rearPath := opts.ReaRPath
	if rearPath == "" {
		rearPath, err = exec.LookPath("rear")
		if err != nil {
			return Media{}, fmt.Errorf("bmr: rear not found on PATH: %w", err)
		}
	}

	startedAt := time.Now().UTC()
	tail, err := runReaR(ctx, rearPath, opts.ConfigPath, opts.OnProgress)
	if err != nil {
		if tail != "" {
			return Media{}, fmt.Errorf("bmr: rear mkrescue failed: %w\n%s", err, tail)
		}
		return Media{}, fmt.Errorf("bmr: rear mkrescue failed: %w", err)
	}

	iso, err := newestISO(cfg.OutputDir, before, startedAt)
	if err != nil {
		return Media{}, err
	}
	return iso, nil
}

// runReaR executes the build, forwarding output to onProgress as it arrives
// and returning the tail of that output for use in an error message.
func runReaR(ctx context.Context, rearPath, configPath string, onProgress func(string)) (string, error) {
	args := []string{"-v"}
	// ReaR reads /etc/rear/local.conf by default; -c takes a config directory,
	// so only override when the caller placed the file elsewhere.
	if dir := filepath.Dir(configPath); configPath != "" && configPath != DefaultReaRConfig {
		args = append(args, "-c", dir)
	}
	args = append(args, "mkrescue")

	cmd := exec.CommandContext(ctx, rearPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var (
		mu   sync.Mutex
		last []string
		wg   sync.WaitGroup
	)
	consume := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		// ReaR can emit long device-mapper lines; the default 64K token limit
		// would abort the scan and silently truncate the build log.
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			// One lock covers both the tail buffer and the callback, so a
			// caller never sees two lines at once from the two readers.
			mu.Lock()
			last = append(last, line)
			if len(last) > tailLines {
				last = last[len(last)-tailLines:]
			}
			if onProgress != nil {
				onProgress(line)
			}
			mu.Unlock()
		}
	}
	wg.Add(2)
	go consume(stdout)
	go consume(stderr)
	wg.Wait()

	waitErr := cmd.Wait()

	mu.Lock()
	tail := strings.Join(last, "\n")
	mu.Unlock()
	return tail, waitErr
}

// isoSnapshot records the ISO files present in dir before a build.
func isoSnapshot(dir string) (map[string]bool, error) {
	seen := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return seen, nil
		}
		return nil, fmt.Errorf("bmr: read media output directory: %w", err)
	}
	for _, e := range entries {
		if isISO(e.Name()) {
			seen[e.Name()] = true
		}
	}
	return seen, nil
}

// newestISO finds the image this build produced: a file that was not there
// before, or failing that one modified since the build started.
//
// A ReaR that exits 0 without writing an image is a failed build, not a
// successful build of nothing — reporting otherwise hands an operator media
// that does not exist.
func newestISO(dir string, before map[string]bool, startedAt time.Time) (Media, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Media{}, fmt.Errorf("bmr: read media output directory: %w", err)
	}

	type candidate struct {
		name    string
		info    os.FileInfo
		modTime time.Time
	}
	var candidates []candidate
	for _, e := range entries {
		if e.IsDir() || !isISO(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fresh := !before[e.Name()]
		// A rebuild overwrites the previous image in place, so an existing
		// name with a new mtime counts too.
		if !fresh && info.ModTime().Before(startedAt.Add(-time.Second)) {
			continue
		}
		candidates = append(candidates, candidate{e.Name(), info, info.ModTime()})
	}
	if len(candidates) == 0 {
		return Media{}, fmt.Errorf(
			"bmr: rear reported success but produced no ISO in %s", dir)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	winner := candidates[0]

	return Media{
		ISOPath:   filepath.Join(dir, winner.name),
		SizeBytes: winner.info.Size(),
		BuiltAt:   winner.info.ModTime().UTC(),
	}, nil
}

func isISO(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".iso")
}
