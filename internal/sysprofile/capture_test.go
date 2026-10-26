package sysprofile

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// Capture must succeed on any host. Tools that are missing become warnings, not
// errors, because a partial profile is more useful at recovery time than none.
func TestCaptureNeverFails(t *testing.T) {
	profile, err := Capture(context.Background())
	if err != nil {
		t.Fatalf("Capture returned an error: %v", err)
	}
	if profile == nil {
		t.Fatal("Capture returned a nil profile")
	}
	if profile.CapturedAt.IsZero() {
		t.Error("CapturedAt was not set")
	}
	if profile.Arch == "" {
		t.Error("Arch was not set")
	}
}

func TestCaptureRespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// External commands cannot run, but capture still returns a usable profile.
	profile, err := Capture(ctx)
	if err != nil {
		t.Fatalf("Capture with a cancelled context: %v", err)
	}
	if profile == nil {
		t.Fatal("expected a profile even with a cancelled context")
	}
}

// The profile is persisted as JSON in the repository, so it must round-trip.
func TestProfileJSONRoundTrip(t *testing.T) {
	original := &Profile{
		CapturedAt: time.Now().UTC().Truncate(time.Second),
		Hostname:   "web-01",
		OS:         OSRelease{ID: "ubuntu", VersionID: "24.04", PrettyName: "Ubuntu 24.04.1 LTS"},
		Kernel:     "6.8.0-40-generic",
		Disks: []BlockDevice{{
			Name: "sda", Path: "/dev/sda", Type: "disk", Size: 274877906944,
			Children: []BlockDevice{{
				Name: "sda1", Type: "part", Size: 536870912, FSType: "vfat",
				UUID: "A1B2-C3D4", MountPoint: "/boot/efi",
				PartUUID: "11111111-1111-1111-1111-111111111111",
			}},
		}},
		LVM: &LVMLayout{
			VolumeGroups: []VolumeGroup{{Name: "vg0", SizeBytes: 272190275584}},
		},
		Mounts: []FstabEntry{{Source: "UUID=abc", MountPoint: "/", FSType: "xfs", Options: "defaults"}},
		Boot:   BootInfo{Firmware: "uefi", ESPMountPoint: "/boot/efi", Bootloader: "grub"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Profile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Hostname != original.Hostname {
		t.Errorf("Hostname = %q, want %q", decoded.Hostname, original.Hostname)
	}
	if len(decoded.Disks) != 1 || len(decoded.Disks[0].Children) != 1 {
		t.Fatalf("disk tree did not survive: %+v", decoded.Disks)
	}
	// PARTUUID is what a rebuilt fstab resolves against; it must survive.
	if got := decoded.Disks[0].Children[0].PartUUID; got != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("PartUUID = %q", got)
	}
	if decoded.LVM == nil || len(decoded.LVM.VolumeGroups) != 1 {
		t.Errorf("LVM layout did not survive: %+v", decoded.LVM)
	}
	if decoded.Boot.Firmware != "uefi" || decoded.Boot.Bootloader != "grub" {
		t.Errorf("Boot did not survive: %+v", decoded.Boot)
	}
}

// A host with no LVM must omit the section entirely rather than emitting an
// empty object, so the stored profile stays honest about what was found.
func TestProfileOmitsEmptySections(t *testing.T) {
	data, err := json.Marshal(&Profile{CapturedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"lvm", "disks", "raid", "mounts", "warnings"} {
		if _, present := raw[key]; present {
			t.Errorf("empty section %q should be omitted", key)
		}
	}
}
