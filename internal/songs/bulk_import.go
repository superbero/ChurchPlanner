package songs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

const (
	maxBulkImportRows = 200
	maxBulkImportSize = 15 << 20
)

type bulkImportRecord struct {
	CSVRow        int    `json:"csv_row"`
	Title         string `json:"title"`
	Author        string `json:"author"`
	Key           string `json:"key"`
	Tempo         int    `json:"tempo"`
	SourceContent string `json:"source_content"`
}

type bulkImportPreviewRow struct {
	bulkImportRecord
	Errors    []string
	Duplicate bool
	Selected  bool
}

func (h *Handler) ShowBulkImport(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, r, http.StatusOK, "songs_bulk_import", nil)
}

func (h *Handler) DownloadBulkImportTemplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="song-library-import-template.csv"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"title", "author", "key", "tempo", "source_content"})
	_ = writer.Write([]string{
		"Amazing Grace", "John Newton", "G", "90",
		"{title: Amazing Grace}\n{artist: John Newton}\n{key: G}\n{tempo: 90}\n[G]Amazing grace, how sweet the sound",
	})
	writer.Flush()
	if err := writer.Error(); err != nil {
		httpx.ServerError(w, err)
	}
}

func (h *Handler) PreviewBulkImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBulkImportSize)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeImportRequestError(w, err, "The CSV file could not be read.")
		return
	}
	file, _, err := r.FormFile("csv_file")
	if err != nil {
		h.renderBulkImport(w, r, http.StatusBadRequest, "Choose a CSV file to continue.", nil, "", nil)
		return
	}
	defer file.Close()

	records, err := parseBulkImportCSV(file)
	if err != nil {
		h.renderBulkImport(w, r, http.StatusUnprocessableEntity, err.Error(), nil, "", nil)
		return
	}
	rows := validateBulkImportRecords(records)
	payload, err := json.Marshal(records)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	var selected []int
	for _, row := range rows {
		if len(row.Errors) == 0 && !row.Duplicate {
			selected = append(selected, row.CSVRow)
		}
	}
	h.renderBulkImport(w, r, http.StatusOK, "", rows, string(payload), selected)
}

func (h *Handler) SaveBulkImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBulkImportSize)
	if err := r.ParseForm(); err != nil {
		writeImportRequestError(w, err, "The selected songs could not be read.")
		return
	}

	var records []bulkImportRecord
	decoder := json.NewDecoder(strings.NewReader(r.FormValue("payload")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&records); err != nil || len(records) > maxBulkImportRows {
		h.renderBulkImport(w, r, http.StatusBadRequest, "The import preview expired or is invalid. Upload the CSV again.", nil, "", nil)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		h.renderBulkImport(w, r, http.StatusBadRequest, "The import preview expired or is invalid. Upload the CSV again.", nil, "", nil)
		return
	}
	selected := make(map[int]bool)
	for _, raw := range r.Form["selected_rows"] {
		index, err := strconv.Atoi(raw)
		if err != nil || index < 1 {
			h.renderBulkImport(w, r, http.StatusBadRequest, "One or more selected rows are invalid.", nil, "", nil)
			return
		}
		selected[index] = true
	}
	if len(selected) == 0 {
		h.renderBulkImport(w, r, http.StatusBadRequest, "Select at least one valid song to import.", nil, "", nil)
		return
	}

	toImport := make([]bulkImportRecord, 0, len(selected))
	seenRows := make(map[int]bool, len(records))
	for _, record := range records {
		if record.CSVRow < 1 || seenRows[record.CSVRow] {
			h.renderBulkImport(w, r, http.StatusBadRequest, "The import preview contains duplicate or invalid rows.", nil, "", nil)
			return
		}
		seenRows[record.CSVRow] = true
		if !selected[record.CSVRow] {
			continue
		}
		if err := validateBulkImportRecord(record); err != nil {
			h.renderBulkImport(w, r, http.StatusBadRequest, "A selected row is invalid. Upload the CSV and review it again.", nil, "", nil)
			return
		}
		toImport = append(toImport, record)
	}
	for row := range selected {
		if !seenRows[row] {
			h.renderBulkImport(w, r, http.StatusBadRequest, "A selected row is missing from the import preview.", nil, "", nil)
			return
		}
	}

	imported, skipped, err := h.svc.ImportMany(r.Context(), toImport)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_bulk_import", map[string]any{
		"Imported": imported,
		"Skipped":  skipped,
		"Complete": true,
	})
}

func (h *Handler) renderBulkImport(w http.ResponseWriter, r *http.Request, status int, message string, rows []bulkImportPreviewRow, payload string, selected []int) {
	validCount, invalidCount, duplicateCount := 0, 0, 0
	selectedSet := make(map[int]bool, len(selected))
	for _, row := range selected {
		selectedSet[row] = true
	}
	for i := range rows {
		rows[i].Selected = selectedSet[rows[i].CSVRow]
		switch {
		case len(rows[i].Errors) > 0:
			invalidCount++
		case rows[i].Duplicate:
			duplicateCount++
		default:
			validCount++
		}
	}
	h.renderer.Render(w, r, status, "songs_bulk_import", map[string]any{
		"Error":          message,
		"Rows":           rows,
		"Payload":        payload,
		"SelectedRows":   selected,
		"ValidCount":     validCount,
		"InvalidCount":   invalidCount,
		"DuplicateCount": duplicateCount,
	})
}

