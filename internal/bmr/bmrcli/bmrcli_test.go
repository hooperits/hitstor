package bmrcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/bmr"
)

// run executes the bmr command tree with the given args and returns what it
// wrote and whether it failed.
func run(t *testing.T, opts Options, args ...string) (string, error) {
	t.Helper()

	cmd := New(opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), err
}

func TestCheckPrintsEveryGapAtOnce(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	t.Setenv("PATH", t.TempDir())

	out, err := run(t, Options{}, "check")

	// An unready host must exit non-zero so the command can gate a
	// provisioning script.
	if err == nil {
		t.Error("check succeeded on a host with no tooling at all")
	}
	for _, want := range []string{"NOT ready", "Blockers:", "rear"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// The whole point is seeing every gap in one pass, not one per run.
	if !strings.Contains(out, "Tools:") {
		t.Errorf("no tool table printed:\n%s", out)
	}
}

func TestCheckJSONIsMachineReadable(t *testing.T) {
	out, err := run(t, Options{JSON: func() bool { return true }}, "check")
	if err != nil && runtime.GOOS == "linux" {
		// Non-zero exit is expected on an unready host; the output still has
		// to parse.
		_ = err
	}

	var readiness bmr.Readiness
	if err := json.Unmarshal([]byte(out), &readiness); err != nil {
		t.Fatalf("check --json did not emit valid JSON: %v\n%s", err, out)
	}
	if readiness.Platform != runtime.GOOS {
		t.Errorf("Platform = %q, want %q", readiness.Platform, runtime.GOOS)
	}
}

func TestGenerateConfigWritesBothArtifacts(t *testing.T) {
	root := t.TempDir()
	confPath := filepath.Join(root, "local.conf")
	hookPath := filepath.Join(root, "recover.sh")

	out, err := run(t, Options{}, "generate-config",
		"--config-path", confPath,
		"--hook-script", hookPath,
		"--output-dir", filepath.Join(root, "media"),
		"--agent-binary", "/usr/bin/hitstor-agent",
		"--server", "backup.example.com:8420",
	)
	if err != nil {
		t.Fatalf("generate-config: %v\n%s", err, out)
	}

	conf, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(conf), "BACKUP=EXTERNAL") {
		t.Errorf("config does not hand recovery to HitStor:\n%s", conf)
	}
	hook, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if !strings.Contains(string(hook), "backup.example.com:8420") {
		t.Errorf("hook does not carry the server address:\n%s", hook)
	}
}

func TestGenerateConfigStdoutWritesNothingToDisk(t *testing.T) {
	root := t.TempDir()
	confPath := filepath.Join(root, "local.conf")

	out, err := run(t, Options{}, "generate-config", "--stdout",
		"--config-path", confPath,
		"--hook-script", filepath.Join(root, "recover.sh"),
		"--output-dir", filepath.Join(root, "media"),
	)
	if err != nil {
		t.Fatalf("generate-config --stdout: %v", err)
	}
	if !strings.Contains(out, "BACKUP=EXTERNAL") {
		t.Errorf("configuration not printed:\n%s", out)
	}
	// --stdout is for reviewing before committing to a file; writing anyway
	// would defeat that.
	if _, err := os.Stat(confPath); !os.IsNotExist(err) {
		t.Error("--stdout wrote the config to disk anyway")
	}
}

func TestGenerateConfigFallsBackToTheServerFromItsHost(t *testing.T) {
	root := t.TempDir()
	hookPath := filepath.Join(root, "recover.sh")

	// hitstor-agent knows its server already; an operator should not have to
	// retype it.
	if _, err := run(t, Options{ServerAddr: func() string { return "configured:8420" }},
		"generate-config",
		"--config-path", filepath.Join(root, "local.conf"),
		"--hook-script", hookPath,
		"--output-dir", filepath.Join(root, "media"),
	); err != nil {
		t.Fatalf("generate-config: %v", err)
	}

	hook, _ := os.ReadFile(hookPath)
	if !strings.Contains(string(hook), "configured:8420") {
		t.Errorf("hook did not pick up the host's configured server:\n%s", hook)
	}
}

