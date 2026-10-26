package ui

import (
	"fmt"
	"io"
	"strings"
)

// AlignLeft aligns column content to the left.
const AlignLeft = 0

// AlignRight aligns column content to the right.
const AlignRight = 1

// Column defines a table column with header text and alignment.
type Column struct {
	Header   string
	Align    int
	MinWidth int
}

// Table renders aligned columnar data with Unicode separators.
type Table struct {
	columns []Column
	rows    [][]string
}

// NewTable creates a table with the given column definitions.
func NewTable(columns ...Column) *Table {
	return &Table{columns: columns}
}

// AddRow appends a data row to the table. Values correspond to columns in order.
func (t *Table) AddRow(values ...string) {
	// Pad or truncate to match column count.
	row := make([]string, len(t.columns))
	for i := range row {
		if i < len(values) {
			row[i] = values[i]
		}
	}
	t.rows = append(t.rows, row)
}

// Render writes the table to w with optional color support.
// If color is nil, no styling is applied to headers.
func (t *Table) Render(w io.Writer, color *ColorWriter) {
	if len(t.columns) == 0 {
		return
	}

	// Compute column widths.
	widths := make([]int, len(t.columns))
	for i, col := range t.columns {
		widths[i] = len(col.Header)
		if col.MinWidth > widths[i] {
			widths[i] = col.MinWidth
		}
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Render header.
	t.renderRow(w, t.headerValues(), widths, color, true)

	// Render separator.
	parts := make([]string, len(t.columns))
	for i, width := range widths {
		parts[i] = strings.Repeat("─", width)
	}
	fmt.Fprintln(w, strings.Join(parts, "  "))

	// Render data rows.
	for _, row := range t.rows {
		t.renderRow(w, row, widths, color, false)
	}
}

func (t *Table) headerValues() []string {
	headers := make([]string, len(t.columns))
	for i, col := range t.columns {
		headers[i] = col.Header
	}
	return headers
}

func (t *Table) renderRow(w io.Writer, values []string, widths []int, color *ColorWriter, isHeader bool) {
	parts := make([]string, len(t.columns))
	for i, col := range t.columns {
		val := ""
		if i < len(values) {
			val = values[i]
		}

		if isHeader && color != nil {
			val = color.Bold(val)
		}

		if col.Align == AlignRight && !isHeader {
			// Right-align: pad on the left.
			padding := widths[i] - len(values[i])
			if padding < 0 {
				padding = 0
			}
			parts[i] = strings.Repeat(" ", padding) + val
		} else {
			// Left-align: pad on the right.
			plainLen := len(values[i])
			if isHeader {
				plainLen = len(t.columns[i].Header)
			}
			padding := widths[i] - plainLen
			if padding < 0 {
				padding = 0
			}
			parts[i] = val + strings.Repeat(" ", padding)
		}
	}
	fmt.Fprintln(w, strings.Join(parts, "  "))
}
