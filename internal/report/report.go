// Package report renders HitStor's PDF reports: an executive summary of
// how well the backups protect the servers over a period, and a daily
// status of every run. The dashboard gathers the data (Input); this
// package only judges and lays it out, so reports are tested without a
// server.
package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/cron"
	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/runlog"
)

// Report types.
const (
	TypeSummary = "summary" // protection summary, for a week or a month
	TypeDaily   = "daily"   // every run of the last day
)

// Types lists the report types in display order.
var Types = []string{TypeSummary, TypeDaily}

// Input is what a report is made of.
type Input struct {
	Type      string
	From, To  time.Time // the period: [From, To)
	Generated time.Time
	Location  *time.Location // dates are shown in it
	Catalog   *i18n.Catalog  // language
	Version   string         // HitStor version, in the footer
	Scope     []string       // the agents covered; empty: all
	Logo      []byte         // PNG shown in the header, optional

	Jobs  []Job
	Runs  []runlog.Run // runs that ended in the period
	Repos []Repo
	// ActiveAlerts are unresolved ransomware alerts.
	ActiveAlerts int
	// HistorySince is when the run history starts: failures before it
	// are unknown (successes come from the snapshots).
	HistorySince time.Time
}

// Job is a backup job and how each of its backup kinds stands.
type Job struct {
	ID, Name, Agent, Repo string
	Plugin                string // workload plugin; empty for file jobs
	Kinds                 []Kind // file jobs have one, with Kind ""
	// Restore verification of the latest backup: ok, failed, error,
	// skipped or empty when never verified.
	VerifyStatus string
	VerifyAt     *time.Time
}

// Kind is one backup kind of a job: its schedule and latest outcomes.
type Kind struct {
	Kind          string        // full, differential, log; "" for file jobs
	Interval      time.Duration // the longest wait between scheduled runs; 0: on demand
	LastOK        *time.Time
	LastFailureAt *time.Time
	LastError     string
	Partial       bool // the latest run stored some objects and failed others
}

// Repo is a repository's storage.
type Repo struct {
	Name                string
	TotalBytes          uint64
	DiskTotal, DiskFree uint64 // 0 when unknown (object storage)
	Snapshots           int
	AddedInPeriod       uint64 // new data stored in the period
}

// Statuses of a job or kind, worst last.
const (
	StatusManual  = "manual"  // on demand: no objective to meet
	StatusOK      = "ok"      // protected within its objective
	StatusWarning = "warning" // within its objective, but the latest run failed or was partial
	StatusLate    = "late"    // the latest good backup is older than the objective
	StatusNever   = "never"   // scheduled but never backed up
)

func statusRank(s string) int {
	switch s {
	case StatusOK:
		return 1
	case StatusWarning:
		return 2
	case StatusLate:
		return 3
	case StatusNever:
		return 4
	}
	return 0
}

// Objective is how old the latest good backup of a kind may be: its
// schedule's longest gap and half again, so one missed run already shows
// while a slow run does not.
func Objective(interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	slack := interval / 2
	if slack < 30*time.Minute {
		slack = 30 * time.Minute
	}
	return interval + slack
}

// Status judges a kind at now.
func (k Kind) Status(now time.Time) string {
	failedLast := k.LastFailureAt != nil && (k.LastOK == nil || k.LastFailureAt.After(*k.LastOK))
	switch {
	case k.Interval <= 0:
		if failedLast || k.Partial {
			return StatusWarning
		}
		return StatusManual
	case k.LastOK == nil:
		return StatusNever
	case now.Sub(*k.LastOK) > Objective(k.Interval):
		return StatusLate
	case failedLast || k.Partial:
		return StatusWarning
	}
	return StatusOK
}

// Status is the worst status of the job's kinds; a failed restore
// verification makes a protected job a warning.
func (j Job) Status(now time.Time) string {
	worst := StatusManual
	for _, k := range j.Kinds {
		if s := k.Status(now); statusRank(s) > statusRank(worst) {
			worst = s
		}
	}
	if worst == StatusOK && (j.VerifyStatus == "failed" || j.VerifyStatus == "error") {
		return StatusWarning
	}
	return worst
}

