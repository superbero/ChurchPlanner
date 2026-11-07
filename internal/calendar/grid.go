package calendar

import "time"

// dateKey is the map key used for "which cell does this belong to".
// Keying by full date (not day-of-month) lets the leading/trailing days of
// neighbouring months show their plans too.
func dateKey(t time.Time) string { return t.Format("2006-01-02") }

// civil strips the time-of-day and location, keeping only the calendar date.
// DATE columns arrive as UTC midnight; this makes comparisons location-proof.
func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// gridBounds returns the first date shown (the weekStart on or before the 1st
// of the month) and how many week rows are needed (4 to 6).
func gridBounds(firstOfMonth time.Time, weekStart time.Weekday) (start time.Time, weeks int) {
	offset := (int(firstOfMonth.Weekday()) - int(weekStart) + 7) % 7
	start = firstOfMonth.AddDate(0, 0, -offset)
	daysInMonth := firstOfMonth.AddDate(0, 1, -1).Day()
	weeks = (offset + daysInMonth + 6) / 7
	return start, weeks
}

// daysInRange returns each date in [start, end] clamped to [from, to]
// (all inclusive). Clamping means a blockout spanning years costs at most
// one grid's worth of iterations instead of looping over its full length.
func daysInRange(start, end, from, to time.Time) []time.Time {
	s, e, f, t := civil(start), civil(end), civil(from), civil(to)
	if s.Before(f) {
		s = f
	}
	if e.After(t) {
		e = t
	}
	var out []time.Time
	for d := s; !d.After(e); d = d.AddDate(0, 0, 1) {
		out = append(out, d)
	}
	return out
}
