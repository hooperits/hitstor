// Package cron provides a pure Go 5-field cron expression parser.
// Supported syntax: minute hour day-of-month month day-of-week.
// Each field supports wildcards (*), lists (1,3,5), ranges (1-5),
// and steps (*/15, 1-5/2). As in Quartz, day-of-month also takes L (the
// last day of the month), and day-of-week takes D#N (the Nth weekday D of
// the month, N 1-5) and DL (the last weekday D of the month).
package cron

import (
	"fmt"
	"strconv"
	"strings"
)

// Expression holds a parsed cron expression as bitsets.
type Expression struct {
	Minute uint64 // Bits 0-59.
	Hour   uint64 // Bits 0-23.
	Dom    uint64 // Bits 1-31.
	Month  uint64 // Bits 1-12.
	Dow    uint64 // Bits 0-6 (0=Sunday).
	// LastDom is set by L in day-of-month: the last day of the month.
	LastDom bool
	// NthDow holds, per weekday (0=Sunday), the weeks of the month set by
	// D#N (bit N-1) and DL (bit lastWeekBit).
	NthDow [7]uint8
	Raw    string
}

// lastWeekBit marks "the last such weekday of the month" in NthDow.
const lastWeekBit = 5

// fieldDef describes one cron field.
type fieldDef struct {
	name string
	min  int
	max  int
}

var fields = [5]fieldDef{
	{"minute", 0, 59},
	{"hour", 0, 23},
	{"day-of-month", 1, 31},
	{"month", 1, 12},
	{"day-of-week", 0, 6},
}

// Parse parses a 5-field cron expression into an Expression.
func Parse(expr string) (Expression, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return Expression{}, fmt.Errorf("cron: empty expression")
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return Expression{}, fmt.Errorf("cron: expected 5 fields, got %d", len(parts))
	}

	var e Expression
	var bitsets [5]uint64
	for i, part := range parts {
		var special bool
		switch i {
		case 2:
			part, special = e.takeLastDom(part)
		case 4:
			var err error
			if part, special, err = e.takeNthDow(part); err != nil {
				return Expression{}, err
			}
		}
		if part == "" && special {
			continue // only L, D#N, or DL items
		}
		bs, err := parseField(part, fields[i])
		if err != nil {
			return Expression{}, err
		}
		bitsets[i] = bs
	}

	e.Minute, e.Hour, e.Dom, e.Month, e.Dow = bitsets[0], bitsets[1], bitsets[2], bitsets[3], bitsets[4]
	e.Raw = expr
	return e, nil
}

// takeLastDom removes L from a day-of-month field, recording it.
func (e *Expression) takeLastDom(field string) (rest string, found bool) {
	var kept []string
	for _, item := range strings.Split(field, ",") {
		if item == "L" {
			e.LastDom = true
			found = true
			continue
		}
		kept = append(kept, item)
	}
	return strings.Join(kept, ","), found
}

// takeNthDow removes D#N and DL items from a day-of-week field, recording
// them.
func (e *Expression) takeNthDow(field string) (rest string, found bool, err error) {
	var kept []string
	for _, item := range strings.Split(field, ",") {
		day, week, ok := strings.Cut(item, "#")
		bit := -1
		switch {
		case ok:
			n, err := strconv.Atoi(week)
			if err != nil || n < 1 || n > 5 {
				return "", false, fmt.Errorf("cron: day-of-week field: invalid week %q in %q (allowed 1-5)", week, item)
			}
			bit = n - 1
		case len(item) > 1 && strings.HasSuffix(item, "L"):
			day, bit = strings.TrimSuffix(item, "L"), lastWeekBit
		default:
			kept = append(kept, item)
			continue
		}
		d, err := strconv.Atoi(day)
		if err != nil || d < 0 || d > 6 {
			return "", false, fmt.Errorf("cron: day-of-week field: invalid day %q in %q (allowed 0-6)", day, item)
		}
		e.NthDow[d] |= 1 << bit
		found = true
	}
	return strings.Join(kept, ","), found, nil
}

