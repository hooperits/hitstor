//go:build linux

package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/logstore"
)

func readServerLogFile(f logstore.Filter) ([]logstore.Entry, error) {
	cfg := logstore.ConfigSnapshot()
	if !cfg.FileEnabled {
		cfg.FileEnabled = true
	}
	return logstore.ReadRotatedFileLines(cfg, f.Since, f.Limit)
}

func (d *Dashboard) fetchOSLogs(r *http.Request, f logstore.Filter) ([]logstore.Entry, string) {
	ctx := r.Context()
	var entries []logstore.Entry
	var warnings []string

	if f.Source == "" || f.Source == "all" || f.Source == "system" || f.Source == "server" {
		unitLines, err := runJournalctl(ctx, f, "-u", "hitstor-server", "-u", "hitstor-agent")
		if err != nil {
			warnings = append(warnings, "journalctl (hitstor): "+err.Error())
			if fileEntries, fileErr := readServerLogFile(f); fileErr == nil && len(fileEntries) > 0 {
				entries = append(entries, fileEntries...)
			}
		} else {
			entries = append(entries, parseJournalLines(unitLines, "system")...)
		}

		if f.Source == "" || f.Source == "all" || f.Source == "system" {
			kernelLines, err := runJournalctl(ctx, f, "-k", "-p", "err")
			if err != nil {
				warnings = append(warnings, "journalctl (kernel): "+err.Error())
			} else {
				entries = append(entries, parseJournalLines(kernelLines, "system")...)
			}
		}
	}
	return entries, strings.Join(warnings, "; ")
}

func runJournalctl(ctx context.Context, f logstore.Filter, extra ...string) ([]byte, error) {
	args := []string{"-q", "-o", "cat", "-n", "200", "--no-pager"}
	if !f.Since.IsZero() {
		args = append(args, "--since", f.Since.UTC().Format(time.RFC3339))
	}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	return cmd.Output()
}

func parseJournalLines(raw []byte, source string) []logstore.Entry {
	var out []logstore.Entry
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var j map[string]any
		if err := json.Unmarshal([]byte(line), &j); err == nil {
			out = append(out, journalJSONToEntry(j, source))
			continue
		}
		if e, ok := parseSlogTextLine(line); ok {
			e.Source = source
			out = append(out, e)
			continue
		}
		out = append(out, logstore.Entry{
			Time:    time.Now().UTC(),
			Level:   "info",
			Source:  source,
			Message: line,
		})
	}
	return out
}

func journalJSONToEntry(j map[string]any, source string) logstore.Entry {
	msg, _ := j["msg"].(string)
	if msg == "" {
		msg, _ = j["MESSAGE"].(string)
	}
	level, _ := j["level"].(string)
	if level == "" {
		level = "info"
	}
	ts := time.Now().UTC()
	if t, ok := j["time"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, t); err == nil {
			ts = parsed.UTC()
		} else if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			ts = parsed.UTC()
		}
	}
	return logstore.Entry{
		Time:    ts,
		Level:   strings.ToLower(level),
		Source:  source,
		Message: msg,
	}
}
