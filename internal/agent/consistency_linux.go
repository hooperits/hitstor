//go:build linux

package agent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type mountInfo struct {
	target string
	source string
	fstype string
}

func prepareConsistencyPlatform(sess *ConsistencySession, backupPaths []string) (*ConsistencySession, error) {
	byMount := make(map[string]mountInfo)
	for _, p := range backupPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		mi, err := lookupMount(abs)
		if err != nil {
			slog.Warn("consistency: mount lookup failed", "path", abs, "error", err)
			continue
		}
		byMount[mi.target] = mi
	}
	if len(byMount) == 0 {
		return sess, fmt.Errorf("consistency: could not resolve mounts for paths")
	}

	readMap := make(map[string]string) // original abs → snapshot abs
	var errs []error
	used := ConsistencyNone

	for _, mi := range byMount {
		var (
			snapRoot string
			cleanup  func()
			method   ConsistencyMethod
			err      error
		)
		switch {
		case mi.fstype == "btrfs":
			snapRoot, cleanup, err = createBtrfsSnapshot(mi)
			method = ConsistencyBtrfs
		case looksLikeLVM(mi.source):
			snapRoot, cleanup, err = createLVMSnapshot(mi)
			method = ConsistencyLVM
		default:
			slog.Info("consistency: no snapshot provider for mount",
				"mount", mi.target, "fstype", mi.fstype, "source", mi.source)
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s on %s: %w", method, mi.target, err))
			if cleanup != nil {
				cleanup()
			}
			continue
		}
		if cleanup != nil {
			sess.cleanup = append(sess.cleanup, cleanup)
		}
		sess.snapshotRoots = append(sess.snapshotRoots, snapshotRoot{
			originalRoot: mi.target,
			snapshotRoot: snapRoot,
		})
		used = method
		for _, p := range backupPaths {
			abs, aerr := filepath.Abs(p)
			if aerr != nil {
				abs = p
			}
			if abs == mi.target || strings.HasPrefix(abs, mi.target+string(filepath.Separator)) {
				readMap[abs] = translatePathUnderRoot(abs, mi.target, snapRoot)
			}
		}
		logConsistency(method, []string{mi.target}, snapRoot)
	}

	if len(sess.snapshotRoots) == 0 {
		if err := fmtJoined(errs); err != nil {
			return sess, err
		}
		return sess, fmt.Errorf("consistency: no LVM or btrfs snapshot could be created (need root + lvm2/btrfs-progs)")
	}

	readPaths := make([]string, 0, len(backupPaths))
	for _, p := range backupPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if rp, ok := readMap[abs]; ok {
			readPaths = append(readPaths, rp)
		} else {
			readPaths = append(readPaths, p)
			slog.Warn("consistency: path not under snapshotted mount, using live path", "path", p)
		}
	}
	sess.ReadPaths = readPaths
	sess.Method = used
	if len(errs) > 0 {
		slog.Warn("consistency: partial snapshot success", "errors", fmtJoined(errs))
	}
	return sess, nil
}

