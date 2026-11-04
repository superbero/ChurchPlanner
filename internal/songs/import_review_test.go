package songs

import "testing"

func TestParsedImportSource(t *testing.T) {
	parsed := ParsedSong{
		Title:  "Grace",
		Author: "John Newton",
		Key:    "G",
		Tempo:  92,
		Lyrics: "Amazing grace\nHow sweet the sound",
		Chords: "C       G\nAm  F",
	}

	got := parsedImportSource(parsed)
	want := "{title: Grace}\n{artist: John Newton}\n{key: G}\n{tempo: 92}\n[C]Amazing [G]grace\n[Am]How [F]sweet the sound"
	if got != want {
		t.Fatalf("parsedImportSource() = %q, want %q", got, want)
	}
}

func TestChordLineToChordProPreservesUnicodeLyrics(t *testing.T) {
	got := chordLineToChordPro("Éternel est fidèle", "C       G")
	want := "[C]Éternel [G]est fidèle"
	if got != want {
		t.Fatalf("chordLineToChordPro() = %q, want %q", got, want)
	}
}
