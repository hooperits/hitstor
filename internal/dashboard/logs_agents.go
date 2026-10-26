package dashboard

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/logstore"
)

func (d *Dashboard) fetchAgentLogs(r *http.Request, f logstore.Filter) ([]logstore.Entry, []string, string) {
	if d.registry == nil {
		return nil, nil, ""
	}
	wantAgent := ""
	if strings.HasPrefix(f.Source, "agent:") {
		wantAgent = strings.TrimPrefix(f.Source, "agent:")
	}
	if f.Source != "" && f.Source != "all" && !strings.HasPrefix(f.Source, "agent:") {
		return nil, nil, ""
	}

	agents := d.registry.ListAgents()
	var names []string
	var entries []logstore.Entry
	var warnings []string

	for _, ag := range agents {
		if ag.AgentAddr == "" {
			continue
		}
		names = append(names, ag.Name)
		if wantAgent != "" && ag.Name != wantAgent {
			continue
		}
		lines, err := d.fetchAgentLogsRPC(r.Context(), ag.Name, ag.AgentAddr, f)
		if err != nil {
			warnings = append(warnings, ag.Name+": "+err.Error())
			continue
		}
		entries = append(entries, lines...)
	}
	return entries, names, strings.Join(warnings, "; ")
}

func (d *Dashboard) fetchAgentLogsRPC(ctx context.Context, name, addr string, f logstore.Filter) ([]logstore.Entry, error) {
	token, err := d.registry.DecryptToken(name)
	if err != nil {
		return nil, err
	}
	client, err := d.dialManagedAgent(name, addr)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	since := formatLogSince(f.Since)
	limit := int32(f.Limit)
	if limit <= 0 {
		limit = 500
	}
	resp, err := client.FetchLogs(ctx, token, since, f.Level, f.Query, limit)
	if err != nil {
		return nil, err
	}
	source := "agent:" + name
	out := make([]logstore.Entry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		ts, parseErr := time.Parse(time.RFC3339Nano, e.Time)
		if parseErr != nil {
			ts, parseErr = time.Parse(time.RFC3339, e.Time)
		}
		if parseErr != nil {
			ts = time.Now().UTC()
		}
		entry := logstore.Entry{
			Time:      ts.UTC(),
			Level:     e.Level,
			Source:    source,
			Component: e.Component,
			Message:   e.Message,
			Fields:    e.Fields,
		}
		if entry.Source == "" || entry.Source == "agent" {
			entry.Source = source
		}
		out = append(out, entry)
	}
	return out, nil
}

func formatLogSince(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	age := time.Since(t)
	switch {
	case age <= 20*time.Minute:
		return "15m"
	case age <= 90*time.Minute:
		return "1h"
	case age <= 36*time.Hour:
		return "24h"
	case age <= 8*24*time.Hour:
		return "7d"
	default:
		return ""
	}
}
