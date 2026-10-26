package bmr

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeTool writes an executable stub named name into dir.
func fakeTool(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
}

// isolatedPath points PATH at an empty dir so the host's real tooling cannot
// leak into a check and make the result depend on the developer's machine.
func isolatedPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	return dir
}

func TestCheckReportsMissingReaRAsABlocker(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	isolatedPath(t)

	got := Check(context.Background())

	if got.Ready {
		t.Error("host reported ready with no ReaR installed")
	}
	if got.ReaRInstalled {
		t.Error("ReaRInstalled true with an empty PATH")
	}
	if len(got.Blockers) == 0 {
		t.Fatal("no blockers reported for a host that cannot build media")
	}
	if !strings.Contains(strings.Join(got.Blockers, "\n"), "rear") {
		t.Errorf("blockers do not name the missing tool: %v", got.Blockers)
	}
}

func TestCheckReadsReaRVersion(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	dir := isolatedPath(t)
	fakeTool(t, dir, "rear", `echo "Relax-and-Recover 2.7 / 2023-12-05"`)

	got := Check(context.Background())

	if !got.ReaRInstalled {
		t.Fatal("ReaR not detected despite being on PATH")
	}
	if !strings.Contains(got.ReaRVersion, "2.7") {
		t.Errorf("ReaRVersion = %q, want it to carry the reported version", got.ReaRVersion)
	}
	if got.ReaRPath != filepath.Join(dir, "rear") {
		t.Errorf("ReaRPath = %q, want %q", got.ReaRPath, filepath.Join(dir, "rear"))
	}
}

func TestCheckReportsEveryToolNotJustTheFirstMissingOne(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	dir := isolatedPath(t)
	fakeTool(t, dir, "lsblk", "exit 0")

	got := Check(context.Background())

	if len(got.Tools) < 2 {
		t.Fatalf("Check reported %d tools, want the full dependency list", len(got.Tools))
	}
	var sawFound, sawMissing bool
	for _, tool := range got.Tools {
		if tool.Name == "lsblk" {
			sawFound = tool.Found
		}
		if !tool.Found {
			sawMissing = true
		}
		if tool.Purpose == "" {
			t.Errorf("tool %q has no purpose explaining why it is needed", tool.Name)
		}
	}
	if !sawFound {
		t.Error("lsblk not reported as found despite being on PATH")
	}
	if !sawMissing {
		t.Error("no tool reported missing on an otherwise empty PATH")
	}
}

func TestCheckSeparatesOptionalToolsFromBlockers(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery media is Linux-only this phase")
	}
	isolatedPath(t)

	got := Check(context.Background())

	// A missing optional tool degrades the profile rather than stopping the
	// build, so it must land in Warnings, never in Blockers.
	blockers := strings.Join(got.Blockers, "\n")
	for _, tool := range got.Tools {
		if tool.Found || tool.Required {
			continue
		}
		if strings.Contains(blockers, tool.Name) {
			t.Errorf("optional tool %q reported as a blocker", tool.Name)
		}
	}
	if len(got.Warnings) == 0 {
		t.Error("no warnings reported despite missing optional tooling")
	}
}

func TestCheckStampsTheTimeItRan(t *testing.T) {
	got := Check(context.Background())
	if got.CheckedAt.IsZero() {
		t.Error("CheckedAt is zero; a readiness report is only meaningful with its age")
	}
	if got.Platform != runtime.GOOS {
		t.Errorf("Platform = %q, want %q", got.Platform, runtime.GOOS)
	}
}

func TestCheckRefusesNonLinuxHosts(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this asserts the unsupported-platform path")
	}
	got := Check(context.Background())
	if got.Ready {
		t.Errorf("%s reported ready to build Linux recovery media", runtime.GOOS)
	}
	if len(got.Blockers) == 0 {
		t.Error("no blocker explaining that the platform is unsupported")
	}
}
