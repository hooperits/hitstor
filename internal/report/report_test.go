package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/runlog"
)

func ptr(t time.Time) *time.Time { return &t }

// sample is a week of two servers: a file job that is fine, a SQL Server
// plan whose log backups fail since yesterday, and a job never run.
func sample(typ, lang string) Input {
	now := time.Date(2026, 10, 30, 7, 0, 0, 0, time.UTC)
	from := now.Add(-7 * 24 * time.Hour)
	if typ == TypeDaily {
		from = now.Add(-24 * time.Hour)
	}
	in := Input{Type: typ, From: from, To: now, Generated: now, Location: time.UTC, Catalog: i18n.Get(lang), Version: "0.2.0",
		HistorySince: now.Add(-3 * 24 * time.Hour), ActiveAlerts: 0}
	in.Jobs = []Job{
		{ID: "a", Name: "Documentos", Agent: "srv-files", Repo: "local", Kinds: []Kind{{Interval: 24 * time.Hour, LastOK: ptr(now.Add(-5 * time.Hour))}},
			VerifyStatus: "ok", VerifyAt: ptr(now.Add(-4 * time.Hour))},
		{ID: "b", Name: "SQL producción", Agent: "srv-sql", Repo: "local", Plugin: "mssql", Kinds: []Kind{
			{Kind: "full", Interval: 7 * 24 * time.Hour, LastOK: ptr(now.Add(-2 * 24 * time.Hour))},
			{Kind: "differential", Interval: 24 * time.Hour, LastOK: ptr(now.Add(-6 * time.Hour))},
			{Kind: "log", Interval: time.Hour, LastOK: ptr(now.Add(-20 * time.Hour)), LastFailureAt: ptr(now.Add(-30 * time.Minute)),
				LastError: "VDI: the SQL Server did not answer in time"},
		}},
		{ID: "c", Name: "Contabilidad", Agent: "srv-sql", Repo: "local", Kinds: []Kind{{Interval: 24 * time.Hour}}},
		{ID: "d", Name: "Copia manual", Agent: "srv-files", Repo: "local", Kinds: []Kind{{LastOK: ptr(now.Add(-40 * 24 * time.Hour))}}},
	}
	for t := from; t.Before(now); t = t.Add(time.Hour) {
		res := runlog.ResultOK
		var errMsg string
		var objs []string
		if t.After(now.Add(-20 * time.Hour)) {
			res, errMsg = runlog.ResultFailed, "VDI: the SQL Server did not answer in time"
		}
		if t.Hour() == 3 && t.Day()%3 == 0 {
			res, objs = runlog.ResultPartial, []string{"MSSQLSERVER/Reportes: database is offline"}
		}
		in.Runs = append(in.Runs, runlog.Run{Time: t, JobName: "SQL producción", Agent: "srv-sql", Kind: "log", Result: res, Error: errMsg,
			FailedObjects: objs, BytesProcessed: 120 << 20, BytesAdded: 30 << 20, Duration: 42})
		if t.Hour() == 2 {
			in.Runs = append(in.Runs, runlog.Run{Time: t.Add(10 * time.Minute), JobName: "Documentos", Agent: "srv-files", Result: runlog.ResultOK,
				BytesProcessed: 85 << 30, BytesAdded: 1200 << 20, Duration: 1830})
		}
	}
	in.Repos = []Repo{{Name: "local", TotalBytes: 1300 << 30, DiskTotal: 4 << 40, DiskFree: 2600 << 30, Snapshots: 812, AddedInPeriod: 38 << 30}}
	return in
}

