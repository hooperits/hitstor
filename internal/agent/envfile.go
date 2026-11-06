package agent

import (
	"os"
	"slices"
	"strings"
)

// registrationEnvKeys are the agent.env settings that registration owns:
// each registration replaces all of them, so a CA certificate from an
// earlier registration does not linger.
var registrationEnvKeys = []string{"HITSTOR_AGENT_SERVER", "HITSTOR_AGENT_API_KEY", "HITSTOR_AGENT_CA_CERT"}

// UpdateEnvFile rewrites the KEY=value file at path: lines for the keys in
// replace are dropped, then set is appended in a stable order. Comments and
// every other setting are kept. The file is written with mode 0600.
func UpdateEnvFile(path string, set map[string]string, replace []string) error {
	var kept []string
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			k, _, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && (slices.Contains(replace, k) || set[k] != "") {
				continue
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			kept = append(kept, line)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		ia, ib := slices.Index(registrationEnvKeys, a), slices.Index(registrationEnvKeys, b)
		if ia >= 0 && ib >= 0 {
			return ia - ib
		}
		return strings.Compare(a, b)
	})
	var b strings.Builder
	for _, line := range kept {
		b.WriteString(line + "\n")
	}
	for _, k := range keys {
		b.WriteString(k + "=" + set[k] + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0600)
}
