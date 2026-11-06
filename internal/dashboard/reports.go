package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/cron"
	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/mailer"
	"github.com/hooperits/hitstor/internal/report"
	"github.com/hooperits/hitstor/internal/runlog"
	"github.com/hooperits/hitstor/internal/server"
	"github.com/hooperits/hitstor/internal/users"
)

// Report periods of the protection summary.
const (
	periodWeek  = "week"
	periodMonth = "month"
)

// reportsState guards the scheduled reports in the configuration, which
// both the API and the scheduler change.
type reportsState struct {
	mu   sync.Mutex
	runs *runlog.Log
}

// SetRunLog gives the dashboard the run history its reports read.
func (d *Dashboard) SetRunLog(runs *runlog.Log) {
	d.reports.mu.Lock()
	d.reports.runs = runs
	d.reports.mu.Unlock()
}

func (d *Dashboard) runLog() *runlog.Log {
	d.reports.mu.Lock()
	defer d.reports.mu.Unlock()
	return d.reports.runs
}

// reportSpan is the period a report covers when it ends at to.
func reportSpan(typ, period string) time.Duration {
	switch {
	case typ == report.TypeDaily:
		return 24 * time.Hour
	case period == periodMonth:
		return 30 * 24 * time.Hour
	}
	return 7 * 24 * time.Hour
}

func reportLocation(tz string) *time.Location {
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

// kindInterval is how often a backup kind is scheduled to run; 0 when on
// demand.
func kindInterval(j server.BackupJob, e *server.PlanEntry, now time.Time) time.Duration {
	if e != nil && server.UsesPlan(j) {
		if !e.Enabled {
			return 0
		}
		return report.ScheduleInterval(e.CronExpression, e.Timezone, now)
	}
	switch {
	case !j.ScheduleEnabled:
		return 0
	case j.CronExpression != "":
		return report.ScheduleInterval(j.CronExpression, j.Timezone, now)
	case j.ScheduleHours > 0:
		return time.Duration(j.ScheduleHours) * time.Hour
	}
	return 0
}

// reportJob is how a job stands, for reports.
func reportJob(j server.BackupJob, now time.Time) report.Job {
	out := report.Job{ID: j.ID, Name: j.Name, Agent: j.AgentName, Repo: j.RepoName, Plugin: j.Plugin,
		VerifyStatus: j.LastVerifyStatus, VerifyAt: j.LastVerifyAt}
	if j.JobType != server.JobTypeWorkload {
		out.Kinds = []report.Kind{{Interval: kindInterval(j, nil, now), LastOK: j.LastBackup, LastFailureAt: j.LastErrorAt, LastError: j.LastError}}
		return out
	}
	for _, e := range server.EffectivePlan(j) {
		st := server.KindState(j, e.Kind)
		out.Kinds = append(out.Kinds, report.Kind{Kind: e.Kind, Interval: kindInterval(j, &e, now), LastOK: st.LastBackup,
			LastFailureAt: st.LastErrorAt, LastError: st.LastError, Partial: st.Partial})
	}
	return out
}

// reportInput gathers what a report on [from, to) shows, for the agents in
// scope (all when empty).
func (d *Dashboard) reportInput(typ string, scope []string, from, to time.Time, loc *time.Location, c *i18n.Catalog) (report.Input, error) {
	in := report.Input{Type: typ, From: from, To: to, Generated: time.Now(), Location: loc, Catalog: c,
		Version: d.opts.Version, Scope: scope, Logo: emailLogo()}
	inScope := func(agent string) bool { return len(scope) == 0 || slices.Contains(scope, agent) }

	jobs := map[string]server.BackupJob{}
	repos := map[string]bool{}
	for _, j := range d.registry.ListJobs() {
		if !inScope(j.AgentName) {
			continue
		}
		jobs[j.ID] = j
		repos[j.RepoName] = true
		in.Jobs = append(in.Jobs, reportJob(j, to))
	}

	// The run history, and the snapshots it does not cover: runs from
	// before it existed, which all succeeded.
	runs := d.runLog()
	var seen []string
	if runs != nil {
		recorded, err := runs.Range(from, to)
		if err != nil {
			return in, err
		}
		for _, r := range recorded {
			if inScope(r.Agent) {
				in.Runs = append(in.Runs, r)
			}
			if r.SnapshotID != "" {
				seen = append(seen, r.SnapshotID)
			}
		}
		in.HistorySince = runs.Since()
	}
	covered := func(id string) bool {
		for _, s := range seen {
			if strings.HasPrefix(id, s) {
				return true
			}
		}
		return false
	}
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			sums, err := d.getSnapshotSummaries(mr)
			if err != nil {
				slog.Warn("report: list snapshots", "repo", mr.Name, "error", err)
				continue
			}
			var added uint64
			for _, s := range sums {
				if s.Time.Before(from) || !s.Time.Before(to) {
					continue
				}
				jobID := tagValue(s.Tags, "job:")
				j, ok := jobs[jobID]
				if !ok {
					continue // another agent's, or not a job's
				}
				added += s.Summary.BytesAdded
				if covered(s.ID) {
					continue
				}
				in.Runs = append(in.Runs, runlog.Run{Time: s.Time, JobID: jobID, JobName: j.Name, Agent: j.AgentName, Repo: mr.Name,
					Plugin: j.Plugin, Kind: server.KindFromTags(s.Tags), Result: runlog.ResultOK, SnapshotID: s.ID,
					FilesNew: s.Summary.FilesNew, BytesProcessed: s.Summary.BytesProcessed, BytesAdded: s.Summary.BytesAdded,
					Duration: s.Summary.TotalDuration})
			}
			if len(scope) > 0 && !repos[mr.Name] {
				continue
			}
			m := d.getRepoMetrics(mr, nil, false)
			in.Repos = append(in.Repos, report.Repo{Name: mr.Name, TotalBytes: m.TotalBytes, DiskTotal: m.DiskTotal,
				DiskFree: m.DiskFree, Snapshots: m.Snapshots, AddedInPeriod: added})
		}
	}
	slices.SortStableFunc(in.Runs, func(a, b runlog.Run) int { return a.Time.Compare(b.Time) })

	for _, a := range d.registry.ListAlerts() {
		if a.Status == "active" && inScope(a.AgentName) {
			in.ActiveAlerts++
		}
	}
	return in, nil
}

