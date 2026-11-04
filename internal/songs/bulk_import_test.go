package songs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	completed := render(map[string]any{"Complete": true, "Imported": 2, "Skipped": 1})
	for _, expected := range []string{"Import complete", "2 songs were added", "1 duplicate row was skipped"} {
		if !strings.Contains(completed, expected) {
			t.Errorf("bulk import completion is missing %q", expected)
		}
	}
}
