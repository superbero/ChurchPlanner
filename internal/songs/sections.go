package songs

import (
	"regexp"
	"strings"
)

// Section is a chordpro source section with its normalized label.
type Section struct {
	Label string
	Lines []string
}

var (
	// {start_of_verse}, {start_of_verse: Verse 1}, {soc}, {sov}, {sob}, {sot}, {soi}
	startDirectiveRE = regexp.MustCompile(`^\{\s*(?:start_of_(\w+)|so(c|v|b|t|i))(?::\s*(.+?))?\s*\}$`)
	endDirectiveRE   = regexp.MustCompile(`^\{\s*(?:end_of_\w+|eo(c|v|b|t|i))\s*\}$`)

	// [Verse 1], [Chorus], [Bridge], [Pre-Chorus 2], [Tag]
	plainLabelRE = regexp.MustCompile(`^\[\s*(verse|chorus|bridge|pre[- ]?chorus|tag|intro|outro|interlude|instrumental|breakdown|vamp|refrain|ending)(?:\s+(\d+))?\s*\]$`)
)

var shortKind = map[string]string{
	"verse":        "V",
	"chorus":       "C",
	"bridge":       "B",
	"prechorus":    "PC",
	"pre-chorus":   "PC",
	"tag":          "T",
	"intro":        "I",
	"outro":        "O",
	"interlude":    "IL",
	"instrumental": "IB",
	"breakdown":    "BD",
	"vamp":         "VP",
	"refrain":      "RF",
	"ending":       "END",
}

func shortKindFor(kind string) string {
	k := strings.ToLower(strings.ReplaceAll(kind, " ", ""))
	if s, ok := shortKind[k]; ok {
		return s
	}
	return strings.ToUpper(kind)
}

// normalizeLabel turns "start_of_verse: Verse 1" -> "V1"; {soc} -> "C"; etc.
func normalizeLabel(kind, title string) string {
	short := shortKindFor(kind)
	num := extractTrailingNumber(title)
	if num == "" {
		num = extractTrailingNumber(kind)
	}
	// For singleton sections like C/B/I/O without title, no number.
	return short + num
}

func extractTrailingNumber(s string) string {
	s = strings.TrimSpace(s)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] < '0' || s[i] > '9' {
			if i == len(s)-1 {
				return ""
			}
			return s[i+1:]
		}
	}
	return s
}

// ParseSections splits ChordPro source into labeled sections.
func ParseSections(source string) []Section {
	var sections []Section
	var current *Section
	flush := func() {
		if current != nil && len(current.Lines) > 0 {
			sections = append(sections, *current)
		}
		current = nil
	}

	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)

		if m := startDirectiveRE.FindStringSubmatch(trimmed); m != nil {
			flush()
			kind, title := m[1], m[3]
			// Short-form shorthand: {soc}, {sov}, ...
			if kind == "" && m[2] != "" {
				switch m[2] {
				case "c":
					kind = "chorus"
				case "v":
					kind = "verse"
				case "b":
					kind = "bridge"
				case "t":
					kind = "tag"
				case "i":
					kind = "intro"
				}
			}
			current = &Section{Label: normalizeLabel(kind, title)}
			continue
		}
		if endDirectiveRE.MatchString(trimmed) {
			flush()
			continue
		}
		if m := plainLabelRE.FindStringSubmatch(trimmed); m != nil {
			flush()
			kind := strings.ToLower(strings.ReplaceAll(m[1], " ", ""))
			num := m[2]
			current = &Section{Label: shortKindFor(kind) + num}
			continue
		}

		if current == nil {
			if trimmed == "" {
				continue
			}
			current = &Section{Label: ""}
		}
		current.Lines = append(current.Lines, line)
	}
	flush()
	return sections
}

// SequenceOrder splits a sequence string into normalized labels.
func SequenceOrder(seq string) []string {
	fields := strings.FieldsFunc(seq, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' ||
			r == ',' || r == '|' || r == '·' || r == ';'
	})
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		out = append(out, normalizeSeqLabel(f))
	}
	return out
}

func normalizeSeqLabel(s string) string {
	// Try to match "Chorus", "Verse 1", "Bridge", etc.
	if m := plainLabelRE.FindStringSubmatch("[" + s + "]"); m != nil {
		kind := strings.ToLower(strings.ReplaceAll(m[1], " ", ""))
		return shortKindFor(kind) + m[2]
	}
	// Already a short label like "V1", "C", "B"
	s = strings.ToUpper(strings.TrimSpace(s))
	return s
}

// pickSections walks the sequence and returns matching sections in order.
// Unknown sequence labels are skipped. Empty sequence -> all sections in
// document order.
func pickSections(sections []Section, seq []string) []Section {
	if len(seq) == 0 {
		return sections
	}
	byLabel := map[string][]Section{}
	for _, s := range sections {
		byLabel[s.Label] = append(byLabel[s.Label], s)
	}
	var out []Section
	used := map[string]int{}
	for _, label := range seq {
		matches, ok := byLabel[label]
		if !ok {
			// fall back: match by prefix (e.g. "V" matches "V1")
			for k, v := range byLabel {
				if strings.HasPrefix(k, label) {
					matches = v
					break
				}
			}
		}
		if len(matches) == 0 {
			continue
		}
		idx := used[label]
		if idx >= len(matches) {
			idx = len(matches) - 1
		}
		out = append(out, matches[idx])
		used[label]++
	}
	return out
}
