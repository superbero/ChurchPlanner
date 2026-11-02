package songs

import (
	"encoding/xml"
	"errors"
	"strings"
)

type openlyricsXML struct {
	XMLName xml.Name `xml:"song"`
	Properties struct {
		Titles struct {
			Title []string `xml:"title"`
		} `xml:"titles"`
		Authors struct {
			Author []string `xml:"author"`
		} `xml:"authors"`
		Key   string `xml:"key"`
		Tempo string `xml:"tempo"`
	} `xml:"properties"`
	Lyrics struct {
		Verses []struct {
			Name  string `xml:"name,attr"`
			Lines struct {
				Line []string `xml:"line"`
			} `xml:"lines"`
		} `xml:"verse"`
	} `xml:"lyrics"`
}

// ParseOpenLyrics reads the OpenLyrics 0.8+ XML standard.
// OpenLyrics is lyric-focused; chords are usually absent, so Chords is empty.
func ParseOpenLyrics(input string) (ParsedSong, error) {
	var root openlyricsXML
	if err := xml.Unmarshal([]byte(input), &root); err != nil {
		return ParsedSong{}, err
	}
	if len(root.Properties.Titles.Title) == 0 {
		return ParsedSong{}, errors.New("openlyrics: missing <title>")
	}

	var lyricLines, chordLines []string
	for _, v := range root.Lyrics.Verses {
		for _, line := range v.Lines.Line {
			lyricLines = append(lyricLines, line)
			chordLines = append(chordLines, "")
		}
		// Blank line between verses
		lyricLines = append(lyricLines, "")
		chordLines = append(chordLines, "")
	}

	tempo := 0
	if n := digitsOnly(root.Properties.Tempo); n != "" {
		tempo, _ = parseInt(n)
	}

	return ParsedSong{
		Title:  strings.TrimSpace(root.Properties.Titles.Title[0]),
		Author: strings.TrimSpace(strings.Join(root.Properties.Authors.Author, ", ")),
		Key:    strings.TrimSpace(root.Properties.Key),
		Tempo:  tempo,
		Lyrics: strings.TrimSpace(strings.Join(lyricLines, "\n")),
		Chords: strings.TrimSpace(strings.Join(chordLines, "\n")),
		Format: "openlyrics",
		Source: input,
	}, nil
}