// LastOK is the job's latest good backup of any kind.
func (j Job) LastOK() *time.Time {
	var last *time.Time
	for _, k := range j.Kinds {
		if k.LastOK != nil && (last == nil || k.LastOK.After(*last)) {
			last = k.LastOK
		}
	}
	return last
}

// ScheduleInterval is the longest wait between two runs of a cron schedule
// over the next five weeks (so monthly schedules count), in its time zone.
// Zero when the expression is invalid.
func ScheduleInterval(expr, tz string, now time.Time) time.Duration {
	e, err := cron.Parse(expr)
	if err != nil {
		return 0
	}
	loc := time.Local
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	t := e.Next(now.In(loc))
	if t.IsZero() {
		return 0
	}
	end := t.Add(35 * 24 * time.Hour)
	var longest time.Duration
	for i := 0; i < 10000 && t.Before(end); i++ {
		n := e.Next(t)
		if n.IsZero() {
			break
		}
		if gap := n.Sub(t); gap > longest {
			longest = gap
		}
		t = n
	}
	return longest
}

// Overall levels of a report.
const (
	LevelGood      = "good"
	LevelAttention = "attention"
	LevelCritical  = "critical"
)

// Issue is something a reader should act on.
type Issue struct {
	Job, Agent string
	Level      string // attention or critical
	Problem    string
	Since      *time.Time
}

// summary are the figures both reports show.
type summary struct {
	runs, ok, partial, failed int
	processed, added          uint64
	jobs, current             int // jobs with an objective, and those meeting it
	issues                    []Issue
	level                     string
}

func (in Input) now() time.Time { return in.To }

// summarize counts the runs and judges the jobs.
func (in Input) summarize() summary {
	c := in.Catalog
	var s summary
	for _, r := range in.Runs {
		s.runs++
		switch r.Result {
		case runlog.ResultOK:
			s.ok++
		case runlog.ResultPartial:
			s.partial++
		default:
			s.failed++
		}
		s.processed += r.BytesProcessed
		s.added += r.BytesAdded
	}
	now := in.now()
	for _, j := range in.Jobs {
		st := j.Status(now)
		if st != StatusManual {
			s.jobs++
			if st == StatusOK || st == StatusWarning {
				s.current++
			}
		}
		for _, k := range j.Kinds {
			kname := kindName(c, k.Kind)
			switch k.Status(now) {
			case StatusNever:
				s.issues = append(s.issues, Issue{Job: j.Name, Agent: j.Agent, Level: LevelCritical,
					Problem: c.Tf("{kind}: never backed up", "kind", kname)})
			case StatusLate:
				s.issues = append(s.issues, Issue{Job: j.Name, Agent: j.Agent, Level: LevelCritical, Since: k.LastOK,
					Problem: c.Tf("{kind}: no good backup for {age}", "kind", kname, "age", formatAge(c, now.Sub(*k.LastOK)))})
			case StatusWarning:
				if k.LastFailureAt != nil && (k.LastOK == nil || k.LastFailureAt.After(*k.LastOK)) {
					s.issues = append(s.issues, Issue{Job: j.Name, Agent: j.Agent, Level: LevelAttention, Since: k.LastFailureAt,
						Problem: c.Tf("{kind}: the latest run failed: {error}", "kind", kname, "error", firstLine(k.LastError))})
				} else if k.Partial {
					s.issues = append(s.issues, Issue{Job: j.Name, Agent: j.Agent, Level: LevelAttention, Since: k.LastOK,
						Problem: c.Tf("{kind}: the latest run left some objects out", "kind", kname)})
				}
			}
		}
		if j.VerifyStatus == "failed" || j.VerifyStatus == "error" {
			s.issues = append(s.issues, Issue{Job: j.Name, Agent: j.Agent, Level: LevelAttention, Since: j.VerifyAt,
				Problem: c.T("The test restore of the latest backup failed")})
		}
	}
	if in.ActiveAlerts > 0 {
		s.issues = append(s.issues, Issue{Level: LevelCritical,
			Problem: c.Tf("{n} active ransomware alerts", "n", in.ActiveAlerts)})
	}
	for _, r := range in.Repos {
		if r.DiskTotal > 0 && float64(r.DiskFree)/float64(r.DiskTotal) < 0.10 {
			s.issues = append(s.issues, Issue{Job: r.Name, Level: LevelCritical,
				Problem: c.Tf("Repository disk almost full: {free} free", "free", formatBytes(c, r.DiskFree))})
		}
	}
	sort.SliceStable(s.issues, func(i, j int) bool {
		if s.issues[i].Level != s.issues[j].Level {
			return s.issues[i].Level == LevelCritical
		}
		return s.issues[i].Agent+s.issues[i].Job < s.issues[j].Agent+s.issues[j].Job
	})
	s.level = LevelGood
	for _, is := range s.issues {
		if is.Level == LevelCritical {
			s.level = LevelCritical
			break
		}
		s.level = LevelAttention
	}
	return s
}

