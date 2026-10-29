package dashboard

import (
	"regexp"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/cron"
	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/server"
)

// planKindView is one backup kind of a workload job's plan, for display.
type planKindView struct {
	Kind         string             `json:"kind"`
	Label        string             `json:"label"`
	Enabled      bool               `json:"enabled"`
	Cron         string             `json:"cron_expression,omitempty"`
	Timezone     string             `json:"timezone,omitempty"`
	Schedule     string             `json:"schedule"`
	NextRun      string             `json:"next_run,omitempty"`
	NextRunAt    *time.Time         `json:"next_run_at,omitempty"`
	Next         bool               `json:"next"` // the kind that runs next
	LastBackup   *time.Time         `json:"last_backup,omitempty"`
	Failed       bool               `json:"failed"`
	LastError    string             `json:"last_error,omitempty"`
	LastErrorAt  *time.Time         `json:"last_error_at,omitempty"`
	FullRequired []fullRequiredItem `json:"full_required,omitempty"`
	// Partial marks a latest run that backed up some objects and failed
	// others; Failures are failed objects that need no full backup, and
	// Excluded the objects left out by design.
	Partial  bool                `json:"partial,omitempty"`
	Failures []objectOutcomeView `json:"failures,omitempty"`
	Excluded []objectOutcomeView `json:"excluded,omitempty"`
}