func lookupMount(path string) (mountInfo, error) {
	// findmnt -n -T PATH -o TARGET,SOURCE,FSTYPE
	out, err := activeRunner.CombinedOutput("findmnt", "-n", "-T", path, "-o", "TARGET,SOURCE,FSTYPE")
	if err != nil {
		return mountInfo{}, fmt.Errorf("findmnt: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	line := strings.TrimSpace(string(out))
	// Split from the right: FSTYPE is last field, SOURCE second-to-last.
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return mountInfo{}, fmt.Errorf("findmnt unexpected output: %q", line)
	}
	fstype := fields[len(fields)-1]
	source := fields[len(fields)-2]
	target := strings.Join(fields[:len(fields)-2], " ")
	return mountInfo{target: target, source: source, fstype: fstype}, nil
}

func looksLikeLVM(source string) bool {
	s := strings.ToLower(source)
	if strings.HasPrefix(s, "/dev/mapper/") {
		return true
	}
	if strings.HasPrefix(s, "/dev/") {
		rest := strings.TrimPrefix(s, "/dev/")
		// /dev/vg/lv → one slash in rest
		return strings.Count(rest, "/") == 1 && !strings.HasPrefix(rest, "disk/") && !strings.HasPrefix(rest, "sd")
	}
	return false
}

func createBtrfsSnapshot(mi mountInfo) (snapRoot string, cleanup func(), err error) {
	// Snapshot the mount target as a read-only subvolume sibling when possible.
	// Prefer creating under the mount: <mount>/.hitstor-snap-<ts>
	// If that fails (not a subvolume root), try the subvolume path from btrfs subvolume show.
	ts := time.Now().UnixNano()
	name := fmt.Sprintf(".hitstor-snap-%d", ts)
	dest := filepath.Join(mi.target, name)

	out, err := activeRunner.CombinedOutput("btrfs", "subvolume", "snapshot", "-r", mi.target, dest)
	if err != nil {
		return "", nil, fmt.Errorf("btrfs snapshot: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	cleanup = func() {
		slog.Info("deleting btrfs snapshot", "path", dest)
		if out, err := activeRunner.CombinedOutput("btrfs", "subvolume", "delete", dest); err != nil {
			slog.Warn("btrfs subvolume delete failed", "path", dest, "error", err, "output", strings.TrimSpace(string(out)))
		}
	}
	return dest, cleanup, nil
}

func createLVMSnapshot(mi mountInfo) (snapRoot string, cleanup func(), err error) {
	vg, lv, err := parseLVMPath(mi.source)
	if err != nil {
		return "", nil, err
	}
	snapName := fmt.Sprintf("hitstor-%s-%d", sanitizeName(lv), time.Now().UnixNano()%1_000_000_000)
	// Size: 1G COW area is a practical default for short-lived backup snapshots.
	out, err := activeRunner.CombinedOutput("lvcreate", "--snapshot", "-L", "1G", "-n", snapName, vg+"/"+lv)
	if err != nil {
		// Thin pools often want -l 100%ORIGIN instead of fixed size.
		out2, err2 := activeRunner.CombinedOutput("lvcreate", "--snapshot", "-l", "100%ORIGIN", "-n", snapName, vg+"/"+lv)
		if err2 != nil {
			return "", nil, fmt.Errorf("lvcreate snapshot: %w (%s); retry: %v (%s)",
				err, strings.TrimSpace(string(out)), err2, strings.TrimSpace(string(out2)))
		}
	}

	dev := filepath.Join("/dev", vg, snapName)
	if _, statErr := os.Stat(dev); statErr != nil {
		// Device mapper name often uses vg-lv form.
		alt := filepath.Join("/dev/mapper", vg+"-"+snapName)
		if _, err2 := os.Stat(alt); err2 == nil {
			dev = alt
		}
	}

	mnt, err := os.MkdirTemp("", "hitstor-lvm-*")
	if err != nil {
		_, _ = activeRunner.CombinedOutput("lvremove", "-f", vg+"/"+snapName)
		return "", nil, fmt.Errorf("mkdir mount: %w", err)
	}

	out, err = activeRunner.CombinedOutput("mount", "-o", "ro", dev, mnt)
	if err != nil {
		_, _ = activeRunner.CombinedOutput("lvremove", "-f", vg+"/"+snapName)
		_ = os.RemoveAll(mnt)
		return "", nil, fmt.Errorf("mount snapshot: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	cleanup = func() {
		slog.Info("removing LVM snapshot", "vg", vg, "lv", snapName, "mount", mnt)
		if out, err := activeRunner.CombinedOutput("umount", mnt); err != nil {
			slog.Warn("umount failed", "mount", mnt, "error", err, "output", strings.TrimSpace(string(out)))
		}
		if out, err := activeRunner.CombinedOutput("lvremove", "-f", vg+"/"+snapName); err != nil {
			slog.Warn("lvremove failed", "lv", vg+"/"+snapName, "error", err, "output", strings.TrimSpace(string(out)))
		}
		_ = os.RemoveAll(mnt)
	}
	return mnt, cleanup, nil
}

func parseLVMPath(source string) (vg, lv string, err error) {
	// /dev/mapper/vg-lv  or  /dev/vg/lv
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "/dev/mapper/") {
		name := strings.TrimPrefix(source, "/dev/mapper/")
		// vg-lv with hyphens doubled for literal hyphens in names — keep simple split on first '-'.
		i := strings.Index(name, "-")
		if i <= 0 || i == len(name)-1 {
			return "", "", fmt.Errorf("cannot parse mapper name %q", name)
		}
		return name[:i], name[i+1:], nil
	}
	if strings.HasPrefix(source, "/dev/") {
		rest := strings.TrimPrefix(source, "/dev/")
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], nil
		}
	}
	return "", "", fmt.Errorf("not an LVM device path: %s", source)
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 32 {
		out = out[:32]
	}
	if out == "" {
		return "snap"
	}
	return out
}
