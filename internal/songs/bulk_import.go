package songs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

const (
	maxBulkImportRows = 200
	maxBulkImportSize = 15 << 20

	bulkImportActionNew         = "new"
	bulkImportActionSkip        = "skip"
	bulkImportActionArrangement = "arrangement"
)

type bulkImportRecord struct {
	CSVRow        int    `json:"csv_row"`
	SourceName    string `json:"source_name"`
	Title         string `json:"title"`
	Author        string `json:"author"`
	Key           string `json:"key"`
	Tempo         int    `json:"tempo"`
	SourceContent string `json:"source_content"`
	ImportError   string `json:"import_error"`
}

type bulkImportPreviewRow struct {
	bulkImportRecord
	Errors    []string
	Duplicate bool
	Matches   []SongImportMatch
	Decision  string
}

type SongImportMatch struct {
	ID               string
	Title            string
	Author           string
	ExactTitleAuthor bool
	Archived         bool
}

type bulkImportDecision struct {
	Action     string
	TargetSong string
}

type bulkImportStore interface {
	FindBulkImportMatches(context.Context, []bulkImportRecord) (map[string][]SongImportMatch, error)
	ImportMany(context.Context, []bulkImportRecord, map[int]bulkImportDecision) (int, int, int, error)
}

var ErrBulkImportTargetNotFound = errors.New("bulk import target song no longer matches")

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
		writeImportRequestError(w, err, "The uploaded files could not be read.")
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["csv_file"]
	}
	if len(files) == 0 {
		h.renderBulkImport(w, r, http.StatusBadRequest, "Choose one or more song files to continue.", nil, "")
		return
	}
	if len(files) > maxBulkImportRows {
		h.renderBulkImport(w, r, http.StatusUnprocessableEntity,
			fmt.Sprintf("A bulk import can contain at most %d files or songs.", maxBulkImportRows),
			nil, "")
		return
	}
	csvCount := 0
	for _, file := range files {
		if strings.EqualFold(filepath.Ext(file.Filename), ".csv") {
			csvCount++
		}
	}
	if csvCount > 0 && (csvCount != 1 || len(files) != 1) {
		h.renderBulkImport(w, r, http.StatusUnprocessableEntity,
			"Upload a CSV by itself, or select multiple song files without a CSV.", nil, "")
		return
	}

	var records []bulkImportRecord
	var err error
	if csvCount == 1 {
		file, openErr := files[0].Open()
		if openErr != nil {
			httpx.ServerError(w, openErr)
			return
		}
		records, err = parseBulkImportCSV(file)
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		for i := range records {
			records[i].SourceName = filepath.Base(files[0].Filename)
		}
	} else {
		records = make([]bulkImportRecord, 0, len(files))
		for i, header := range files {
			file, openErr := header.Open()
			if openErr != nil {
				httpx.ServerError(w, openErr)
				return
			}
			rawBytes, readErr := io.ReadAll(file)
			closeErr := file.Close()
			if readErr != nil {
				httpx.ServerError(w, readErr)
				return
			}
			if closeErr != nil {
				httpx.ServerError(w, closeErr)
				return
			}
			parsed, sourceContent, _, parseErr := parseUploadedSong(header.Filename, rawBytes)
			title := strings.TrimSpace(parsed.Title)
			if title == "" {
				title = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
			}
			record := bulkImportRecord{
				CSVRow:        i + 1,
				SourceName:    filepath.Base(header.Filename),
				Title:         title,
				Author:        parsed.Author,
				Key:           parsed.Key,
				Tempo:         parsed.Tempo,
				SourceContent: sourceContent,
			}
			if parseErr != nil {
				record.ImportError = fmt.Sprintf("%s: %s", filepath.Base(header.Filename), parseErr)
			}
			records = append(records, record)
		}
	}
	if err != nil {
		h.renderBulkImport(w, r, http.StatusUnprocessableEntity, err.Error(), nil, "")
		return
	}
	if len(records) > maxBulkImportRows {
		h.renderBulkImport(w, r, http.StatusUnprocessableEntity,
			fmt.Sprintf("A bulk import can contain at most %d files or songs.", maxBulkImportRows),
			nil, "")
		return
	}
	rows := validateBulkImportRecords(records)
	matches, err := h.bulkImport.FindBulkImportMatches(r.Context(), records)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	for i := range rows {
		rows[i].Matches = matches[bulkImportMatchKey(rows[i].Title, rows[i].Author)]
		rows[i].Decision = bulkImportActionNew
		if len(rows[i].Matches) > 0 || rows[i].Duplicate {
			rows[i].Decision = bulkImportActionSkip
		}
	}
	payload, err := json.Marshal(records)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderBulkImport(w, r, http.StatusOK, "", rows, string(payload))
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
		h.renderBulkImport(w, r, http.StatusBadRequest, "The import preview expired or is invalid. Upload the CSV again.", nil, "")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		h.renderBulkImport(w, r, http.StatusBadRequest, "The import preview expired or is invalid. Upload the CSV again.", nil, "")
		return
	}
	toImport := make([]bulkImportRecord, 0, len(records))
	decisions := make(map[int]bulkImportDecision, len(records))
	seenRows := make(map[int]bool, len(records))
	for _, record := range records {
		if record.CSVRow < 1 || seenRows[record.CSVRow] {
			h.renderBulkImport(w, r, http.StatusBadRequest, "The import preview contains duplicate or invalid rows.", nil, "")
			return
		}
		seenRows[record.CSVRow] = true
		if err := validateBulkImportRecord(record); err != nil {
			continue
		}
		action := r.FormValue(fmt.Sprintf("decision_%d", record.CSVRow))
		targetSong := strings.TrimSpace(r.FormValue(fmt.Sprintf("target_%d", record.CSVRow)))
		if action != bulkImportActionNew && action != bulkImportActionSkip && action != bulkImportActionArrangement {
			h.renderBulkImport(w, r, http.StatusBadRequest, "Choose an import decision for every valid song.", nil, "")
			return
		}
		if action == bulkImportActionArrangement && targetSong == "" {
			h.renderBulkImport(w, r, http.StatusBadRequest, "Choose an existing song before adding an arrangement.", nil, "")
			return
		}
		toImport = append(toImport, record)
		decisions[record.CSVRow] = bulkImportDecision{Action: action, TargetSong: targetSong}
	}
	if len(toImport) == 0 {
		h.renderBulkImport(w, r, http.StatusBadRequest, "There are no valid songs to review or import.", nil, "")
		return
	}

	imported, arrangements, skipped, err := h.bulkImport.ImportMany(r.Context(), toImport, decisions)
	if err != nil {
		if errors.Is(err, ErrBulkImportTargetNotFound) {
			h.renderBulkImport(w, r, http.StatusConflict, "A selected existing song no longer matches this import. Upload the files again and review the updated matches.", nil, "")
			return
		}
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_bulk_import", map[string]any{
		"Imported":     imported,
		"Arrangements": arrangements,
		"Skipped":      skipped,
		"Complete":     true,
	})
}