func TestGenerateConfigRejectsARelativePath(t *testing.T) {
	root := t.TempDir()

	// The hook is executed inside the rescue system, where a relative path
	// resolves somewhere else or not at all — and only fails once a machine
	// is already dead.
	_, err := run(t, Options{}, "generate-config",
		"--config-path", filepath.Join(root, "local.conf"),
		"--hook-script", "recover.sh",
		"--output-dir", filepath.Join(root, "media"),
	)
	if err == nil {
		t.Fatal("a relative hook path was accepted")
	}
}

func TestBuildMediaReportsBlockersRatherThanRunning(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()

	out, err := run(t, Options{}, "build-media",
		"--config-path", filepath.Join(root, "local.conf"),
		"--hook-script", filepath.Join(root, "recover.sh"),
		"--output-dir", filepath.Join(root, "media"),
	)
	if err == nil {
		t.Fatal("build-media ran on a host with no ReaR installed")
	}
	if !strings.Contains(out, "Blockers:") {
		t.Errorf("blockers not shown before giving up:\n%s", out)
	}
	// Telling the operator the escape hatch exists is the difference between
	// a dead end and a decision.
	if !strings.Contains(out, "--force") {
		t.Errorf("no mention of --force:\n%s", out)
	}
}

func TestCommandTreeExposesTheDocumentedSubcommands(t *testing.T) {
	cmd := New(Options{})

	found := map[string]bool{}
	for _, sub := range cmd.Commands() {
		found[sub.Name()] = true
		if sub.Short == "" {
			t.Errorf("subcommand %q has no description", sub.Name())
		}
	}
	for _, want := range []string{"check", "generate-config", "build-media"} {
		if !found[want] {
			t.Errorf("subcommand %q missing from the bmr command tree", want)
		}
	}
}

func TestBuildMediaStreamsProgressAndNamesTheISO(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}

	root := t.TempDir()
	outDir := filepath.Join(root, "media")
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// A stub ReaR on PATH, so --force reaches the real build path.
	bin := t.TempDir()
	script := "#!/bin/sh\necho 'Creating ISO image'\nprintf '%4096s' '' > " +
		filepath.Join(outDir, "rear-host.iso") + "\n"
	if err := os.WriteFile(filepath.Join(bin, "rear"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	out, err := run(t, Options{}, "build-media", "--force",
		"--config-path", filepath.Join(root, "local.conf"),
		"--hook-script", filepath.Join(root, "recover.sh"),
		"--output-dir", outDir,
		"--agent-binary", "/usr/bin/hitstor-agent",
	)
	if err != nil {
		t.Fatalf("build-media: %v\n%s", err, out)
	}

	// A build runs for minutes; silence is indistinguishable from a hang.
	if !strings.Contains(out, "Creating ISO image") {
		t.Errorf("ReaR output was not streamed:\n%s", out)
	}
	if !strings.Contains(out, "rear-host.iso") {
		t.Errorf("the finished image is not named:\n%s", out)
	}
	if !strings.Contains(out, "KiB") {
		t.Errorf("no human-readable size reported:\n%s", out)
	}
}

func TestBuildMediaJSONEmitsTheMediaRecord(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}

	root := t.TempDir()
	outDir := filepath.Join(root, "media")
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "rear"),
		[]byte("#!/bin/sh\necho noise\nprintf '%512s' '' > "+
			filepath.Join(outDir, "rear-host.iso")+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	out, err := run(t, Options{JSON: func() bool { return true }}, "build-media", "--force",
		"--config-path", filepath.Join(root, "local.conf"),
		"--hook-script", filepath.Join(root, "recover.sh"),
		"--output-dir", outDir,
		"--agent-binary", "/usr/bin/hitstor-agent",
	)
	if err != nil {
		t.Fatalf("build-media --json: %v\n%s", err, out)
	}

	// Progress lines on stdout would corrupt the JSON a script is parsing.
	var media bmr.Media
	if err := json.Unmarshal([]byte(out), &media); err != nil {
		t.Fatalf("build-media --json did not emit valid JSON: %v\n%s", err, out)
	}
	if media.SizeBytes != 512 {
		t.Errorf("SizeBytes = %d, want 512", media.SizeBytes)
	}
	if filepath.Base(media.ISOPath) != "rear-host.iso" {
		t.Errorf("ISOPath = %q", media.ISOPath)
	}
}
