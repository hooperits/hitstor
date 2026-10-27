package dashboard

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/logstore"
)

type logsAPIResponse struct {
	Entries          []logstore.Entry `json:"entries"`
	SourcesAvailable logsSourceInfo   `json:"sources_available"`
	Summary          logsSummary      `json:"summary"`
	Warnings         []string         `json:"warnings,omitempty"`
	Retention        logsRetention    `json:"retention"`
}

type logsSourceInfo struct {
	Server bool     `json:"server"`
	System bool     `json:"system"`
	Agents []string `json:"agents"`
}

type logsSummary struct {
	Total     int            `json:"total"`
	ByLevel   map[string]int `json:"by_level"`
	Truncated bool           `json:"truncated"`
}

type logsRetention struct {
	MaxEntries     int  `json:"log_max_entries"`
	RotateMaxMB    int  `json:"log_rotate_max_mb"`
	RotateMaxFiles int  `json:"log_rotate_max_files"`
	FileEnabled    bool `json:"log_file_enabled"`
}

func (d *Dashboard) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	d.render(w, r, "logs", d.newPageData("View Logs", "Server and integration activity", "logs", nil))
}

func (d *Dashboard) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	resp := d.collectLogs(r, false)
	writeJSON(w, resp)
}

func (d *Dashboard) handleAPILogsExport(w http.ResponseWriter, r *http.Request) {
	resp := d.collectLogs(r, parseBoolQuery(r, "include_rotated"))
	format := logstore.ParseExportFormat(r.URL.Query().Get("format"))
	entries := resp.Entries
	if len(entries) == 0 {
		entries = []logstore.Entry{}
	}
	w.Header().Set("Content-Type", logstore.ContentType(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, logstore.ExportFilename(format)))
	_ = logstore.Export(w, entries, format)
}

func (d *Dashboard) collectLogs(r *http.Request, includeRotated bool) logsAPIResponse {
	f := parseLogFilter(r)
	cfg := d.logStoreConfig()
	warnings := []string{}
	entries := logstore.Query(f)

	if cfg.FileEnabled {
		fileEntries, err := logstore.ReadRotatedFileLines(cfg, f.Since, f.Limit)
		if err != nil {
			warnings = append(warnings, "log file: "+err.Error())
		} else {
			entries = mergeLogEntries(entries, fileEntries, f)
		}
	} else if includeRotated {
		rotated, err := logstore.ReadRotatedFileLines(cfg, f.Since, f.Limit)
		if err != nil {
			warnings = append(warnings, "rotated files: "+err.Error())
		} else {
			entries = mergeLogEntries(entries, rotated, f)
		}
	}

	osEntries, osWarn := d.fetchOSLogs(r, f)
	if osWarn != "" {
		warnings = append(warnings, osWarn)
	}
	entries = mergeLogEntries(entries, osEntries, f)

	agentEntries, agentNames, agentWarn := d.fetchAgentLogs(r, f)
	if agentWarn != "" {
		warnings = append(warnings, agentWarn)
	}
	entries = mergeLogEntries(entries, agentEntries, f)

	entries = dedupeLogEntries(entries)
	limited := sortAndLimitEntries(entries, f.Limit)

	systemSupported := runtime.GOOS == "linux" || runtime.GOOS == "windows"

	return logsAPIResponse{
		Entries: limited,
		Summary: computeLogsSummary(limited, f.Limit),
		SourcesAvailable: logsSourceInfo{
			Server: true,
			System: systemSupported,
			Agents: agentNames,
		},
		Warnings: warnings,
		Retention: logsRetention{
			MaxEntries:     cfg.MaxEntries,
			RotateMaxMB:    cfg.RotateMaxMB,
			RotateMaxFiles: cfg.RotateMaxFiles,
			FileEnabled:    cfg.FileEnabled,
		},
	}
}

func (d *Dashboard) logStoreConfig() logstore.Config {
	cfg, err := d.requireConfig()
	if err != nil {
		return logstore.DefaultConfig()
	}
	return config.LogStoreConfig(*cfg)
}

func parseLogFilter(r *http.Request) logstore.Filter {
	q := r.URL.Query()
	limit := 500
	if v := q.Get("limit"); v != "" {
		if n, err := parseIntDefault(v, 500); err == nil {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	since := parseSince(q.Get("since"))
	return logstore.Filter{
		Source: strings.TrimSpace(q.Get("source")),
		Level:  strings.TrimSpace(q.Get("level")),
		Since:  since,
		Query:  strings.TrimSpace(q.Get("q")),
		Limit:  limit,
	}
}

func parseSince(s string) time.Time {
	switch strings.TrimSpace(s) {
	case "15m":
		return time.Now().UTC().Add(-15 * time.Minute)
	case "1h":
		return time.Now().UTC().Add(-1 * time.Hour)
	case "24h", "1d":
		return time.Now().UTC().Add(-24 * time.Hour)
	case "7d":
		return time.Now().UTC().Add(-7 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}

func parseBoolQuery(r *http.Request, key string) bool {
	v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	return v == "1" || v == "true" || v == "yes"
}

func mergeLogEntries(a, b []logstore.Entry, f logstore.Filter) []logstore.Entry {
	out := append([]logstore.Entry{}, a...)
	for _, e := range b {
		if e.Matches(f) {
			out = append(out, e)
		}
	}
	return out
}

func sortAndLimitEntries(entries []logstore.Entry, limit int) []logstore.Entry {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Time.Before(entries[j].Time)
	})
	if limit <= 0 {
		limit = 500
	}
	if len(entries) > limit {
		return entries[len(entries)-limit:]
	}
	return entries
}

func parseIntDefault(s string, def int) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return def, err
	}
	return n, nil
}