// objectOutcomeView is an object of the latest run that produced no
// backup, explained in the reader's language.
type objectOutcomeView struct {
	Object  string `json:"object"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// kindLabel names a backup kind in c's language.
func kindLabel(c *i18n.Catalog, kind string) string {
	switch kind {
	case "full":
		return c.T("Full")
	case "differential":
		return c.T("Differential")
	case "log":
		return c.T("Transaction log")
	}
	return kind
}

// buildPlanView describes each kind of j's plan: its schedule, next run and
// last outcome. Nil for jobs without a plan.
func buildPlanView(c *i18n.Catalog, j server.BackupJob, now time.Time) []planKindView {
	if !server.UsesPlan(j) {
		return nil
	}
	dueKind, _, hasDue := server.DueKind(j, now)
	var out []planKindView
	for _, e := range server.EffectivePlan(j) {
		st := server.KindState(j, e.Kind)
		v := planKindView{
			Kind: e.Kind, Label: kindLabel(c, e.Kind), Enabled: e.Enabled,
			Cron: e.CronExpression, Timezone: e.Timezone,
			Schedule:   c.T("On demand only"),
			LastBackup: st.LastBackup,
		}
		if e.Enabled {
			if expr, err := cron.Parse(e.CronExpression); err == nil {
				v.Schedule = describeCron(c, expr)
			}
			if next := server.NextKindRun(j, e, now); !next.IsZero() {
				loc := time.Local
				if e.Timezone != "" {
					if l, err := time.LoadLocation(e.Timezone); err == nil {
						loc = l
					}
				}
				v.NextRun, v.NextRunAt = dateTimeIn(c, next.In(loc)), &next
			}
		}
		v.Next = hasDue && !j.RunNow && e.Kind == dueKind && v.NextRunAt != nil
		if st.LastErrorAt != nil && (st.LastBackup == nil || st.LastErrorAt.After(*st.LastBackup)) {
			v.Failed, v.LastError, v.LastErrorAt = true, st.LastError, st.LastErrorAt
			if len(st.Objects) == 0 {
				// Plugins and agents that report no objects say it in the error.
				v.FullRequired = parseFullRequired(c, e.Kind, st.LastError)
			}
		}
		v.Partial = st.Partial && !v.Failed
		for _, o := range st.Objects {
			switch {
			case o.Status == "excluded":
				v.Excluded = append(v.Excluded, objectOutcome(c, o))
			case chainReasons[o.Reason]:
				v.FullRequired = append(v.FullRequired, fullRequiredFor(c, e.Kind, o.Object, o.Reason, o.Message))
			default:
				v.Failures = append(v.Failures, objectOutcome(c, o))
			}
		}
		out = append(out, v)
	}
	return out
}

// planStatus is a plan job's status: pending, disabled, failed (a kind's
// latest run backed up nothing), partial (a kind's latest run left some
// objects out that it should have backed up), overdue (a kind missed its
// slot by over an hour), ok, or never.
func planStatus(j server.BackupJob, plan []planKindView, now time.Time) string {
	if j.RunNow {
		return "pending"
	}
	enabled, ran, partial := false, false, false
	for _, k := range plan {
		if k.Failed {
			return "failed"
		}
		enabled = enabled || k.Enabled
		ran = ran || k.LastBackup != nil
		partial = partial || k.Partial
	}
	if partial {
		return "partial"
	}
	if !enabled {
		return "disabled"
	}
	for _, k := range plan {
		if k.NextRunAt != nil && k.LastBackup != nil && now.Sub(*k.NextRunAt) > time.Hour {
			return "overdue"
		}
	}
	if ran {
		return "ok"
	}
	return "never"
}

// planSummary sums a plan up in one line for job lists: each scheduled
// kind and when it runs.
func planSummary(c *i18n.Catalog, plan []planKindView) (schedule, next string) {
	var parts []string
	for _, k := range plan {
		if k.Enabled {
			parts = append(parts, k.Label+": "+k.Schedule)
		}
		if k.Next {
			next = k.Label + " · " + k.NextRun
		}
	}
	if len(parts) == 0 {
		return c.T("On demand only"), ""
	}
	return strings.Join(parts, " · "), next
}

// fullRequiredItem is one database a differential or log backup could not
// cover until a full backup is taken (see the mssql plugin's checkChain).
type fullRequiredItem struct {
	Object  string `json:"object"`
	Reason  string `json:"reason"`  // no-full, external-full, external-log, unknown-base
	Message string `json:"message"` // the reason in the reader's language
	// When, By and Where describe the other tool's backup, if any.
	When  string `json:"when,omitempty"`
	By    string `json:"by,omitempty"`
	Where string `json:"where,omitempty"`
	Raw   string `json:"raw,omitempty"`
}

const fullRequiredPrefix = "full backup required: "

var (
	fullRequiredItemRe = regexp.MustCompile(`^(.+?) \(([a-z-]+)(?:: (.*))?\)$`)
	otherBackupRe      = regexp.MustCompile(`^(full|log backup) of (.+?) by (.*) to (.*)$`)
)

// chainReasons are the reasons a differential or log backup needs a full
// backup first (see the mssql plugin's checkChain).
var chainReasons = map[string]bool{"no-full": true, "external-full": true, "external-log": true, "unknown-base": true, "not-held": true}

// parseFullRequired reads the plugin's "full backup required" error, which
// may follow the agent's own context ("db: ..."). Nil for other errors.
func parseFullRequired(c *i18n.Catalog, kind, msg string) []fullRequiredItem {
	i := strings.Index(msg, fullRequiredPrefix)
	if i < 0 {
		return nil
	}
	var out []fullRequiredItem
	for _, part := range strings.Split(msg[i+len(fullRequiredPrefix):], "; ") {
		m := fullRequiredItemRe.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			out = append(out, fullRequiredItem{Object: part, Message: c.T("A full backup is required first.")})
			continue
		}
		out = append(out, fullRequiredFor(c, kind, m[1], m[2], m[3]))
	}
	return out
}

// fullRequiredFor explains why object's kind backup needs a full first.
// detail is the plugin's, e.g. "full of <when> by <who> to <where>".
func fullRequiredFor(c *i18n.Catalog, kind, object, reason, detail string) fullRequiredItem {
	it := fullRequiredItem{Object: object, Reason: reason, Raw: detail}
	if o := otherBackupRe.FindStringSubmatch(detail); o != nil {
		it.When, it.By, it.Where = o[2], o[3], o[4]
	}
	switch reason {
	case "no-full":
		if kind == "log" {
			it.Message = c.T("No full backup has started this database's log chain yet.")
		} else {
			it.Message = c.T("This database has no full backup to base a differential on.")
		}
	case "external-full":
		it.Message = c.T("The full backup it depends on was taken by another tool, so HitStor could not restore it.")
	case "external-log":
		it.Message = c.T("Another tool took the last log backup, which leaves a gap in HitStor's log chain.")
	case "unknown-base":
		it.Message = c.T("SQL Server has no record of the full backup it depends on.")
	case "not-held":
		it.Message = c.T("HitStor took the backup it depends on but does not hold it, so the chain has a gap.")
	default:
		it.Message = c.T("A full backup is required first.")
	}
	return it
}

// objectOutcome explains an object left out by design, or one that failed
// for a reason a full backup would not fix.
func objectOutcome(c *i18n.Catalog, o server.ObjectOutcome) objectOutcomeView {
	v := objectOutcomeView{Object: o.Object, Reason: o.Reason, Detail: o.Message}
	switch o.Reason {
	case "simple-recovery":
		v.Message, v.Detail = c.T("SIMPLE recovery: no log backups; it restores to its latest full or differential backup."), ""
	case "full-only":
		v.Message, v.Detail = c.T("master takes only full backups."), ""
	case "not-online":
		v.Message = c.T("The database is not online.")
	case "error":
		v.Message = c.T("The backup failed.")
	default:
		v.Message = o.Reason
	}
	if v.Object == "" {
		v.Object = c.T("(run)")
	}
	return v
}
