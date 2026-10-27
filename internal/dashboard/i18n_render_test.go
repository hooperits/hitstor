package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

// TestPagesRenderInEveryLanguage renders every dashboard page in every
// language, so a template that fails to execute (or a broken translation
// call) shows up as a test failure instead of a 500 in production.
func TestPagesRenderInEveryLanguage(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	const jobID = "0123456789abcdef"
	if err := dash.registry.AddJob(server.BackupJob{
		ID:              jobID,
		Name:            "nightly",
		AgentName:       "prod-server",
		RepoName:        "default",
		Paths:           []string{"/data"},
		ScheduleEnabled: true,
		ScheduleHours:   24,
		CreatedAt:       time.Now(),
	}); err != nil {
		t.Fatalf("add job: %v", err)
	}
	snapID, err := dash.repo.SaveSnapshot(&repository.Snapshot{
		Time:     time.Now(),
		Hostname: "prod-server",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	pages := []string{
		"/", "/backups", "/backups/" + jobID, "/restores", "/disaster",
		"/agents", "/agents/prod-server", "/snapshots/" + snapID.String(),
		"/storage", "/storage/default", "/logs", "/settings", "/login",
	}
	h := dash.Handler()
	for _, lang := range i18n.Languages() {
		for _, page := range pages {
			req := httptest.NewRequest("GET", page, nil)
			req.AddCookie(&http.Cookie{Name: languageCookieName, Value: lang.Code})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if page == "/login" && rec.Code == http.StatusSeeOther {
				continue // no password configured: login redirects to the dashboard
			}
			if rec.Code != http.StatusOK {
				t.Errorf("%s %s: status %d: %s", lang.Code, page, rec.Code, strings.TrimSpace(rec.Body.String()))
				continue
			}
			body := rec.Body.String()
			if !strings.Contains(body, `<html lang="`+lang.Code+`"`) {
				t.Errorf("%s %s: page not rendered in %s", lang.Code, page, lang.Code)
			}
			if strings.Contains(body, "{{") {
				t.Errorf("%s %s: unrendered template action in output", lang.Code, page)
			}
			// A template that fails mid-page still answers 200 (headers are
			// already sent) but cuts the page short and appends the error.
			if i := strings.Index(body, "template: "); i >= 0 {
				t.Errorf("%s %s: template error: %s", lang.Code, page, strings.SplitN(body[i:], "\n", 2)[0])
			}
			if !strings.HasSuffix(strings.TrimSpace(body), "</html>") {
				t.Errorf("%s %s: page truncated", lang.Code, page)
			}
		}
	}
}

func TestSpanishPageIsTranslated(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()
	req := httptest.NewRequest("GET", "/backups", nil)
	req.Header.Set("Accept-Language", "es-PA,es;q=0.9")
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"Respaldos", "Agentes", "Almacenamiento", "Cerrar sesión"} {
		if !strings.Contains(body, want) {
			t.Errorf("Spanish /backups page lacks %q", want)
		}
	}
}
