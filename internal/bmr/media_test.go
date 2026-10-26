package bmr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteConfigFilesPlacesBothArtifacts(t *testing.T) {
	root := t.TempDir()
	opts := ConfigOptions{
		OutputDir:   filepath.Join(root, "media"),
		AgentBinary: "/usr/bin/hitstor-agent",
		HookScript:  filepath.Join(root, "hook", "recover.sh"),
		ServerAddr:  "srv:8420",
	}
	confPath := filepath.Join(root, "etc", "rear", "local.conf")

	if err := WriteConfigFiles(confPath, opts); err != nil {
		t.Fatalf("WriteConfigFiles: %v", err)
	}

	conf, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(conf), "BACKUP=EXTERNAL") {
		t.Errorf("config not written as expected:\n%s", conf)
	}

	info, err := os.Stat(opts.HookScript)
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	// ReaR execs the hook directly; a non-executable file fails the recovery
	// run at the worst possible moment.
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("hook script mode %v is not executable", info.Mode().Perm())
	}
}

func TestWriteConfigFilesIsRerunnable(t *testing.T) {
	root := t.TempDir()
	opts := ConfigOptions{
		OutputDir:   filepath.Join(root, "media"),
		AgentBinary: "/usr/bin/hitstor-agent",
		HookScript:  filepath.Join(root, "recover.sh"),
	}
	confPath := filepath.Join(root, "local.conf")

	// Every media build regenerates these, so a second run must overwrite
	// cleanly rather than fail on an existing file.
	for i := 0; i < 2; i++ {
		if err := WriteConfigFiles(confPath, opts); err != nil {
			t.Fatalf("WriteConfigFiles run %d: %v", i+1, err)
		}
	}
}

func TestBuildMediaRefusesWhenTheHostIsNotReady(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	t.Setenv("PATH", t.TempDir())

	_, err := BuildMedia(context.Background(), BuildOptions{
		Config:     ConfigOptions{OutputDir: t.TempDir()},
		ConfigPath: filepath.Join(t.TempDir(), "local.conf"),
	})
	if err == nil {
		t.Fatal("BuildMedia ran on a host with no ReaR installed")
	}
	var notReady *NotReadyError
	if !errors.As(err, &notReady) {
		t.Fatalf("error %v is not a *NotReadyError; callers cannot tell a readiness gap from a build failure", err)
	}
	if len(notReady.Readiness.Blockers) == 0 {
		t.Error("NotReadyError carries no blockers to show the operator")
	}
}

func TestBuildMediaStreamsProgressAndReportsTheISO(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	root := t.TempDir()
	outDir := filepath.Join(root, "media")
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		t.Fatal(err)
	}
	iso := filepath.Join(outDir, "rear-testhost.iso")

	bin := t.TempDir()
	fakeTool(t, bin, "rear", "echo 'Creating ISO image'\n"+
		"echo 'Wrote ISO image' >&2\n"+
		"printf 'iso-bytes' > "+iso)

	var lines []string
	got, err := BuildMedia(context.Background(), BuildOptions{
		Config:     ConfigOptions{OutputDir: outDir, AgentBinary: "/usr/bin/hitstor-agent", HookScript: filepath.Join(root, "recover.sh")},
		ConfigPath: filepath.Join(root, "local.conf"),
		ReaRPath:   filepath.Join(bin, "rear"),
		SkipCheck:  true,
		OnProgress: func(line string) { lines = append(lines, line) },
	})
	if err != nil {
		t.Fatalf("BuildMedia: %v", err)
	}
	if got.ISOPath != iso {
		t.Errorf("ISOPath = %q, want %q", got.ISOPath, iso)
	}
	if got.SizeBytes != int64(len("iso-bytes")) {
		t.Errorf("SizeBytes = %d, want %d", got.SizeBytes, len("iso-bytes"))
	}
	if got.BuiltAt.IsZero() {
		t.Error("BuiltAt is zero; the dashboard shows media age from it")
	}

	// A build takes minutes. Without streamed output an operator watching the
	// dashboard cannot tell a slow build from a hung one.
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Creating ISO image") {
		t.Errorf("stdout not streamed to OnProgress, got: %v", lines)
	}
	if !strings.Contains(joined, "Wrote ISO image") {
		t.Errorf("stderr not streamed to OnProgress, got: %v", lines)
	}
}

func TestBuildMediaSurfacesReaRFailureOutput(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	root := t.TempDir()
	bin := t.TempDir()
	fakeTool(t, bin, "rear", "echo 'ERROR: no bootloader found' >&2\nexit 1")

	_, err := BuildMedia(context.Background(), BuildOptions{
		Config:     ConfigOptions{OutputDir: filepath.Join(root, "media"), AgentBinary: "/usr/bin/hitstor-agent", HookScript: filepath.Join(root, "recover.sh")},
		ConfigPath: filepath.Join(root, "local.conf"),
		ReaRPath:   filepath.Join(bin, "rear"),
		SkipCheck:  true,
	})
	if err == nil {
		t.Fatal("BuildMedia reported success for a failing rear run")
	}
	// "exit status 1" alone sends an operator digging through logs on a host
	// they may be about to lose.
	if !strings.Contains(err.Error(), "no bootloader found") {
		t.Errorf("error does not carry ReaR's own diagnosis: %v", err)
	}
}

func TestBuildMediaFailsWhenNoISOAppears(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	root := t.TempDir()
	bin := t.TempDir()
	// A ReaR that exits 0 without producing an image would otherwise be
	// reported as a successful build of nothing.
	fakeTool(t, bin, "rear", "exit 0")

	_, err := BuildMedia(context.Background(), BuildOptions{
		Config:     ConfigOptions{OutputDir: filepath.Join(root, "media"), AgentBinary: "/usr/bin/hitstor-agent", HookScript: filepath.Join(root, "recover.sh")},
		ConfigPath: filepath.Join(root, "local.conf"),
		ReaRPath:   filepath.Join(bin, "rear"),
		SkipCheck:  true,
	})
	if err == nil {
		t.Fatal("BuildMedia reported success with no ISO on disk")
	}
}
