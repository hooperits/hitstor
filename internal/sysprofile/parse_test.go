package sysprofile

import (
	"testing"
)

// Recorded from `lsblk -J -b -o NAME,PATH,TYPE,SIZE,FSTYPE,UUID,LABEL,MOUNTPOINT,PARTUUID`
// on a UEFI host with an LVM root, trimmed to the interesting devices.
const lsblkFixture = `{
   "blockdevices": [
      {
         "name": "loop0", "path": "/dev/loop0", "type": "loop", "size": 874858496,
         "fstype": "iso9660", "uuid": null, "label": null, "mountpoint": "/snap/core", "partuuid": null
      },{
         "name": "sda", "path": "/dev/sda", "type": "disk", "size": 274877906944,
         "fstype": null, "uuid": null, "label": null, "mountpoint": null, "partuuid": null,
         "children": [
            {
               "name": "sda1", "path": "/dev/sda1", "type": "part", "size": 536870912,
               "fstype": "vfat", "uuid": "A1B2-C3D4", "label": "EFI", "mountpoint": "/boot/efi",
               "partuuid": "11111111-1111-1111-1111-111111111111"
            },{
               "name": "sda2", "path": "/dev/sda2", "type": "part", "size": 2147483648,
               "fstype": "ext4", "uuid": "8f3e-boot-uuid", "label": "boot", "mountpoint": "/boot",
               "partuuid": "22222222-2222-2222-2222-222222222222"
            },{
               "name": "sda3", "path": "/dev/sda3", "type": "part", "size": 272190275584,
               "fstype": "LVM2_member", "uuid": "pv-uuid-here", "label": null, "mountpoint": null,
               "partuuid": "33333333-3333-3333-3333-333333333333",
               "children": [
                  {
                     "name": "vg0-root", "path": "/dev/mapper/vg0-root", "type": "lvm",
                     "size": 214748364800, "fstype": "xfs", "uuid": "root-fs-uuid",
                     "label": null, "mountpoint": "/", "partuuid": null
                  }
               ]
            }
         ]
      }
   ]
}`

func TestParseLsblkTopLevelDisks(t *testing.T) {
	devs, err := parseLsblk([]byte(lsblkFixture))
	if err != nil {
		t.Fatalf("parseLsblk: %v", err)
	}

	// loop devices are not part of a recoverable disk layout.
	if len(devs) != 1 {
		t.Fatalf("expected 1 disk, got %d: %+v", len(devs), devs)
	}
	sda := devs[0]
	if sda.Name != "sda" || sda.Type != "disk" {
		t.Errorf("unexpected disk: %+v", sda)
	}
	if sda.Size != 274877906944 {
		t.Errorf("Size = %d, want 274877906944", sda.Size)
	}
	if len(sda.Children) != 3 {
		t.Fatalf("expected 3 partitions, got %d", len(sda.Children))
	}
}

func TestParseLsblkCapturesPartitionIdentity(t *testing.T) {
	devs, err := parseLsblk([]byte(lsblkFixture))
	if err != nil {
		t.Fatal(err)
	}
	esp := devs[0].Children[0]

	if esp.FSType != "vfat" {
		t.Errorf("FSType = %q, want vfat", esp.FSType)
	}
	if esp.UUID != "A1B2-C3D4" {
		t.Errorf("UUID = %q, want A1B2-C3D4", esp.UUID)
	}
	// PARTUUID is what a recreated fstab and bootloader resolve against.
	if esp.PartUUID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("PartUUID = %q", esp.PartUUID)
	}
	if esp.MountPoint != "/boot/efi" {
		t.Errorf("MountPoint = %q, want /boot/efi", esp.MountPoint)
	}
}

func TestParseLsblkNestedLVMVolume(t *testing.T) {
	devs, err := parseLsblk([]byte(lsblkFixture))
	if err != nil {
		t.Fatal(err)
	}
	pv := devs[0].Children[2]
	if len(pv.Children) != 1 {
		t.Fatalf("expected the LV nested under the PV, got %d children", len(pv.Children))
	}
	lv := pv.Children[0]
	if lv.Type != "lvm" || lv.MountPoint != "/" {
		t.Errorf("unexpected LV: %+v", lv)
	}
	if lv.FSType != "xfs" {
		t.Errorf("LV FSType = %q, want xfs", lv.FSType)
	}
}

