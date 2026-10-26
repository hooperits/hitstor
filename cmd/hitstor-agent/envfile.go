package main

import (
	"os"
	"strings"
)

// loadEnvFile reads a KEY=VALUE env file and applies values to global flags.
// On Linux, systemd's EnvironmentFile= handles this automatically; on Windows
// we must parse the file manually since there is no equivalent mechanism.
func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // File doesn't exist yet (fresh install).
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "HITSTOR_AGENT_SERVER":
			if serverFlag == "" {
				serverFlag = v
			}
		case "HITSTOR_AGENT_API_KEY":
			if apiKeyFlag == "" {
				apiKeyFlag = v
			}
		case "HITSTOR_AGENT_CA_CERT":
			if caCertFlag == "" {
				caCertFlag = v
			}
		}
	}
}