func tagValue(tags []string, prefix string) string {
	for _, t := range tags {
		if v, ok := strings.CutPrefix(t, prefix); ok {
			return v
		}
	}
	return ""
}

// renderReport makes a report's PDF and its file name.
func (d *Dashboard) renderReport(rc config.ReportConfig, to time.Time) ([]byte, report.Input, string, error) {
	loc := reportLocation(rc.Timezone)
	c := i18n.Get(d.reportLanguage(rc.Language))
	in, err := d.reportInput(rc.Type, rc.Agents, to.Add(-reportSpan(rc.Type, rc.Period)), to, loc, c)
	if err != nil {
		return nil, in, "", err
	}
	pdf, err := report.Render(in)
	if err != nil {
		return nil, in, "", err
	}
	name := "hitstor-" + rc.Type + "-" + to.In(loc).Format("2006-01-02") + ".pdf"
	return pdf, in, name, nil
}

func (d *Dashboard) reportLanguage(lang string) string {
	if i18n.Supported(lang) {
		return lang
	}
	return d.language(nil)
}

// sendReport emails a report to its recipients.
func (d *Dashboard) sendReport(ctx context.Context, rc config.ReportConfig, to time.Time) error {
	if !d.emailConfigured() {
		return errors.New("email is not set up")
	}
	pdf, in, name, err := d.renderReport(rc, to)
	if err != nil {
		return err
	}
	c := in.Catalog
	_, verdict, detail := report.Verdict(in)
	title := report.Title(c, rc.Type)
	if rc.Name != "" && rc.Name != title {
		title = rc.Name + " · " + title
	}
	period := in.From.In(in.Location).Format("02/01/2006 15:04") + " – " + in.To.In(in.Location).Format("02/01/2006 15:04")
	e := emailContent{Lang: c.Code, Subject: title + ": " + verdict,
		Paragraphs: []string{
			c.Tf("Attached is the report {title} for {period}.", "title", report.Title(c, rc.Type), "period", period),
			verdict + ". " + detail,
		},
		Footer: []string{c.T("You receive this report because an administrator added your address to it in HitStor.")}}
	if d.cfg != nil && d.cfg.Email.DashboardURL != "" {
		e.Button, e.Link = c.T("Open HitStor"), strings.TrimRight(d.cfg.Email.DashboardURL, "/")+"/"
		e.LinkHint = c.T("If the button does not work, copy this address into your browser:")
	}
	msg, err := renderEmail(rc.Recipients[0], e)
	if err != nil {
		return err
	}
	msg.To = rc.Recipients
	msg.Attachments = []mailer.Attachment{{Name: name, ContentType: "application/pdf", Data: pdf}}
	return d.sendEmail(ctx, msg)
}

