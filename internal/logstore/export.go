package logstore

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ExportFormat is jsonl, text, or csv.
type ExportFormat string

const (
	FormatJSONL ExportFormat = "jsonl"
	FormatText  ExportFormat = "text"
	FormatCSV   ExportFormat = "csv"
)

func ParseExportFormat(s string) ExportFormat {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "text", "txt":
		return FormatText
	case "csv":
		return FormatCSV
	default:
		return FormatJSONL
	}
}

// Export writes filtered entries to w.
func Export(w io.Writer, entries []Entry, format ExportFormat) error {
	switch format {
	case FormatText:
		for _, e := range entries {
			if _, err := io.WriteString(w, e.formatText()+"\n"); err != nil {
				return err
			}
		}
		return nil
	case FormatCSV:
		var buf bytes.Buffer
		cw := csv.NewWriter(&buf)
		_ = cw.Write([]string{"time", "level", "source", "component", "message", "fields_json"})
		for _, e := range entries {
			fieldsJSON, _ := json.Marshal(e.Fields)
			_ = cw.Write([]string{
				e.Time.UTC().Format(time.RFC3339Nano),
				strings.ToUpper(e.Level),
				e.Source,
				e.Component,
				e.Message,
				string(fieldsJSON),
			})
		}
		cw.Flush()
		_, err := w.Write(buf.Bytes())
		return err
	default:
		for _, e := range entries {
			b, err := json.Marshal(e)
			if err != nil {
				return err
			}
			if _, err := w.Write(append(b, '\n')); err != nil {
				return err
			}
		}
		return nil
	}
}

// ReadRotatedFileLines reads JSONL lines from rotated log files on disk.
func ReadRotatedFileLines(cfg Config, since time.Time, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 500
	}
	var out []Entry
	for _, path := range RotatedPaths(cfg) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var e Entry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				continue
			}
			if !since.IsZero() && e.Time.Before(since) {
				continue
			}
			out = append(out, e)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// ExportFilename builds a download filename for the given format.
func ExportFilename(format ExportFormat) string {
	ts := time.Now().UTC().Format("2006-01-02T150405Z")
	ext := string(format)
	if ext == "text" {
		ext = "log"
	}
	return fmt.Sprintf("hitstor-logs-%s.%s", ts, ext)
}

// ContentType returns the HTTP content type for an export format.
func ContentType(format ExportFormat) string {
	switch format {
	case FormatCSV:
		return "text/csv; charset=utf-8"
	case FormatText:
		return "text/plain; charset=utf-8"
	default:
		return "application/x-ndjson; charset=utf-8"
	}
}
