// Package sysprofile captures the system layout a host needs in order to be
// rebuilt from a filesystem backup: disks and partitions, filesystem
// identities, LVM and software RAID, the mount table, and bootloader state.
//
// A file-level backup restores the contents of a machine but not its shape.
// Without the shape, a restore onto a blank disk produces an unbootable system:
// no partition table, no filesystem UUIDs for /etc/fstab to resolve, and no
// bootloader. Storing the profile alongside the snapshot makes the backup
// self-describing, so layout can be reconstructed even if the rescue media is
// lost.
//
// Capture is best-effort by design. Every input is optional; a missing tool or
// an unreadable file degrades that section to empty and records a warning
// rather than failing the backup.
package sysprofile

import "time"

// Profile is a point-in-time description of a host's storage and boot layout.
type Profile struct {
	CapturedAt time.Time `json:"captured_at"`
	Hostname   string    `json:"hostname,omitempty"`
	OS         OSRelease `json:"os"`
	Kernel     string    `json:"kernel,omitempty"`
	Arch       string    `json:"arch,omitempty"`

	Disks  []BlockDevice `json:"disks,omitempty"`
	LVM    *LVMLayout    `json:"lvm,omitempty"`
	RAID   []MDArray     `json:"raid,omitempty"`
	Mounts []FstabEntry  `json:"mounts,omitempty"`
	Boot   BootInfo      `json:"boot"`

	// Warnings records inputs that could not be captured, so an operator can
	// see what a recovery plan will be missing before they need it.
	Warnings []string `json:"warnings,omitempty"`
}

// BlockDevice is one node of the block-device tree: a disk, a partition, or a
// mapped device such as an LVM logical volume.
type BlockDevice struct {
	Name       string `json:"name"`
	Path       string `json:"path,omitempty"`
	Type       string `json:"type,omitempty"`
	Size       int64  `json:"size,omitempty"`
	FSType     string `json:"fstype,omitempty"`
	UUID       string `json:"uuid,omitempty"`
	Label      string `json:"label,omitempty"`
	MountPoint string `json:"mountpoint,omitempty"`
	// PartUUID identifies the partition itself rather than the filesystem on
	// it. Recreated fstab entries and bootloader configuration resolve against
	// this, so it must survive into the profile.
	PartUUID string `json:"partuuid,omitempty"`

	Children []BlockDevice `json:"children,omitempty"`
}

// LVMLayout describes the LVM stack: physical volumes, volume groups, and
// logical volumes.
type LVMLayout struct {
	PhysicalVolumes []PhysicalVolume `json:"physical_volumes,omitempty"`
	VolumeGroups    []VolumeGroup    `json:"volume_groups,omitempty"`
	LogicalVolumes  []LogicalVolume  `json:"logical_volumes,omitempty"`
}

// PhysicalVolume is an LVM PV.
type PhysicalVolume struct {
	Name      string `json:"name"`
	UUID      string `json:"uuid,omitempty"`
	VGName    string `json:"vg_name,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// VolumeGroup is an LVM VG.
type VolumeGroup struct {
	Name            string `json:"name"`
	UUID            string `json:"uuid,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	ExtentSizeBytes int64  `json:"extent_size_bytes,omitempty"`
}

// LogicalVolume is an LVM LV.
type LogicalVolume struct {
	Name      string `json:"name"`
	VGName    string `json:"vg_name,omitempty"`
	UUID      string `json:"uuid,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Path      string `json:"path,omitempty"`
}

// MDArray is a Linux software RAID array as reported by /proc/mdstat.
type MDArray struct {
	Name    string   `json:"name"`
	State   string   `json:"state,omitempty"`
	Level   string   `json:"level,omitempty"`
	Devices []string `json:"devices,omitempty"`
}

// FstabEntry is one line of /etc/fstab.
type FstabEntry struct {
	Source     string `json:"source"`
	MountPoint string `json:"mount_point"`
	FSType     string `json:"fstype,omitempty"`
	Options    string `json:"options,omitempty"`
	Dump       string `json:"dump,omitempty"`
	Pass       string `json:"pass,omitempty"`
}

// BootInfo describes how the host boots.
type BootInfo struct {
	// Firmware is "uefi" or "bios".
	Firmware string `json:"firmware,omitempty"`
	// ESPMountPoint is where the EFI system partition is mounted, if any.
	ESPMountPoint string `json:"esp_mount_point,omitempty"`
	// Bootloader is the detected loader, e.g. "grub" or "systemd-boot".
	Bootloader string `json:"bootloader,omitempty"`
	// EFIEntries are the boot entries reported by efibootmgr.
	EFIEntries []string `json:"efi_entries,omitempty"`
}

// OSRelease holds the identifying fields of /etc/os-release.
type OSRelease struct {
	ID         string `json:"id,omitempty"`
	VersionID  string `json:"version_id,omitempty"`
	PrettyName string `json:"pretty_name,omitempty"`
}
