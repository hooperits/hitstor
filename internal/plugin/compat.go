package plugin

import (
	"fmt"
	"strconv"
	"strings"
)

// CompatibilityError reports plugin/core version mismatch.
type CompatibilityError struct {
	PluginVersion  string
	MinCoreVersion string
	CoreVersion    string
	Reason         string
}

func (e *CompatibilityError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return fmt.Sprintf("plugin %s requires HitStor >= %s (running %s)", e.PluginVersion, e.MinCoreVersion, e.CoreVersion)
}

// CheckCompatibility validates plugin metadata against the running core version.
func CheckCompatibility(coreVersion string, meta PluginMetadata) error {
	if meta.ProtocolVersion != 0 && meta.ProtocolVersion != int32(HandshakeConfig.ProtocolVersion) {
		return &CompatibilityError{
			PluginVersion: meta.Version,
			CoreVersion:   coreVersion,
			Reason: fmt.Sprintf(
				"protocol version mismatch: plugin %d, host %d",
				meta.ProtocolVersion, HandshakeConfig.ProtocolVersion,
			),
		}
	}
	if strings.TrimSpace(meta.MinCoreVersion) == "" || coreVersion == "dev" {
		return nil
	}
	if compareVersions(coreVersion, meta.MinCoreVersion) < 0 {
		return &CompatibilityError{
			PluginVersion:  meta.Version,
			MinCoreVersion: meta.MinCoreVersion,
			CoreVersion:    coreVersion,
		}
	}
	return nil
}

// compareVersions returns -1 if a<b, 0 if equal, 1 if a>b.
// Supports optional leading "v" and dotted numeric segments.
func compareVersions(a, b string) int {
	aParts := versionParts(a)
	bParts := versionParts(b)
	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if v == "" {
		return nil
	}
	segments := strings.Split(v, "-")[0]
	parts := strings.Split(segments, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}