func (h *Handler) renderBulkImport(w http.ResponseWriter, r *http.Request, status int, message string, rows []bulkImportPreviewRow, payload string) {
	validCount, invalidCount, duplicateCount := 0, 0, 0
	for i := range rows {
		switch {
		case len(rows[i].Errors) > 0:
			invalidCount++
		case rows[i].Duplicate || len(rows[i].Matches) > 0:
			duplicateCount++
		default:
			validCount++
		}
	}
	h.renderer.Render(w, r, status, "songs_bulk_import", map[string]any{
		"Error":          message,
		"Rows":           rows,
		"Payload":        payload,
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
	if record.ImportError != "" {
		return errors.New(record.ImportError)
	}
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

func bulkImportIdentityTitle(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

func bulkImportMatchKey(title, author string) string {
	return bulkImportIdentityTitle(title) + "\x00" + strings.ToLower(strings.TrimSpace(author))
}

func (r *Repository) FindBulkImportMatches(ctx context.Context, records []bulkImportRecord) (map[string][]SongImportMatch, error) {
	titles := make([]string, 0, len(records))
	authors := make([]string, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		title := bulkImportIdentityTitle(record.Title)
		author := strings.ToLower(strings.TrimSpace(record.Author))
		key := bulkImportMatchKey(title, author)
		if title == "" || seen[key] {
			continue
		}
		seen[key] = true
		titles = append(titles, title)
		authors = append(authors, author)
	}
	matches := make(map[string][]SongImportMatch, len(titles))
	if len(titles) == 0 {
		return matches, nil
	}
	rows, err := r.db.Query(ctx, `
		WITH requested AS (
			SELECT * FROM unnest($1::text[], $2::text[]) AS requested(title, author)
		), candidates AS (
			SELECT DISTINCT requested.title AS requested_title,
				requested.author AS source_author,
				s.id AS song_id, s.title AS song_title, s.author AS song_author,
				lower(btrim(s.author)) = requested.author AS exact_author,
				s.archived AS archived
			FROM requested
			JOIN songs s ON lower(btrim(s.title)) = requested.title
		)
		SELECT requested_title, source_author, song_id, song_title, song_author, exact_author, archived
		FROM (
			SELECT candidates.*,
				row_number() OVER (
					PARTITION BY requested_title, source_author
					ORDER BY exact_author DESC, song_author, song_id
				) AS match_position
			FROM candidates
		) ranked
		WHERE match_position <= 5
		ORDER BY requested_title, source_author, match_position
	`, titles, authors)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var title string
		var sourceAuthor string
		var match SongImportMatch
		if err := rows.Scan(&title, &sourceAuthor, &match.ID, &match.Title, &match.Author, &match.ExactTitleAuthor, &match.Archived); err != nil {
			return nil, err
		}
		matches[bulkImportMatchKey(title, sourceAuthor)] = append(matches[bulkImportMatchKey(title, sourceAuthor)], match)
	}
	return matches, rows.Err()
}

func (r *Repository) ImportMany(ctx context.Context, records []bulkImportRecord, decisions map[int]bulkImportDecision) (imported, arrangements, skipped int, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback(ctx)

	for _, record := range records {
		if err := validateBulkImportRecord(record); err != nil {
			return 0, 0, 0, fmt.Errorf("CSV record %d: %w", record.CSVRow, err)
		}
		decision, ok := decisions[record.CSVRow]
		if !ok {
			return 0, 0, 0, fmt.Errorf("CSV record %d: import decision is required", record.CSVRow)
		}
		switch decision.Action {
		case bulkImportActionSkip:
			skipped++
			continue
		case bulkImportActionNew:
			var songID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO songs (title, author)
				VALUES ($1, $2)
				RETURNING id
			`, strings.TrimSpace(record.Title), strings.TrimSpace(record.Author)).Scan(&songID); err != nil {
				return 0, 0, 0, err
			}
			if err := insertBulkImportArrangement(ctx, tx, songID, record, true); err != nil {
				return 0, 0, 0, err
			}
			imported++
		case bulkImportActionArrangement:
			var songID string
			err := tx.QueryRow(ctx, `
				SELECT id FROM songs
				WHERE id = $1 AND lower(btrim(title)) = $2
				FOR UPDATE
			`, decision.TargetSong, bulkImportIdentityTitle(record.Title)).Scan(&songID)
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, 0, 0, ErrBulkImportTargetNotFound
			}
			if err != nil {
				return 0, 0, 0, err
			}
			var hasDefault bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM song_arrangements
					WHERE song_id = $1 AND is_default = TRUE
				)
			`, songID).Scan(&hasDefault); err != nil {
				return 0, 0, 0, err
			}
			if err := insertBulkImportArrangement(ctx, tx, songID, record, !hasDefault); err != nil {
				return 0, 0, 0, err
			}
			arrangements++
		default:
			return 0, 0, 0, fmt.Errorf("CSV record %d: invalid import decision", record.CSVRow)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, err
	}
	return imported, arrangements, skipped, nil
}

func insertBulkImportArrangement(ctx context.Context, tx pgx.Tx, songID string, record bulkImportRecord, isDefault bool) error {
	lyrics, chords := parseChart(record.SourceContent, "chordpro")
	name := "Imported arrangement"
	if isDefault {
		name = "Original"
	} else {
		name = fmt.Sprintf("Imported arrangement (row %d)", record.CSVRow)
	}
	var arrangementID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO song_arrangements (
			song_id, name, bpm, original_key, source_content, format,
			primary_language, lyrics, chords, is_default
		) VALUES ($1, $2, $3, $4, $5, 'chordpro', 'en', $6, $7, $8)
		RETURNING id
	`, songID, name, record.Tempo, strings.TrimSpace(record.Key), record.SourceContent,
		lyrics, chords, isDefault).Scan(&arrangementID); err != nil {
		return err
	}
	if strings.TrimSpace(record.Key) != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO arrangement_keys (arrangement_id, name, key, is_default)
			VALUES ($1, 'Original', $2, TRUE)
		`, arrangementID, strings.TrimSpace(record.Key)); err != nil {
			return err
		}
	}
	return nil
}
