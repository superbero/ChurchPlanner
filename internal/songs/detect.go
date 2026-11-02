package songs

import "strings"

// DetectFormat returns one of: chordpro, opensong, openlyrics, text.
func DetectFormat(filename, content string) string {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".cho") ||
		strings.HasSuffix(lower, ".pro") ||
		strings.HasSuffix(lower, ".chordpro") {
		return "chordpro"
	}

	trimmed := strings.TrimSpace(content)

	// XML-based formats: inspect root element
	if strings.HasPrefix(trimmed, "<") {
		if strings.Contains(trimmed, "openlyrics.info") ||
			strings.Contains(trimmed, "xmlns=\"http://openlyrics.info") {
			return "openlyrics"
		}
		if strings.HasPrefix(trimmed, "<song") || strings.Contains(trimmed, "<lyrics>") {
			return "opensong"
		}
		if strings.Contains(trimmed, "<?xml") {
			// Generic XML — try OpenSong by default if it looks like a song root
			return "opensong"
		}
	}

	if directiveRE.MatchString(content) || chordLineRE.MatchString(content) {
		return "chordpro"
	}
	return "text"
}
