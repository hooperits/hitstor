package ui

import (
	"bytes"
	"os"
	"testing"
)

func TestColorWriter_Enabled(t *testing.T) {
	// bytes.Buffer is not a terminal, so colors should be disabled.
	var buf bytes.Buffer
	cw := NewColorWriter(&buf, false)
	if cw.Enabled() {
		t.Error("expected colors disabled for non-terminal writer")
	}
}

func TestColorWriter_Suppressed(t *testing.T) {
	// When suppress=true, colors should be disabled regardless.
	cw := NewColorWriter(os.Stdout, true)
	if cw.Enabled() {
		t.Error("expected colors disabled when suppress=true")
	}

	// All methods should return plain text.
	if got := cw.Green("ok"); got != "ok" {
		t.Errorf("Green() = %q, want %q", got, "ok")
	}
	if got := cw.Yellow("warn"); got != "warn" {
		t.Errorf("Yellow() = %q, want %q", got, "warn")
	}
	if got := cw.Red("err"); got != "err" {
		t.Errorf("Red() = %q, want %q", got, "err")
	}
	if got := cw.Cyan("id"); got != "id" {
		t.Errorf("Cyan() = %q, want %q", got, "id")
	}
	if got := cw.Bold("title"); got != "title" {
		t.Errorf("Bold() = %q, want %q", got, "title")
	}
}

func TestColorWriter_NoColor_EnvVar(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	// Even with suppress=false and a real file, NO_COLOR should disable colors.
	cw := NewColorWriter(os.Stdout, false)
	if cw.Enabled() {
		t.Error("expected colors disabled when NO_COLOR is set")
	}
	if got := cw.Green("ok"); got != "ok" {
		t.Errorf("Green() = %q, want %q with NO_COLOR", got, "ok")
	}
}

func TestColorWriter_NonTerminal(t *testing.T) {
	// bytes.Buffer is not a terminal.
	var buf bytes.Buffer
	cw := NewColorWriter(&buf, false)
	if cw.Enabled() {
		t.Error("expected colors disabled for non-terminal")
	}
	if got := cw.Red("error"); got != "error" {
		t.Errorf("Red() = %q, want %q for non-terminal", got, "error")
	}
}

func TestColorWriter_ColorCodes_WhenEnabled(t *testing.T) {
	// Manually create an enabled ColorWriter to test ANSI output.
	cw := &ColorWriter{w: os.Stdout, enabled: true}

	tests := []struct {
		name   string
		fn     func(string) string
		input  string
		prefix string
		suffix string
	}{
		{"Green", cw.Green, "ok", "\033[32m", "\033[0m"},
		{"Yellow", cw.Yellow, "warn", "\033[33m", "\033[0m"},
		{"Red", cw.Red, "err", "\033[31m", "\033[0m"},
		{"Cyan", cw.Cyan, "id", "\033[36m", "\033[0m"},
		{"Bold", cw.Bold, "title", "\033[1m", "\033[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.input)
			want := tt.prefix + tt.input + tt.suffix
			if got != want {
				t.Errorf("%s(%q) = %q, want %q", tt.name, tt.input, got, want)
			}
		})
	}
}