// --- Scheduler ---

// RunReportScheduler sends the scheduled reports until ctx ends. A report
// whose time came while the server was down is sent if it is less than
// six hours late; older slots are skipped.
func (d *Dashboard) RunReportScheduler(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			d.sendDueReports(ctx, now)
		}
	}
}

func (d *Dashboard) sendDueReports(ctx context.Context, now time.Time) {
	if d.cfg == nil {
		return
	}
	d.reports.mu.Lock()
	var due []config.ReportConfig
	var slots []time.Time
	for _, rc := range d.cfg.Reports {
		if !rc.Enabled {
			continue
		}
		expr, err := cron.Parse(rc.Cron)
		if err != nil {
			continue
		}
		from := rc.Created
		if rc.LastSent != nil {
			from = *rc.LastSent
		}
		slot := expr.Next(from.In(reportLocation(rc.Timezone)))
		if slot.IsZero() || slot.After(now) {
			continue
		}
		due = append(due, rc)
		slots = append(slots, slot)
	}
	d.reports.mu.Unlock()

	for i, rc := range due {
		var err error
		if now.Sub(slots[i]) > 6*time.Hour {
			err = fmt.Errorf("skipped: the server was not running at %s", slots[i].Format(time.RFC3339))
		} else {
			sctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			err = d.sendReport(sctx, rc, slots[i])
			cancel()
		}
		if err != nil {
			slog.Warn("scheduled report not sent", "report", rc.Name, "type", rc.Type, "error", err)
		} else {
			slog.Info("scheduled report sent", "report", rc.Name, "type", rc.Type, "recipients", len(rc.Recipients))
		}
		d.markReport(rc.ID, now, err)
	}
}

// markReport records a report's latest attempt.
func (d *Dashboard) markReport(id string, at time.Time, sendErr error) {
	d.reports.mu.Lock()
	defer d.reports.mu.Unlock()
	for i := range d.cfg.Reports {
		if d.cfg.Reports[i].ID == id {
			t := at
			d.cfg.Reports[i].LastSent = &t
			d.cfg.Reports[i].LastError = ""
			if sendErr != nil {
				d.cfg.Reports[i].LastError = sendErr.Error()
			}
		}
	}
	if err := d.saveConfig(); err != nil {
		slog.Warn("report: save configuration", "error", err)
	}
}

// --- API ---

// reportJSON is a scheduled report as the dashboard shows it.
type reportJSON struct {
	config.ReportConfig
	Title    string     `json:"title"`
	Schedule string     `json:"schedule"` // in words
	NextRun  *time.Time `json:"next_run,omitempty"`
}

