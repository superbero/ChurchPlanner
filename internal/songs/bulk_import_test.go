package songs

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
)

func TestParseBulkImportCSVSupportsChordProAliasAndMultilineContent(t *testing.T) {
	input := "title,author,key,tempo,chordpro\r\n" +
		"Test Song,Test Artist,G,92,\"[G]First line\r\n" +
		"[C]Second line\"\r\n"

	records, err := parseBulkImportCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseBulkImportCSV() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Title != "Test Song" || record.Author != "Test Artist" ||
		record.Key != "G" || record.Tempo != 92 ||
		record.SourceContent != "[G]First line\n[C]Second line" {
		t.Fatalf("parsed record = %#v", record)
	}
}

func TestParseBulkImportCSVReportsInvalidTempoAndRowLimit(t *testing.T) {
	records, err := parseBulkImportCSV(strings.NewReader(
		"title,lyrics,tempo\nTest Song,Words,not-a-number\n",
	))
	if err != nil {
		t.Fatalf("parseBulkImportCSV() error = %v", err)
	}
	if err := validateBulkImportRecord(records[0]); err == nil {
		t.Fatal("invalid tempo passed record validation")
	}

	var input strings.Builder
	input.WriteString("title,lyrics\n")
	for i := 0; i <= maxBulkImportRows; i++ {
		input.WriteString("Song,Words\n")
	}
	if _, err := parseBulkImportCSV(strings.NewReader(input.String())); err == nil {
		t.Fatal("CSV over the row limit was accepted")
	}
}

func TestParseUploadedSongSupportsSingleSongImportFormats(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		content     string
		wantTitle   string
		wantAuthor  string
		wantKey     string
		wantContent string
	}{
		{
			name:        "ChordPro",
			filename:    "song.cho",
			content:     "{title: Grace}\n{artist: Newton}\n{key: G}\n{tempo: 90}\n[G]Amazing grace",
			wantTitle:   "Grace",
			wantAuthor:  "Newton",
			wantKey:     "G",
			wantContent: "{title: Grace}\n{artist: Newton}\n{key: G}\n{tempo: 90}\n[G]Amazing grace",
		},
		{
			name:        "plain text",
			filename:    "song.txt",
			content:     "Amazing grace, how sweet the sound",
			wantContent: "Amazing grace, how sweet the sound",
		},
		{
			name:        "OpenSong",
			filename:    "song.xml",
			content:     `<song><title>Grace</title><author>Newton</author><key>G</key><tempo>90</tempo><lyrics>.G` + "\n" + `Amazing grace</lyrics></song>`,
			wantTitle:   "Grace",
			wantAuthor:  "Newton",
			wantKey:     "G",
			wantContent: "{title: Grace}\n{artist: Newton}\n{key: G}\n{tempo: 90}\n[G]Amazing grace",
		},
		{
			name:        "OpenLyrics",
			filename:    "song.xml",
			content:     `<song xmlns="http://openlyrics.info/namespace/2009/song"><properties><titles><title>Grace</title></titles><authors><author>Newton</author></authors><key>G</key><tempo>90</tempo></properties><lyrics><verse><lines><line>Amazing grace</line></lines></verse></lyrics></song>`,
			wantTitle:   "Grace",
			wantAuthor:  "Newton",
			wantKey:     "G",
			wantContent: "{title: Grace}\n{artist: Newton}\n{key: G}\n{tempo: 90}\nAmazing grace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, inferred, rawText, err := parseUploadedSong(tt.filename, []byte(tt.content))
			if err != nil {
				t.Fatalf("parseUploadedSong() error = %v", err)
			}
			if parsed.Title != tt.wantTitle || parsed.Author != tt.wantAuthor ||
				parsed.Key != tt.wantKey || inferred != tt.wantContent {
				t.Errorf("parsed = %#v, inferred = %q", parsed, inferred)
			}
			if rawText != tt.content {
				t.Errorf("raw text = %q, want %q", rawText, tt.content)
			}
		})
	}

	if _, _, _, err := parseUploadedSong("song.docx", []byte("unsupported")); err == nil {
		t.Fatal("unsupported file type was accepted")
	}
}

func TestParseUploadedSongExtractsPDFText(t *testing.T) {
	document := fpdf.New("P", "mm", "A4", "")
	document.AddPage()
	document.SetFont("Arial", "", 12)
	document.Cell(40, 10, "Amazing Grace")
	var content bytes.Buffer
	if err := document.Output(&content); err != nil {
		t.Fatalf("create test PDF: %v", err)
	}

	_, inferred, rawText, err := parseUploadedSong("song.pdf", content.Bytes())
	if err != nil {
		t.Fatalf("parseUploadedSong() error = %v", err)
	}
	if !strings.Contains(rawText, "Amazing Grace") || !strings.Contains(inferred, "Amazing Grace") {
		t.Fatalf("extracted PDF text = %q; ChordPro = %q", rawText, inferred)
	}
}

func TestBulkImportTemplateRendersSelectedRowsAndCompletion(t *testing.T) {
	renderer := newSongTemplateRenderer(t)
	render := func(data map[string]any) string {
		t.Helper()
		response := httptest.NewRecorder()
		renderer.Render(response, httptest.NewRequest(http.MethodGet, "/songs/import/bulk", nil),
			http.StatusOK, "songs_bulk_import", data)
		if response.Code != http.StatusOK {
			t.Fatalf("render status = %d, want %d", response.Code, http.StatusOK)
		}
		return response.Body.String()
	}

	preview := render(map[string]any{
		"Rows": []bulkImportPreviewRow{{
			bulkImportRecord: bulkImportRecord{
				CSVRow: 2, Title: "Test song", SourceContent: "[G]Test lyrics",
			},
			Selected: true,
		}},
		"Payload":      `[{"csv_row":2,"title":"Test song","source_content":"[G]Test lyrics"}]`,
		"ValidCount":   1,
		"SelectedRows": []int{2},
	})
	for _, expected := range []string{
		"Test song", "Import selected songs", `value="2" checked`,
		`name="payload"`, "[G]Test lyrics",
	} {
		if !strings.Contains(preview, expected) {
			t.Errorf("bulk import preview is missing %q", expected)
		}
	}
	upload := render(map[string]any{})
	for _, expected := range []string{`name="files" multiple`, ".pdf", ".chordpro", ".xml", "song list"} {
		if !strings.Contains(upload, expected) {
			t.Errorf("bulk upload form is missing %q", expected)
		}
	}

	completed := render(map[string]any{"Complete": true, "Imported": 2, "Skipped": 1})
	for _, expected := range []string{"Import complete", "2 songs were added", "1 duplicate row was skipped"} {
		if !strings.Contains(completed, expected) {
			t.Errorf("bulk import completion is missing %q", expected)
		}
	}
}

func TestPreviewBulkImportAcceptsMultipleSongFiles(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for filename, content := range map[string]string{
		"song.chordpro": "{title: Grace}\n{artist: Newton}\n[G]Amazing grace",
		"plain.txt":     "A plain song",
		"bad.docx":      "not a supported import type",
	} {
		part, err := writer.CreateFormFile("files", filename)
		if err != nil {
			t.Fatalf("CreateFormFile() error = %v", err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatalf("write upload part: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart writer close: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/songs/import/bulk/preview", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler := &Handler{renderer: newSongTemplateRenderer(t)}
	handler.PreviewBulkImport(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	for _, expected := range []string{
		"Grace", "Newton", "plain", "song.chordpro", "plain.txt", "bad.docx",
		"3 songs found", "Unsupported file type",
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("bulk preview response is missing %q", expected)
		}
	}
}
