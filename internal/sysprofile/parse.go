package sysprofile

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// parseLsblk decodes `lsblk -J -b -o ...` output into the disk tree, keeping
// only real disks. Loop, ram, and rom devices describe the running system
// rather than its recoverable layout.
func parseLsblk(data []byte) ([]BlockDevice, error) {
	var report struct {
		BlockDevices []lsblkNode `json:"blockdevices"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("sysprofile: parse lsblk output: %w", err)
	}

	var disks []BlockDevice
	for _, n := range report.BlockDevices {
		if !isRecoverableDisk(n.Type) {
			continue
		}
		disks = append(disks, n.toBlockDevice())
	}
	return disks, nil
}

// lsblkNode mirrors one lsblk device entry. Fields are pointers or json.Number
// where lsblk emits null for "not applicable".
type lsblkNode struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	Type       string      `json:"type"`
	Size       json.Number `json:"size"`
	FSType     *string     `json:"fstype"`
	UUID       *string     `json:"uuid"`
	Label      *string     `json:"label"`
	MountPoint *string     `json:"mountpoint"`
	PartUUID   *string     `json:"partuuid"`
	Children   []lsblkNode `json:"children"`
}

func (n lsblkNode) toBlockDevice() BlockDevice {
	dev := BlockDevice{
		Name:       n.Name,
		Path:       n.Path,
		Type:       n.Type,
		Size:       parseJSONNumber(n.Size),
		FSType:     derefString(n.FSType),
		UUID:       derefString(n.UUID),
		Label:      derefString(n.Label),
		MountPoint: derefString(n.MountPoint),
		PartUUID:   derefString(n.PartUUID),
	}
	for _, c := range n.Children {
		dev.Children = append(dev.Children, c.toBlockDevice())
	}
	return dev
}

// isRecoverableDisk reports whether a top-level lsblk type participates in the
// layout we would have to recreate.
func isRecoverableDisk(devType string) bool {
	switch devType {
	case "disk", "raid0", "raid1", "raid4", "raid5", "raid6", "raid10":
		return true
	default:
		return false
	}
}

// parseFstab reads /etc/fstab, skipping comments and blank lines. Malformed
// lines are dropped rather than failing the capture.
func parseFstab(data []byte) []FstabEntry {
	var entries []FstabEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		entry := FstabEntry{
			Source:     fields[0],
			MountPoint: fields[1],
			FSType:     fields[2],
		}
		if len(fields) > 3 {
			entry.Options = fields[3]
		}
		if len(fields) > 4 {
			entry.Dump = fields[4]
		}
		if len(fields) > 5 {
			entry.Pass = fields[5]
		}
		entries = append(entries, entry)
	}
	return entries
}

// parseOSRelease extracts the identifying fields of /etc/os-release, stripping
// the surrounding quotes so values are directly usable.
func parseOSRelease(data []byte) OSRelease {
	var os OSRelease
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		value = strings.Trim(value, `"'`)
		switch key {
		case "ID":
			os.ID = value
		case "VERSION_ID":
			os.VersionID = value
		case "PRETTY_NAME":
			os.PrettyName = value
		}
	}
	return os
}

// parseMdstat extracts array definitions from /proc/mdstat. Only the header
// line of each array is needed; the block-count line that follows is derivable
// from the member devices.
func parseMdstat(data []byte) []MDArray {
	var arrays []MDArray
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, found := strings.Cut(line, " : ")
		if !found || !strings.HasPrefix(name, "md") {
			continue
		}

		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}

		array := MDArray{Name: strings.TrimSpace(name), State: fields[0]}
		for _, f := range fields[1:] {
			// The level appears before the member list; members carry a
			// bracketed role index such as "sda1[0]".
			if strings.HasPrefix(f, "raid") || f == "linear" || f == "multipath" {
				array.Level = f
				continue
			}
			if idx := strings.IndexByte(f, '['); idx > 0 {
				array.Devices = append(array.Devices, f[:idx])
			}
		}
		arrays = append(arrays, array)
	}
	return arrays
}

// lvmReport is the envelope shared by pvs, vgs, and lvs --reportformat json.
type lvmReport struct {
	Report []struct {
		PV []map[string]string `json:"pv"`
		VG []map[string]string `json:"vg"`
		LV []map[string]string `json:"lv"`
	} `json:"report"`
}

func decodeLVMReport(data []byte) (lvmReport, error) {
	var report lvmReport
	if err := json.Unmarshal(data, &report); err != nil {
		return lvmReport{}, fmt.Errorf("sysprofile: parse lvm report: %w", err)
	}
	return report, nil
}

// parsePVReport decodes `pvs --reportformat json`.
func parsePVReport(data []byte) ([]PhysicalVolume, error) {
	report, err := decodeLVMReport(data)
	if err != nil {
		return nil, err
	}
	var pvs []PhysicalVolume
	for _, section := range report.Report {
		for _, row := range section.PV {
			pvs = append(pvs, PhysicalVolume{
				Name:      row["pv_name"],
				UUID:      row["pv_uuid"],
				VGName:    row["vg_name"],
				SizeBytes: parseLVMSize(row["pv_size"]),
			})
		}
	}
	return pvs, nil
}

// parseVGReport decodes `vgs --reportformat json`.
func parseVGReport(data []byte) ([]VolumeGroup, error) {
	report, err := decodeLVMReport(data)
	if err != nil {
		return nil, err
	}
	var vgs []VolumeGroup
	for _, section := range report.Report {
		for _, row := range section.VG {
			vgs = append(vgs, VolumeGroup{
				Name:            row["vg_name"],
				UUID:            row["vg_uuid"],
				SizeBytes:       parseLVMSize(row["vg_size"]),
				ExtentSizeBytes: parseLVMSize(row["vg_extent_size"]),
			})
		}
	}
	return vgs, nil
}

// parseLVReport decodes `lvs --reportformat json`.
func parseLVReport(data []byte) ([]LogicalVolume, error) {
	report, err := decodeLVMReport(data)
	if err != nil {
		return nil, err
	}
	var lvs []LogicalVolume
	for _, section := range report.Report {
		for _, row := range section.LV {
			lvs = append(lvs, LogicalVolume{
				Name:      row["lv_name"],
				VGName:    row["vg_name"],
				UUID:      row["lv_uuid"],
				SizeBytes: parseLVMSize(row["lv_size"]),
				Path:      row["lv_path"],
			})
		}
	}
	return lvs, nil
}

// parseLVMSize converts an LVM size such as "4194304B" into bytes. LVM prefixes
// approximate values with '<' or '>'. An unparseable size yields 0 rather than
// an error, since a missing number must not fail the capture.
func parseLVMSize(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "<>~")
	s = strings.TrimSuffix(s, "B")
	if s == "" {
		return 0
	}
	// LVM emits fractional sizes for non-byte units; truncate to whole bytes.
	if value, err := strconv.ParseInt(s, 10, 64); err == nil {
		return value
	}
	if value, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(value)
	}
	return 0
}

func parseJSONNumber(n json.Number) int64 {
	if n == "" {
		return 0
	}
	value, err := n.Int64()
	if err != nil {
		return 0
	}
	return value
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
