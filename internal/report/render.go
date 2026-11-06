package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/runlog"
)

// Render draws the report as a PDF.
func Render(in Input) ([]byte, error) {
	if in.Catalog == nil {
		in.Catalog = i18n.Get(i18n.Default)
	}
	if in.Generated.IsZero() {
		in.Generated = time.Now()
	}
	d, err := newDoc(in, Title(in.Catalog, in.Type))
	if err != nil {
		return nil, err
	}
	s := in.summarize()
	if in.Type == TypeDaily {
		d.daily(s)
	} else {
		d.summaryReport(s)
	}
	return d.finish()
}

// Verdict is a report's overall level and its one-line explanation, also
// used in the email that carries it.
func Verdict(in Input) (level, title, detail string) {
	s := in.summarize()
	return s.level, verdictTitle(in.Catalog, s), verdictDetail(in, s)
}

func verdictTitle(c *i18n.Catalog, s summary) string {
	switch s.level {
	case LevelCritical:
		return c.T("Action needed")
	case LevelAttention:
		return c.T("Protected, with warnings")
	}
	return c.T("Everything is protected")
}

func verdictDetail(in Input, s summary) string {
	c := in.Catalog
	parts := []string{}
	if s.jobs > 0 {
		parts = append(parts, c.Tf("{n} of {total} scheduled jobs have a recent enough backup.", "n", s.current, "total", s.jobs))
	}
	if s.runs > 0 {
		parts = append(parts, c.Tf("{ok} of {runs} runs in the period succeeded.", "ok", s.ok, "runs", s.runs))
	} else {
		parts = append(parts, c.T("No backup ran in the period."))
	}
	if n := len(s.issues); n == 1 {
		parts = append(parts, c.T("1 problem needs attention (see below)."))
	} else if n > 1 {
		parts = append(parts, c.Tf("{n} problems need attention (see below).", "n", n))
	}
	return strings.Join(parts, " ")
}

func statusLabel(c *i18n.Catalog, status string) (string, tone) {
	switch status {
	case StatusOK:
		return c.T("Protected"), toneOK
	case StatusWarning:
		return c.T("Warning"), toneWarn
	case StatusLate:
		return c.T("Late"), toneFail
	case StatusNever:
		return c.T("Never backed up"), toneFail
	}
	return c.T("On demand"), toneNeutral
}

func resultLabel(c *i18n.Catalog, result string) (string, tone) {
	switch result {
	case runlog.ResultOK:
		return c.T("Successful"), toneOK
	case runlog.ResultPartial:
		return c.T("Partial"), toneWarn
	}
	return c.T("Failed"), toneFail
}

func (d *doc) historyNote() {
	in := d.in
	if !in.HistorySince.IsZero() && in.HistorySince.After(in.From) {
		d.paragraph(in.Catalog.Tf("Failed runs are recorded since {date}; before then this report shows only the backups that completed.",
			"date", in.formatTime(in.HistorySince)), 8, colMuted)
		d.gap(6)
	}
}

