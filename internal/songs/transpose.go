package songs

import (
	"regexp"
	"strings"
)

var noteIndex = map[string]int{
	"C": 0, "B#": 0,
	"C#": 1, "Db": 1,
	"D": 2,
	"D#": 3, "Eb": 3,
	"E": 4, "Fb": 4,
	"F": 5, "E#": 5,
	"F#": 6, "Gb": 6,
	"G": 7,
	"G#": 8, "Ab": 8,
	"A": 9,
	"A#": 10, "Bb": 10,
	"B": 11, "Cb": 11,
}

var sharpNotes = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
var flatNotes = []string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}

var chordRE = regexp.MustCompile(`^([A-G][#b]?)(.*?)(?:/([A-G][#b]?))?$`)

func keyUsesFlats(key string) bool {
	minor := strings.HasSuffix(key, "m")
	root := strings.TrimSuffix(key, "m")
	if minor {
		return map[string]bool{
			"D": true, "G": true, "C": true, "F": true,
			"Bb": true, "Eb": true, "Ab": true,
		}[root]
	}
	return map[string]bool{
		"F": true, "Bb": true, "Eb": true, "Ab": true,
		"Db": true, "Gb": true, "Cb": true,
	}[root]
}

func KeyIndex(key string) (int, bool) {
	root := strings.TrimSuffix(key, "m")
	idx, ok := noteIndex[root]
	return idx, ok
}

// TransposeKey shifts a musical key by N semitones and picks the right accidentals.
func TransposeKey(key string, semitones int) string {
	if key == "" {
		return key
	}
	minor := strings.HasSuffix(key, "m")
	root := strings.TrimSuffix(key, "m")
	idx, ok := noteIndex[root]
	if !ok {
		return key
	}
	newIdx := ((idx+semitones)%12 + 12) % 12

	notes := sharpNotes
	tmp := notes[newIdx]
	if minor {
		tmp += "m"
	}
	if keyUsesFlats(tmp) {
		notes = flatNotes
	}
	out := notes[newIdx]
	if minor {
		out += "m"
	}
	return out
}

// TransposeChord transposes a single chord symbol (root, quality, optional /bass).
func TransposeChord(chord string, semitones int, flats bool) string {
	m := chordRE.FindStringSubmatch(strings.TrimSpace(chord))
	if m == nil {
		return chord
	}
	root, quality, bass := m[1], m[2], m[3]
	idx, ok := noteIndex[root]
	if !ok {
		return chord
	}
	newIdx := ((idx+semitones)%12 + 12) % 12
	notes := sharpNotes
	if flats {
		notes = flatNotes
	}
	out := notes[newIdx] + quality
	if bass != "" {
		if bi, ok := noteIndex[bass]; ok {
			nbi := ((bi+semitones)%12 + 12) % 12
			out += "/" + notes[nbi]
		} else {
			out += "/" + bass
		}
	}
	return out
}

// TransposeChordProText rewrites every [Chord] token in a text.
func TransposeChordProText(text string, semitones int, flats bool) string {
	return chordLineRE.ReplaceAllStringFunc(text, func(match string) string {
		inner := strings.TrimSuffix(strings.TrimPrefix(match, "["), "]")
		return "[" + TransposeChord(inner, semitones, flats) + "]"
	})
}

// ParseChartLine splits a ChordPro line into chord positions and plain lyrics.
// NOTE: ChartLine and ChordPos types live in model.go.
func ParseChartLine(line string) (string, []ChordPos) {
	var buf strings.Builder
	var chords []ChordPos
	pos, i := 0, 0
	for i < len(line) {
		if line[i] == '[' {
			if j := strings.IndexByte(line[i:], ']'); j > 0 {
				chords = append(chords, ChordPos{Position: pos, Chord: line[i+1 : i+j]})
				i += j + 1
				continue
			}
		}
		buf.WriteByte(line[i])
		pos++
		i++
	}
	return buf.String(), chords
}
