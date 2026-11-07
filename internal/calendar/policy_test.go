package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

func TestBlockoutPolicy(t *testing.T) {
	const me, other = "user-me", "user-other"

	tests := []struct {
		role        string
		seesAll     bool
		editOwn     bool
		editOther   bool
		addForOther bool
	}{
		{auth.RoleAdmin, true, true, true, true},
		{auth.RoleManager, true, true, true, managersMayAddForOthers},
		{auth.RoleEditor, false, true, false, false},
		{auth.RoleViewer, false, true, false, false},
		{auth.RoleScheduledViewer, false, true, false, false},
		{"unknown", false, true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := (viewer{ID: me, Role: tt.role}).seesAll(); got != tt.seesAll {
				t.Errorf("seesAll = %v, want %v", got, tt.seesAll)
			}
			if got := canEditBlockout(tt.role, me, me); got != tt.editOwn {
				t.Errorf("edit own = %v, want %v", got, tt.editOwn)
			}
			if got := canEditBlockout(tt.role, me, other); got != tt.editOther {
				t.Errorf("edit other's = %v, want %v", got, tt.editOther)
			}
			if got := canAddBlockoutFor(tt.role, me, me); !got {
				t.Errorf("everyone must be able to add for themselves")
			}
			if got := canAddBlockoutFor(tt.role, me, other); got != tt.addForOther {
				t.Errorf("add for other = %v, want %v", got, tt.addForOther)
			}
		})
	}
}

func TestPolicyRejectsEmptyIdentities(t *testing.T) {
	// An empty actor ID must never count as "the owner" of an empty owner ID.
	if canEditBlockout(auth.RoleViewer, "", "") {
		t.Error("empty actor must not own anything")
	}
	if canAddBlockoutFor(auth.RoleAdmin, "", "someone") {
		t.Error("missing actor must not add")
	}
	if canAddBlockoutFor(auth.RoleAdmin, "admin", "") {
		t.Error("missing target must not be allowed")
	}
}

func TestParseBlockoutInput(t *testing.T) {
	tests := []struct {
		name       string
		start, end string
		reason     string
		wantErr    string // substring; empty means success
	}{
		{"ok", "2026-11-08", "2026-11-10", "Vacation", ""},
		{"single day", "2026-11-08", "2026-11-08", "", ""},
		{"trims reason", "2026-11-08", "2026-11-08", "  trip  ", ""},
		{"missing start", "", "2026-11-08", "", "required"},
		{"missing end", "2026-11-08", "", "", "required"},
		{"bad format", "08/11/2026", "2026-11-10", "", "look like"},
		{"end before start", "2026-11-10", "2026-11-08", "", "before"},
		{"absurd span", "2026-01-01", "2099-01-01", "", "too long"},
		{"reason too long", "2026-11-08", "2026-11-08", strings.Repeat("é", maxReasonLen+1), "too long"},
		{"reason at limit", "2026-11-08", "2026-11-08", strings.Repeat("é", maxReasonLen), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := parseBlockoutInput(tt.start, tt.end, tt.reason)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if in.Reason != strings.TrimSpace(tt.reason) {
					t.Errorf("reason not trimmed: %q", in.Reason)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestBlockoutsByDayClampsAndShares(t *testing.T) {
	from, last := d(2026, time.November, 1), d(2026, time.November, 30)
	rows := []BlockoutRow{
		{ID: "a", Starts: d(2026, time.November, 3), Ends: d(2026, time.November, 4)},
		{ID: "b", Starts: d(2020, time.January, 1), Ends: d(2099, time.December, 31)}, // huge
	}
	got := blockoutsByDay(rows, from, last)

	if n := len(got["2026-11-03"]); n != 2 {
		t.Errorf("Nov 3 has %d blockouts, want 2", n)
	}
	if n := len(got["2026-11-05"]); n != 1 {
		t.Errorf("Nov 5 has %d blockouts, want 1", n)
	}
	if _, ok := got["2026-12-01"]; ok {
		t.Error("days outside the grid must not be produced")
	}
}

func TestOverlapsRange(t *testing.T) {
	from, to := d(2026, time.November, 1), d(2026, time.December, 1) // [Nov 1, Dec 1)
	tests := []struct {
		name       string
		start, end time.Time
		want       bool
	}{
		{"inside", d(2026, time.November, 5), d(2026, time.November, 6), true},
		{"starts before, ends in", d(2026, time.October, 28), d(2026, time.November, 2), true},
		{"starts in, ends after", d(2026, time.November, 29), d(2026, time.December, 3), true},
		{"ends the day before", d(2026, time.October, 20), d(2026, time.October, 31), false},
		{"starts on exclusive end", d(2026, time.December, 1), d(2026, time.December, 2), false},
		{"covers whole month", d(2026, time.October, 1), d(2027, time.January, 1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := overlapsRange(tt.start, tt.end, from, to); got != tt.want {
				t.Errorf("overlapsRange = %v, want %v", got, tt.want)
			}
		})
	}
}