func (d *doc) issues(s summary) {
	c, in := d.in.Catalog, d.in
	d.heading(c.T("Needs attention"))
	if len(s.issues) == 0 {
		d.paragraph(c.T("Nothing needs attention."), 9.5, colMuted)
		d.gap(10)
		return
	}
	var rows [][]cell
	for _, is := range s.issues {
		t, label := toneFail, c.T("Critical")
		if is.Level == LevelAttention {
			t, label = toneWarn, c.T("Warning")
		}
		since := "—"
		if is.Since != nil {
			since = in.formatTime(*is.Since)
		}
		rows = append(rows, []cell{chipCell(label, t), {text: orDash(is.Job), bold: true}, {text: orDash(is.Agent)}, {text: is.Problem}, {text: since, muted: true}})
	}
	d.table([]column{{c.T("Level"), 0.13, false}, {c.T("Job"), 0.19, false}, {c.T("Agent"), 0.15, false},
		{c.T("Problem"), 0.37, false}, {c.T("Since"), 0.16, false}}, rows)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// --- Protection summary ---

func (d *doc) summaryReport(s summary) {
	in, c := d.in, d.in.Catalog
	d.banner(s.level, verdictTitle(c, s), verdictDetail(in, s))

	var used, free, total uint64
	for _, r := range in.Repos {
		used += r.TotalBytes
		free += r.DiskFree
		total += r.DiskTotal
	}
	rate := kpi{value: formatPercent(c, s.ok, s.runs), label: c.T("Success rate"),
		note: c.Tf("{ok} of {runs} runs", "ok", s.ok, "runs", s.runs), c: rateColor(s)}
	current := kpi{value: fmt.Sprintf("%d/%d", s.current, s.jobs), label: c.T("Jobs up to date"),
		note: c.T("within their objective"), c: colOK}
	if s.current < s.jobs {
		current.c = colFail
	}
	data := kpi{value: formatBytes(c, s.processed), label: c.T("Data backed up"),
		note: c.Tf("{bytes} new after deduplication", "bytes", formatBytes(c, s.added)), c: colText}
	storage := kpi{value: formatBytes(c, used), label: c.T("Storage used"), c: colText}
	if total > 0 {
		storage.note = c.Tf("{free} free on disk", "free", formatBytes(c, free))
	}
	d.tiles([]kpi{rate, current, data, storage})

	d.heading(c.T("Runs per day"))
	d.barChart(d.dailyBars(), [3]string{c.T("Successful runs"), c.T("Partial runs"), c.T("Failed runs")})

	d.issues(s)

	// Agents.
	d.heading(c.T("Agents"))
	type agg struct {
		jobs, runs, ok int
		last           *time.Time
		worst          string
	}
	agents := map[string]*agg{}
	var names []string
	get := func(name string) *agg {
		if a, ok := agents[name]; ok {
			return a
		}
		a := &agg{worst: StatusManual}
		agents[name] = a
		names = append(names, name)
		return a
	}
	for _, j := range in.Jobs {
		a := get(j.Agent)
		a.jobs++
		if l := j.LastOK(); l != nil && (a.last == nil || l.After(*a.last)) {
			a.last = l
		}
		if st := j.Status(in.now()); statusRank(st) > statusRank(a.worst) {
			a.worst = st
		}
	}
	for _, r := range in.Runs {
		a := get(r.Agent)
		a.runs++
		if r.Result == runlog.ResultOK {
			a.ok++
		}
	}
	sort.Strings(names)
	var rows [][]cell
	for _, n := range names {
		a := agents[n]
		label, t := statusLabel(c, a.worst)
		if a.worst == StatusLate || a.worst == StatusNever {
			label = c.T("At risk") // some of its jobs are not protected
		}
		rows = append(rows, []cell{{text: n, bold: true}, {text: fmt.Sprint(a.jobs)}, {text: fmt.Sprintf("%d/%d", a.ok, a.runs)},
			{text: formatPercent(c, a.ok, a.runs)}, {text: in.formatMaybe(a.last)}, chipCell(label, t)})
	}
	if len(rows) == 0 {
		d.paragraph(c.T("No agents in this report."), 9.5, colMuted)
		d.gap(10)
	} else {
		d.table([]column{{c.T("Agent"), 0.24, false}, {c.T("Jobs"), 0.09, true}, {c.T("Runs OK"), 0.13, true},
			{c.T("Success"), 0.12, true}, {c.T("Latest good backup"), 0.22, false}, {c.T("Status"), 0.20, false}}, rows)
	}

	// Jobs.
	d.heading(c.T("Jobs"))
	d.jobTable()

	// Storage.
	if len(in.Repos) > 0 {
		d.heading(c.T("Storage"))
		var rows [][]cell
		for _, r := range in.Repos {
			free := "—"
			if r.DiskTotal > 0 {
				free = c.Tf("{free} ({pct})", "free", formatBytes(c, r.DiskFree), "pct", formatPercent(c, int(r.DiskFree/1e6), int(r.DiskTotal/1e6)))
			}
			rows = append(rows, []cell{{text: r.Name, bold: true}, {text: formatBytes(c, r.TotalBytes)}, {text: formatBytes(c, r.AddedInPeriod)},
				{text: free}, {text: fmt.Sprint(r.Snapshots)}})
		}
		d.table([]column{{c.T("Repository"), 0.28, false}, {c.T("Used"), 0.16, true}, {c.T("New in the period"), 0.18, true},
			{c.T("Free on disk"), 0.22, true}, {c.T("Snapshots"), 0.16, true}}, rows)
	}
	d.historyNote()
}

func rateColor(s summary) rgb {
	switch {
	case s.runs == 0:
		return colMuted
	case s.failed > 0:
		return colFail
	case s.partial > 0:
		return colWarn
	}
	return colOK
}

// dailyBars counts the runs of each day of the period.
func (d *doc) dailyBars() []bar {
	in := d.in
	loc := in.loc()
	start := in.From.In(loc)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	var bars []bar
	index := map[string]int{}
	for t := start; t.Before(in.To); t = t.AddDate(0, 0, 1) {
		label := t.Format("02/01")
		if !spanish(in.Catalog) {
			label = t.Format("Jan 2")
		}
		index[t.Format("2006-01-02")] = len(bars)
		bars = append(bars, bar{label: label})
	}
	for _, r := range in.Runs {
		i, ok := index[r.Time.In(loc).Format("2006-01-02")]
		if !ok {
			continue
		}
		switch r.Result {
		case runlog.ResultOK:
			bars[i].ok++
		case runlog.ResultPartial:
			bars[i].partial++
		default:
			bars[i].fail++
		}
	}
	return bars
}

// jobTable lists each job's kinds with their latest good backup and status.
func (d *doc) jobTable() {
	in, c := d.in, d.in.Catalog
	jobs := append([]Job(nil), in.Jobs...)
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].Agent != jobs[j].Agent {
			return jobs[i].Agent < jobs[j].Agent
		}
		return jobs[i].Name < jobs[j].Name
	})
	var rows [][]cell
	for _, j := range jobs {
		for i, k := range j.Kinds {
			name, agent := j.Name, j.Agent
			if i > 0 {
				name, agent = "", ""
			}
			label, t := statusLabel(c, k.Status(in.now()))
			every := c.T("On demand")
			if k.Interval > 0 {
				every = c.Tf("every {age}", "age", formatAge(c, k.Interval))
			}
			rows = append(rows, []cell{{text: name, bold: true}, {text: agent}, {text: kindName(c, k.Kind)},
				{text: every, muted: true}, {text: in.formatMaybe(k.LastOK)}, chipCell(label, t)})
		}
	}
	if len(rows) == 0 {
		d.paragraph(c.T("No jobs in this report."), 9.5, colMuted)
		d.gap(10)
		return
	}
	d.table([]column{{c.T("Job"), 0.22, false}, {c.T("Agent"), 0.15, false}, {c.T("Kind"), 0.12, false},
		{c.T("Schedule"), 0.13, false}, {c.T("Latest good backup"), 0.20, false}, {c.T("Status"), 0.18, false}}, rows)
}