// parseField parses a single cron field (e.g., "*/15", "1-5", "1,3,5").
func parseField(s string, fd fieldDef) (uint64, error) {
	var result uint64
	for _, item := range strings.Split(s, ",") {
		bits, err := parseItem(item, fd)
		if err != nil {
			return 0, err
		}
		result |= bits
	}
	if result == 0 {
		return 0, fmt.Errorf("cron: %s field: no values set", fd.name)
	}
	return result, nil
}

// parseItem parses one element of a cron field: *, N, N-M, */S, N-M/S.
func parseItem(s string, fd fieldDef) (uint64, error) {
	stepParts := strings.SplitN(s, "/", 2)
	rangePart := stepParts[0]
	step := 1

	if len(stepParts) == 2 {
		v, err := strconv.Atoi(stepParts[1])
		if err != nil || v <= 0 {
			return 0, fmt.Errorf("cron: %s field: invalid step %q", fd.name, stepParts[1])
		}
		step = v
	}

	var lo, hi int

	if rangePart == "*" {
		lo = fd.min
		hi = fd.max
	} else if dashIdx := strings.IndexByte(rangePart, '-'); dashIdx >= 0 {
		var err error
		lo, err = strconv.Atoi(rangePart[:dashIdx])
		if err != nil {
			return 0, fmt.Errorf("cron: %s field: invalid value %q", fd.name, rangePart[:dashIdx])
		}
		hi, err = strconv.Atoi(rangePart[dashIdx+1:])
		if err != nil {
			return 0, fmt.Errorf("cron: %s field: invalid value %q", fd.name, rangePart[dashIdx+1:])
		}
		if lo > hi {
			return 0, fmt.Errorf("cron: %s field: invalid range %d-%d", fd.name, lo, hi)
		}
	} else {
		v, err := strconv.Atoi(rangePart)
		if err != nil {
			return 0, fmt.Errorf("cron: %s field: invalid value %q", fd.name, rangePart)
		}
		lo = v
		hi = v
	}

	if lo < fd.min || hi > fd.max {
		return 0, fmt.Errorf("cron: %s field: value out of range %d-%d (allowed %d-%d)", fd.name, lo, hi, fd.min, fd.max)
	}

	var bits uint64
	for i := lo; i <= hi; i += step {
		bits |= 1 << uint(i)
	}
	return bits, nil
}

// allSet checks if all bits from lo to hi are set.
func (e Expression) allSet(bitset uint64, lo, hi int) bool {
	for i := lo; i <= hi; i++ {
		if bitset&(1<<uint(i)) == 0 {
			return false
		}
	}
	return true
}

// singleBit returns the value of the single set bit, or -1 if not exactly one.
func (e Expression) singleBit(bitset uint64, lo, hi int) int {
	found := -1
	for i := lo; i <= hi; i++ {
		if bitset&(1<<uint(i)) != 0 {
			if found >= 0 {
				return -1
			}
			found = i
		}
	}
	return found
}

// detectStep detects a regular step pattern starting from lo. Returns step or 0.
func (e Expression) detectStep(bitset uint64, lo, hi int) int {
	// Find first set bit.
	first := -1
	for i := lo; i <= hi; i++ {
		if bitset&(1<<uint(i)) != 0 {
			first = i
			break
		}
	}
	if first < 0 || first != lo {
		return 0
	}

	// Find second set bit for step.
	second := -1
	for i := first + 1; i <= hi; i++ {
		if bitset&(1<<uint(i)) != 0 {
			second = i
			break
		}
	}
	if second < 0 {
		return 0
	}

	step := second - first
	// Verify all bits match the step pattern.
	for i := lo; i <= hi; i++ {
		shouldSet := (i-lo)%step == 0
		isSet := bitset&(1<<uint(i)) != 0
		if shouldSet != isSet {
			return 0
		}
	}
	return step
}

// weekdayBits returns the bitset for Monday through Friday (bits 1-5).
func weekdayBits() uint64 {
	var bits uint64
	for i := 1; i <= 5; i++ {
		bits |= 1 << uint(i)
	}
	return bits
}

// formatTime formats hour and minute into "H:MM AM/PM".
func formatTime(hour, minute int) string {
	period := "AM"
	h := hour
	if h == 0 {
		h = 12
	} else if h == 12 {
		period = "PM"
	} else if h > 12 {
		h -= 12
		period = "PM"
	}
	return fmt.Sprintf("%d:%02d %s", h, minute, period)
}