func (d *Dashboard) toReportJSON(r *http.Request, rc config.ReportConfig) reportJSON {
	c := d.catalog(r)
	out := reportJSON{ReportConfig: rc, Title: report.Title(c, rc.Type)}
	if expr, err := cron.Parse(rc.Cron); err == nil {
		out.Schedule = describeCron(c, expr)
		if rc.Enabled {
			next := expr.Next(time.Now().In(reportLocation(rc.Timezone)))
			out.NextRun = &next
		}
	}
	return out
}

// MarshalJSON flattens the configuration's fields with the view's.
func (r reportJSON) MarshalJSON() ([]byte, error) {
	rc := r.ReportConfig
	return json.Marshal(map[string]any{
		"id": rc.ID, "name": rc.Name, "type": rc.Type, "period": rc.Period, "cron": rc.Cron, "timezone": rc.Timezone,
		"recipients": rc.Recipients, "agents": rc.Agents, "language": rc.Language, "enabled": rc.Enabled,
		"created": rc.Created, "last_sent": rc.LastSent, "last_error": rc.LastError,
		"title": r.Title, "schedule": r.Schedule, "next_run": r.NextRun,
	})
}

func (d *Dashboard) handleReportsPage(w http.ResponseWriter, r *http.Request) {
	var agents []string
	for _, a := range d.registry.ListAgents() {
		agents = append(agents, a.Name)
	}
	c := d.catalog(r)
	var types []map[string]string
	for _, t := range report.Types {
		types = append(types, map[string]string{"type": t, "title": report.Title(c, t), "description": report.Description(c, t)})
	}
	data := d.newPageData("Reports", "PDF reports emailed on a schedule", "reports", map[string]any{
		"Agents":       agents,
		"Types":        types,
		"EmailEnabled": d.emailConfigured(),
		"Languages":    i18n.Languages(),
	})
	d.render(w, r, "reports", data)
}

func (d *Dashboard) handleAPIReports(w http.ResponseWriter, r *http.Request) {
	out := []reportJSON{}
	if d.cfg != nil {
		d.reports.mu.Lock()
		list := slices.Clone(d.cfg.Reports)
		d.reports.mu.Unlock()
		for _, rc := range list {
			out = append(out, d.toReportJSON(r, rc))
		}
	}
	writeJSON(w, out)
}

// reportRequest is a report as the dashboard sends it.
type reportRequest struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Period     string   `json:"period"`
	Cron       string   `json:"cron"`
	Timezone   string   `json:"timezone"`
	Recipients []string `json:"recipients"`
	Agents     []string `json:"agents"`
	Language   string   `json:"language"`
	Enabled    bool     `json:"enabled"`
}

// check validates a request and returns it as a configuration.
func (req reportRequest) check(c *i18n.Catalog, rc config.ReportConfig) (config.ReportConfig, error) {
	if !slices.Contains(report.Types, req.Type) {
		return rc, errors.New(c.T("Choose a type of report."))
	}
	if _, err := cron.Parse(req.Cron); err != nil {
		return rc, fmt.Errorf("%s %v", c.T("Invalid schedule:"), err)
	}
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			return rc, errors.New(c.T("Unknown time zone."))
		}
	}
	var to []string
	for _, a := range req.Recipients {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		addr, err := users.NormalizeEmail(a)
		if err != nil {
			return rc, errors.New(c.Tf("{address} is not a valid email address.", "address", a))
		}
		if !slices.Contains(to, addr) {
			to = append(to, addr)
		}
	}
	if len(to) == 0 {
		return rc, errors.New(c.T("Add at least one recipient."))
	}
	if len(to) > 50 {
		return rc, errors.New(c.T("A report can have at most 50 recipients."))
	}
	period := ""
	if req.Type == report.TypeSummary {
		period = periodWeek
		if req.Period == periodMonth {
			period = periodMonth
		}
	}
	name := strings.TrimSpace(req.Name)
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	lang := ""
	if i18n.Supported(req.Language) {
		lang = req.Language
	}
	rc.Name, rc.Type, rc.Period, rc.Cron, rc.Timezone = name, req.Type, period, req.Cron, req.Timezone
	rc.Recipients, rc.Agents, rc.Language, rc.Enabled = to, slices.Compact(slices.Sorted(slices.Values(req.Agents))), lang, req.Enabled
	return rc, nil
}

