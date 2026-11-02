package songs

import (
	"regexp"
	"strconv"
	"strings"
)

type ParsedSong struct {
	Title  string
	Author string
	Key    string
	Tempo  int
	Lyrics string
	Chords string
	Format string
	Source string
}

var directiveRE = regexp.MustCompile(`^\s*\{([a-zA-Z_]+):\s*(.*?)\}\s*$`)
var chordLineRE = regexp.MustCompile(`\[[^\]]+\]`)

// ParseChordPro keeps chordLines and lyricLines aligned line-by-line.
func ParseChordPro(input string) ParsedSong {
	out := ParsedSong{Format: "chordpro", Source: input}
	var lyricLines, chordLines []string

	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimRight(line, "\r")

		if m := directiveRE.FindStringSubmatch(line); m != nil {
			key := strings.ToLower(m[1])
			val := strings.TrimSpace(m[2])
			switch key {
			case "title", "t":
				out.Title = val
			case "artist", "author", "composer", "a":
				out.Author = val
			case "key":
				out.Key = val
			case "tempo":
				if n, err := strconv.Atoi(digitsOnly(val)); err == nil {
					out.Tempo = n
				}
			}
			continue
		}

		if chordLineRE.MatchString(line) {
			chordLines = append(chordLines, line)
			lyricLines = append(lyricLines, chordLineRE.ReplaceAllString(line, ""))
		} else {
			// FIX: keep chord/lyric line counts in sync.
			chordLines = append(chordLines, "")
			lyricLines = append(lyricLines, line)
		}
	}

	out.Lyrics = strings.TrimSpace(strings.Join(lyricLines, "\n"))
	out.Chords = strings.TrimSpace(strings.Join(chordLines, "\n"))
	return out
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