// --- Daily status ---

func (d *doc) daily(s summary) {
	in, c := d.in, d.in.Catalog
	d.banner(s.level, verdictTitle(c, s), verdictDetail(in, s))
	d.tiles([]kpi{
		{value: fmt.Sprint(s.runs), label: c.T("Runs"), note: formatBytes(c, s.processed), c: colText},
		{value: fmt.Sprint(s.ok), label: c.T("Successful runs"), note: formatPercent(c, s.ok, s.runs), c: colOK},
		{value: fmt.Sprint(s.partial), label: c.T("Partial runs"), note: c.T("some objects left out"), c: pick(s.partial > 0, colWarn, colMuted)},
		{value: fmt.Sprint(s.failed), label: c.T("Failed runs"), note: c.T("nothing stored"), c: pick(s.failed > 0, colFail, colMuted)},
	})

	// Failures in detail: the same failure repeated (hourly log backups
	// failing all night) is one row with its count and time span.
	if groups := failureGroups(in.Runs, c); len(groups) > 0 {
		d.heading(c.T("Failures and warnings"))
		var rows [][]cell
		for _, g := range groups {
			label, t := resultLabel(c, g.result)
			when := in.formatClock(g.first)
			if g.count > 1 {
				when += "–" + in.formatClock(g.last)
			}
			rows = append(rows, []cell{{text: when, muted: true}, {text: orDash(g.job), bold: true}, {text: g.agent},
				{text: kindName(c, g.kind)}, chipCell(label, t), {text: "×" + fmt.Sprint(g.count)}, {text: orDash(g.reason)}})
		}
		d.table([]column{{c.T("Time"), 0.12, false}, {c.T("Job"), 0.16, false}, {c.T("Agent"), 0.12, false},
			{c.T("Kind"), 0.09, false}, {c.T("Result"), 0.11, false}, {c.T("Times"), 0.07, true}, {c.T("Reason"), 0.33, false}}, rows)
	}

	d.issues(s)

	d.heading(c.T("How each job stands"))
	d.jobTable()

	d.heading(c.T("Every run"))
	if len(in.Runs) == 0 {
		d.paragraph(c.T("No backup ran in the period."), 9.5, colMuted)
	} else {
		var rows [][]cell
		for i := len(in.Runs) - 1; i >= 0; i-- { // newest first
			r := in.Runs[i]
			label, t := resultLabel(c, r.Result)
			rows = append(rows, []cell{{text: in.formatClock(r.Time), muted: true}, {text: orDash(r.JobName), bold: true}, {text: r.Agent},
				{text: kindName(c, r.Kind)}, chipCell(label, t), {text: formatDuration(c, r.Duration), muted: true},
				{text: bytesOrDash(c, r.BytesProcessed)}, {text: bytesOrDash(c, r.BytesAdded)}})
		}
		d.table([]column{{c.T("Time"), 0.08, false}, {c.T("Job"), 0.20, false}, {c.T("Agent"), 0.14, false},
			{c.T("Kind"), 0.10, false}, {c.T("Result"), 0.13, false}, {c.T("Duration"), 0.12, true},
			{c.T("Processed"), 0.12, true}, {c.T("New"), 0.11, true}}, rows)
	}
	d.historyNote()
}

