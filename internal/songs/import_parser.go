package songs

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

var errUnsupportedImportFile = errors.New("Unsupported file type. Upload a PDF, ChordPro, OpenSong/OpenLyrics XML, or plain-text file.")

func parseUploadedSong(filename string, rawBytes []byte) (ParsedSong, string, string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	var rawText string
	switch ext {
	case ".pdf":
		pdfReader, err := pdf.NewReader(bytes.NewReader(rawBytes), int64(len(rawBytes)))
		if err != nil {
			return ParsedSong{}, "", "", errors.New("Could not read PDF file.")
		}
		text, err := pdfReader.GetPlainText()
		if err != nil {
			return ParsedSong{}, "", "", errors.New("Could not extract text from PDF.")
		}
		var textBuf bytes.Buffer
		if _, err := textBuf.ReadFrom(text); err != nil {
			return ParsedSong{}, "", "", fmt.Errorf("Could not extract text from PDF: %w", err)
		}
		rawText = textBuf.String()
	case ".cho", ".pro", ".chordpro", ".txt", ".xml":
		rawText = string(rawBytes)
	default:
		return ParsedSong{}, "", "", errUnsupportedImportFile
	}

	parsed, inferred, err := parseImportText(filename, rawText)
	return parsed, inferred, rawText, err
}

func parseImportText(filename, rawText string) (ParsedSong, string, error) {
	format := DetectFormat(filename, rawText)
	var parsed ParsedSong
	var inferred string
	var err error
	switch format {
	case "opensong":
		parsed, err = ParseOpenSong(rawText)
		if err == nil {
			inferred = parsedImportSource(parsed)
		}
	case "openlyrics":
		parsed, err = ParseOpenLyrics(rawText)
		if err == nil {
			inferred = parsedImportSource(parsed)
		}
	case "chordpro":
		parsed = ParseChordPro(rawText)
		inferred = rawText
	default:
		inferred = InferChordPro(rawText)
		parsed = ParseChordPro(inferred)
	}
	if err != nil {
		return ParsedSong{}, "", errors.New("Could not read this XML as a supported OpenSong or OpenLyrics song.")
	}
	return parsed, inferred, nil
}
