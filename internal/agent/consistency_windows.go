//go:build windows

package agent

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

func prepareConsistencyPlatform(sess *ConsistencySession, backupPaths []string) (*ConsistencySession, error) {
	volumes := UniqueVolumes(backupPaths)
	if len(volumes) == 0 {
		return sess, fmt.Errorf("VSS: no volumes detected for paths %v", backupPaths)
	}

	if !isElevatedWindows() {
		slog.Warn("VSS works best when the agent runs elevated (Administrator/SYSTEM); continuing anyway")
	}

	snaps, err := CreateVSSSnapshots(volumes)
	if err != nil {
		return sess, err
	}
	if len(snaps) == 0 {
		return sess, fmt.Errorf("VSS: no snapshots created")
	}

	sess.Method = ConsistencyVSS
	sess.cleanup = append(sess.cleanup, func() { DeleteVSSSnapshots(snaps) })

	readPaths := make([]string, 0, len(backupPaths))
	for _, p := range backupPaths {
		vol := VolumeForPath(p)
		translated := p
		for _, snap := range snaps {
			if strings.EqualFold(normalizeVolumeRoot(snap.Volume), normalizeVolumeRoot(vol)) {
				translated = translatePathUnderRoot(p, snap.Volume, ensureDeviceSuffix(snap.DevicePath))
				sess.snapshotRoots = append(sess.snapshotRoots, snapshotRoot{
					originalRoot: snap.Volume,
					snapshotRoot: ensureDeviceSuffix(snap.DevicePath),
				})
				break
			}
		}
		readPaths = append(readPaths, translated)
	}
	// Dedupe snapshotRoots by original volume.
	sess.snapshotRoots = uniqueSnapshotRoots(sess.snapshotRoots)
	sess.ReadPaths = readPaths
	logConsistency(ConsistencyVSS, readPaths, fmt.Sprintf("%d volume(s)", len(snaps)))
	return sess, nil
}

