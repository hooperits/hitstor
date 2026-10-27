// Package i18n provides message catalogs for user-facing text.
//
// Messages are keyed by their English text, so English needs no catalog and
// a missing translation falls back to the English original. Each language is
// a directory locales/<code>/ holding a _meta.json file and any number of
// JSON fragments ({"English text": "translation"}) that are merged together;
// a message may appear in several fragments only with the same translation.
// Adding a language only requires adding its directory.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Default is the language used when no other preference matches.
const Default = "en"

//go:embed all:locales
var localesFS embed.FS

// Language describes an available language.
type Language struct {
	Code string `json:"code"`
	// Name is the language's own name for itself, e.g. "Español".
	Name string `json:"name"`
	// Complete marks a catalog that must translate every message; tests
	// enforce it so incomplete languages can be added gradually.
	Complete bool `json:"complete"`
}

// Catalog holds the translations of one language.
type Catalog struct {
	Language
	messages map[string]string
}

var (
	catalogs = map[string]*Catalog{}
	ordered  []Language
)

func init() {
	if err := load(localesFS); err != nil {
		panic(err)
	}
}

func load(fsys fs.FS) error {
	dirs, err := fs.ReadDir(fsys, "locales")
	if err != nil {
		return fmt.Errorf("i18n: read locales: %w", err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := path.Join("locales", d.Name())
		c := &Catalog{messages: map[string]string{}}
		meta, err := fs.ReadFile(fsys, path.Join(dir, "_meta.json"))
		if err != nil {
			return fmt.Errorf("i18n: %s: %w", dir, err)
		}
		if err := json.Unmarshal(meta, &c.Language); err != nil {
			return fmt.Errorf("i18n: %s/_meta.json: %w", dir, err)
		}
		if c.Code != d.Name() {
			return fmt.Errorf("i18n: %s/_meta.json: code %q does not match directory", dir, c.Code)
		}
		files, err := fs.ReadDir(fsys, dir)
		if err != nil {
			return fmt.Errorf("i18n: %s: %w", dir, err)
		}
		for _, f := range files {
			if f.IsDir() || f.Name() == "_meta.json" || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			data, err := fs.ReadFile(fsys, path.Join(dir, f.Name()))
			if err != nil {
				return fmt.Errorf("i18n: %s/%s: %w", dir, f.Name(), err)
			}
			var msgs map[string]string
			if err := json.Unmarshal(data, &msgs); err != nil {
				return fmt.Errorf("i18n: %s/%s: %w", dir, f.Name(), err)
			}
			for k, v := range msgs {
				if prev, dup := c.messages[k]; dup && prev != v {
					return fmt.Errorf("i18n: %s: message %q translated differently (%q, %q)", dir, k, prev, v)
				}
				c.messages[k] = v
			}
		}
		catalogs[c.Code] = c
		ordered = append(ordered, c.Language)
	}
	if _, ok := catalogs[Default]; !ok {
		return fmt.Errorf("i18n: missing default language %q", Default)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Code < ordered[j].Code })
	return nil
}

// Languages returns the available languages sorted by code.
func Languages() []Language {
	return append([]Language(nil), ordered...)
}

// Supported reports whether code names an available language.
func Supported(code string) bool {
	_, ok := catalogs[code]
	return ok
}

// Get returns the catalog for code, or the default catalog.
func Get(code string) *Catalog {
	if c, ok := catalogs[code]; ok {
		return c
	}
	return catalogs[Default]
}

// T translates msg, falling back to msg itself.
func (c *Catalog) T(msg string) string {
	if v, ok := c.messages[msg]; ok && v != "" {
		return v
	}
	return msg
}

// Tf translates msg and replaces {name} placeholders with the given
// name/value pairs, e.g. Tf("{n} agents", "n", 3).
func (c *Catalog) Tf(msg string, pairs ...any) string {
	out := c.T(msg)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = strings.ReplaceAll(out, "{"+fmt.Sprint(pairs[i])+"}", fmt.Sprint(pairs[i+1]))
	}
	return out
}

// Messages returns a copy of the catalog's translations (for client-side use).
func (c *Catalog) Messages() map[string]string {
	out := make(map[string]string, len(c.messages))
	for k, v := range c.messages {
		out[k] = v
	}
	return out
}

// Has reports whether the catalog translates msg.
func (c *Catalog) Has(msg string) bool {
	_, ok := c.messages[msg]
	return ok
}

// Negotiate picks a language from an Accept-Language header value, falling
// back to fallback (when supported) and then to Default.
func Negotiate(acceptLanguage, fallback string) string {
	type pref struct {
		code string
		q    float64
	}
	var prefs []pref
	for i, part := range strings.Split(acceptLanguage, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag == "" {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			f = strings.TrimSpace(f)
			if strings.HasPrefix(f, "q=") {
				if _, err := fmt.Sscanf(f[2:], "%g", &q); err != nil {
					q = 0
				}
			}
		}
		// Keep header order for equal weights.
		prefs = append(prefs, pref{tag, q - float64(i)*1e-6})
	}
	sort.SliceStable(prefs, func(i, j int) bool { return prefs[i].q > prefs[j].q })
	for _, p := range prefs {
		if p.q <= 0 {
			continue
		}
		if Supported(p.code) {
			return p.code
		}
		base, _, _ := strings.Cut(p.code, "-")
		if Supported(base) {
			return base
		}
	}
	if Supported(fallback) {
		return fallback
	}
	return Default
}