// --- Formatting ---

func spanish(c *i18n.Catalog) bool { return c.Code == "es" }

func decimal(c *i18n.Catalog, v float64, prec int) string {
	s := fmt.Sprintf("%.*f", prec, v)
	if spanish(c) {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

func formatBytes(c *i18n.Catalog, b uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	v, i := float64(b), 0
	for v >= 1000 && i < len(units)-1 {
		v /= 1000
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", b)
	}
	prec := 1
	if v >= 100 {
		prec = 0
	}
	return decimal(c, v, prec) + " " + units[i]
}

func formatPercent(c *i18n.Catalog, part, total int) string {
	if total == 0 {
		return "—"
	}
	p := 100 * float64(part) / float64(total)
	if p > 99.5 && part < total {
		p = 99.5 // never round a failure away
	}
	prec := 0
	if p != math.Trunc(p) && p > 99 {
		prec = 1
	}
	return decimal(c, p, prec) + " %"
}

func formatDuration(c *i18n.Catalog, sec float64) string {
	d := time.Duration(sec * float64(time.Second))
	switch {
	case sec <= 0:
		return "—"
	case d < time.Minute:
		return c.Tf("{n} s", "n", int(math.Round(sec)))
	case d < time.Hour:
		return c.Tf("{m} min {s} s", "m", int(d/time.Minute), "s", int(d%time.Minute/time.Second))
	}
	return c.Tf("{h} h {m} min", "h", int(d/time.Hour), "m", int(d%time.Hour/time.Minute))
}

// formatAge says how long ago, roughly: "3 h", "2 days".
func formatAge(c *i18n.Catalog, d time.Duration) string {
	switch {
	case d < time.Hour:
		return c.Tf("{n} min", "n", int(d/time.Minute))
	case d < 48*time.Hour:
		return c.Tf("{n} h", "n", int(d/time.Hour))
	}
	return c.Tf("{n} days", "n", int(d/(24*time.Hour)))
}

func (in Input) loc() *time.Location {
	if in.Location != nil {
		return in.Location
	}
	return time.Local
}

func (in Input) formatTime(t time.Time) string {
	t = t.In(in.loc())
	if spanish(in.Catalog) {
		return t.Format("02/01/2006 15:04")
	}
	return t.Format("Jan 2, 2006 15:04")
}

func (in Input) formatDate(t time.Time) string {
	t = t.In(in.loc())
	if spanish(in.Catalog) {
		return t.Format("02/01/2006")
	}
	return t.Format("Jan 2, 2006")
}

func (in Input) formatClock(t time.Time) string { return t.In(in.loc()).Format("15:04") }

func (in Input) formatMaybe(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return in.formatTime(*t)
}

// kindName names a backup kind; file jobs back up files.
func kindName(c *i18n.Catalog, kind string) string {
	switch kind {
	case "":
		return c.T("Files")
	case "full":
		return c.T("Full")
	case "differential":
		return c.T("Differential")
	case "log":
		return c.T("Log")
	}
	return kind
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 160 {
		s = string(r[:157]) + "…"
	}
	return s
}

// Title names a report type.
func Title(c *i18n.Catalog, typ string) string {
	if typ == TypeDaily {
		return c.T("Daily backup status")
	}
	return c.T("Protection summary")
}

// Description says what a report type is for.
func Description(c *i18n.Catalog, typ string) string {
	if typ == TypeDaily {
		return c.T("Every run of the last 24 hours, the failures with their reason, and how each job stands. For the team that runs the backups.")
	}
	return c.T("Whether the servers are protected: success rate, jobs within their objective, what needs attention, and storage. For managers, weekly or monthly.")
}