func ensureDeviceSuffix(dev string) string {
	if dev == "" {
		return dev
	}
	// VSS DeviceObject is typically without trailing slash; paths need separator.
	if strings.HasSuffix(dev, `\`) || strings.HasSuffix(dev, `/`) {
		return dev
	}
	return dev + `\`
}

func uniqueSnapshotRoots(in []snapshotRoot) []snapshotRoot {
	seen := make(map[string]bool)
	var out []snapshotRoot
	for _, r := range in {
		k := strings.ToUpper(filepath.Clean(r.originalRoot))
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

// isElevatedWindows reports whether the process likely has admin rights.
// Best-effort: failed check returns false without blocking VSS.
func isElevatedWindows() bool {
	// `net session` succeeds only for elevated processes.
	_, err := activeRunner.CombinedOutput("net", "session")
	return err == nil
}

// --- VSS create/delete (hardened) ---

// VSSSnapshot holds info about a created volume shadow copy.
type VSSSnapshot struct {
	ShadowID   string // VSS shadow copy ID (GUID)
	DevicePath string // e.g., \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy5
	Volume     string // e.g., C:\
}

// CreateVSSSnapshots creates shadow copies for the given volumes with one retry
// on transient failures and human-readable Win32_ShadowCopy return codes.
func CreateVSSSnapshots(volumes []string) ([]VSSSnapshot, error) {
	var snapshots []VSSSnapshot
	for _, vol := range volumes {
		vol = normalizeVolumeRoot(vol)
		snap, err := createVSSSnapshotWithRetry(vol, 2)
		if err != nil {
			DeleteVSSSnapshots(snapshots)
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

func createVSSSnapshotWithRetry(vol string, attempts int) (VSSSnapshot, error) {
	var last error
	for i := 1; i <= attempts; i++ {
		snap, err := createOneVSSSnapshot(vol)
		if err == nil {
			return snap, nil
		}
		last = err
		slog.Warn("VSS create attempt failed", "volume", vol, "attempt", i, "error", err)
		if i < attempts {
			time.Sleep(time.Duration(i) * 500 * time.Millisecond)
		}
	}
	return VSSSnapshot{}, last
}

func createOneVSSSnapshot(vol string) (VSSSnapshot, error) {
	slog.Info("creating VSS snapshot", "volume", vol)

	// Prefer CIM (Get-CimInstance / Invoke-CimMethod); fall back messages still clear.
	// ContextContext/Context is not available here — keep script self-contained.
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$vol = '%s'
try {
  $r = Invoke-CimMethod -ClassName Win32_ShadowCopy -MethodName Create -Arguments @{Volume=$vol; Context='ClientAccessible'}
} catch {
  $r = (Get-WmiObject -List Win32_ShadowCopy).Create($vol, 'ClientAccessible')
}
if ($null -eq $r) { Write-Error 'VSS Create returned null'; exit 2 }
$code = [int]$r.ReturnValue
if ($code -ne 0) {
  $msg = switch ($code) {
    1 { 'Access denied (run agent as Administrator/SYSTEM)' }
    2 { 'Invalid argument' }
    3 { 'Specified volume not found' }
    4 { 'Specified volume not supported' }
    5 { 'Unsupported shadow copy context' }
    6 { 'Insufficient storage' }
    7 { 'Volume in use' }
    8 { 'Maximum number of shadow copies reached' }
    9 { 'Another shadow copy operation is in progress' }
    10 { 'Shadow copy provider vetoed the operation' }
    11 { 'Provider not registered' }
    12 { 'Provider failure' }
    13 { 'Unknown error' }
    default { "Win32_ShadowCopy.Create ReturnValue=$code" }
  }
  Write-Error "VSS create failed for $vol : $msg ($code)"
  exit 1
}
$id = $r.ShadowID
$dev = $null
# Prefer CIM lookup
$sc = Get-CimInstance Win32_ShadowCopy -Filter "ID='$id'" -ErrorAction SilentlyContinue
if ($sc) { $dev = $sc.DeviceObject }
if (-not $dev) {
  $sc2 = Get-WmiObject Win32_ShadowCopy | Where-Object { $_.ID -eq $id }
  if ($sc2) { $dev = $sc2.DeviceObject }
}
if (-not $dev) { Write-Error "VSS device path not found for $id"; exit 3 }
Write-Output "$id|$dev"
`, escapePS(vol))

	out, err := activeRunner.CombinedOutput("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", ps)
	text := strings.TrimSpace(string(out))
	if err != nil {
		return VSSSnapshot{}, fmt.Errorf("VSS create for %s: %w: %s", vol, err, text)
	}
	parts := strings.SplitN(text, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return VSSSnapshot{}, fmt.Errorf("VSS create for %s: unexpected output: %s", vol, text)
	}
	snap := VSSSnapshot{ShadowID: parts[0], DevicePath: parts[1], Volume: vol}
	slog.Info("VSS snapshot created", "volume", vol, "shadow_id", snap.ShadowID, "device", snap.DevicePath)
	return snap, nil
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// DeleteVSSSnapshots removes the given shadow copies.
func DeleteVSSSnapshots(snapshots []VSSSnapshot) {
	for _, snap := range snapshots {
		if snap.ShadowID == "" {
			continue
		}
		slog.Info("deleting VSS snapshot", "shadow_id", snap.ShadowID, "volume", snap.Volume)
		ps := fmt.Sprintf(`
$ErrorActionPreference = 'Continue'
$id = '%s'
$sc = Get-CimInstance Win32_ShadowCopy -Filter "ID='$id'" -ErrorAction SilentlyContinue
if ($sc) { Remove-CimInstance -InputObject $sc; exit 0 }
Get-WmiObject Win32_ShadowCopy | Where-Object { $_.ID -eq $id } | ForEach-Object { $_.Delete() }
`, escapePS(snap.ShadowID))
		out, err := activeRunner.CombinedOutput("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", ps)
		if err != nil {
			slog.Warn("failed to delete VSS snapshot",
				"shadow_id", snap.ShadowID,
				"error", err,
				"output", strings.TrimSpace(string(out)),
			)
		}
	}
}

// VolumeForPath returns the volume root for the given absolute path.
// e.g., "C:\Users\jc" → "C:\"
func VolumeForPath(path string) string {
	vol := filepath.VolumeName(path)
	if vol == "" {
		return ""
	}
	return normalizeVolumeRoot(vol + `\`)
}

// TranslatePathsForVSS is retained for callers/tests; prefer ConsistencySession.
func TranslatePathsForVSS(paths []string, snapshots []VSSSnapshot) (translated []string, reverseMap map[string]string) {
	volMap := make(map[string]string)
	for _, snap := range snapshots {
		volMap[strings.ToUpper(normalizeVolumeRoot(snap.Volume))] = ensureDeviceSuffix(snap.DevicePath)
	}
	reverseMap = make(map[string]string)
	for _, p := range paths {
		vol := strings.ToUpper(VolumeForPath(p))
		if dev, ok := volMap[vol]; ok {
			translated = append(translated, translatePathUnderRoot(p, normalizeVolumeRoot(vol), dev))
			reverseMap[dev] = normalizeVolumeRoot(vol)
		} else {
			translated = append(translated, p)
		}
	}
	return translated, reverseMap
}

// UniqueVolumes returns the deduplicated list of volume roots from paths.
func UniqueVolumes(paths []string) []string {
	seen := make(map[string]bool)
	var vols []string
	for _, p := range paths {
		vol := VolumeForPath(p)
		key := strings.ToUpper(vol)
		if vol != "" && !seen[key] {
			seen[key] = true
			vols = append(vols, vol)
		}
	}
	return vols
}
