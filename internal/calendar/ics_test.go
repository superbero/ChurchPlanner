package calendar

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var sampleStart = time.Date(2026, time.November, 8, 9, 0, 0, 0, time.UTC)

func sampleEvent() Event {
	return Event{
		UID:      "assignment-1@churchplanner",
		Summary:  "Sunday Service - Drums",
		Start:    sampleStart,
		End:      sampleStart.Add(90 * time.Minute),
		Modified: sampleStart.Add(-48 * time.Hour),
	}
}

func TestEscapeText(t *testing.T) {
	got := escapeText("a,b;c\\d\r\ne\nf")
	want := `a\,b\;c\\d\ne\nf`
	if got != want {
		t.Errorf("escapeText() = %q, want %q", got, want)
	}
}

func TestBuildCalendarStructure(t *testing.T) {
	out := string(BuildCalendar("My schedule", []Event{sampleEvent()}))

	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n",
		"VERSION:2.0\r\n",
		"BEGIN:VEVENT\r\n",
		"UID:assignment-1@churchplanner\r\n",
		"DTSTART:20261108T090000Z\r\n",
		"DTEND:20261108T103000Z\r\n",
		"SUMMARY:Sunday Service - Drums\r\n",
		"STATUS:CONFIRMED\r\n",
		"END:VEVENT\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if !strings.HasSuffix(out, "END:VCALENDAR\r\n") {
		t.Errorf("output must end with END:VCALENDAR + CRLF")
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Errorf("found a bare LF; every line must end in CRLF")
	}
}

func TestBuildCalendarIsDeterministic(t *testing.T) {
	a := BuildCalendar("x", []Event{sampleEvent()})
	b := BuildCalendar("x", []Event{sampleEvent()})
	if string(a) != string(b) {
		t.Error("same input must give identical output (ETag depends on it)")
	}
}

func TestTentativeStatus(t *testing.T) {
	e := sampleEvent()
	e.Tentative = true
	if !strings.Contains(string(BuildCalendar("x", []Event{e})), "STATUS:TENTATIVE\r\n") {
		t.Error("tentative event should produce STATUS:TENTATIVE")
	}
}

func TestFoldingKeepsLinesShortAndRunesIntact(t *testing.T) {
	long := strings.Repeat("é", 100) // 2 bytes each
	e := sampleEvent()
	e.Summary = long
	out := string(BuildCalendar("x", []Event{e}))

	for _, l := range strings.Split(out, "\r\n") {
		if len(l) > 75 {
			t.Errorf("line is %d octets (max 75): %q", len(l), l)
		}
		if !utf8.ValidString(l) {
			t.Errorf("a multi-byte character was split: %q", l)
		}
	}
	unfolded := strings.ReplaceAll(out, "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+long+"\r\n") {
		t.Error("unfolding did not restore the original summary")
	}
}

func TestNoPropertyInjection(t *testing.T) {
	e := sampleEvent()
	e.Summary = "Hi\r\nATTACH:http://evil.example/x"
	e.Description = "line1\nORGANIZER:mailto:evil@example.com"
	e.URL = "https://ok.example/\r\nATTACH:http://evil.example/y"
	out := string(BuildCalendar("x", []Event{e}))

	if strings.Contains(out, "\r\nATTACH") || strings.Contains(out, "\r\nORGANIZER") {
		t.Errorf("user-controlled text injected a new property:\n%s", out)
	}
}

func TestSubscribeLinks(t *testing.T) {
	l := subscribeLinks("https://church.example.org/", "TOKEN", "My schedule")

	if l.HTTPS != "https://church.example.org/calendar/feed/TOKEN.ics" {
		t.Errorf("HTTPS = %q", l.HTTPS)
	}
	if string(l.Webcal) != "webcal://church.example.org/calendar/feed/TOKEN.ics" {
		t.Errorf("Webcal = %q", l.Webcal)
	}
	if !strings.HasPrefix(string(l.Google),
		"https://calendar.google.com/calendar/render?cid=webcal%3A%2F%2Fchurch.example.org") {
		t.Errorf("Google = %q", l.Google)
	}
	if !strings.HasPrefix(string(l.Outlook365), "https://outlook.office.com/calendar/0/addfromweb?name=My+schedule&url=webcal%3A%2F%2F") {
		t.Errorf("Outlook365 = %q", l.Outlook365)
	}
	if !strings.HasPrefix(string(l.OutlookLive), "https://outlook.live.com/calendar/0/addfromweb?") {
		t.Errorf("OutlookLive = %q", l.OutlookLive)
	}
}
