package dashboard

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/cron"
	"github.com/hooperits/hitstor/internal/i18n"
)

var (
	// {{t "Text"}} and {{tf "Text {x}" ...}} in templates.
	tmplKeyRe = regexp.MustCompile(`\{\{-?\s*(?:t|tf)\s+"((?:[^"\\]|\\.)*)"`)
	// t('Text') / t("Text") in JavaScript (templates' <script> blocks and static/*.js).
	jsKeyRe = regexp.MustCompile(`(?:^|[^\w.$])t\(\s*(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)")`)
	// Go call sites: newPageData("Title", "Subtitle", ...), c.T("..."), c.Tf("..."), cat.T(...).
	goTitleRe = regexp.MustCompile(`newPageData\(\s*"((?:[^"\\]|\\.)*)",\s*"((?:[^"\\]|\\.)*)"`)
	goSubRe   = regexp.MustCompile(`newPageData\(\s*[^",]+,\s*"((?:[^"\\]|\\.)*)"`)
	goTRe     = regexp.MustCompile(`\.(?:T|Tf)\(\s*"((?:[^"\\]|\\.)*)"`)
	goErrRe   = regexp.MustCompile(`"Error":\s*"((?:[^"\\]|\\.)*)"`)
)

func unescape(s string) string {
	return strings.NewReplacer(`\"`, `"`, `\'`, `'`, `\\`, `\`).Replace(s)
}

// usedMessages collects every translatable message referenced by the dashboard.
func usedMessages(t *testing.T) map[string][]string {
	t.Helper()
	used := map[string][]string{}
	add := func(key, where string) {
		if key = unescape(key); key != "" {
			used[key] = append(used[key], where)
		}
	}
	scanJS := func(src, where string) {
		for _, m := range jsKeyRe.FindAllStringSubmatch(src, -1) {
			add(m[1]+m[2], where)
		}
	}
	err := fs.WalkDir(templateFS, "templates", func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		b, err := fs.ReadFile(templateFS, p)
		if err != nil {
			return err
		}
		for _, m := range tmplKeyRe.FindAllStringSubmatch(string(b), -1) {
			add(m[1], p)
		}
		scanJS(string(b), p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fs.WalkDir(staticFS, "static", func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || !strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".min.js") {
			return err
		}
		b, err := fs.ReadFile(staticFS, p)
		if err != nil {
			return err
		}
		scanJS(string(b), p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Messages translated from runtime values ({{t .Status}}, t(job.status)),
	// one per line in i18n-dynamic/*.txt.
	dynFiles, _ := filepath.Glob("i18n-dynamic/*.txt")
	for _, f := range dynFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				add(line, f)
			}
		}
	}
	goFiles, _ := filepath.Glob("*.go")
	for _, f := range goFiles {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, m := range goTitleRe.FindAllStringSubmatch(src, -1) {
			add(m[1], f)
			add(m[2], f)
		}
		for _, m := range goSubRe.FindAllStringSubmatch(src, -1) {
			add(m[1], f)
		}
		for _, re := range []*regexp.Regexp{goTRe, goErrRe} {
			for _, m := range re.FindAllStringSubmatch(src, -1) {
				add(m[1], f)
			}
		}
	}
	return used
}

// TestCatalogsComplete fails when a language marked complete misses a message,
// or when a catalog keeps translations that nothing uses any more.
func TestCatalogsComplete(t *testing.T) {
	used := usedMessages(t)
	for _, lang := range i18n.Languages() {
		if lang.Code == i18n.Default {
			continue
		}
		c := i18n.Get(lang.Code)
		if lang.Complete {
			var missing []string
			for key, where := range used {
				if !c.Has(key) {
					missing = append(missing, key+"  ("+where[0]+")")
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("%s: %d untranslated messages:\n  %s", lang.Code, len(missing), strings.Join(missing, "\n  "))
			}
		}
		var unused []string
		for key := range c.Messages() {
			if _, ok := used[key]; !ok {
				unused = append(unused, key)
			}
		}
		sort.Strings(unused)
		if len(unused) > 0 {
			t.Errorf("%s: %d translations not used anywhere:\n  %s", lang.Code, len(unused), strings.Join(unused, "\n  "))
		}
	}
}

// TestCatalogPlaceholders checks translations keep the {placeholders} of the original.
func TestCatalogPlaceholders(t *testing.T) {
	ph := regexp.MustCompile(`\{[a-zA-Z0-9_]+\}`)
	for _, lang := range i18n.Languages() {
		for key, val := range i18n.Get(lang.Code).Messages() {
			got := ph.FindAllString(val, -1)
			if allowed, ok := formatPatterns[key]; ok {
				for _, p := range got {
					if !slices.Contains(allowed, strings.Trim(p, "{}")) {
						t.Errorf("%s: %q → %q: unknown placeholder %s (allowed: %v)", lang.Code, key, val, p, allowed)
					}
				}
				continue
			}
			want := ph.FindAllString(key, -1)
			sort.Strings(want)
			sort.Strings(got)
			if strings.Join(want, ",") != strings.Join(got, ",") {
				t.Errorf("%s: %q → %q: placeholders %v, want %v", lang.Code, key, val, got, want)
			}
		}
	}
}

func TestLanguageSelection(t *testing.T) {
	d := &Dashboard{}
	req := func(cookie, accept string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: languageCookieName, Value: cookie})
		}
		if accept != "" {
			r.Header.Set("Accept-Language", accept)
		}
		return r
	}
	if got := d.language(req("", "")); got != "en" {
		t.Errorf("no preference = %q, want en", got)
	}
	if got := d.language(req("", "es-PA,es;q=0.9")); got != "es" {
		t.Errorf("browser es = %q, want es", got)
	}
	if got := d.language(req("en", "es")); got != "en" {
		t.Errorf("cookie en = %q, want en", got)
	}
	if got := d.language(req("zz", "es")); got != "es" {
		t.Errorf("unsupported cookie = %q, want es", got)
	}
	d.cfg = &config.Config{ServeDashboardLanguage: "es"}
	if got := d.language(req("", "en-US")); got != "es" {
		t.Errorf("config default = %q, want es", got)
	}
	if got := d.language(req("en", "")); got != "en" {
		t.Errorf("cookie over config = %q, want en", got)
	}
}

func TestSetLanguageHandler(t *testing.T) {
	d := &Dashboard{}
	for _, tc := range []struct {
		url, wantLoc string
		wantCode     int
	}{
		{"/language?set=es&next=/backups?x=1", "/backups?x=1", http.StatusSeeOther},
		{"/language?set=es&next=https://evil.example", "/", http.StatusSeeOther},
		{"/language?set=es&next=//evil.example", "/", http.StatusSeeOther},
		{"/language?set=zz", "", http.StatusBadRequest},
	} {
		rec := httptest.NewRecorder()
		d.handleSetLanguage(rec, httptest.NewRequest("GET", tc.url, nil))
		if rec.Code != tc.wantCode {
			t.Errorf("%s: code %d, want %d", tc.url, rec.Code, tc.wantCode)
			continue
		}
		if tc.wantCode == http.StatusSeeOther {
			if loc := rec.Header().Get("Location"); loc != tc.wantLoc {
				t.Errorf("%s: Location %q, want %q", tc.url, loc, tc.wantLoc)
			}
			if !strings.Contains(rec.Header().Get("Set-Cookie"), languageCookieName+"=es") {
				t.Errorf("%s: language cookie not set", tc.url)
			}
		}
	}
}

func TestDateAndScheduleFormats(t *testing.T) {
	en, es := i18n.Get("en"), i18n.Get("es")
	ts := time.Date(2026, time.January, 2, 15, 4, 0, 0, time.UTC)
	if got := dateTimeIn(en, ts); got != "Jan 2 3:04 PM" {
		t.Errorf("en dateTimeIn = %q", got)
	}
	if got := dateTimeIn(es, ts); got != "2 ene 15:04" {
		t.Errorf("es dateTimeIn = %q", got)
	}
	expr, err := cron.Parse("30 2 * * *")
	if err != nil {
		t.Fatal(err)
	}
	if got := describeCron(en, expr); got != "Every day at 2:30 AM" {
		t.Errorf("en describeCron = %q", got)
	}
	if got := describeCron(es, expr); got != "Todos los días a las 02:30" {
		t.Errorf("es describeCron = %q", got)
	}
}
