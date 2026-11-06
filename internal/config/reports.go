package config

import "time"

// ReportConfig is a report emailed on a schedule.
type ReportConfig struct {
	ID         string   `yaml:"id"`
	Name       string   `yaml:"name,omitempty"`
	Type       string   `yaml:"type"`             // summary, daily
	Period     string   `yaml:"period,omitempty"` // summary: week or month
	Cron       string   `yaml:"cron"`
	Timezone   string   `yaml:"timezone,omitempty"`
	Recipients []string `yaml:"recipients"`
	Agents     []string `yaml:"agents,omitempty"` // empty: every agent
	Language   string   `yaml:"language,omitempty"`
	Enabled    bool     `yaml:"enabled"`

	Created   time.Time  `yaml:"created"`
	LastSent  *time.Time `yaml:"last_sent,omitempty"` // last attempt, sent or not
	LastError string     `yaml:"last_error,omitempty"`
}
