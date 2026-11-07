package calendar

import (
	"bytes"
	"strings"
	"time"
	"unicode/utf8"
)

// Event is one VEVENT. Times are converted to UTC on output, so no VTIMEZONE
// block is needed and every client shows the correct local time.
type Event struct {
	UID         string // must be stable across requests so clients update instead of duplicate
	Summary     string
	Description string
	Location    string
	URL         string
	Start, End  time.Time
	Modified    time.Time // last change; also used as DTSTAMP so output is deterministic (ETag-friendly)
	Tentative   bool      // e.g. assignment not yet accepted
}

const icsTimeFmt = "20060102T150405Z"

// Order matters: "\r\n" must be tried before "\r".
var icsTextEscaper = strings.NewReplacer(
	`\`, `\\`, `;`, `\;`, `,`, `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)

var icsLineStripper = strings.NewReplacer("\r", "", "\n", "")

// escapeText escapes a TEXT value. Escaping newlines is also what stops a
// plan title like "x\nATTACH:http://evil" from injecting extra properties.
func escapeText(s string) string { return icsTextEscaper.Replace(s) }

// writeLine writes one content line, folded at 75 octets (RFC 5545 §3.1),
// never splitting a multi-byte UTF-8 character.
func writeLine(b *bytes.Buffer, line string) {
	limit := 75
	for len(line) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		limit = 74 // continuation lines start with a space, which counts
	}
	b.WriteString(line)
	b.WriteString("\r\n")
}

// BuildCalendar renders a complete VCALENDAR document.
func BuildCalendar(name string, events []Event) []byte {
	var b bytes.Buffer
	line := func(s string) { writeLine(&b, s) }

	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//ChurchPlanner//Schedule Feed//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:" + escapeText(name))
	// Hints only: Apple honours them; Google and Outlook decide their own refresh rate.
	line("REFRESH-INTERVAL;VALUE=DURATION:PT1H")
	line("X-PUBLISHED-TTL:PT1H")

	for _, e := range events {
		stamp := e.Modified
		if stamp.IsZero() {
			stamp = e.Start
		}
		line("BEGIN:VEVENT")
		line("UID:" + icsLineStripper.Replace(e.UID))
		line("DTSTAMP:" + stamp.UTC().Format(icsTimeFmt))
		line("DTSTART:" + e.Start.UTC().Format(icsTimeFmt))
		line("DTEND:" + e.End.UTC().Format(icsTimeFmt))
		if !e.Modified.IsZero() {
			line("LAST-MODIFIED:" + e.Modified.UTC().Format(icsTimeFmt))
		}
		line("SUMMARY:" + escapeText(e.Summary))
		if e.Description != "" {
			line("DESCRIPTION:" + escapeText(e.Description))
		}
		if e.Location != "" {
			line("LOCATION:" + escapeText(e.Location))
		}
		if e.URL != "" {
			line("URL:" + icsLineStripper.Replace(e.URL))
		}
		if e.Tentative {
			line("STATUS:TENTATIVE")
		} else {
			line("STATUS:CONFIRMED")
		}
		line("END:VEVENT")
	}

	line("END:VCALENDAR")
	return b.Bytes()
}
