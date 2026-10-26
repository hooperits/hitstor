package cron

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		// Valid expressions.
		{"all wildcards", "* * * * *", false},
		{"daily at 2 AM", "0 2 * * *", false},
		{"weekdays at 3:30", "30 3 * * 1-5", false},
		{"every 15 minutes", "*/15 * * * *", false},
		{"specific day-of-month", "0 0 1 * *", false},
		{"lists", "0,15,30,45 * * * *", false},
		{"range with step", "0 0 1-15/3 * *", false},
		{"month list", "0 0 1 1,6,12 *", false},
		{"day-of-week list", "0 9 * * 1,3,5", false},
		{"complex", "5,10 1-3 1,15 */2 0,6", false},

		// Invalid expressions.
		{"empty string", "", true},
		{"too few fields", "* * *", true},
		{"too many fields", "* * * * * *", true},
		{"minute out of range", "60 * * * *", true},
		{"hour out of range", "0 25 * * *", true},
		{"dom out of range", "0 0 32 * *", true},
		{"month out of range", "0 0 * 13 *", true},
		{"dow out of range", "0 0 * * 8", true},
		{"invalid range", "5-2 * * * *", true},
		{"non-numeric", "abc * * * *", true},
		{"negative value", "-1 * * * *", true},
		{"dom zero", "0 0 0 * *", true},
		{"month zero", "0 0 * 0 *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr = %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestNext(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")

	tests := []struct {
		name string
		expr string
		from time.Time
		want time.Time
	}{
		{
			name: "daily at 2 AM from 1 AM",
			expr: "0 2 * * *",
			from: time.Date(2026, 3, 3, 1, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 3, 2, 0, 0, 0, time.UTC),
		},
		{
			name: "daily at 2 AM from 3 AM (next day)",
			expr: "0 2 * * *",
			from: time.Date(2026, 3, 3, 3, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 4, 2, 0, 0, 0, time.UTC),
		},
		{
			name: "weekdays at 3:30 from Wednesday",
			expr: "30 3 * * 1-5",
			from: time.Date(2026, 3, 4, 4, 0, 0, 0, time.UTC),  // Wed after 3:30
			want: time.Date(2026, 3, 5, 3, 30, 0, 0, time.UTC), // Thu
		},
		{
			name: "weekdays at 3:30 from Friday (skip weekend)",
			expr: "30 3 * * 1-5",
			from: time.Date(2026, 3, 6, 4, 0, 0, 0, time.UTC),  // Fri after 3:30
			want: time.Date(2026, 3, 9, 3, 30, 0, 0, time.UTC), // Mon
		},
		{
			name: "every 15 minutes",
			expr: "*/15 * * * *",
			from: time.Date(2026, 3, 3, 10, 7, 0, 0, time.UTC),
			want: time.Date(2026, 3, 3, 10, 15, 0, 0, time.UTC),
		},
		{
			name: "first of month",
			expr: "0 0 1 * *",
			from: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "exact match advances to next",
			expr: "0 2 * * *",
			from: time.Date(2026, 3, 3, 2, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 4, 2, 0, 0, 0, time.UTC),
		},
		{
			name: "timezone aware daily at 2 AM ET",
			expr: "0 2 * * *",
			from: time.Date(2026, 3, 3, 1, 0, 0, 0, loc),
			want: time.Date(2026, 3, 3, 2, 0, 0, 0, loc),
		},
		{
			name: "year wrap",
			expr: "0 0 1 1 *",
			from: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			want: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := Parse(tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.expr, err)
			}
			got := e.Next(tt.from)
			if !got.Equal(tt.want) {
				t.Errorf("Next(%v) = %v, want %v", tt.from, got, tt.want)
			}
		})
	}
}

func TestNextImpossibleExpression(t *testing.T) {
	// Feb 30 never exists — Next() should return zero time.
	e, err := Parse("0 0 30 2 *")
	if err != nil {
		t.Fatalf("Parse unexpected error: %v", err)
	}
	got := e.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if !got.IsZero() {
		t.Errorf("Next() for impossible expression = %v, want zero time", got)
	}
}

func TestNextDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	t.Run("spring forward skips non-existent 2:30 AM", func(t *testing.T) {
		// 2026 DST spring forward: March 8, 2:00 AM → 3:00 AM ET.
		// Schedule at 2:30 AM should skip to next valid occurrence.
		e, _ := Parse("30 2 * * *")
		from := time.Date(2026, 3, 7, 3, 0, 0, 0, loc) // Day before DST
		got := e.Next(from)

		// 2:30 AM on March 8 doesn't exist in ET, should skip to March 9.
		want := time.Date(2026, 3, 9, 2, 30, 0, 0, loc)
		if !got.Equal(want) {
			t.Errorf("spring forward: Next(%v) = %v, want %v", from, got, want)
		}
	})

	t.Run("fall back returns first occurrence", func(t *testing.T) {
		// 2026 DST fall back: November 1, 2:00 AM → 1:00 AM ET.
		// 1:30 AM occurs twice. Should return the first (EDT, lower Unix).
		e, _ := Parse("30 1 * * *")
		from := time.Date(2026, 10, 31, 2, 0, 0, 0, loc) // Day before fall back
		got := e.Next(from)

		// First 1:30 AM on Nov 1 is in EDT (UTC-4).
		want := time.Date(2026, 11, 1, 1, 30, 0, 0, loc)
		if !got.Equal(want) {
			t.Errorf("fall back: Next(%v) = %v, want %v", from, got, want)
		}

		// Verify it's the earlier (EDT) occurrence by checking Unix timestamp.
		// EDT 1:30 AM = UTC 5:30 AM, EST 1:30 AM = UTC 6:30 AM.
		if got.Unix() >= time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC).Unix() {
			t.Errorf("fall back: got later occurrence (EST), want earlier (EDT)")
		}
	})

	t.Run("missed window advances to next future occurrence", func(t *testing.T) {
		// Daily at 2 AM, last ran yesterday at 2 AM, now it's 5 AM (missed).
		// Next() should return tomorrow's 2 AM, not today's already-passed 2 AM.
		e, _ := Parse("0 2 * * *")
		from := time.Date(2026, 3, 3, 5, 0, 0, 0, loc)
		got := e.Next(from)
		want := time.Date(2026, 3, 4, 2, 0, 0, 0, loc)
		if !got.Equal(want) {
			t.Errorf("missed window: Next(%v) = %v, want %v", from, got, want)
		}
	})

	t.Run("impossible expression Feb 30 returns zero after 4 years", func(t *testing.T) {
		e, _ := Parse("0 0 30 2 *")
		from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
		got := e.Next(from)
		if !got.IsZero() {
			t.Errorf("impossible expression: Next() = %v, want zero time", got)
		}
	})
}

func TestDescribe(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"every minute", "* * * * *", "Every minute"},
		{"daily at 2 AM", "0 2 * * *", "Every day at 2:00 AM"},
		{"daily at 2:30 PM", "30 14 * * *", "Every day at 2:30 PM"},
		{"weekdays at 3:30 AM", "30 3 * * 1-5", "Every weekday at 3:30 AM"},
		{"every 15 minutes", "*/15 * * * *", "Every 15 minutes"},
		{"first of month midnight", "0 0 1 * *", "Day 1 of every month at 12:00 AM"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := Parse(tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.expr, err)
			}
			got := e.Describe()
			if got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}