func (d *Dashboard) handleAPICreateReport(w http.ResponseWriter, r *http.Request) {
	if _, err := d.requireConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req reportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	rc, err := req.check(d.catalog(r), config.ReportConfig{ID: randomToken(8), Created: time.Now().UTC()})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	d.reports.mu.Lock()
	d.cfg.Reports = append(d.cfg.Reports, rc)
	err = d.saveConfig()
	d.reports.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, d.toReportJSON(r, rc))
}

// findReport returns the index of report id in the configuration, under
// d.reports.mu.
func (d *Dashboard) findReport(id string) int {
	if d.cfg == nil {
		return -1
	}
	return slices.IndexFunc(d.cfg.Reports, func(rc config.ReportConfig) bool { return rc.ID == id })
}

func (d *Dashboard) handleAPIUpdateReport(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	d.reports.mu.Lock()
	i := d.findReport(r.PathValue("id"))
	if i < 0 {
		d.reports.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	rc, err := req.check(d.catalog(r), d.cfg.Reports[i])
	if err != nil {
		d.reports.mu.Unlock()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if rc.Cron != d.cfg.Reports[i].Cron || rc.Timezone != d.cfg.Reports[i].Timezone {
		// A new schedule starts now: no catching up on the old one.
		now := time.Now().UTC()
		rc.LastSent, rc.Created = nil, now
	}
	d.cfg.Reports[i] = rc
	err = d.saveConfig()
	d.reports.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, d.toReportJSON(r, rc))
}

func (d *Dashboard) handleAPIDeleteReport(w http.ResponseWriter, r *http.Request) {
	d.reports.mu.Lock()
	i := d.findReport(r.PathValue("id"))
	if i < 0 {
		d.reports.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	d.cfg.Reports = slices.Delete(d.cfg.Reports, i, i+1)
	err := d.saveConfig()
	d.reports.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (d *Dashboard) reportByID(id string) (config.ReportConfig, bool) {
	d.reports.mu.Lock()
	defer d.reports.mu.Unlock()
	if i := d.findReport(id); i >= 0 {
		return d.cfg.Reports[i], true
	}
	return config.ReportConfig{}, false
}

// handleAPISendReport sends a report now, covering the period up to now.
func (d *Dashboard) handleAPISendReport(w http.ResponseWriter, r *http.Request) {
	rc, ok := d.reportByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	if err := d.sendReport(ctx, rc, time.Now()); err != nil {
		http.Error(w, d.catalog(r).Tf("The report could not be sent: {error}", "error", err.Error()), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"sent_to": rc.Recipients})
}

// handleAPIReportPDF downloads a report as it would be sent now: a saved
// one (?id=) or any type (?type=&period=&lang=&agent=).
func (d *Dashboard) handleAPIReportPDF(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rc := config.ReportConfig{Type: q.Get("type"), Period: q.Get("period"), Language: q.Get("lang"), Agents: q["agent"]}
	if id := q.Get("id"); id != "" {
		var ok bool
		if rc, ok = d.reportByID(id); !ok {
			http.NotFound(w, r)
			return
		}
	}
	if !slices.Contains(report.Types, rc.Type) {
		http.Error(w, "unknown report type", http.StatusBadRequest)
		return
	}
	if rc.Language == "" {
		rc.Language = d.language(r)
	}
	pdf, _, name, err := d.renderReport(rc, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	disposition := "attachment"
	if q.Get("inline") != "" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", disposition+`; filename="`+name+`"`)
	w.Write(pdf)
}