func TestParseLsblkRejectsGarbage(t *testing.T) {
	if _, err := parseLsblk([]byte("not json")); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

const fstabFixture = `# /etc/fstab: static file system information.
#
# <file system> <mount point>   <type>  <options>       <dump>  <pass>
UUID=root-fs-uuid /              xfs     defaults        0 0
UUID=8f3e-boot-uuid /boot        ext4    defaults        0 2
PARTUUID=11111111-1111-1111-1111-111111111111 /boot/efi vfat umask=0077 0 1
/dev/mapper/vg0-swap none        swap    sw              0 0

tmpfs           /tmp            tmpfs   defaults        0 0
`

func TestParseFstabSkipsCommentsAndBlanks(t *testing.T) {
	entries := parseFstab([]byte(fstabFixture))
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d: %+v", len(entries), entries)
	}
}

func TestParseFstabFields(t *testing.T) {
	entries := parseFstab([]byte(fstabFixture))

	root := entries[0]
	if root.Source != "UUID=root-fs-uuid" {
		t.Errorf("Source = %q", root.Source)
	}
	if root.MountPoint != "/" {
		t.Errorf("MountPoint = %q", root.MountPoint)
	}
	if root.FSType != "xfs" {
		t.Errorf("FSType = %q", root.FSType)
	}
	if root.Options != "defaults" {
		t.Errorf("Options = %q", root.Options)
	}

	esp := entries[2]
	if esp.Source != "PARTUUID=11111111-1111-1111-1111-111111111111" {
		t.Errorf("ESP Source = %q", esp.Source)
	}
	if esp.MountPoint != "/boot/efi" {
		t.Errorf("ESP MountPoint = %q", esp.MountPoint)
	}
}

func TestParseFstabEmpty(t *testing.T) {
	if got := parseFstab(nil); len(got) != 0 {
		t.Errorf("expected no entries, got %+v", got)
	}
}

const osReleaseFixture = `PRETTY_NAME="Ubuntu 24.04.1 LTS"
NAME="Ubuntu"
VERSION_ID="24.04"
VERSION="24.04.1 LTS (Noble Numbat)"
ID=ubuntu
ID_LIKE=debian
`

func TestParseOSRelease(t *testing.T) {
	os := parseOSRelease([]byte(osReleaseFixture))
	if os.ID != "ubuntu" {
		t.Errorf("ID = %q, want ubuntu", os.ID)
	}
	if os.VersionID != "24.04" {
		t.Errorf("VersionID = %q, want 24.04", os.VersionID)
	}
	// Quotes must be stripped so the value is usable directly.
	if os.PrettyName != "Ubuntu 24.04.1 LTS" {
		t.Errorf("PrettyName = %q", os.PrettyName)
	}
}

func TestParseOSReleaseEmpty(t *testing.T) {
	if got := parseOSRelease(nil); got.ID != "" {
		t.Errorf("expected zero value, got %+v", got)
	}
}

const mdstatFixture = `Personalities : [raid1] [raid6] [raid5] [raid4]
md0 : active raid1 sdb1[1] sda1[0]
      976630464 blocks super 1.2 [2/2] [UU]

md1 : active raid5 sdc1[2] sdb2[1] sda2[0]
      1953260928 blocks super 1.2 level 5, 512k chunk, algorithm 2 [3/3] [UUU]

unused devices: <none>
`

func TestParseMdstat(t *testing.T) {
	arrays := parseMdstat([]byte(mdstatFixture))
	if len(arrays) != 2 {
		t.Fatalf("expected 2 arrays, got %d: %+v", len(arrays), arrays)
	}

	md0 := arrays[0]
	if md0.Name != "md0" {
		t.Errorf("Name = %q, want md0", md0.Name)
	}
	if md0.Level != "raid1" {
		t.Errorf("Level = %q, want raid1", md0.Level)
	}
	if md0.State != "active" {
		t.Errorf("State = %q, want active", md0.State)
	}
	if len(md0.Devices) != 2 {
		t.Errorf("Devices = %v, want 2 members", md0.Devices)
	}

	if arrays[1].Level != "raid5" {
		t.Errorf("md1 Level = %q, want raid5", arrays[1].Level)
	}
}

