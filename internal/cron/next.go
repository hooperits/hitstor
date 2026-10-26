package cron

import "time"

// maxSearchYears limits the search to prevent infinite loops on impossible
// expressions (e.g., Feb 30).
const maxSearchYears = 4

// Next returns the next time matching the expression after from.
// Returns zero time if no match is found within 4 years.
func (e Expression) Next(from time.Time) time.Time {
	// Start one minute after from, truncated to the minute.
	t := from.Truncate(time.Minute).Add(time.Minute)
	loc := t.Location()
	deadline := from.AddDate(maxSearchYears, 0, 0)

	for t.Before(deadline) {
		// Check month.
		month := int(t.Month())
		if e.Month&(1<<uint(month)) == 0 {
			month = nextSetBit(e.Month, month, 1, 12)
			if month < 0 {
				// Wrap to next year.
				month = nextSetBit(e.Month, 0, 1, 12)
				t = time.Date(t.Year()+1, time.Month(month), 1, 0, 0, 0, 0, loc)
			} else {
				t = time.Date(t.Year(), time.Month(month), 1, 0, 0, 0, 0, loc)
			}
			continue
		}

		// Check day (dom AND dow).
		day := t.Day()
		if !e.dayMatches(t) {
			// Advance to next day.
			t = time.Date(t.Year(), t.Month(), day+1, 0, 0, 0, 0, loc)
			continue
		}

		// Check hour.
		hour := t.Hour()
		if e.Hour&(1<<uint(hour)) == 0 {
			hour = nextSetBit(e.Hour, hour, 0, 23)
			if hour < 0 {
				// Wrap to next day.
				t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
				continue
			}
			t = time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, loc)
			// DST spring-forward: time.Date may adjust the time.
			if t.Hour() != hour {
				t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
				continue
			}
			continue
		}

		// Check minute.
		minute := t.Minute()
		if e.Minute&(1<<uint(minute)) == 0 {
			minute = nextSetBit(e.Minute, minute, 0, 59)
			if minute < 0 {
				// Wrap to next hour.
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
				continue
			}
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, 0, 0, loc)
			// DST check.
			if t.Minute() != minute || t.Hour() != hour {
				t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
				continue
			}
			continue
		}

		// All fields match. Verify DST didn't shift us.
		candidate := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		if candidate.Hour() != t.Hour() || candidate.Minute() != t.Minute() {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}

		return candidate
	}

	return time.Time{}
}

// dayMatches checks if the given time matches the day-of-month and
// day-of-week constraints. Standard cron behavior: if both dom and dow
// are restricted (not all-wildcards), match EITHER. If only one is
// restricted, match that one.
func (e Expression) dayMatches(t time.Time) bool {
	day := t.Day()
	dow := int(t.Weekday()) // 0=Sunday

	domRestricted := !allBitsSet(e.Dom, 1, 31)
	dowRestricted := !allBitsSet(e.Dow, 0, 6)

	domMatch := e.Dom&(1<<uint(day)) != 0
	dowMatch := e.Dow&(1<<uint(dow)) != 0

	if domRestricted && dowRestricted {
		return domMatch || dowMatch
	}
	if domRestricted {
		return domMatch
	}
	if dowRestricted {
		return dowMatch
	}
	return true
}

// nextSetBit finds the next set bit > current in the bitset within [lo, hi].
// Returns -1 if no bit is set after current.
func nextSetBit(bitset uint64, current, lo, hi int) int {
	for i := current + 1; i <= hi; i++ {
		if bitset&(1<<uint(i)) != 0 {
			return i
		}
	}
	return -1
}

// allBitsSet checks if all bits from lo to hi are set.
func allBitsSet(bitset uint64, lo, hi int) bool {
	for i := lo; i <= hi; i++ {
		if bitset&(1<<uint(i)) == 0 {
			return false
		}
	}
	return true
}
