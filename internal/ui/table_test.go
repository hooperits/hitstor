package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestTable_EmptyTable(t *testing.T) {
	tbl := NewTable(
		Column{Header: "ID", Align: AlignLeft},
		Column{Header: "Name", Align: AlignLeft},
	)

	var buf bytes.Buffer
	tbl.Render(&buf, nil)

	output := buf.String()
	if !strings.Contains(output, "ID") {
		t.Error("expected header row with 'ID'")
	}
	if !strings.Contains(output, "──") {
		t.Error("expected Unicode separator")
	}
	// Should only have header + separator, no data rows.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines (header + separator), got %d", len(lines))
	}
}

func TestTable_SingleRow(t *testing.T) {
	tbl := NewTable(
		Column{Header: "ID", Align: AlignLeft},
		Column{Header: "Value", Align: AlignLeft},
	)
	tbl.AddRow("abc", "hello")

	var buf bytes.Buffer
	tbl.Render(&buf, nil)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (header + separator + 1 row), got %d", len(lines))
	}
	if !strings.Contains(lines[2], "abc") {
		t.Error("expected data row to contain 'abc'")
	}
	if !strings.Contains(lines[2], "hello") {
		t.Error("expected data row to contain 'hello'")
	}
}

func TestTable_ColumnAlignment(t *testing.T) {
	tbl := NewTable(
		Column{Header: "Name", Align: AlignLeft},
		Column{Header: "Count", Align: AlignRight},
	)
	tbl.AddRow("alpha", "42")
	tbl.AddRow("beta", "1234")

	var buf bytes.Buffer
	tbl.Render(&buf, nil)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Data rows should have right-aligned Count column.
	// "42" should be padded with spaces on the left to match "1234" width.
	if len(lines) < 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}

	// The row with "42" should have leading space before it.
	row1 := lines[2]
	if !strings.Contains(row1, "  42") {
		t.Errorf("expected right-aligned '42' with padding, got %q", row1)
	}
}

func TestTable_DynamicWidths(t *testing.T) {
	tbl := NewTable(
		Column{Header: "ID", Align: AlignLeft},
		Column{Header: "Description", Align: AlignLeft},
	)
	tbl.AddRow("a1b2c3d4", "short")
	tbl.AddRow("e5f6g7h8", "a much longer description value")

	var buf bytes.Buffer
	tbl.Render(&buf, nil)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Separator should be wide enough for the longest value.
	sep := lines[1]
	// The separator for Description column should be at least 30 chars of ─.
	if !strings.Contains(sep, strings.Repeat("─", 30)) {
		t.Errorf("expected separator to be at least 30 wide for Description, got %q", sep)
	}
}

func TestTable_MinWidth(t *testing.T) {
	tbl := NewTable(
		Column{Header: "ID", Align: AlignLeft, MinWidth: 10},
	)
	tbl.AddRow("abc")

	var buf bytes.Buffer
	tbl.Render(&buf, nil)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	sep := lines[1]
	// Separator should be at least MinWidth=10 chars of ─.
	if len(strings.TrimSpace(sep)) < 10 {
		t.Errorf("expected separator at least 10 wide (MinWidth), got %q", sep)
	}
}

func TestTable_NoColumns(t *testing.T) {
	tbl := NewTable()

	var buf bytes.Buffer
	tbl.Render(&buf, nil)

	if buf.Len() != 0 {
		t.Error("expected no output for table with no columns")
	}
}

func TestTable_WithColor(t *testing.T) {
	// Create an enabled ColorWriter manually.
	cw := &ColorWriter{w: &bytes.Buffer{}, enabled: true}

	tbl := NewTable(
		Column{Header: "Name", Align: AlignLeft},
	)
	tbl.AddRow("value")

	var buf bytes.Buffer
	tbl.Render(&buf, cw)

	output := buf.String()
	// Header should contain bold ANSI codes.
	if !strings.Contains(output, "\033[1m") {
		t.Error("expected bold ANSI code in header when color enabled")
	}
}
