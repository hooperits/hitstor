//go:build windows

package dashboard

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/logstore"
)

func (d *Dashboard) fetchOSLogsPlatform(r *http.Request, f logstore.Filter) ([]logstore.Entry, string) {
	if f.Source != "" && f.Source != "all" && f.Source != "system" {
		return nil, ""
	}
	cfg := d.logStoreConfig()
	if !cfg.FileEnabled {
		return nil, "system logs on Windows use the log file; enable log_file_enabled in Settings"
	}
	data, err := os.ReadFile(cfg.FilePath)
	if err != nil {
		return nil, "read log file: " + err.Error()
	}
	var out []logstore.Entry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if e, ok := logstore.ParseSlogTextLine(line); ok {
			e.Source = "system"
			out = append(out, e)
			continue
		}
		out = append(out, logstore.Entry{
			Time:    time.Now().UTC(),
			Level:   "info",
			Source:  "system",
			Message: line,
		})
	}
	return out, ""
}
