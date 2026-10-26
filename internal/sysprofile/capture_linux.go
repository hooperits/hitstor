//go:build linux

package sysprofile

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// captureTimeout bounds each external command. A wedged storage tool must not
// stall a backup.
const captureTimeout = 15 * time.Second

// Capture builds a Profile for the running host.
//
// It never fails on a missing tool or unreadable file: each input that cannot
// be collected is recorded in Profile.Warnings and its section is left empty.
// A partial profile is far more useful at recovery time than none.
func Capture(ctx context.Context) (*Profile, error) {
	profile := &Profile{
		CapturedAt: time.Now().UTC(),
		Arch:       runtime.GOARCH,
	}

	if hostname, err := os.Hostname(); err == nil {
		profile.Hostname = hostname
	}

	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		profile.OS = parseOSRelease(data)
	} else {
		profile.warn("read /etc/os-release: %v", err)
	}

	if out, err := runCommand(ctx, "uname", "-r"); err == nil {
		profile.Kernel = strings.TrimSpace(string(out))
	} else {
		profile.warn("uname -r: %v", err)
	}

	profile.captureDisks(ctx)
	profile.captureLVM(ctx)
	profile.captureRAID()
	profile.captureMounts()
	profile.captureBoot(ctx)

	return profile, nil
}

// captureDisks records the block-device tree.
func (p *Profile) captureDisks(ctx context.Context) {
	out, err := runCommand(ctx, "lsblk", "-J", "-b", "-o",
		"NAME,PATH,TYPE,SIZE,FSTYPE,UUID,LABEL,MOUNTPOINT,PARTUUID")
	if err != nil {
		p.warn("lsblk: %v", err)
		return
	}
	disks, err := parseLsblk(out)
	if err != nil {
		p.warn("%v", err)
		return
	}
	p.Disks = disks
}

// captureLVM records the LVM stack. A host without LVM reports nothing, which
// is not a warning.
func (p *Profile) captureLVM(ctx context.Context) {
	layout := &LVMLayout{}

	if out, err := runCommand(ctx, "pvs", "--reportformat", "json", "--units", "b",
		"-o", "pv_name,pv_uuid,vg_name,pv_size"); err == nil {
		if pvs, perr := parsePVReport(out); perr == nil {
			layout.PhysicalVolumes = pvs
		} else {
			p.warn("%v", perr)
		}
	}

	if out, err := runCommand(ctx, "vgs", "--reportformat", "json", "--units", "b",
		"-o", "vg_name,vg_uuid,vg_size,vg_extent_size"); err == nil {
		if vgs, verr := parseVGReport(out); verr == nil {
			layout.VolumeGroups = vgs
		} else {
			p.warn("%v", verr)
		}
	}

	if out, err := runCommand(ctx, "lvs", "--reportformat", "json", "--units", "b",
		"-o", "lv_name,vg_name,lv_uuid,lv_size,lv_path"); err == nil {
		if lvs, lerr := parseLVReport(out); lerr == nil {
			layout.LogicalVolumes = lvs
		} else {
			p.warn("%v", lerr)
		}
	}

	if len(layout.PhysicalVolumes) > 0 || len(layout.VolumeGroups) > 0 || len(layout.LogicalVolumes) > 0 {
		p.LVM = layout
	}
}

// captureRAID records software RAID arrays. /proc/mdstat is absent on hosts
// without the md driver, which is normal.
func (p *Profile) captureRAID() {
	data, err := os.ReadFile("/proc/mdstat")
	if err != nil {
		return
	}
	p.RAID = parseMdstat(data)
}

// captureMounts records the persistent mount table.
func (p *Profile) captureMounts() {
	data, err := os.ReadFile("/etc/fstab")
	if err != nil {
		p.warn("read /etc/fstab: %v", err)
		return
	}
	p.Mounts = parseFstab(data)
}

// captureBoot records firmware type, the ESP, and the detected bootloader.
func (p *Profile) captureBoot(ctx context.Context) {
	// The presence of efivars is the reliable UEFI signal.
	if _, err := os.Stat("/sys/firmware/efi"); err == nil {
		p.Boot.Firmware = "uefi"
	} else {
		p.Boot.Firmware = "bios"
	}

	p.Boot.ESPMountPoint = p.findESPMountPoint()
	p.Boot.Bootloader = detectBootloader()

	if p.Boot.Firmware != "uefi" {
		return
	}
	out, err := runCommand(ctx, "efibootmgr")
	if err != nil {
		// efibootmgr is frequently absent; note it without alarming.
		p.warn("efibootmgr: %v", err)
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Boot0") {
			p.Boot.EFIEntries = append(p.Boot.EFIEntries, strings.TrimSpace(line))
		}
	}
}

// findESPMountPoint locates the EFI system partition in the captured disk tree.
func (p *Profile) findESPMountPoint() string {
	var search func(devs []BlockDevice) string
	search = func(devs []BlockDevice) string {
		for _, d := range devs {
			if d.FSType == "vfat" && (d.MountPoint == "/boot/efi" || d.MountPoint == "/efi") {
				return d.MountPoint
			}
			if found := search(d.Children); found != "" {
				return found
			}
		}
		return ""
	}
	return search(p.Disks)
}

// detectBootloader identifies the installed bootloader from its on-disk state.
func detectBootloader() string {
	for _, path := range []string{"/boot/grub2/grub.cfg", "/boot/grub/grub.cfg"} {
		if _, err := os.Stat(path); err == nil {
			return "grub"
		}
	}
	if _, err := os.Stat("/boot/loader/entries"); err == nil {
		return "systemd-boot"
	}
	return ""
}

// runCommand executes name with args under a timeout, returning stdout.
func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(name); err != nil {
		return nil, fmt.Errorf("%s not available: %w", name, err)
	}

	ctx, cancel := context.WithTimeout(ctx, captureTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", name, err)
	}
	return out, nil
}

// warn appends a formatted warning to the profile.
func (p *Profile) warn(format string, args ...any) {
	p.Warnings = append(p.Warnings, fmt.Sprintf(format, args...))
}