// A host without software RAID has a mdstat with only the Personalities line.
func TestParseMdstatNoArrays(t *testing.T) {
	arrays := parseMdstat([]byte("Personalities : \nunused devices: <none>\n"))
	if len(arrays) != 0 {
		t.Errorf("expected no arrays, got %+v", arrays)
	}
}

// Recorded from `vgs --reportformat json --units b -o vg_name,vg_uuid,vg_size,vg_extent_size`.
const vgsFixture = `{
      "report": [
          {
              "vg": [
                  {"vg_name":"vg0", "vg_uuid":"aBcDeF-1234", "vg_size":"272190275584B", "vg_extent_size":"4194304B"}
              ]
          }
      ]
  }`

func TestParseVGReport(t *testing.T) {
	vgs, err := parseVGReport([]byte(vgsFixture))
	if err != nil {
		t.Fatalf("parseVGReport: %v", err)
	}
	if len(vgs) != 1 {
		t.Fatalf("expected 1 VG, got %d", len(vgs))
	}
	if vgs[0].Name != "vg0" {
		t.Errorf("Name = %q, want vg0", vgs[0].Name)
	}
	// The trailing B unit must be stripped so the size is a usable number.
	if vgs[0].SizeBytes != 272190275584 {
		t.Errorf("SizeBytes = %d, want 272190275584", vgs[0].SizeBytes)
	}
	if vgs[0].ExtentSizeBytes != 4194304 {
		t.Errorf("ExtentSizeBytes = %d, want 4194304", vgs[0].ExtentSizeBytes)
	}
}

const lvsFixture = `{
      "report": [
          {
              "lv": [
                  {"lv_name":"root", "vg_name":"vg0", "lv_uuid":"lv-uuid-1", "lv_size":"214748364800B", "lv_path":"/dev/vg0/root"},
                  {"lv_name":"swap", "vg_name":"vg0", "lv_uuid":"lv-uuid-2", "lv_size":"8589934592B", "lv_path":"/dev/vg0/swap"}
              ]
          }
      ]
  }`

func TestParseLVReport(t *testing.T) {
	lvs, err := parseLVReport([]byte(lvsFixture))
	if err != nil {
		t.Fatalf("parseLVReport: %v", err)
	}
	if len(lvs) != 2 {
		t.Fatalf("expected 2 LVs, got %d", len(lvs))
	}
	if lvs[0].Name != "root" || lvs[0].VGName != "vg0" {
		t.Errorf("unexpected LV: %+v", lvs[0])
	}
	if lvs[0].SizeBytes != 214748364800 {
		t.Errorf("SizeBytes = %d", lvs[0].SizeBytes)
	}
	if lvs[1].Path != "/dev/vg0/swap" {
		t.Errorf("Path = %q", lvs[1].Path)
	}
}

const pvsFixture = `{
      "report": [
          {
              "pv": [
                  {"pv_name":"/dev/sda3", "pv_uuid":"pv-uuid-here", "vg_name":"vg0", "pv_size":"272190275584B"}
              ]
          }
      ]
  }`

func TestParsePVReport(t *testing.T) {
	pvs, err := parsePVReport([]byte(pvsFixture))
	if err != nil {
		t.Fatalf("parsePVReport: %v", err)
	}
	if len(pvs) != 1 {
		t.Fatalf("expected 1 PV, got %d", len(pvs))
	}
	if pvs[0].Name != "/dev/sda3" || pvs[0].VGName != "vg0" {
		t.Errorf("unexpected PV: %+v", pvs[0])
	}
	if pvs[0].SizeBytes != 272190275584 {
		t.Errorf("SizeBytes = %d", pvs[0].SizeBytes)
	}
}

// LVM tools emit an empty report on a host without LVM; that is not an error.
func TestParseLVMReportEmpty(t *testing.T) {
	empty := `{"report":[{"vg":[]}]}`
	vgs, err := parseVGReport([]byte(empty))
	if err != nil {
		t.Fatalf("empty report should not error: %v", err)
	}
	if len(vgs) != 0 {
		t.Errorf("expected no VGs, got %+v", vgs)
	}
}

func TestParseLVMSizeVariants(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"4194304B", 4194304},
		{"4194304", 4194304},
		{"", 0},
		{"not-a-size", 0},
		{"<8589934592B", 8589934592}, // lvm prefixes approximate values with '<'
	}
	for _, tc := range cases {
		if got := parseLVMSize(tc.in); got != tc.want {
			t.Errorf("parseLVMSize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