func parseBulkImportCSV(input io.Reader) ([]bulkImportRecord, error) {
	reader := csv.NewReader(input)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return nil, errors.New("The CSV file is empty or could not be read.")
	}
	columns := make(map[string]int, len(header))
	for i, name := range header {
		name = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))
		columns[name] = i
	}
	titleColumn, hasTitle := columns["title"]
	contentColumn, hasContent := columns["source_content"]
	if !hasContent {
		contentColumn, hasContent = columns["chordpro"]
	}
	if !hasContent {
		contentColumn, hasContent = columns["lyrics"]
	}
	if !hasTitle || !hasContent {
		return nil, errors.New("CSV headers must include title and one content column: source_content, chordpro, or lyrics.")
	}

	var records []bulkImportRecord
	recordNumber := 1
	for {
		values, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Could not read CSV record %d: %w", recordNumber+1, err)
		}
		recordNumber++
		if len(values) == 0 || allCSVFieldsBlank(values) {
			continue
		}
		if len(records) >= maxBulkImportRows {
			return nil, fmt.Errorf("A bulk import can contain at most %d songs.", maxBulkImportRows)
		}
		value := func(column string) string {
			index, ok := columns[column]
			if !ok || index >= len(values) {
				return ""
			}
			return strings.TrimSpace(values[index])
		}
		if titleColumn >= len(values) || contentColumn >= len(values) {
			return nil, fmt.Errorf("CSV record %d is missing required columns.", recordNumber)
		}
		content := strings.TrimSpace(values[contentColumn])
		if _, ok := columns["source_content"]; !ok {
			if _, chordproColumn := columns["chordpro"]; chordproColumn {
				content = strings.TrimSpace(values[columns["chordpro"]])
			} else {
				content = strings.TrimSpace(values[columns["lyrics"]])
			}
		}
		tempo := 0
		if rawTempo := value("tempo"); rawTempo != "" {
			tempo, err = strconv.Atoi(rawTempo)
			if err != nil {
				tempo = -1
			}
		}
		records = append(records, bulkImportRecord{
			CSVRow:        recordNumber,
			Title:         strings.TrimSpace(values[titleColumn]),
			Author:        value("author"),
			Key:           value("key"),
			Tempo:         tempo,
			SourceContent: content,
		})
	}
	if len(records) == 0 {
		return nil, errors.New("The CSV contains a header but no song rows.")
	}
	return records, nil
}

func allCSVFieldsBlank(fields []string) bool {
	for _, field := range fields {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func validateBulkImportRecord(record bulkImportRecord) error {
	if strings.TrimSpace(record.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(record.SourceContent) == "" {
		return errors.New("source content is required")
	}
	if record.Tempo < 0 || record.Tempo > 400 {
		return errors.New("tempo must be between 0 and 400")
	}
	if key := strings.TrimSpace(record.Key); key != "" {
		if _, ok := KeyIndex(key); !ok {
			return errors.New("key must be a valid musical key")
		}
	}
	if len(record.Title) > 300 {
		return errors.New("title must be 300 characters or fewer")
	}
	return nil
}

func validateBulkImportRecords(records []bulkImportRecord) []bulkImportPreviewRow {
	seen := make(map[string]bool, len(records))
	rows := make([]bulkImportPreviewRow, 0, len(records))
	for _, record := range records {
		row := bulkImportPreviewRow{bulkImportRecord: record}
		if err := validateBulkImportRecord(record); err != nil {
			row.Errors = append(row.Errors, err.Error())
		}
		identity := strings.ToLower(strings.TrimSpace(record.Title)) + "\x00" +
			strings.ToLower(strings.TrimSpace(record.Author))
		if len(row.Errors) == 0 && row.Title != "" {
			if seen[identity] {
				row.Duplicate = true
			} else {
				seen[identity] = true
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (r *Repository) ImportMany(ctx context.Context, records []bulkImportRecord) (imported, skipped int, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if err := validateBulkImportRecord(record); err != nil {
			return 0, 0, fmt.Errorf("CSV record %d: %w", record.CSVRow, err)
		}
		identity := strings.ToLower(strings.TrimSpace(record.Title)) + "\x00" +
			strings.ToLower(strings.TrimSpace(record.Author))
		if seen[identity] {
			skipped++
			continue
		}
		seen[identity] = true

		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM songs
				WHERE lower(btrim(title)) = lower(btrim($1))
				  AND lower(btrim(author)) = lower(btrim($2))
			)
		`, record.Title, record.Author).Scan(&exists); err != nil {
			return 0, 0, err
		}
		if exists {
			skipped++
			continue
		}

		var songID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO songs (title, author)
			VALUES ($1, $2)
			RETURNING id
		`, strings.TrimSpace(record.Title), strings.TrimSpace(record.Author)).Scan(&songID); err != nil {
			return 0, 0, err
		}
		lyrics, chords := parseChart(record.SourceContent, "chordpro")
		var arrangementID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO song_arrangements (
				song_id, name, bpm, original_key, source_content, format,
				primary_language, lyrics, chords, is_default
			) VALUES ($1, 'Original', $2, $3, $4, 'chordpro', 'en', $5, $6, TRUE)
			RETURNING id
		`, songID, record.Tempo, strings.TrimSpace(record.Key), record.SourceContent,
			lyrics, chords).Scan(&arrangementID); err != nil {
			return 0, 0, err
		}
		if strings.TrimSpace(record.Key) != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO arrangement_keys (arrangement_id, name, key, is_default)
				VALUES ($1, 'Original', $2, TRUE)
			`, arrangementID, strings.TrimSpace(record.Key)); err != nil {
				return 0, 0, err
			}
		}
		imported++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return imported, skipped, nil
}

func (s *Service) ImportMany(ctx context.Context, records []bulkImportRecord) (int, int, error) {
	return s.repo.ImportMany(ctx, records)
}
