package dashboard

import (
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/server"
)

func TestJobOverdueFollowsTheSchedule(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) // a Wednesday
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	weekly := server.BackupJob{ScheduleEnabled: true, CronExpression: "0 4 * * 0", Timezone: "UTC"}
	daily := server.BackupJob{ScheduleEnabled: true, CronExpression: "0 1 * * *", Timezone: "UTC"}
	hourly := server.BackupJob{ScheduleEnabled: true, ScheduleHours: 6}
	cases := []struct {
		name string
		job  server.BackupJob
		last *time.Time
		want bool
	}{
		{"weekly, backed up 3 days ago", weekly, ago(3 * 24 * time.Hour), false},
		{"weekly, backed up 11 days ago", weekly, ago(11 * 24 * time.Hour), true},
		{"daily, backed up 30 h ago", daily, ago(30 * time.Hour), false},
		{"daily, backed up 40 h ago", daily, ago(40 * time.Hour), true},
		{"every 6 h, backed up 8 h ago", hourly, ago(8 * time.Hour), false},
		{"every 6 h, backed up 10 h ago", hourly, ago(10 * time.Hour), true},
		{"never backed up", daily, nil, false},
	}
	for _, c := range cases {
		c.job.LastBackup = c.last
		if got := jobOverdue(c.job, now); got != c.want {
			t.Errorf("%s: overdue = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStaleThresholdUsesConfig(t *testing.T) {
	d := &Dashboard{}
	if got := d.staleThreshold(); got != 25*time.Hour {
		t.Fatalf("default = %v", got)
	}
	d.cfg = &config.Config{AlertStaleThreshold: 72}
	if got := d.staleThreshold(); got != 72*time.Hour {
		t.Fatalf("configured = %v", got)
	}
	last := time.Now().Add(-48 * time.Hour)
	v := buildAgentView(server.AgentRecord{Name: "a", LastBackup: &last}, 0, time.Now(), d.staleThreshold())
	if v.BackupStatus != "ok" {
		t.Fatalf("48 h old backup with a 72 h threshold: status %q", v.BackupStatus)
	}
}
