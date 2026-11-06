package cron

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

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

// mondayFirst lists cron weekdays in calendar order.
var mondayFirst = []int{1, 2, 3, 4, 5, 6, 0}

// isRun reports whether days (cron weekdays) are consecutive in
// mondayFirst order.
func isRun(days []int) bool {
	pos := func(d int) int { return (d + 6) % 7 }
	for i := 1; i < len(days); i++ {
		if pos(days[i]) != pos(days[i-1])+1 {
			return false
		}
	}
	return true
}

var (
	weekdayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	ordinals     = [6]string{"first", "second", "third", "fourth", "fifth", "last"}
)

// DescribeWith is Describe with the sentences translated by tr (English
// messages with {placeholders}, e.g. "Every day") and times of day
// formatted by clock. A sentence is the days the expression runs on and
// when it runs on them, e.g. "Every Monday and Friday" + "at 2:00 AM". It
// falls back to the raw expression for patterns it cannot put in words.
func (e Expression) DescribeWith(tr func(msg string, pairs ...any) string, clock func(hour, minute int) string) string {
	if e.allSet(e.Minute, 0, 59) && e.allSet(e.Hour, 0, 23) &&
		e.allSet(e.Dom, 1, 31) && e.allSet(e.Month, 1, 12) && e.allSet(e.Dow, 0, 6) && !e.LastDom && !e.hasNthDow() {
		return tr("Every minute")
	}
	if !e.allSet(e.Month, 1, 12) {
		return e.Raw
	}
	days, everyDay := e.describeDays(tr)
	times, repeating := e.describeTimes(tr, clock)
	if days == "" || times == "" {
		return e.Raw
	}
	switch {
	case everyDay && repeating:
		return upperFirst(times)
	case repeating:
		return tr("{days}, {times}", "days", days, "times", times)
	}
	return tr("{days} {times}", "days", days, "times", times)
}

// describeDays describes the days the expression runs on; everyDay is set
// when that is every day.
func (e Expression) describeDays(tr func(string, ...any) string) (string, bool) {
	domAll := e.allSet(e.Dom, 1, 31) && !e.LastDom
	dowAll := e.allSet(e.Dow, 0, 6) && !e.hasNthDow()
	switch {
	case domAll && dowAll:
		return tr("Every day"), true
	case !domAll && !dowAll:
		return "", false // either one; rare enough to show raw
	case !domAll:
		var list []string
		for d := 1; d <= 31; d++ {
			if e.Dom&(1<<uint(d)) != 0 {
				list = append(list, fmt.Sprint(d))
			}
		}
		joined := joinList(tr, list)
		switch {
		case len(list) == 0:
			return tr("The last day of every month"), false
		case e.LastDom:
			return tr("Days {days} and the last day of every month", "days", joined), false
		case len(list) == 1:
			return tr("Day {day} of every month", "day", joined), false
		}
		return tr("Days {days} of every month", "days", joined), false
	}

	if e.hasNthDow() {
		if e.Dow != 0 {
			return "", false
		}
		// Only a full grid of weeks × weekdays reads well.
		var weeks uint8
		var names []string
		for _, d := range mondayFirst {
			n := e.NthDow[d]
			if n == 0 {
				continue
			}
			if weeks != 0 && n != weeks {
				return "", false
			}
			weeks = n
			names = append(names, tr(weekdayNames[d]))
		}
		var nth []string
		for b := 0; b <= lastWeekBit; b++ {
			if weeks&(1<<uint(b)) != 0 {
				nth = append(nth, tr(ordinals[b]))
			}
		}
		return tr("The {nth} {days} of every month", "nth", joinList(tr, nth), "days", joinList(tr, names)), false
	}
	if e.Dow == weekdayBits() {
		return tr("Every weekday"), false
	}
	var days []int
	for _, d := range mondayFirst {
		if e.Dow&(1<<uint(d)) != 0 {
			days = append(days, d)
		}
	}
	// A run of three or more days in calendar order reads as a range.
	if len(days) >= 3 && len(days) < 7 && isRun(days) {
		return tr("{from} to {to}", "from", tr(weekdayNames[days[0]]), "to", tr(weekdayNames[days[len(days)-1]])), false
	}
	var names []string
	for _, d := range days {
		names = append(names, tr(weekdayNames[d]))
	}
	return tr("Every {days}", "days", joinList(tr, names)), false
}

// describeTimes describes when, on a matching day, the expression runs;
// repeating is set for "every N minutes/hours" rather than times of day.
func (e Expression) describeTimes(tr func(string, ...any) string, clock func(int, int) string) (string, bool) {
	hours := setBits(e.Hour, 0, 23)
	minutes := setBits(e.Minute, 0, 59)
	firstH, lastH := hours[0], hours[len(hours)-1]

	// Every N minutes, all day or within a window of hours.
	if step := e.detectStep(e.Minute, 0, 59); step > 1 || (len(minutes) == 60) {
		if step == 0 {
			step = 1
		}
		if !contiguous(hours) {
			return "", false
		}
		if len(hours) == 24 {
			return tr("every {n} minutes", "n", step), true
		}
		return tr("every {n} minutes from {from} to {to}", "n", step,
			"from", clock(firstH, 0), "to", clock(lastH, minutes[len(minutes)-1])), true
	}
	if len(minutes) != 1 {
		return "", false
	}
	m := minutes[0]
	if len(hours) == 1 {
		return tr("at {time}", "time", clock(firstH, m)), false
	}
	// Two times read better as a list than as "every 12 hours".
	if step := progression(hours); step > 0 && len(hours) > 2 {
		wholeDay := firstH < step && lastH+step > 23
		switch {
		case step == 1 && wholeDay:
			return tr("every hour at minute {m}", "m", fmt.Sprintf("%02d", m)), true
		case wholeDay:
			return tr("every {n} hours starting at {time}", "n", step, "time", clock(firstH, m)), true
		case step == 1:
			return tr("every hour from {from} to {to}", "from", clock(firstH, m), "to", clock(lastH, m)), true
		}
		return tr("every {n} hours from {from} to {to}", "n", step, "from", clock(firstH, m), "to", clock(lastH, m)), true
	}
	var list []string
	for _, h := range hours {
		list = append(list, clock(h, m))
	}
	return tr("at {times}", "times", joinList(tr, list)), false
}

func setBits(bitset uint64, lo, hi int) []int {
	var out []int
	for i := lo; i <= hi; i++ {
		if bitset&(1<<uint(i)) != 0 {
			out = append(out, i)
		}
	}
	return out
}

func contiguous(v []int) bool { return progression(v) == 1 || len(v) == 1 }

// progression returns the common difference of v, or 0 if v is not an
// arithmetic progression of at least two values.
func progression(v []int) int {
	if len(v) < 2 {
		return 0
	}
	step := v[1] - v[0]
	for i := 2; i < len(v); i++ {
		if v[i]-v[i-1] != step {
			return 0
		}
	}
	return step
}

// joinList joins items as "a, b and c", translating the last separator.
func joinList(tr func(string, ...any) string, items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return tr("{a} and {b}", "a", strings.Join(items[:len(items)-1], ", "), "b", items[len(items)-1])
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}
