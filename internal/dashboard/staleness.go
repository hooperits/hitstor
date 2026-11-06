package dashboard

import (
	"time"

	"github.com/hooperits/hitstor/internal/report"
	"github.com/hooperits/hitstor/internal/server"
)

// defaultStaleThreshold is how long an agent may go without a backup before
// it shows as stale, unless alert_stale_threshold says otherwise.
const defaultStaleThreshold = 25 * time.Hour

// staleThreshold is the configured alert_stale_threshold, the same limit the
// stale-agent alerts use.
func (d *Dashboard) staleThreshold() time.Duration {
	if d.cfg != nil && d.cfg.AlertStaleThreshold > 0 {
		return time.Duration(d.cfg.AlertStaleThreshold) * time.Hour
	}
	return defaultStaleThreshold
}

// jobOverdue reports whether a scheduled job's latest backup is older than
// its schedule allows, by the same rule as the reports: the longest gap of
// its schedule plus half of it.
func jobOverdue(j server.BackupJob, now time.Time) bool {
	if j.LastBackup == nil {
		return false
	}
	objective := report.Objective(kindInterval(j, nil, now))
	return objective > 0 && now.Sub(*j.LastBackup) > objective
}
