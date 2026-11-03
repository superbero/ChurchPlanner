package songs

import (
	"regexp"
	"strings"
)

var chordLineRegex = regexp.MustCompile(`^[A-G][#b]?(m|maj|min|dim|aug|sus|add)?[0-9]*(/[A-G][#b]?)?$`)

// InferChordPro takes raw text (e.g., from a PDF) and tries to guess the ChordPro format.
// It looks for lines that are mostly chords and merges them with the lyric line below.
func InferChordPro(raw string) string {
	lines := strings.Split(raw, "\n")
	var out []string

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			out = append(out, "")
			continue
		}

		// Check if this line looks like a chord line
		if isChordLine(line) && i+1 < len(lines) {
			lyricLine := strings.TrimSpace(lines[i+1])

			// If the next line is empty, just output the chords as a standalone line
			if lyricLine == "" {
				out = append(out, "["+strings.Join(strings.Fields(line), "] [")+"]")
				continue
			}

			// Merge chords into lyrics based on word boundaries (naive approximation)
			chords := strings.Fields(line)
			words := strings.Fields(lyricLine)

			var merged []string
			for j, word := range words {
				if j < len(chords) {
					merged = append(merged, "["+chords[j]+"]"+word)
				} else {
					merged = append(merged, word)
				}
			}
			out = append(out, strings.Join(merged, " "))
			i++ // Skip the lyric line since we merged it
		} else {
			out = append(out, lines[i])
		}
	}
	return strings.Join(out, "\n")
}

func isChordLine(line string) bool {
	words := strings.Fields(line)
	if len(words) == 0 || len(words) > 12 {
		return false // Too long to be a chord line
	}

	chordCount := 0
	for _, w := range words {
		// Clean up punctuation
		w = strings.Trim(w, "|:() ")
		if chordLineRegex.MatchString(w) {
			chordCount++
		}
	}
	// If more than 60% of the words look like chords, call it a chord line
	return float64(chordCount)/float64(len(words)) > 0.6
}
