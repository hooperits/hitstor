package dashboard

import (
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/logstore"
)

// parseSlogTextLine delegates to logstore for journal/slog text parsing.
func parseSlogTextLine(line string) (logstore.Entry, bool) {
	return logstore.ParseSlogTextLine(line)
}

// dedupeLogEntries removes duplicate lines merged from ring, files, and journal.
// Prefers the entry with the richest metadata (non-system source, parsed level).
func dedupeLogEntries(entries []logstore.Entry) []logstore.Entry {
	if len(entries) == 0 {
		return entries
	}
	type slot struct {
		entry logstore.Entry
		score int
	}
	seen := make(map[string]slot, len(entries))
	order := make([]string, 0, len(entries))

	for _, e := range entries {
		if logstore.IsNoise(e) {
			continue
		}
		key := dedupeKey(e)
		score := entryScore(e)
		if prev, ok := seen[key]; ok {
			if score > prev.score {
				seen[key] = slot{entry: e, score: score}
			}
			continue
		}
		seen[key] = slot{entry: e, score: score}
		order = append(order, key)
	}

	out := make([]logstore.Entry, 0, len(seen))
	for _, key := range order {
		out = append(out, seen[key].entry)
	}
	return out
}

func dedupeKey(e logstore.Entry) string {
	msg := logstore.NormalizeMessage(e.Message)
	ts := e.Time.UTC().Truncate(2 * time.Second).Format(time.RFC3339)
	return ts + "|" + msg
}

func entryScore(e logstore.Entry) int {
	score := 0
	if e.Source != "" && e.Source != "system" {
		score += 2
	}
	if e.Level != "" && !strings.EqualFold(e.Level, "info") {
		score += 1
	}
	if len(e.Fields) > 0 {
		score += 1
	}
	return score
}

// computeLogsSummary builds level counts for the API response.
func computeLogsSummary(entries []logstore.Entry, limit int) logsSummary {
	byLevel := map[string]int{
		"error": 0,
		"warn":  0,
		"info":  0,
		"debug": 0,
	}
	for _, e := range entries {
		switch strings.ToLower(e.Level) {
		case "error":
			byLevel["error"]++
		case "warn", "warning":
			byLevel["warn"]++
		case "debug":
			byLevel["debug"]++
		default:
			byLevel["info"]++
		}
	}
	return logsSummary{
		Total:     len(entries),
		ByLevel:   byLevel,
		Truncated: len(entries) >= limit && limit > 0,
	}
}
