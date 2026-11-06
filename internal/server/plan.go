package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/cron"
)

// A workload job's backup plan gives each backup kind of one selection its
// own schedule, as database backup tools do: weekly fulls, daily
// differentials and frequent log backups of the same databases. The
// server picks the kind that is due next and hands it to the agent as the
// job's BackupKind, so agents need no notion of plans.

// PlanEntry schedules one backup kind of a workload job.
type PlanEntry struct {
	Kind           string `json:"kind"`
	Enabled        bool   `json:"enabled"` // false: on demand only
	CronExpression string `json:"cron_expression,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
}

// KindRun is the outcome of a workload job's runs of one kind.
type KindRun struct {
	LastBackup  *time.Time `json:"last_backup,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	LastErrorAt *time.Time `json:"last_error_at,omitempty"`
	// Objects are the objects of the latest run that produced no item:
	// excluded by design or failed. Partial marks a run that stored some
	// objects and failed others.
	Objects []ObjectOutcome `json:"objects,omitempty"`
	Partial bool            `json:"partial,omitempty"`
}

// ObjectOutcome is a plugin's report on one object that produced no item.
type ObjectOutcome struct {
	Object  string `json:"object"`
	Status  string `json:"status"` // excluded, failed
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// failedObjects counts the failed objects among outcomes.
func failedObjects(outcomes []ObjectOutcome) int {
	n := 0
	for _, o := range outcomes {
		if o.Status == "failed" {
			n++
		}
	}
	return n
}

// failed reports whether the kind's latest run failed.
func (k KindRun) failed() bool {
	return k.LastErrorAt != nil && (k.LastBackup == nil || k.LastErrorAt.After(*k.LastBackup))
}

// kindRank orders kinds when two are due at once: the larger backup first,
// since it makes the smaller one redundant (see kindCovers).
func kindRank(kind string) int {
	switch kind {
	case "full":
		return 0
	case "differential":
		return 1
	case "log":
		return 2
	}
	return 3
}

// kindCovers reports whether a successful run of kind also fills a slot of
// other that falls at the same time: a full holds everything a
// differential taken then would. Log backups are never covered, since
// they keep the log chain.
func kindCovers(kind, other string) bool {
	return kind == "full" && other == "differential"
}

// EffectivePlan returns the job's plan. Workload jobs made before plans
// run their one BackupKind on the job's schedule; they read as a one-entry
// plan. Filesystem jobs have no plan.
func EffectivePlan(j BackupJob) []PlanEntry {
	if j.JobType != JobTypeWorkload {
		return nil
	}
	if len(j.Plan) > 0 {
		return j.Plan
	}
	kind := j.BackupKind
	if kind == "" {
		kind = "full"
	}
	return []PlanEntry{{Kind: kind, Enabled: j.ScheduleEnabled && j.CronExpression != "", CronExpression: j.CronExpression, Timezone: j.Timezone}}
}

// UsesPlan reports whether the job is scheduled by its plan: workload jobs
// with a plan or a cron schedule. Older interval-hour jobs keep the
// filesystem jobs' scheduling.
func UsesPlan(j BackupJob) bool {
	return j.JobType == JobTypeWorkload && (len(j.Plan) > 0 || j.CronExpression != "" || !j.ScheduleEnabled)
}

// KindState returns the job's record of kind's runs. Jobs made before plans
// kept one record for the whole job.
func KindState(j BackupJob, kind string) KindRun {
	if r, ok := j.KindRuns[kind]; ok {
		return r
	}
	if len(j.Plan) == 0 && j.JobType == JobTypeWorkload {
		if legacy := j.BackupKind; legacy == kind || (legacy == "" && kind == "full") {
			return KindRun{LastBackup: j.LastBackup, LastError: j.LastError, LastErrorAt: j.LastErrorAt}
		}
	}
	return KindRun{}
}

// planLocation resolves an entry's timezone; unknown zones fall back to UTC.
func planLocation(tz string) *time.Location {
	if tz == "" {
		return time.Local
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.UTC
}

// NextKindRun returns when the entry's kind runs next: the first slot after
// its last success, or after its last failure so that a failed run waits
// for the next slot instead of retrying on every poll. A kind that never
// ran is due at the current slot if now falls in one. Zero when the entry
// is on demand only or its expression is invalid.
func NextKindRun(j BackupJob, e PlanEntry, now time.Time) time.Time {
	if !e.Enabled || e.CronExpression == "" {
		return time.Time{}
	}
	expr, err := cron.Parse(e.CronExpression)
	if err != nil {
		return time.Time{}
	}
	loc := planLocation(e.Timezone)
	st := KindState(j, e.Kind)
	from := now.Add(-time.Minute)
	switch {
	case st.failed():
		from = *st.LastErrorAt
	case st.LastBackup != nil:
		from = *st.LastBackup
	}
	return expr.Next(from.In(loc))
}

// DueKind returns the kind the job should run next and when: the kind of a
// manual run when one is pending, else the plan entry whose next slot comes
// first (on ties, the larger backup). ok is false when nothing is scheduled.
func DueKind(j BackupJob, now time.Time) (kind string, at time.Time, ok bool) {
	plan := EffectivePlan(j)
	if j.RunNow {
		kind = j.RunNowKind
		if kind == "" && len(plan) > 0 {
			kind = plan[0].Kind
		}
		return kind, now, true
	}
	for _, e := range plan {
		next := NextKindRun(j, e, now)
		if next.IsZero() {
			continue
		}
		if !ok || next.Before(at) || (next.Equal(at) && kindRank(e.Kind) < kindRank(kind)) {
			kind, at, ok = e.Kind, next, true
		}
	}
	return kind, at, ok
}

// ValidatePlan checks a plan against the kinds the plugin offers.
func ValidatePlan(plan []PlanEntry, kinds []string) error {
	seen := map[string]bool{}
	for _, e := range plan {
		if e.Kind == "" {
			return fmt.Errorf("plan entry without a kind")
		}
		if len(kinds) > 0 && !containsString(kinds, e.Kind) {
			return fmt.Errorf("the plugin does not take %s backups (%s)", e.Kind, strings.Join(kinds, ", "))
		}
		if seen[e.Kind] {
			return fmt.Errorf("plan schedules %s backups twice", e.Kind)
		}
		seen[e.Kind] = true
		if e.Enabled {
			if _, err := cron.Parse(e.CronExpression); err != nil {
				return fmt.Errorf("%s schedule: invalid cron expression: %w", e.Kind, err)
			}
		}
		if e.Timezone != "" {
			if _, err := time.LoadLocation(e.Timezone); err != nil {
				return fmt.Errorf("%s schedule: unknown timezone %s", e.Kind, e.Timezone)
			}
		}
	}
	return nil
}

// KindFromTags returns the backup kind recorded in a session's tags.
func KindFromTags(tags []string) string {
	for _, t := range tags {
		if k, ok := strings.CutPrefix(t, "kind:"); ok {
			return k
		}
	}
	return ""
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
