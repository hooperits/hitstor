// Package update provides version checking against the GitHub Releases API.
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultURL is the default update check endpoint.
const DefaultURL = "https://api.github.com/repos/hooperits/hitstor/releases/latest"

// CheckResult holds the outcome of a version check.
type CheckResult struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	ReleaseURL      string
}

// Check contacts the update endpoint and compares the current version against
// the latest release. Network errors are returned as a zero CheckResult with
// a nil error (silent failure).
func Check(currentVersion, updateURL string) (CheckResult, error) {
	if updateURL == "" {
		updateURL = DefaultURL
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(updateURL)
	if err != nil {
		return CheckResult{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CheckResult{}, nil
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return CheckResult{}, nil
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	result := CheckResult{
		CurrentVersion: current,
		LatestVersion:  latest,
		ReleaseURL:     release.HTMLURL,
	}

	if latest != "" && CompareVersions(current, latest) {
		result.UpdateAvailable = true
	}

	return result, nil
}

// CompareVersions returns true if latest is newer than current.
// Both versions should be in "major.minor.patch" format (no "v" prefix).
func CompareVersions(current, latest string) bool {
	curParts := parseVersion(current)
	latParts := parseVersion(latest)

	for i := 0; i < 3; i++ {
		if latParts[i] > curParts[i] {
			return true
		}
		if latParts[i] < curParts[i] {
			return false
		}
	}
	return false
}

// parseVersion splits a version string into [major, minor, patch].
// Non-numeric parts are treated as 0.
func parseVersion(v string) [3]int {
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		// Strip any pre-release suffix (e.g., "1-beta").
		clean := strings.SplitN(parts[i], "-", 2)[0]
		n, err := strconv.Atoi(clean)
		if err != nil {
			continue
		}
		result[i] = n
	}
	return result
}

// FormatResult returns a human-readable string for a CheckResult.
func FormatResult(r CheckResult) string {
	if r.UpdateAvailable {
		return fmt.Sprintf("Update available: v%s (current: v%s)\nRelease: %s",
			r.LatestVersion, r.CurrentVersion, r.ReleaseURL)
	}
	if r.LatestVersion != "" {
		return fmt.Sprintf("You are running the latest version (v%s).", r.CurrentVersion)
	}
	return "Could not check for updates."
}
