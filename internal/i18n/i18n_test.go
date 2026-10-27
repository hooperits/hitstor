package i18n

import "testing"

func TestDefaultLanguageLoaded(t *testing.T) {
	if !Supported(Default) {
		t.Fatalf("default language %q not loaded", Default)
	}
	if Get("zz").Code != Default {
		t.Fatalf("unknown language should fall back to %q", Default)
	}
}

func TestTranslateFallsBackToEnglish(t *testing.T) {
	c := Get(Default)
	if got := c.T("Some text without translation"); got != "Some text without translation" {
		t.Fatalf("T() = %q", got)
	}
	if got := c.Tf("{n} agents", "n", 3); got != "3 agents" {
		t.Fatalf("Tf() = %q", got)
	}
}

func TestSpanishCatalog(t *testing.T) {
	c := Get("es")
	if c.Code != "es" || c.Name == "" {
		t.Fatalf("es catalog not loaded: %+v", c.Language)
	}
	if got := c.Tf("{n} days ago", "n", 4); got != "hace 4 días" {
		t.Fatalf("Tf() = %q", got)
	}
}

func TestNegotiate(t *testing.T) {
	cases := []struct {
		header, fallback, want string
	}{
		{"", "", "en"},
		{"es-PA,es;q=0.9,en;q=0.8", "", "es"},
		{"fr-FR,fr;q=0.9", "", "en"},
		{"fr-FR,fr;q=0.9", "es", "es"},
		{"en-US,en;q=0.9,es;q=0.8", "", "en"},
		{"fr;q=1, es;q=0.5", "", "es"},
		{"es;q=0, en", "", "en"},
	}
	for _, tc := range cases {
		if got := Negotiate(tc.header, tc.fallback); got != tc.want {
			t.Errorf("Negotiate(%q, %q) = %q, want %q", tc.header, tc.fallback, got, tc.want)
		}
	}
}
