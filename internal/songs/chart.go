package songs

import (
	"strings"
)

// BuildChart renders an arrangement's chart in the given key.
// If displayKey is empty, arrangement's original_key is used.
func BuildChart(a *Arrangement, displayKey string) []RenderedSection {
	if a == nil {
		return nil
	}

	source := a.SourceContent
	lyrics := a.Lyrics
	chords := a.Chords

	// If source is present, reparse so section directives are respected.
	var sections []Section
	switch a.Format {
	case "chordpro", "":
		sections = ParseSections(source)
	case "opensong":
		if p, err := ParseOpenSong(source); err == nil {
			sections = sectionsFromFlat(p.Chords, p.Lyrics)
		}
	case "openlyrics":
		if p, err := ParseOpenLyrics(source); err == nil {
			sections = sectionsFromFlat(p.Chords, p.Lyrics)
		}
	}
	if len(sections) == 0 {
		// Manual entry: use stored flat lyrics/chords (line-aligned)
		sections = sectionsFromFlat(chords, lyrics)
	}

	// Apply transposition from arrangement original_key -> displayKey
	semis := 0
	flats := false
	if a.OriginalKey != "" && displayKey != "" && a.OriginalKey != displayKey {
		if oi, ok1 := KeyIndex(a.OriginalKey); ok1 {
			if ci, ok2 := KeyIndex(displayKey); ok2 {
				semis = ci - oi
				if semis > 6 {
					semis -= 12
				}
				if semis < -6 {
					semis += 12
				}
				flats = keyUsesFlats(displayKey)
			}
		}
	}
	if semis != 0 {
		for i := range sections {
			for j, line := range sections[i].Lines {
				sections[i].Lines[j] = TransposeChordProText(line, semis, flats)
			}
		}
	}

	selected := pickSections(sections, SequenceOrder(a.Sequence))

	var out []RenderedSection
	for _, s := range selected {
		rs := RenderedSection{Label: s.Label}
		for _, line := range s.Lines {
			lyric, chordPos := ParseChartLine(line)
			rs.Lines = append(rs.Lines, ChartLine{Chords: chordPos, Lyric: lyric})
		}
		out = append(out, rs)
	}
	return out
}

// sectionsFromFlat wraps line-aligned chords/lyrics into a single section.
func sectionsFromFlat(chords, lyrics string) []Section {
	chordLines := strings.Split(chords, "\n")
	lyricLines := strings.Split(lyrics, "\n")
	if len(chordLines) < len(lyricLines) {
		for len(chordLines) < len(lyricLines) {
			chordLines = append(chordLines, "")
		}
	}
	var lines []string
	n := len(lyricLines)
	if len(chordLines) > n {
		n = len(chordLines)
	}
	for i := 0; i < n; i++ {
		cl, ll := "", ""
		if i < len(chordLines) {
			cl = chordLines[i]
		}
		if i < len(lyricLines) {
			ll = lyricLines[i]
		}
		if cl == "" && ll == "" {
			lines = append(lines, "")
			continue
		}
		// Reconstruct a chordpro-style line: overlay chords above lyrics
		lines = append(lines, overlayChordLine(cl, ll))
	}
	return []Section{{Label: "", Lines: lines}}
}

// overlayChordLine re-attaches chords parsed from an aligned chord line to the
// lyric line, producing ChordPro text so ParseChartLine can re-extract positions.
func overlayChordLine(chordLine, lyricLine string) string {
	if chordLine == "" {
		return lyricLine
	}
	// Chord line may already be in bracket format
	if strings.ContainsRune(chordLine, '[') {
		return chordLine
	}
	// Chord line is plain text: extract chord tokens with their positions.
	type tok struct {
		pos   int
		chord string
	}
	var toks []tok
	i := 0
	for i < len(chordLine) {
		if chordLine[i] == ' ' {
			i++
			continue
		}
		start := i
		for i < len(chordLine) && chordLine[i] != ' ' {
			i++
		}
		toks = append(toks, tok{pos: start, chord: chordLine[start:i]})
	}
	if len(toks) == 0 {
		return lyricLine
	}
	// Build chordpro by inserting brackets at appropriate character positions.
	var b strings.Builder
	lyricIdx := 0
	ti := 0
	for lyricIdx <= len(lyricLine) {
		if ti < len(toks) && toks[ti].pos == lyricIdx {
			b.WriteByte('[')
			b.WriteString(toks[ti].chord)
			b.WriteByte(']')
			ti++
			continue
		}
		if lyricIdx == len(lyricLine) {
			break
		}
		b.WriteByte(lyricLine[lyricIdx])
		lyricIdx++
	}
	for ; ti < len(toks); ti++ {
		b.WriteByte('[')
		b.WriteString(toks[ti].chord)
		b.WriteByte(']')
	}
	return b.String()
}
