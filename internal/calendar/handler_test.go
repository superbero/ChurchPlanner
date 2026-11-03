package calendar

import (
	"testing"
	"time"
)

func TestRequestedMonth(t *testing.T) {
	now := time.Date(2026, time.November, 3, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		year      string
		month     string
		wantYear  int
		wantMonth int
		wantError bool
	}{
		{name: "default current month", wantYear: 2026, wantMonth: 11},
		{name: "valid month", year: "2025", month: "2", wantYear: 2025, wantMonth: 2},
		{name: "missing year", month: "2", wantError: true},
		{name: "invalid year", year: "x", month: "2", wantError: true},
		{name: "month out of range", year: "2026", month: "13", wantError: true},
		{name: "year out of range", year: "0", month: "1", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year, month, err := requestedMonth(tt.year, tt.month, now)
			if (err != nil) != tt.wantError {
				t.Fatalf("requestedMonth() error = %v, wantError %v", err, tt.wantError)
			}
			if err == nil && (year != tt.wantYear || month != tt.wantMonth) {
				t.Errorf("requestedMonth() = %d/%d, want %d/%d",
					year, month, tt.wantYear, tt.wantMonth)
			}
		})
	}
}
