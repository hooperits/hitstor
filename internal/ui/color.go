package ui

import (
	"io"
	"os"
)

// ColorWriter conditionally applies ANSI escape codes to output.
// When enabled, color methods wrap strings with ANSI codes.
// When disabled, they return strings unmodified.
type ColorWriter struct {
	w       io.Writer
	enabled bool
}

// NewColorWriter creates a ColorWriter. Colors are enabled unless suppress is true,
// NO_COLOR env var is set, or the writer is not an interactive terminal.
func NewColorWriter(w io.Writer, suppress bool) *ColorWriter {
	enabled := !suppress && os.Getenv("NO_COLOR") == "" && isTerminal(w)
	return &ColorWriter{w: w, enabled: enabled}
}

// Enabled returns true if color output is active.
func (c *ColorWriter) Enabled() bool {
	return c.enabled
}

// Green wraps s with green ANSI codes if enabled.
func (c *ColorWriter) Green(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[32m" + s + "\033[0m"
}

// Yellow wraps s with yellow ANSI codes if enabled.
func (c *ColorWriter) Yellow(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[33m" + s + "\033[0m"
}

// Red wraps s with red ANSI codes if enabled.
func (c *ColorWriter) Red(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[31m" + s + "\033[0m"
}

// Cyan wraps s with cyan ANSI codes if enabled.
func (c *ColorWriter) Cyan(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[36m" + s + "\033[0m"
}

// Bold wraps s with bold ANSI codes if enabled.
func (c *ColorWriter) Bold(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[1m" + s + "\033[0m"
}
