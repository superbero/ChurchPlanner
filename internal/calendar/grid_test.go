package calendar

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestGridBounds(t *testing.T) {
	tests := []struct {
		name      string
		first     time.Time
		weekStart time.Weekday
		wantStart time.Time
		wantWeeks int
	}{
		// Feb 2026 starts on a Sunday and has 28 days: exactly 4 rows.
		{"4 rows, sunday start", d(2026, time.February, 1), time.Sunday, d(2026, time.February, 1), 4},
		// Nov 2026 starts on a Sunday, 30 days: 5 rows.
		{"5 rows", d(2026, time.November, 1), time.Sunday, d(2026, time.November, 1), 5},
		// Aug 2026 starts on a Saturday, 31 days: needs 6 rows.
		{"6 rows", d(2026, time.August, 1), time.Sunday, d(2026, time.July, 26), 6},
		// Same month, Monday start: Nov 1 2026 is a Sunday, so 6 days of padding.
		{"monday start", d(2026, time.November, 1), time.Monday, d(2026, time.October, 26), 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, weeks := gridBounds(tt.first, tt.weekStart)
			if !start.Equal(tt.wantStart) || weeks != tt.wantWeeks {
				t.Errorf("gridBounds() = %s, %d; want %s, %d",
					dateKey(start), weeks, dateKey(tt.wantStart), tt.wantWeeks)
			}
		})
	}
}

func TestDaysInRange(t *testing.T) {
	from, to := d(2026, time.November, 1), d(2026, time.November, 30)

	t.Run("inside", func(t *testing.T) {
		got := daysInRange(d(2026, time.November, 3), d(2026, time.November, 5), from, to)
		if len(got) != 3 {
			t.Fatalf("got %d days, want 3", len(got))
		}
	})
	t.Run("clamped on both sides", func(t *testing.T) {
		got := daysInRange(d(2020, time.January, 1), d(2099, time.December, 31), from, to)
		if len(got) != 30 {
			t.Fatalf("got %d days, want 30", len(got))
		}
	})
	t.Run("outside", func(t *testing.T) {
		got := daysInRange(d(2026, time.December, 1), d(2026, time.December, 5), from, to)
		if len(got) != 0 {
			t.Fatalf("got %d days, want 0", len(got))
		}
	})
	t.Run("single day", func(t *testing.T) {
		got := daysInRange(d(2026, time.November, 7), d(2026, time.November, 7), from, to)
		if len(got) != 1 {
			t.Fatalf("got %d days, want 1", len(got))
		}
	})
}