func TestStatus(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for name, tc := range map[string]struct {
		k    Kind
		want string
	}{
		"recent":            {Kind{Interval: day, LastOK: ptr(now.Add(-20 * time.Hour))}, StatusOK},
		"one missed run":    {Kind{Interval: day, LastOK: ptr(now.Add(-37 * time.Hour))}, StatusLate},
		"failed, in time":   {Kind{Interval: day, LastOK: ptr(now.Add(-2 * time.Hour)), LastFailureAt: ptr(now.Add(-time.Hour))}, StatusWarning},
		"failed then ok":    {Kind{Interval: day, LastOK: ptr(now.Add(-time.Hour)), LastFailureAt: ptr(now.Add(-2 * time.Hour))}, StatusOK},
		"never":             {Kind{Interval: day}, StatusNever},
		"on demand":         {Kind{LastOK: ptr(now.Add(-90 * day))}, StatusManual},
		"on demand, failed": {Kind{LastFailureAt: ptr(now)}, StatusWarning},
		"partial":           {Kind{Interval: day, LastOK: ptr(now.Add(-time.Hour)), Partial: true}, StatusWarning},
		"hourly, 40 min":    {Kind{Interval: time.Hour, LastOK: ptr(now.Add(-89 * time.Minute))}, StatusOK},
	} {
		if got := tc.k.Status(now); got != tc.want {
			t.Errorf("%s: %s, want %s", name, got, tc.want)
		}
	}
	j := Job{Kinds: []Kind{{Interval: day, LastOK: ptr(now)}}, VerifyStatus: "failed"}
	if j.Status(now) != StatusWarning {
		t.Error("a failed test restore is not a warning")
	}
}

func TestScheduleInterval(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	for expr, want := range map[string]time.Duration{
		"0 2 * * *":    24 * time.Hour,
		"0 2 * * 1-5":  72 * time.Hour, // Friday to Monday
		"*/15 * * * *": 15 * time.Minute,
		"0 1 1 * *":    31 * 24 * time.Hour,
	} {
		if got := ScheduleInterval(expr, "UTC", now); got != want {
			t.Errorf("%s: %v, want %v", expr, got, want)
		}
	}
	if ScheduleInterval("nonsense", "", now) != 0 {
		t.Error("invalid expression")
	}
}

func TestSummarize(t *testing.T) {
	in := sample(TypeSummary, "en")
	s := in.summarize()
	if s.level != LevelCritical {
		t.Fatalf("level %s", s.level)
	}
	var problems []string
	for _, is := range s.issues {
		problems = append(problems, is.Job+": "+is.Problem)
	}
	all := strings.Join(problems, "\n")
	for _, want := range []string{"SQL producción: Log: no good backup", "Contabilidad: Files: never backed up"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing issue %q in:\n%s", want, all)
		}
	}
	if strings.Contains(all, "Copia manual") {
		t.Error("an on-demand job is not a problem")
	}
	if s.jobs != 3 || s.current != 1 {
		t.Errorf("jobs %d current %d", s.jobs, s.current)
	}
}

func TestFailureGroups(t *testing.T) {
	in := sample(TypeDaily, "en")
	groups := failureGroups(in.Runs, in.Catalog)
	if len(groups) != 2 {
		t.Fatalf("%d groups", len(groups))
	}
	g := groups[0]
	if g.result != runlog.ResultFailed || g.count != 18 || g.first.Hour() != 12 || g.last.Hour() != 6 {
		t.Fatalf("repeated failures: %+v", *g)
	}
	if groups[1].result != runlog.ResultPartial || !strings.Contains(groups[1].reason, "MSSQLSERVER/Reportes") {
		t.Fatalf("partial run: %+v", *groups[1])
	}
}

func TestRender(t *testing.T) {
	out := os.Getenv("HITSTOR_REPORT_OUT") // set to look at the PDFs
	for _, typ := range Types {
		for _, lang := range []string{"en", "es"} {
			in := sample(typ, lang)
			in.Logo = testLogo(t)
			pdf, err := Render(in)
			if err != nil {
				t.Fatalf("%s/%s: %v", typ, lang, err)
			}
			if !bytes.HasPrefix(pdf, []byte("%PDF-")) || len(pdf) < 5000 {
				t.Fatalf("%s/%s: not a PDF (%d bytes)", typ, lang, len(pdf))
			}
			if out != "" {
				os.WriteFile(filepath.Join(out, typ+"-"+lang+".pdf"), pdf, 0o644)
			}
		}
	}
	// A report with nothing in it still renders.
	if _, err := Render(Input{Type: TypeSummary, From: time.Now().Add(-time.Hour), To: time.Now(), Catalog: i18n.Get("en")}); err != nil {
		t.Fatal(err)
	}
}

func testLogo(t *testing.T) []byte {
	data, err := os.ReadFile("../dashboard/static/favicon.png")
	if err != nil {
		t.Skip("no logo")
	}
	return data
}
