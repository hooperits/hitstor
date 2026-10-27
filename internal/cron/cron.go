// Package cron provides a pure Go 5-field cron expression parser.
// Supported syntax: minute hour day-of-month month day-of-week.
// Each field supports wildcards (*), lists (1,3,5), ranges (1-5),
// and steps (*/15, 1-5/2).
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
	Raw    string
}

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

	var bitsets [5]uint64
	for i, part := range parts {
		bs, err := parseField(part, fields[i])
		if err != nil {
			return Expression{}, err
		}
		bitsets[i] = bs
	}

	return Expression{
		Minute: bitsets[0],
		Hour:   bitsets[1],
		Dom:    bitsets[2],
		Month:  bitsets[3],
		Dow:    bitsets[4],
		Raw:    expr,
	}, nil
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

// Describe returns a human-readable English description of the cron expression.
func (e Expression) Describe() string {
	return e.DescribeWith(englishMessage, formatTime)
}

// englishMessage fills {name} placeholders without translating msg.
func englishMessage(msg string, pairs ...any) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		msg = strings.ReplaceAll(msg, "{"+fmt.Sprint(pairs[i])+"}", fmt.Sprint(pairs[i+1]))
	}
	return msg
}

// DescribeWith is Describe with the sentences translated by tr (English
// messages with {placeholders}, e.g. "Every day at {time}") and times of day
// formatted by clock.
func (e Expression) DescribeWith(tr func(msg string, pairs ...any) string, clock func(hour, minute int) string) string {
	// Every minute.
	if e.allSet(e.Minute, 0, 59) && e.allSet(e.Hour, 0, 23) &&
		e.allSet(e.Dom, 1, 31) && e.allSet(e.Month, 1, 12) && e.allSet(e.Dow, 0, 6) {
		return tr("Every minute")
	}

	// Step pattern: */N in minute field, all other fields wildcard.
	if e.allSet(e.Hour, 0, 23) && e.allSet(e.Dom, 1, 31) &&
		e.allSet(e.Month, 1, 12) && e.allSet(e.Dow, 0, 6) {
		if step := e.detectStep(e.Minute, 0, 59); step > 1 {
			return tr("Every {n} minutes", "n", step)
		}
	}

	// Specific time patterns.
	minBit := e.singleBit(e.Minute, 0, 59)
	hourBit := e.singleBit(e.Hour, 0, 23)

	if minBit >= 0 && hourBit >= 0 {
		timeStr := clock(hourBit, minBit)

		// Specific day-of-month.
		if !e.allSet(e.Dom, 1, 31) && e.allSet(e.Dow, 0, 6) && e.allSet(e.Month, 1, 12) {
			domBit := e.singleBit(e.Dom, 1, 31)
			if domBit >= 0 {
				return tr("Day {day} of every month at {time}", "day", domBit, "time", timeStr)
			}
		}

		// Weekday pattern (Mon-Fri).
		if e.allSet(e.Dom, 1, 31) && e.allSet(e.Month, 1, 12) && e.Dow == weekdayBits() {
			return tr("Every weekday at {time}", "time", timeStr)
		}

		// Every day.
		if e.allSet(e.Dom, 1, 31) && e.allSet(e.Month, 1, 12) && e.allSet(e.Dow, 0, 6) {
			return tr("Every day at {time}", "time", timeStr)
		}
	}

	return e.Raw
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