// failureGroup is the same failure of one job's kind, repeated.
type failureGroup struct {
	job, agent, kind, result, reason string
	first, last                      time.Time
	count                            int
}

// failureGroups groups the failed and partial runs by job, kind, result
// and reason, in order of their first occurrence.
func failureGroups(runs []runlog.Run, c *i18n.Catalog) []*failureGroup {
	var out []*failureGroup
	index := map[string]*failureGroup{}
	for _, r := range runs {
		if r.Result == runlog.ResultOK {
			continue
		}
		why := firstLine(r.Error)
		if objs := r.FailedObjects; len(objs) > 0 {
			more := ""
			if len(objs) > 5 {
				more = "\n" + c.Tf("and {n} more", "n", len(objs)-5)
				objs = objs[:5]
			}
			if why != "" {
				why += "\n"
			}
			why += strings.Join(objs, "\n") + more
		}
		key := strings.Join([]string{r.JobID, r.JobName, r.Agent, r.Kind, r.Result, why}, "\x00")
		g, ok := index[key]
		if !ok {
			g = &failureGroup{job: r.JobName, agent: r.Agent, kind: r.Kind, result: r.Result, reason: why, first: r.Time}
			index[key] = g
			out = append(out, g)
		}
		g.count++
		g.last = r.Time
	}
	return out
}

func bytesOrDash(c *i18n.Catalog, b uint64) string {
	if b == 0 {
		return "—"
	}
	return formatBytes(c, b)
}

func pick(cond bool, a, b rgb) rgb {
	if cond {
		return a
	}
	return b
}
