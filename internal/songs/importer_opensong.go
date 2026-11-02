package songs

import (
	"encoding/xml"
	"errors"
	"strings"
)

type opensongXML struct {
	XMLName xml.Name `xml:"song"`
	Title   string   `xml:"title"`
	Author  string   `xml:"author"`
	Key     string   `xml:"key"`
	Tempo   string   `xml:"tempo"`
	Lyrics  string   `xml:"lyrics"`
}

// ParseOpenSong reads the OpenSong desktop-app XML format.
// Chord lines are prefixed with "." and sit above the lyric lines they belong to.
func ParseOpenSong(input string) (ParsedSong, error) {
	var root opensongXML
	if err := xml.Unmarshal([]byte(input), &root); err != nil {
		return ParsedSong{}, err
	}
	if root.Title == "" {
		return ParsedSong{}, errors.New("opensong: missing <title>")
	}

	var lyricLines, chordLines []string
	lines := strings.Split(root.Lyrics, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)

		// Chord line: starts with "." (optionally followed by a space)
		if strings.HasPrefix(trimmed, ".") {
			chordRaw := strings.TrimPrefix(trimmed, ".")
			chordRaw = strings.TrimPrefix(chordRaw, " ")
			// Next non-chord line is the lyric for this chord line
			var lyricLine string
			if i+1 < len(lines) {
				candidate := strings.TrimRight(lines[i+1], "\r")
				// If next line is also a chord line or section marker, no lyric here
				cTrim := strings.TrimSpace(candidate)
				if !strings.HasPrefix(cTrim, ".") && !isSectionMarker(cTrim) {
					lyricLine = candidate
					i++
				}
			}
			chordLines = append(chordLines, chordRaw)
			lyricLines = append(lyricLines, lyricLine)
			continue
		}

		// Section marker or plain line
		if isSectionMarker(trimmed) {
			chordLines = append(chordLines, "")
			lyricLines = append(lyricLines, "")
			continue
		}

		// Plain lyric line, no chord
		chordLines = append(chordLines, "")
		lyricLines = append(lyricLines, line)
	}

	tempo := 0
	if n := digitsOnly(root.Tempo); n != "" {
		tempo, _ = parseInt(n)
	}

	return ParsedSong{
		Title:  strings.TrimSpace(root.Title),
		Author: strings.TrimSpace(root.Author),
		Key:    strings.TrimSpace(root.Key),
		Tempo:  tempo,
		Lyrics: strings.Join(lyricLines, "\n"),
		Chords: strings.Join(chordLines, "\n"),
		Format: "opensong",
		Source: input,
	}, nil
}

func isSectionMarker(s string) bool {
	// OpenSong uses [V1], [C], [B], [P], etc. as section markers
	if len(s) < 3 || s[0] != '[' || s[len(s)-1] != ']' {
		return false
	}
	// Only treat as marker if content is short alpha+digits (not a chord bracket)
	return len(s) <= 6
}

func parseInt(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("non-digit")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
