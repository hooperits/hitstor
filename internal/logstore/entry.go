package logstore

import (
	"fmt"
	"strings"
	"time"
)

// Entry is a single log record for View Logs and export.
type Entry struct {
	Time      time.Time         `json:"time"`
	Level     string            `json:"level"`
	Source    string            `json:"source"`
	Component string            `json:"component,omitempty"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// Filter selects log entries for query and export.
type Filter struct {
	Source string
	Level  string
	Since  time.Time
	Query  string
	Limit  int
}

// Matches reports whether the entry satisfies the filter.
func (e Entry) Matches(f Filter) bool {
	if !f.Since.IsZero() && e.Time.Before(f.Since) {
		return false
	}
	if f.Source != "" && f.Source != "all" && !sourceMatches(f.Source, e.Source) {
		return false
	}
	if f.Level != "" && !strings.EqualFold(f.Level, e.Level) {
		return false
	}
	if f.Query != "" {
		q := strings.ToLower(f.Query)
		hay := strings.ToLower(e.Message + " " + e.Source + " " + e.Component)
		for _, v := range e.Fields {
			hay += " " + strings.ToLower(v)
		}
		if !strings.Contains(hay, q) {
			return false
		}
	}
	return true
}

func sourceMatches(want, have string) bool {
	if want == have {
		return true
	}
	if strings.HasPrefix(want, "agent:") && have == want {
		return true
	}
	return false
}

func (e Entry) formatText() string {
	ts := e.Time.UTC().Format("2006-01-02 15:04:05")
	src := e.Source
	if src == "" {
		src = "server"
	}
	line := fmt.Sprintf("%s [%s] [%s] %s", ts, strings.ToUpper(e.Level), src, e.Message)
	if len(e.Fields) == 0 {
		return line
	}
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, k+"="+v)
	}
	return line + " " + strings.Join(parts, " ")
}
