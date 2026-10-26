package bmr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// versionTimeout bounds the `rear -V` probe. A wedged tool must not stall a
// readiness check the dashboard is waiting on, and a `rear -V` that takes
// longer than this is broken whatever it eventually prints.
const versionTimeout = 5 * time.Second

// requiredTools are the commands without which `rear mkrescue` cannot produce
// a bootable image at all.
//
// Optional tools shape how complete the captured layout is; their absence
// degrades the media rather than preventing it, so they surface as warnings.
type toolCheck struct {
	name     string
	required bool
	purpose  string
	// alternatives are other command names that satisfy this dependency. ReaR
	// uses whichever ISO writer it finds, for instance, so the absence of one
	// specific name is not itself interesting.
	alternatives []string
}

var toolChecks = []toolCheck{
	{name: "rear", required: true, purpose: "builds the bootable rescue image and recreates disk layout"},
	{name: "lsblk", required: true, purpose: "reads the partition table and filesystem identities"},
	{name: "tar", required: true, purpose: "packs the rescue root filesystem"},
	{
		name:         "mkisofs",
		required:     true,
		purpose:      "writes the ISO image",
		alternatives: []string{"xorrisofs", "genisoimage", "xorriso"},
	},
	{name: "blkid", purpose: "resolves filesystem UUIDs and labels for /etc/fstab"},
	{name: "efibootmgr", purpose: "records UEFI boot entries so the restored system boots"},
	{name: "lvm", purpose: "captures LVM physical volumes, volume groups and logical volumes"},
	{name: "mdadm", purpose: "captures software RAID arrays"},
}

// Check reports whether this host can build bare-metal recovery media.
//
// It never fails. A readiness report is most useful precisely when things are
// missing, so every gap is collected and returned rather than the first one
// aborting the scan.
func Check(ctx context.Context) Readiness {
	r := Readiness{
		Platform:  runtime.GOOS,
		Root:      os.Geteuid() == 0,
		CheckedAt: time.Now().UTC(),
	}

	if runtime.GOOS != "linux" {
		r.Blockers = append(r.Blockers, fmt.Sprintf(
			"bare-metal recovery media is Linux-only in this release; this host runs %s", runtime.GOOS))
		return r
	}

	for _, tc := range toolChecks {
		status := ToolStatus{Name: tc.name, Required: tc.required, Purpose: tc.purpose}
		if path, found := lookAny(tc.names()); found {
			status.Found = true
			status.Path = path
		}
		r.Tools = append(r.Tools, status)

		switch {
		case status.Found:
		case tc.required:
			r.Blockers = append(r.Blockers, fmt.Sprintf(
				"%s is not installed — %s", tc.describe(), tc.purpose))
		default:
			r.Warnings = append(r.Warnings, fmt.Sprintf(
				"%s is not installed — %s; recovery media will be built without it",
				tc.describe(), tc.purpose))
		}

		if tc.name == "rear" && status.Found {
			r.ReaRInstalled = true
			r.ReaRPath = status.Path
			r.ReaRVersion = rearVersion(ctx, status.Path)
		}
	}

	if !r.Root {
		r.Blockers = append(r.Blockers,
			"must run as root — ReaR reads raw block devices and writes a boot image")
	}

	r.Ready = len(r.Blockers) == 0
	return r
}

// names returns the command names that satisfy this dependency.
func (tc toolCheck) names() []string {
	return append([]string{tc.name}, tc.alternatives...)
}

// describe names the dependency for an operator, listing the alternatives when
// several commands would do.
func (tc toolCheck) describe() string {
	if len(tc.alternatives) == 0 {
		return tc.name
	}
	return fmt.Sprintf("%s (or %s)", tc.name, strings.Join(tc.alternatives, ", "))
}

// lookAny returns the path of the first of names found on PATH.
func lookAny(names []string) (string, bool) {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, true
		}
	}
	return "", false
}

// rearVersion asks ReaR to identify itself. Distro packaging varies enough
// that an unparseable answer is reported verbatim rather than discarded.
func rearVersion(ctx context.Context, path string) string {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "-V").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}
