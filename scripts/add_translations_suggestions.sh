#!/usr/bin/env bash
# Translations (EN↔FR + others) and song suggestions.
set -euo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }
[ -d internal/songs ] || { echo "✗ internal/songs/ missing"; exit 1; }
if ls migrations/00007_* >/dev/null 2>&1; then
  echo "✗ migrations/00007_* already exists — aborting"; exit 1
fi

echo "▸ [1/17] Migration 00007_translations.sql"
cat > migrations/00007_translations.sql <<'SQLEOF'
-- +goose Up
ALTER TABLE song_arrangements
    ADD COLUMN primary_language TEXT NOT NULL DEFAULT 'en';

CREATE TABLE arrangement_translations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    arrangement_id  UUID NOT NULL REFERENCES song_arrangements(id) ON DELETE CASCADE,
    language        TEXT NOT NULL,
    lyrics          TEXT NOT NULL DEFAULT '',
    chords          TEXT NOT NULL DEFAULT '',
    source_content  TEXT NOT NULL DEFAULT '',
    format          TEXT NOT NULL DEFAULT 'chordpro',
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (arrangement_id, language)
);

CREATE INDEX arrangement_translations_arr_idx ON arrangement_translations (arrangement_id);

-- +goose Down
DROP TABLE IF EXISTS arrangement_translations;
ALTER TABLE song_arrangements DROP COLUMN IF EXISTS primary_language;
SQLEOF

echo "▸ [2/17] Config: translator settings"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/platform/config/config.go")
src = p.read_text()
if "TranslateAPIURL" in src:
    print("   (already present)")
else:
    src = src.replace(
"""type Config struct {
	Port          string
	DatabaseURL   string
	Env           string
	PublicBaseURL string // e.g. https://church.example.com, used for practice links/QR
}""",
"""type Config struct {
	Port            string
	DatabaseURL     string
	Env             string
	PublicBaseURL   string // e.g. https://church.example.com, used for practice links/QR
	TranslateAPIURL string // empty = translations must be pasted manually
	TranslateAPIKey string // optional, passed through to the endpoint
	PrimaryLanguage string // church's primary language, default "en"
}""")
    src = src.replace(
"""		DatabaseURL:   os.Getenv("DATABASE_URL"),
		Env:           getenv("APP_ENV", "development"),
		PublicBaseURL: os.Getenv("PUBLIC_BASE_URL"),""",
"""		DatabaseURL:     os.Getenv("DATABASE_URL"),
		Env:             getenv("APP_ENV", "development"),
		PublicBaseURL:   os.Getenv("PUBLIC_BASE_URL"),
		TranslateAPIURL: os.Getenv("TRANSLATE_API_URL"),
		TranslateAPIKey: os.Getenv("TRANSLATE_API_KEY"),
		PrimaryLanguage: getenv("PRIMARY_LANGUAGE", "en"),""")
    p.write_text(src)
    print("   ✓ config updated")
PYEOF

if ! grep -q '^TRANSLATE_API_URL' deploy/.env.example; then
    cat >> deploy/.env.example <<'ENVEOF'
# Optional. Empty = translations must be pasted manually.
# Self-hosted LibreTranslate: docker run -p 5000:5000 libretranslate/libretranslate
TRANSLATE_API_URL=
TRANSLATE_API_KEY=
PRIMARY_LANGUAGE=en
ENVEOF
fi

echo "▸ [3/17] httpx render: language helpers in FuncMap"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/platform/httpx/render.go")
src = p.read_text()

if "languageLabel" in src:
    print("   (already present)")
    raise SystemExit(0)

# Insert helpers before funcMap
old = '''// funcMap provides helper functions available in all templates.'''
new = '''type LanguageOption struct {
	Code  string
	Label string
}

var languageLabels = map[string]string{
	"en": "English",
	"fr": "Français",
	"es": "Español",
	"pt": "Português",
	"de": "Deutsch",
	"it": "Italiano",
	"nl": "Nederlands",
	"sw": "Kiswahili",
	"zh": "中文",
	"ko": "한국어",
}

var languageOrder = []string{"en", "fr", "es", "pt", "de", "it", "nl", "sw", "zh", "ko"}

func languageLabel(code string) string {
	if code == "" {
		return ""
	}
	if l, ok := languageLabels[code]; ok {
		return l
	}
	return strings.ToUpper(code)
}

func languageOptions() []LanguageOption {
	out := make([]LanguageOption, 0, len(languageOrder))
	for _, c := range languageOrder {
		out = append(out, LanguageOption{Code: c, Label: languageLabels[c]})
	}
	return out
}

// funcMap provides helper functions available in all templates.'''
if old not in src:
    print("   ! funcMap marker not found")
    raise SystemExit(1)
src = src.replace(old, new, 1)

# Add to FuncMap
old = '''		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
	}
}'''
new = '''		"lower":         strings.ToLower,
		"upper":         strings.ToUpper,
		"languageLabel": languageLabel,
		"languageOptions": languageOptions,
	}
}'''
if old not in src:
    print("   ! FuncMap body not found")
    raise SystemExit(1)
src = src.replace(old, new, 1)

p.write_text(src)
print("   ✓ helpers added")
PYEOF

echo "▸ [4/17] model.go: Arrangement.PrimaryLanguage + ArrangementTranslation"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/model.go")
src = p.read_text()

src = src.replace(
'''	OriginalKey    string    `json:"original_key"`
	SourceContent  string    `json:"source_content"`
	Format         string    `json:"format"`
	Lyrics         string    `json:"lyrics"`
	Chords         string    `json:"chords"`''',
'''	OriginalKey    string    `json:"original_key"`
	SourceContent  string    `json:"source_content"`
	Format         string    `json:"format"`
	PrimaryLanguage string   `json:"primary_language"`
	Lyrics         string    `json:"lyrics"`
	Chords         string    `json:"chords"`''')

src = src.replace(
'''	OriginalKey   string
	SourceContent string
	Format        string
	Archived      bool''',
'''	OriginalKey   string
	SourceContent string
	Format        string
	PrimaryLanguage string
	Archived      bool''')

# Append translation + suggestion types at end
src += '''

type ArrangementTranslation struct {
	ID            string
	ArrangementID string
	Language      string
	Lyrics        string
	Chords        string
	SourceContent string
	Format        string
	Notes         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Suggestion struct {
	SongID          string
	SongTitle       string
	SongAuthor      string
	ArrangementID   string
	ArrangementName string
	Key             string
	Score           float64
	Reason          string
	LastUsedDays    int
	Uses3m          int
}
'''

p.write_text(src)
print("   ✓ model updated")
PYEOF

echo "▸ [5/17] chart.go: BuildChart takes optional translation"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/chart.go")
src = p.read_text()

old = '''func BuildChart(a *Arrangement, displayKey string) []RenderedSection {
	if a == nil {
		return nil
	}

	source := a.SourceContent
	lyrics := a.Lyrics
	chords := a.Chords

	// If source is present, reparse so section directives are respected.
	var sections []Section
	switch a.Format {'''
new = '''func BuildChart(a *Arrangement, translation *ArrangementTranslation, displayKey string) []RenderedSection {
	if a == nil {
		return nil
	}

	source := a.SourceContent
	lyrics := a.Lyrics
	chords := a.Chords
	format := a.Format
	if translation != nil {
		source = translation.SourceContent
		lyrics = translation.Lyrics
		chords = translation.Chords
		format = translation.Format
	}

	// If source is present, reparse so section directives are respected.
	var sections []Section
	switch format {'''
if old not in src:
    print("   ! BuildChart signature not found")
    raise SystemExit(1)
src = src.replace(old, new, 1)

p.write_text(src)
print("   ✓ BuildChart updated")
PYEOF

echo "▸ [6/17] translation.go: types + repo + service methods"
cat > internal/songs/translation.go <<'GOEOF'
package songs

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrTranslationNotFound = errors.New("translation not found")

const translationColumns = `id, arrangement_id, language, lyrics, chords,
	source_content, format, notes, created_at, updated_at`

func scanTranslation(row pgx.Row) (ArrangementTranslation, error) {
	var t ArrangementTranslation
	err := row.Scan(&t.ID, &t.ArrangementID, &t.Language, &t.Lyrics, &t.Chords,
		&t.SourceContent, &t.Format, &t.Notes, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *ArrangementRepository) ListTranslations(ctx context.Context, arrangementID string) ([]ArrangementTranslation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+translationColumns+`
		FROM arrangement_translations
		WHERE arrangement_id = $1
		ORDER BY language
	`, arrangementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArrangementTranslation
	for rows.Next() {
		t, err := scanTranslation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *ArrangementRepository) GetTranslation(ctx context.Context, arrangementID, language string) (ArrangementTranslation, error) {
	t, err := scanTranslation(r.db.QueryRow(ctx, `
		SELECT `+translationColumns+`
		FROM arrangement_translations
		WHERE arrangement_id = $1 AND language = $2
	`, arrangementID, language))
	if errors.Is(err, pgx.ErrNoRows) {
		return ArrangementTranslation{}, ErrTranslationNotFound
	}
	return t, err
}

// UpsertTranslation creates or replaces a translation for (arrangement, lang).
func (r *ArrangementRepository) UpsertTranslation(ctx context.Context, arrangementID, language, source, format, notes string) (ArrangementTranslation, error) {
	lyrics, chords := parseChart(source, format)
	if format == "" {
		format = "chordpro"
	}
	t, err := scanTranslation(r.db.QueryRow(ctx, `
		INSERT INTO arrangement_translations
			(arrangement_id, language, lyrics, chords, source_content, format, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (arrangement_id, language) DO UPDATE SET
			lyrics = EXCLUDED.lyrics,
			chords = EXCLUDED.chords,
			source_content = EXCLUDED.source_content,
			format = EXCLUDED.format,
			notes = EXCLUDED.notes,
			updated_at = now()
		RETURNING `+translationColumns,
		arrangementID, language, lyrics, chords, source, format, notes))
	return t, err
}

func (r *ArrangementRepository) DeleteTranslation(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM arrangement_translations WHERE id = $1`, id)
	return err
}

// --- Service methods on ArrangementService ---

func (s *ArrangementService) ListTranslations(ctx context.Context, arrangementID string) ([]ArrangementTranslation, error) {
	return s.repo.ListTranslations(ctx, arrangementID)
}

func (s *ArrangementService) GetTranslation(ctx context.Context, arrangementID, language string) (ArrangementTranslation, error) {
	return s.repo.GetTranslation(ctx, arrangementID, language)
}

func (s *ArrangementService) UpsertTranslation(ctx context.Context, arrangementID, language, source, format, notes string) (ArrangementTranslation, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return ArrangementTranslation{}, errors.New("language is required")
	}
	if strings.TrimSpace(source) == "" {
		return ArrangementTranslation{}, errors.New("translated source is required")
	}
	return s.repo.UpsertTranslation(ctx, arrangementID, language, source, format, notes)
}

func (s *ArrangementService) DeleteTranslation(ctx context.Context, id string) error {
	return s.repo.DeleteTranslation(ctx, id)
}

// ChartForLanguage returns chart + resolved key + resolved language.
// If requested lang has no translation, falls back to primary language and
// returns the fallback as resolvedLang, plus a boolean flag telling the caller
// that the requested translation is missing.
func (s *ArrangementService) ChartForLanguage(ctx context.Context, arrangementID, displayKey, lang string) (chart []RenderedSection, resolvedKey, resolvedLang string, translationMissing bool, err error) {
	a, err := s.repo.Get(ctx, arrangementID)
	if err != nil {
		return nil, "", "", false, err
	}

	var trans *ArrangementTranslation
	if lang != "" && lang != a.PrimaryLanguage {
		t, terr := s.repo.GetTranslation(ctx, arrangementID, lang)
		if terr == nil {
			trans = &t
		} else {
			translationMissing = true
		}
	}

	dk := displayKey
	if dk == "" {
		if k, err := s.repo.DefaultKeyForArrangement(ctx, arrangementID); err == nil {
			dk = k.Key
		}
	}
	if dk == "" {
		dk = a.OriginalKey
	}

	chart = BuildChart(&a, trans, dk)
	resolvedLang = a.PrimaryLanguage
	if trans != nil {
		resolvedLang = trans.Language
	}
	if resolvedLang == "" {
		resolvedLang = "en"
	}
	return chart, dk, resolvedLang, translationMissing, nil
}
GOEOF

echo "▸ [7/17] translator.go: LibreTranslate client"
cat > internal/songs/translator.go <<'GOEOF'
package songs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrTranslatorDisabled = errors.New("translation service is not configured")

// Translator is a thin LibreTranslate-format client.
// Any provider that speaks POST {base}/translate with {q,source,target,format}
// and returns {translatedText} will work (LibreTranslate, DeepL proxy, etc.).
type Translator struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewTranslator(baseURL, apiKey string) *Translator {
	return &Translator{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *Translator) Enabled() bool { return t != nil && t.baseURL != "" }

type translateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"`
	Target string `json:"target"`
	Format string `json:"format"`
	APIKey string `json:"api_key,omitempty"`
}

type translateResponse struct {
	TranslatedText string `json:"translatedText"`
	Error          string `json:"error,omitempty"`
}

func (t *Translator) Translate(ctx context.Context, text, source, target string) (string, error) {
	if !t.Enabled() {
		return "", ErrTranslatorDisabled
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("nothing to translate")
	}
	if source == target {
		return text, nil
	}
	req := translateRequest{Q: text, Source: source, Target: target, Format: "text", APIKey: t.apiKey}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/translate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("translate request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("translate: status %d: %s", resp.StatusCode, string(raw))
	}
	var out translateResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("translate: decode: %w", err)
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	return out.TranslatedText, nil
}

// StripChordsForTranslation returns lyric-only text for a source in the given format.
func StripChordsForTranslation(source, format string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	switch format {
	case "chordpro", "":
		p := ParseChordPro(source)
		return p.Lyrics
	case "opensong":
		if p, err := ParseOpenSong(source); err == nil {
			return p.Lyrics
		}
	case "openlyrics":
		if p, err := ParseOpenLyrics(source); err == nil {
			return p.Lyrics
		}
	}
	return source
}
GOEOF

echo "▸ [8/17] arrangement_repository.go: primary_language + translation snapshot"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/arrangement_repository.go")
src = p.read_text()

# 1. Column list
src = src.replace(
'''const arrangementColumns = `id, song_id, name, bpm, meter, length_seconds,
	sequence, notes, original_key, source_content, format,
	lyrics, chords, is_default, archived, created_at, updated_at`''',
'''const arrangementColumns = `id, song_id, name, bpm, meter, length_seconds,
	sequence, notes, original_key, source_content, format, primary_language,
	lyrics, chords, is_default, archived, created_at, updated_at`''')

# 2. scanArrangement
src = src.replace(
'''	err := row.Scan(&a.ID, &a.SongID, &a.Name, &a.BPM, &a.Meter, &a.LengthSeconds,
		&a.Sequence, &a.Notes, &a.OriginalKey, &a.SourceContent, &a.Format,
		&a.Lyrics, &a.Chords, &a.IsDefault, &a.Archived,
		&a.CreatedAt, &a.UpdatedAt)''',
'''	err := row.Scan(&a.ID, &a.SongID, &a.Name, &a.BPM, &a.Meter, &a.LengthSeconds,
		&a.Sequence, &a.Notes, &a.OriginalKey, &a.SourceContent, &a.Format,
		&a.PrimaryLanguage,
		&a.Lyrics, &a.Chords, &a.IsDefault, &a.Archived,
		&a.CreatedAt, &a.UpdatedAt)''')

# 3. Create INSERT — add primary_language
src = src.replace(
'''	format := in.Format
	if format == "" {
		format = "manual"
	}
	a, err := scanArrangement(tx.QueryRow(ctx, `
		INSERT INTO song_arrangements (
			song_id, name, bpm, meter, length_seconds,
			sequence, notes, original_key, source_content, format,
			lyrics, chords,
			is_default, archived
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+arrangementColumns,
		songID, in.Name, in.BPM, in.Meter, in.LengthSeconds,
		in.Sequence, in.Notes, in.OriginalKey, in.SourceContent, format,
		in.Lyrics, in.Chords,
		isDefault, in.Archived))''',
'''	format := in.Format
	if format == "" {
		format = "manual"
	}
	primaryLang := in.PrimaryLanguage
	if primaryLang == "" {
		primaryLang = "en"
	}
	a, err := scanArrangement(tx.QueryRow(ctx, `
		INSERT INTO song_arrangements (
			song_id, name, bpm, meter, length_seconds,
			sequence, notes, original_key, source_content, format, primary_language,
			lyrics, chords,
			is_default, archived
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING `+arrangementColumns,
		songID, in.Name, in.BPM, in.Meter, in.LengthSeconds,
		in.Sequence, in.Notes, in.OriginalKey, in.SourceContent, format, primaryLang,
		in.Lyrics, in.Chords,
		isDefault, in.Archived))''')

# 4. Update — include format + primary_language
src = src.replace(
'''	a, err := scanArrangement(tx.QueryRow(ctx, `
		UPDATE song_arrangements SET
			name=$2, bpm=$3, meter=$4, length_seconds=$5,
			sequence=$6, notes=$7, original_key=$8, source_content=$9,
			lyrics=$10, chords=$11, archived=$12, updated_at=now()
		WHERE id=$1
		RETURNING `+arrangementColumns,
		id, in.Name, in.BPM, in.Meter, in.LengthSeconds,
		in.Sequence, in.Notes, in.OriginalKey, in.SourceContent,
		lyrics, chords, in.Archived))''',
'''	updFormat := in.Format
	if updFormat == "" {
		updFormat = "chordpro"
	}
	primaryLang := in.PrimaryLanguage
	if primaryLang == "" {
		primaryLang = "en"
	}
	a, err := scanArrangement(tx.QueryRow(ctx, `
		UPDATE song_arrangements SET
			name=$2, bpm=$3, meter=$4, length_seconds=$5,
			sequence=$6, notes=$7, original_key=$8, source_content=$9,
			format=$10, primary_language=$11, lyrics=$12, chords=$13,
			archived=$14, updated_at=now()
		WHERE id=$1
		RETURNING `+arrangementColumns,
		id, in.Name, in.BPM, in.Meter, in.LengthSeconds,
		in.Sequence, in.Notes, in.OriginalKey, in.SourceContent,
		updFormat, primaryLang,
		lyrics, chords, in.Archived))''')

p.write_text(src)
print("   ✓ arrangement_repository updated")
PYEOF

echo "▸ [9/17] arrangement_service.go: Chart/RenderPDF with lang, backwards-compat wrapper"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/arrangement_service.go")
src = p.read_text()

# Replace Chart method to delegate to ChartForLanguage
old_chart = '''// Chart returns the rendered chart for an arrangement in a display key.
// If displayKey is empty, the arrangement's default key (or original key) is used.
func (s *ArrangementService) Chart(ctx context.Context, arrangementID, displayKey string) ([]RenderedSection, string, error) {
	a, err := s.repo.Get(ctx, arrangementID)
	if err != nil {
		return nil, "", err
	}
	dk := displayKey
	if dk == "" {
		if k, err := s.repo.DefaultKeyForArrangement(ctx, arrangementID); err == nil {
			dk = k.Key
		}
	}
	if dk == "" {
		dk = a.OriginalKey
	}
	return BuildChart(&a, dk), dk, nil
}'''

new_chart = '''// Chart is a backwards-compatible wrapper that returns the chart in the
// arrangement's primary language.
func (s *ArrangementService) Chart(ctx context.Context, arrangementID, displayKey string) ([]RenderedSection, string, error) {
	chart, dk, _, _, err := s.ChartForLanguage(ctx, arrangementID, displayKey, "")
	return chart, dk, err
}'''

if old_chart not in src:
    print("   ! Chart method not found")
    raise SystemExit(1)
src = src.replace(old_chart, new_chart, 1)

# Update RenderPDF to accept lang + use ChartForLanguage
old_pdf = '''// RenderPDF builds a PDF for the arrangement in the given display key.
func (s *ArrangementService) RenderPDF(ctx context.Context, song *Song, arrangementID, displayKey, qrURL string) ([]byte, string, error) {
	a, err := s.repo.Get(ctx, arrangementID)
	if err != nil {
		return nil, "", err
	}
	dk := displayKey
	if dk == "" {
		if k, err := s.repo.DefaultKeyForArrangement(ctx, arrangementID); err == nil {
			dk = k.Key
		}
	}
	if dk == "" {
		dk = a.OriginalKey
	}
	chart := BuildChart(&a, dk)

	data, err := RenderSongPDF(song.Title, song.Author, dk, a.Name, a.Sequence, chart, qrURL)
	if err != nil {
		return nil, "", err
	}
	return data, sanitizeFilename(song.Title+"_"+a.Name) + ".pdf", nil
}'''

new_pdf = '''// RenderPDF builds a PDF for the arrangement, honoring the requested language.
func (s *ArrangementService) RenderPDF(ctx context.Context, song *Song, arrangementID, displayKey, qrURL, lang string) ([]byte, string, error) {
	a, err := s.repo.Get(ctx, arrangementID)
	if err != nil {
		return nil, "", err
	}
	chart, dk, resolvedLang, _, err := s.ChartForLanguage(ctx, arrangementID, displayKey, lang)
	if err != nil {
		return nil, "", err
	}
	data, err := RenderSongPDF(song.Title, song.Author, dk, a.Name, a.Sequence, chart, qrURL, resolvedLang)
	if err != nil {
		return nil, "", err
	}
	suffix := ""
	if resolvedLang != "" && resolvedLang != a.PrimaryLanguage {
		suffix = "_" + resolvedLang
	}
	return data, sanitizeFilename(song.Title+"_"+a.Name) + suffix + ".pdf", nil
}'''

if old_pdf not in src:
    print("   ! RenderPDF method not found")
    raise SystemExit(1)
src = src.replace(old_pdf, new_pdf, 1)

p.write_text(src)
print("   ✓ arrangement_service updated")
PYEOF

echo "▸ [10/17] pdf.go: language param"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/pdf.go")
src = p.read_text()

# Add strings import
src = src.replace(
'''import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"
)''',
'''import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"
)''')

# Add language to signature
src = src.replace(
'''func RenderSongPDF(title, author, key, arrangement, sequence string, chart []RenderedSection, qrURL string) ([]byte, error) {''',
'''func RenderSongPDF(title, author, key, arrangement, sequence string, chart []RenderedSection, qrURL, language string) ([]byte, error) {''')

# Add language to subtitle
src = src.replace(
'''	if key != "" {
		sub += "  —  Key: " + key
	}''',
'''	if key != "" {
		sub += "  —  Key: " + key
	}
	if language != "" && language != "en" {
		sub += "  —  " + strings.ToUpper(language)
	}''')

p.write_text(src)
print("   ✓ pdf.go updated")
PYEOF

echo "▸ [11/17] suggestions.go"
cat > internal/songs/suggestions.go <<'GOEOF'
package songs

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SuggestionService struct {
	db     *pgxpool.Pool
	health *HealthService
}

func NewSuggestionService(db *pgxpool.Pool, health *HealthService) *SuggestionService {
	return &SuggestionService{db: db, health: health}
}

// Suggest returns ranked candidates among songs carrying ALL given tags.
func (s *SuggestionService) Suggest(ctx context.Context, tagSlugs []string, key string, limit int) ([]Suggestion, error) {
	if limit <= 0 {
		limit = 15
	}
	if limit > 100 {
		limit = 100
	}

	// 1. Find candidate songs (tag AND semantics)
	var (
		args  []any
		conds []string
	)
	conds = append(conds, "s.archived = FALSE")
	if len(tagSlugs) > 0 {
		args = append(args, tagSlugs, len(tagSlugs))
		conds = append(conds, fmt.Sprintf(
			`(SELECT COUNT(*) FROM song_tags st JOIN tags t ON t.id = st.tag_id
			  WHERE st.song_id = s.id AND t.slug = ANY($%d)) = $%d`,
			len(args)-1, len(args)))
	}

	q := `SELECT s.id FROM songs s WHERE ` + strings.Join(conds, " AND ")
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var candidateIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		candidateIDs = append(candidateIDs, id)
	}
	rows.Close()
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	// 2. Load health metrics for all candidates (reuse HealthService)
	all, err := s.health.Compute(ctx)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range candidateIDs {
		want[id] = true
	}

	// 3. Load song info + default arrangement info
	info, err := s.loadSongInfo(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}

	// 4. Score + sort
	var out []Suggestion
	for _, h := range all {
		if !want[h.SongID] {
			continue
		}
		si := info[h.SongID]
		score := scoreSuggestion(h, key)
		reason := explainSuggestion(h, key, si.DefaultKey)
		out = append(out, Suggestion{
			SongID:          h.SongID,
			SongTitle:       h.Title,
			SongAuthor:      h.Author,
			ArrangementID:   si.DefaultArrangementID,
			ArrangementName: si.DefaultArrangementName,
			Key:             si.DefaultKey,
			Score:           score,
			Reason:          reason,
			LastUsedDays:    h.DaysSinceUse,
			Uses3m:          h.Uses3m,
		})
	}

	// Sort by score desc (simple insertion sort — lists are small)
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].Score > out[j-1].Score {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type songInfo struct {
	DefaultArrangementID   string
	DefaultArrangementName string
	DefaultKey             string
}

func (s *SuggestionService) loadSongInfo(ctx context.Context, ids []string) (map[string]songInfo, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id,
		       COALESCE(a.id::text, ''),
		       COALESCE(a.name, ''),
		       COALESCE(a.original_key, '')
		FROM songs s
		LEFT JOIN song_arrangements a
		       ON a.song_id = s.id AND a.is_default = TRUE
		WHERE s.id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]songInfo{}
	for rows.Next() {
		var id, aid, aname, akey string
		if err := rows.Scan(&id, &aid, &aname, &akey); err != nil {
			return nil, err
		}
		out[id] = songInfo{DefaultArrangementID: aid, DefaultArrangementName: aname, DefaultKey: akey}
	}
	return out, rows.Err()
}

// scoreSuggestion returns a 0..100 score. Higher = better match right now.
func scoreSuggestion(h SongHealth, requestedKey string) float64 {
	score := 50.0

	switch {
	case h.DaysSinceUse < 0:
		score += 30
	case h.DaysSinceUse > 180:
		score += 25
	case h.DaysSinceUse > 90:
		score += 20
	case h.DaysSinceUse > 30:
		score += 10
	case h.DaysSinceUse < 14:
		score -= 20
	case h.DaysSinceUse < 30:
		score -= 5
	}

	score -= float64(h.Uses3m) * 8

	if requestedKey != "" && h.OriginalKey == requestedKey {
		score += 25
	}

	score -= h.AvgDrift * 3

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func explainSuggestion(h SongHealth, requestedKey, defaultKey string) string {
	var parts []string
	switch {
	case h.DaysSinceUse < 0:
		parts = append(parts, "never sung")
	case h.DaysSinceUse > 90:
		parts = append(parts, fmt.Sprintf("not sung in %dd", h.DaysSinceUse))
	}
	if h.Uses3m >= 3 {
		parts = append(parts, fmt.Sprintf("overused (%dx in 3mo)", h.Uses3m))
	} else if h.Uses3m > 0 {
		parts = append(parts, fmt.Sprintf("%dx in 3mo", h.Uses3m))
	}
	if requestedKey != "" && defaultKey == requestedKey {
		parts = append(parts, "matches requested key")
	}
	if h.AvgDrift >= 2 {
		parts = append(parts, fmt.Sprintf("drifts %.1f semitones", h.AvgDrift))
	}
	if len(parts) == 0 {
		return "balanced fit"
	}
	return strings.Join(parts, " · ")
}
GOEOF

echo "▸ [12/17] handler.go: new handlers + updated call sites"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/handler.go")
src = p.read_text()

# 1. Extend Handler struct with translator + suggestions
src = src.replace(
'''type Handler struct {
	svc      *Service
	arr      *ArrangementService
	tags     *TagService
	revs     *RevisionRepository
	practice *PracticeRepository
	health   *HealthService
	cfg      HandlerConfig
	renderer *httpx.Renderer
}''',
'''type Handler struct {
	svc      *Service
	arr      *ArrangementService
	tags     *TagService
	revs     *RevisionRepository
	practice *PracticeRepository
	health   *HealthService
	sugg     *SuggestionService
	trans    *Translator
	cfg      HandlerConfig
	renderer *httpx.Renderer
}''')

# 2. Extend HandlerConfig with translator + primary language
src = src.replace(
'''type HandlerConfig struct {
	PublicBaseURL string
}''',
'''type HandlerConfig struct {
	PublicBaseURL   string
	TranslateAPIURL string
	TranslateAPIKey string
	PrimaryLanguage string
}''')

# 3. ViewArrangement — add lang handling and translations list
old = '''	displayKey := r.URL.Query().Get("key")
	offsetStr := r.URL.Query().Get("semitones")
	offset, _ := strconv.Atoi(offsetStr)

	// Resolve display key
	resolvedKey := displayKey
	if resolvedKey == "" {
		if k, err := h.arr.DefaultKey(r.Context(), arrID); err == nil {
			resolvedKey = k.Key
		}
	}
	if resolvedKey == "" {
		resolvedKey = a.OriginalKey
	}
	if offset != 0 && resolvedKey != "" {
		resolvedKey = TransposeKey(resolvedKey, offset)
	}

	chart, _, err := h.arr.Chart(r.Context(), arrID, resolvedKey)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	keys, err := h.arr.Keys(r.Context(), arrID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	h.renderer.Render(w, http.StatusOK, "songs_arrangement_view", map[string]any{
		"Song":        song,
		"Arrangement": a,
		"Keys":        keys,
		"DisplayKey":  resolvedKey,
		"Offset":      offset,
		"Chart":       chart,
	})'''

new = '''	displayKey := r.URL.Query().Get("key")
	offsetStr := r.URL.Query().Get("semitones")
	offset, _ := strconv.Atoi(offsetStr)
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))

	// Resolve display key
	resolvedKey := displayKey
	if resolvedKey == "" {
		if k, err := h.arr.DefaultKey(r.Context(), arrID); err == nil {
			resolvedKey = k.Key
		}
	}
	if resolvedKey == "" {
		resolvedKey = a.OriginalKey
	}
	if offset != 0 && resolvedKey != "" {
		resolvedKey = TransposeKey(resolvedKey, offset)
	}

	chart, resolvedKey, resolvedLang, translationMissing, err := h.arr.ChartForLanguage(r.Context(), arrID, resolvedKey, lang)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	keys, err := h.arr.Keys(r.Context(), arrID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	translations, err := h.arr.ListTranslations(r.Context(), arrID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	h.renderer.Render(w, http.StatusOK, "songs_arrangement_view", map[string]any{
		"Song":               song,
		"Arrangement":        a,
		"Keys":               keys,
		"DisplayKey":         resolvedKey,
		"Offset":             offset,
		"Chart":              chart,
		"RequestedLang":      lang,
		"ResolvedLang":       resolvedLang,
		"TranslationMissing": translationMissing,
		"Translations":       translations,
	})'''
if old not in src:
    print("   ! ViewArrangement body not found")
    raise SystemExit(1)
src = src.replace(old, new, 1)

# 4. DownloadPDF — accept lang + new RenderPDF signature
src = src.replace(
'''	displayKey := r.URL.Query().Get("key")

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, "")
	if err != nil {
		httpx.ServerError(w, err)
		return
	}''',
'''	displayKey := r.URL.Query().Get("key")
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, "", lang)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}''')

# 5. ViewPractice — use ChartForLanguage
old_practice = '''	chart, _, err := h.arr.Chart(r.Context(), arrID, displayKey)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	h.renderer.Render(w, http.StatusOK, "songs_practice", map[string]any{
		"Token":       token,
		"Song":        song,
		"Arrangement": arr,
		"DisplayKey":  displayKey,
		"Offset":      offset,
		"Chart":       chart,
		"ExpiresAt":   pt.ExpiresAt,
		"YouTubeID":   youtubeID(song.YouTubeURL),
	})'''
new_practice = '''	lang := strings.TrimSpace(r.URL.Query().Get("lang"))

	chart, displayKey, resolvedLang, translationMissing, err := h.arr.ChartForLanguage(r.Context(), arrID, displayKey, lang)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	translations, _ := h.arr.ListTranslations(r.Context(), arrID)

	h.renderer.Render(w, http.StatusOK, "songs_practice", map[string]any{
		"Token":              token,
		"Song":               song,
		"Arrangement":        arr,
		"DisplayKey":         displayKey,
		"Offset":             offset,
		"Chart":              chart,
		"ExpiresAt":          pt.ExpiresAt,
		"YouTubeID":          youtubeID(song.YouTubeURL),
		"RequestedLang":      lang,
		"ResolvedLang":       resolvedLang,
		"TranslationMissing": translationMissing,
		"Translations":       translations,
	})'''
if old_practice not in src:
    print("   ! ViewPractice render block not found")
    raise SystemExit(1)
src = src.replace(old_practice, new_practice, 1)

# 6. ViewPracticePDF — pass lang through
src = src.replace(
'''	displayKey := r.URL.Query().Get("key")
	if displayKey == "" {
		displayKey = pt.DisplayKey
	}
	qrURL := h.publicBaseURL(r) + "/practice/" + token

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, qrURL)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}''',
'''	displayKey := r.URL.Query().Get("key")
	if displayKey == "" {
		displayKey = pt.DisplayKey
	}
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	qrURL := h.publicBaseURL(r) + "/practice/" + token

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, qrURL, lang)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}''')

# 7. Add new handlers at end (append before publicBaseURL helper)
marker = '''// youtubeID extracts the video ID from common YouTube URL forms.'''
addition = '''// ---- Translation handlers ----

func (h *Handler) NewTranslationForm(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	song, err := h.svc.Get(r.Context(), songID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := h.arr.Get(r.Context(), arrID)
	if err != nil || a.SongID != songID {
		http.NotFound(w, r)
		return
	}
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	if lang == "" {
		lang = "fr"
	}

	// If a translation already exists, pre-fill the form.
	var existing ArrangementTranslation
	if t, err := h.arr.GetTranslation(r.Context(), arrID, lang); err == nil {
		existing = t
	}

	h.renderTranslationForm(w, r, song, a, lang, existing.SourceContent, existing.Format, existing.Notes, "")
}

func (h *Handler) SaveTranslation(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}
	lang := r.FormValue("language")
	source := r.FormValue("source_content")
	format := r.FormValue("format")
	notes := r.FormValue("notes")

	if _, err := h.arr.UpsertTranslation(r.Context(), arrID, lang, source, format, notes); err != nil {
		song, _ := h.svc.Get(r.Context(), songID)
		a, _ := h.arr.Get(r.Context(), arrID)
		h.renderTranslationForm(w, r, song, a, lang, source, format, notes, err.Error())
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID+"?lang="+lang, http.StatusSeeOther)
}

func (h *Handler) DeleteTranslation(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	_ = h.arr.DeleteTranslation(r.Context(), chi.URLParam(r, "tid"))
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID, http.StatusSeeOther)
}

// TranslateSource is the target of a formaction button. It translates the
// arrangement's primary source into the requested language and re-renders
// the translation form with the result pre-filled.
func (h *Handler) TranslateSource(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}
	lang := r.FormValue("language")
	if lang == "" {
		lang = r.URL.Query().Get("lang")
	}

	song, err := h.svc.Get(r.Context(), songID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := h.arr.Get(r.Context(), arrID)
	if err != nil || a.SongID != songID {
		http.NotFound(w, r)
		return
	}

	// Track what to display in the form even on error
	source := r.FormValue("source_content")
	format := r.FormValue("format")
	notes := r.FormValue("notes")

	if !h.trans.Enabled() {
		h.renderTranslationForm(w, r, song, a, lang, source, format, notes,
			"Translation service not configured. Set TRANSLATE_API_URL to enable. You can still paste the translation manually.")
		return
	}

	plain := StripChordsForTranslation(a.SourceContent, a.Format)
	if plain == "" {
		plain = a.Lyrics
	}
	if plain == "" {
		h.renderTranslationForm(w, r, song, a, lang, source, format, notes,
			"No source content to translate.")
		return
	}

	translated, err := h.trans.Translate(r.Context(), plain, a.PrimaryLanguage, lang)
	if err != nil {
		h.renderTranslationForm(w, r, song, a, lang, source, format, notes,
			"Translate failed: "+err.Error())
		return
	}

	h.renderTranslationForm(w, r, song, a, lang, translated, format, notes, "")
}

func (h *Handler) renderTranslationForm(w http.ResponseWriter, r *http.Request, song Song, a Arrangement, lang, source, format, notes, errMsg string) {
	if format == "" {
		format = a.Format
		if format == "" {
			format = "chordpro"
		}
	}
	h.renderer.Render(w, http.StatusOK, "songs_translation_form", map[string]any{
		"Song":             song,
		"Arrangement":      a,
		"Language":         lang,
		"SourceContent":    source,
		"Format":           format,
		"Notes":            notes,
		"Error":            errMsg,
		"TranslatorEnabled": h.trans != nil && h.trans.Enabled(),
	})
}

// ---- Suggestions ----

func (h *Handler) Suggestions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tagSlugs := q["tag"]
	key := strings.TrimSpace(q.Get("key"))
	limit := 15
	if n, _ := strconv.Atoi(q.Get("limit")); n > 0 {
		limit = n
	}

	var suggestions []Suggestion
	var err error
	run := r.URL.Query().Get("run") == "1" || len(tagSlugs) > 0 || key != ""
	if run {
		suggestions, err = h.sugg.Suggest(r.Context(), tagSlugs, key, limit)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
	}

	groups, err := h.tags.ListGrouped(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	selected := map[string]bool{}
	for _, s := range tagSlugs {
		selected[s] = true
	}

	h.renderer.Render(w, http.StatusOK, "songs_suggestions", map[string]any{
		"Songs":            suggestions,
		"TagGroups":        groups,
		"SelectedTagSlugs": selected,
		"Key":              key,
		"Limit":            limit,
		"Ran":              run,
	})
}

'''
if marker not in src:
    print("   ! youtubeID marker not found")
    raise SystemExit(1)
src = src.replace(marker, addition + marker, 1)

p.write_text(src)
print("   ✓ handler updated")
PYEOF

echo "▸ [13/17] routes.go: new routes + suggestion + translator wiring"
cat > internal/songs/routes.go <<'GOEOF'
package songs

import (
	"embed"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

//go:embed templates/*.html
var templatesFS embed.FS

func Templates() fs.FS {
	sub, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}

type Module struct{ handler *Handler }

func New(db *pgxpool.Pool, renderer *httpx.Renderer, cfg HandlerConfig) *Module {
	arrRepo := NewArrangementRepository(db)
	health := NewHealthService(db)
	translator := NewTranslator(cfg.TranslateAPIURL, cfg.TranslateAPIKey)
	return &Module{handler: &Handler{
		svc:      NewService(NewRepository(db), NewHistoryRepository(db)),
		arr:      NewArrangementService(arrRepo),
		tags:     NewTagService(db),
		revs:     NewRevisionRepository(db),
		practice: NewPracticeRepository(db),
		health:   health,
		sugg:     NewSuggestionService(db, health),
		trans:    translator,
		cfg:      cfg,
		renderer: renderer,
	}}
}

// PublicRoutes are mounted outside auth middleware (practice links).
func (m *Module) PublicRoutes(r chi.Router) {
	r.Get("/practice/{token}", m.handler.ViewPractice)
	r.Get("/practice/{token}/pdf", m.handler.ViewPracticePDF)
}

func (m *Module) Routes(r chi.Router) {
	r.Route("/songs", func(r chi.Router) {
		r.Get("/", m.handler.List)
		r.Get("/new", m.handler.New)
		r.Post("/", m.handler.Create)
		r.Get("/import", m.handler.ShowImport)
		r.Post("/import", m.handler.Import)
		r.Get("/reports/health", m.handler.Health)
		r.Get("/suggest", m.handler.Suggestions)
		r.Get("/tags", m.handler.TagsIndex)
		r.Post("/tags/groups", m.handler.CreateTagGroup)
		r.Post("/tags/tags", m.handler.CreateTag)
		r.Post("/tags/{id}/delete", m.handler.DeleteTag)

		r.Get("/{id}", m.handler.View)
		r.Get("/{id}/pdf", m.handler.DownloadPDF)
		r.Get("/{id}/edit", m.handler.Edit)
		r.Post("/{id}", m.handler.Update)
		r.Post("/{id}/delete", m.handler.Delete)
		r.Post("/{id}/history", m.handler.AddHistory)
		r.Post("/{id}/practice-links", m.handler.CreatePracticeLink)

		r.Get("/{id}/arrangements/new", m.handler.NewArrangement)
		r.Post("/{id}/arrangements", m.handler.CreateArrangement)
		r.Get("/{id}/arrangements/{aid}", m.handler.ViewArrangement)
		r.Get("/{id}/arrangements/{aid}/pdf", m.handler.DownloadPDF)
		r.Get("/{id}/arrangements/{aid}/edit", m.handler.EditArrangement)
		r.Post("/{id}/arrangements/{aid}", m.handler.UpdateArrangement)
		r.Post("/{id}/arrangements/{aid}/delete", m.handler.DeleteArrangement)
		r.Post("/{id}/arrangements/{aid}/set-default", m.handler.SetDefaultArrangement)
		r.Get("/{id}/arrangements/{aid}/history", m.handler.ArrangementHistory)
		r.Post("/{id}/arrangements/{aid}/history/{rid}/restore", m.handler.RestoreRevision)

		r.Post("/{id}/arrangements/{aid}/keys", m.handler.CreateArrangementKey)
		r.Post("/{id}/arrangements/{aid}/keys/{kid}/delete", m.handler.DeleteArrangementKey)
		r.Post("/{id}/arrangements/{aid}/keys/{kid}/set-default", m.handler.SetDefaultArrangementKey)

		// Translations
		r.Get("/{id}/arrangements/{aid}/translations/new", m.handler.NewTranslationForm)
		r.Post("/{id}/arrangements/{aid}/translations", m.handler.SaveTranslation)
		r.Post("/{id}/arrangements/{aid}/translations/{tid}/delete", m.handler.DeleteTranslation)
		r.Post("/{id}/arrangements/{aid}/translate", m.handler.TranslateSource)
	})
}
GOEOF

echo "▸ [14/17] web/router.go: pass translator config"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/router.go")
src = p.read_text()
old = '''	songsModule := songs.New(d.DB, renderer, songs.HandlerConfig{
		PublicBaseURL: d.Config.PublicBaseURL,
	})'''
new = '''	songsModule := songs.New(d.DB, renderer, songs.HandlerConfig{
		PublicBaseURL:   d.Config.PublicBaseURL,
		TranslateAPIURL: d.Config.TranslateAPIURL,
		TranslateAPIKey: d.Config.TranslateAPIKey,
		PrimaryLanguage: d.Config.PrimaryLanguage,
	})'''
if old not in src:
    print("   ! songs.New call not found")
    raise SystemExit(1)
p.write_text(src.replace(old, new, 1))
print("   ✓ router updated")
PYEOF

echo "▸ [15/17] Templates: new + modified"
cat > internal/songs/templates/translation_form.html <<'HTMLEOF'
{{define "content"}}
<p><a href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}">← {{.Arrangement.Name}}</a></p>
<h1>Translation · <span class="muted">{{languageLabel .Language}}</span></h1>
<p class="muted">{{.Song.Title}} · {{.Arrangement.Name}} (primary: {{languageLabel .Arrangement.PrimaryLanguage}})</p>
{{with .Error}}<p class="error">{{.}}</p>{{end}}

<details class="primary-source">
    <summary>Primary source ({{languageLabel .Arrangement.PrimaryLanguage}})</summary>
    <pre>{{.Arrangement.SourceContent}}</pre>
</details>

<form method="post" action="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/translations">
    <input type="hidden" name="language" value="{{.Language}}">

    <label>Format
        <select name="format">
            <option value="chordpro" {{if eq .Format "chordpro"}}selected{{end}}>ChordPro</option>
            <option value="opensong" {{if eq .Format "opensong"}}selected{{end}}>OpenSong</option>
            <option value="text"     {{if eq .Format "text"}}selected{{end}}>Plain lyrics</option>
        </select>
    </label>

    <label>Translated source
        <textarea name="source_content" rows="16" style="font-family: monospace" placeholder="Paste the translated lyrics (and chords, if desired) here.">{{.SourceContent}}</textarea>
    </label>

    <label>Notes <textarea name="notes" rows="2">{{.Notes}}</textarea></label>

    {{if .TranslatorEnabled}}
    <p>
        <button type="submit">Save translation</button>
        <button type="submit"
                formaction="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/translate"
                class="button-secondary">
            Auto-translate from {{languageLabel .Arrangement.PrimaryLanguage}}
        </button>
    </p>
    <p class="muted">
        Auto-translate sends the lyrics (chords stripped) to the configured translation service.
        It will <strong>not</strong> place chords for you — review and add chords manually.
    </p>
    {{else}}
    <p>
        <button type="submit">Save translation</button>
    </p>
    <p class="muted">Auto-translate is disabled. Set <code>TRANSLATE_API_URL</code> to enable.</p>
    {{end}}
</form>
{{end}}
HTMLEOF

cat > internal/songs/templates/suggestions.html <<'HTMLEOF'
{{define "content"}}
<p><a href="/songs">← All songs</a></p>
<h1>Suggestions</h1>
<p class="muted">Ranked by last use, recency, overuse, key fit, and drift.</p>

<form method="get" action="/songs/suggest" class="filter-bar">
    <select name="tag" multiple size="4" class="tag-multi">
        {{range .TagGroups}}
            <optgroup label="{{.Group.Name}}">
                {{range .Tags}}
                    <option value="{{.Slug}}" {{if index $.SelectedTagSlugs .Slug}}selected{{end}}>{{.Name}}</option>
                {{end}}
            </optgroup>
        {{end}}
    </select>
    <input type="text" name="key" placeholder="Preferred key (optional)" value="{{.Key}}" size="8">
    <input type="number" name="limit" value="{{.Limit}}" min="1" max="100" style="width:5rem">
    <input type="hidden" name="run" value="1">
    <button type="submit">Suggest</button>
</form>

{{if .Ran}}
    {{if .Songs}}
    <div class="suggestion-grid">
        {{range .Songs}}
        <div class="suggestion-card">
            <h3><a href="/songs/{{.SongID}}">{{.SongTitle}}</a></h3>
            <p class="muted">{{.SongAuthor}}</p>
            <p><strong>Score:</strong> {{printf "%.0f" .Score}} / 100</p>
            {{if .ArrangementName}}
            <p class="muted">{{.ArrangementName}} · {{.Key}}</p>
            {{end}}
            <p class="suggestion-reason">{{.Reason}}</p>
            {{if .ArrangementID}}
            <p><a class="button-secondary" href="/songs/{{.SongID}}/arrangements/{{.ArrangementID}}">Open chart</a></p>
            {{end}}
        </div>
        {{end}}
    </div>
    {{else}}
    <p class="muted">No songs matched those tags.</p>
    {{end}}
{{end}}
{{end}}
HTMLEOF

# Rewrite arrangement_view.html to include language picker + translations management
cat > internal/songs/templates/arrangement_view.html <<'HTMLEOF'
{{define "content"}}
<p><a href="/songs/{{.Song.ID}}">← {{.Song.Title}}</a></p>
<h1>{{.Arrangement.Name}} <span class="muted">· {{.Song.Title}}</span></h1>

<div class="meta">
    <span><strong>Key:</strong> {{.DisplayKey}}{{if .Offset}} ({{if gt .Offset 0}}+{{end}}{{.Offset}}){{end}}</span>
    {{if .Arrangement.OriginalKey}}<span><strong>Original:</strong> {{.Arrangement.OriginalKey}}</span>{{end}}
    {{if .Arrangement.PrimaryLanguage}}<span><strong>Language:</strong> {{languageLabel .ResolvedLang}}</span>{{end}}
    {{if .Arrangement.BPM}}<span><strong>BPM:</strong> {{.Arrangement.BPM}}</span>{{end}}
    {{if .Arrangement.Meter}}<span><strong>Meter:</strong> {{.Arrangement.Meter}}</span>{{end}}
    {{if .Arrangement.LengthSeconds}}<span><strong>Length:</strong> {{.Arrangement.LengthSeconds}}s</span>{{end}}
</div>

{{if .TranslationMissing}}
<p class="error">No translation in {{languageLabel .RequestedLang}} yet. Showing primary ({{languageLabel .ResolvedLang}}).
    <a href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/translations/new?lang={{.RequestedLang}}">Add one →</a></p>
{{end}}

{{if .Arrangement.Sequence}}<p><strong>Sequence:</strong> <code>{{.Arrangement.Sequence}}</code></p>{{end}}

<p>
    <a class="button" href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/pdf?key={{.DisplayKey}}&lang={{.ResolvedLang}}">Download PDF ({{.DisplayKey}}{{if .ResolvedLang}} · {{languageLabel .ResolvedLang}}{{end}})</a>
    <a class="button-secondary" href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/edit">Edit</a>
    <a class="button-secondary" href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/history">History</a>
</p>

<h2>Language</h2>
<div class="lang-picker">
    <a href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}"
       class="lang-chip {{if eq .ResolvedLang .Arrangement.PrimaryLanguage}}active{{end}}">
        {{languageLabel .Arrangement.PrimaryLanguage}} <span class="muted">(primary)</span>
    </a>
    {{range .Translations}}
    <a href="/songs/{{$.Song.ID}}/arrangements/{{$.Arrangement.ID}}?lang={{.Language}}"
       class="lang-chip {{if eq .Language $.ResolvedLang}}active{{end}}">
        {{languageLabel .Language}}
        <form method="post" action="/songs/{{$.Song.ID}}/arrangements/{{$.Arrangement.ID}}/translations/{{.ID}}/delete"
              style="display:inline" onsubmit="return confirm('Delete {{languageLabel .Language}} translation?')">
            <button type="submit" class="link-btn">×</button>
        </form>
    </a>
    {{end}}
    <a href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/translations/new" class="button-secondary">+ Add translation</a>
</div>

<h2>Transpose (transient view)</h2>
<form method="get" action="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}" class="transpose">
    <input type="hidden" name="key" value="{{.DisplayKey}}">
    {{if .ResolvedLang}}<input type="hidden" name="lang" value="{{.ResolvedLang}}">{{end}}
    <button name="semitones" value="-2">−2</button>
    <button name="semitones" value="-1">−1</button>
    <button name="semitones" value="0">reset</button>
    <button name="semitones" value="1">+1</button>
    <button name="semitones" value="2">+2</button>
</form>

<h2>Named keys</h2>
{{if .Keys}}
<ul class="tag-list">
    {{range .Keys}}
    <li>
        <span class="chip">{{.Name}}: {{.Key}}{{if .IsDefault}} (default){{end}}</span>
        {{if not .IsDefault}}
            <form method="post" action="/songs/{{$.Song.ID}}/arrangements/{{$.Arrangement.ID}}/keys/{{.ID}}/set-default" style="display:inline">
                <button class="link-btn" type="submit">★</button>
            </form>
        {{end}}
        <form method="post" action="/songs/{{$.Song.ID}}/arrangements/{{$.Arrangement.ID}}/keys/{{.ID}}/delete" style="display:inline" onsubmit="return confirm('Delete key?')">
            <button class="link-btn" type="submit">×</button>
        </form>
    </li>
    {{end}}
</ul>
{{else}}
<p class="muted">No named keys. Add one below.</p>
{{end}}

<form method="post" action="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/keys" class="inline-form">
    <input type="text" name="name" placeholder="e.g. Female Key" required>
    <input type="text" name="key" placeholder="A" required size="4">
    <label class="check"><input type="checkbox" name="make_default" value="1"> default</label>
    <button type="submit">Add key</button>
</form>

<h2>Chart {{if .ResolvedLang}}· {{languageLabel .ResolvedLang}}{{end}}</h2>
<div class="chart">
    {{range .Chart}}
    <div class="chart-section">
        {{if .Label}}<div class="chart-section-label">{{.Label}}</div>{{end}}
        {{range .Lines}}
        <div class="chart-line">
            {{range .Chords}}<span class="chord" style="left: {{.Position}}ch">{{.Chord}}</span>{{end}}
            <span class="lyric">{{.Lyric}}</span>
        </div>
        {{end}}
    </div>
    {{else}}
    <p class="muted">No content. Edit this arrangement to add the source.</p>
    {{end}}
</div>

{{if .Arrangement.Notes}}<h2>Notes</h2><p>{{.Arrangement.Notes}}</p>{{end}}

<form method="post" action="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/delete" style="margin-top:2rem" onsubmit="return confirm('Delete this arrangement and its keys/translations?')">
    <button type="submit" class="link-btn" style="color:#b42318">Delete arrangement</button>
</form>
{{end}}
HTMLEOF

# arrangement_form.html: add primary_language selector
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/templates/arrangement_form.html")
src = p.read_text()
if "primary_language" in src:
    print("   (already present)")
    raise SystemExit(0)
old = '''    <label>Format
        <select name="format">
            <option value="chordpro"  {{if eq .Arrangement.Format "chordpro"}}selected{{end}}>ChordPro</option>
            <option value="opensong"  {{if eq .Arrangement.Format "opensong"}}selected{{end}}>OpenSong</option>
            <option value="text"      {{if eq .Arrangement.Format "text"}}selected{{end}}>Plain lyrics</option>
            <option value="manual"    {{if eq .Arrangement.Format "manual"}}selected{{end}}>Manual (no parsing)</option>
        </select>
    </label>'''
new = '''    <label>Format
        <select name="format">
            <option value="chordpro"  {{if eq .Arrangement.Format "chordpro"}}selected{{end}}>ChordPro</option>
            <option value="opensong"  {{if eq .Arrangement.Format "opensong"}}selected{{end}}>OpenSong</option>
            <option value="text"      {{if eq .Arrangement.Format "text"}}selected{{end}}>Plain lyrics</option>
            <option value="manual"    {{if eq .Arrangement.Format "manual"}}selected{{end}}>Manual (no parsing)</option>
        </select>
    </label>

    <label>Primary language
        <select name="primary_language">
            {{$primary := .Arrangement.PrimaryLanguage}}
            {{if not $primary}}{{$primary = "en"}}{{end}}
            {{range languageOptions}}
                <option value="{{.Code}}" {{if eq .Code $primary}}selected{{end}}>{{.Label}}</option>
            {{end}}
        </select>
    </label>
    <p class="muted">Translations in other languages are managed from the arrangement's detail page.</p>'''
if old not in src:
    print("   ! Format select not found — abort")
    raise SystemExit(1)
p.write_text(src.replace(old, new, 1))
print("   ✓ arrangement_form.html updated")
PYEOF

# practice.html: add language switcher (optional, but useful for volunteers)
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/templates/practice.html")
src = p.read_text()
if "lang-switcher" in src:
    print("   (already present)")
    raise SystemExit(0)
old = '''    <div class="meta">
        <span><strong>Key:</strong> {{.DisplayKey}}{{if .Offset}} ({{if gt .Offset 0}}+{{end}}{{.Offset}}){{end}}</span>
        {{if .Arrangement.BPM}}<span><strong>BPM:</strong> {{.Arrangement.BPM}}</span>{{end}}
        {{if .Arrangement.LengthSeconds}}<span><strong>Length:</strong> {{.Arrangement.LengthSeconds}}s</span>{{end}}
    </div>'''
new = '''    <div class="meta">
        <span><strong>Key:</strong> {{.DisplayKey}}{{if .Offset}} ({{if gt .Offset 0}}+{{end}}{{.Offset}}){{end}}</span>
        {{if .Arrangement.BPM}}<span><strong>BPM:</strong> {{.Arrangement.BPM}}</span>{{end}}
        {{if .Arrangement.LengthSeconds}}<span><strong>Length:</strong> {{.Arrangement.LengthSeconds}}s</span>{{end}}
        {{if .ResolvedLang}}<span><strong>Language:</strong> {{languageLabel .ResolvedLang}}</span>{{end}}
    </div>

    {{if or .Translations (ne .ResolvedLang .Arrangement.PrimaryLanguage)}}
    <div class="lang-switcher">
        <a href="/practice/{{.Token}}" class="lang-chip {{if eq .ResolvedLang .Arrangement.PrimaryLanguage}}active{{end}}">{{languageLabel .Arrangement.PrimaryLanguage}}</a>
        {{range .Translations}}
        <a href="/practice/{{$.Token}}?lang={{.Language}}" class="lang-chip {{if eq .Language $.ResolvedLang}}active{{end}}">{{languageLabel .Language}}</a>
        {{end}}
    </div>
    {{end}}
    {{if .TranslationMissing}}<p class="muted">No translation in {{languageLabel .RequestedLang}} yet — showing primary.</p>{{end}}'''
if old not in src:
    print("   ! practice meta block not found — abort")
    raise SystemExit(1)
p.write_text(src.replace(old, new, 1))
print("   ✓ practice.html updated")
PYEOF

# list.html: add Suggest button
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/templates/list.html")
src = p.read_text()
if "/songs/suggest" in src:
    print("   (already present)")
    raise SystemExit(0)
needle = '<a class="button-secondary" href="/songs/reports/health">Health</a>'
if needle not in src:
    print("   ! Health button not found — abort")
    raise SystemExit(1)
src = src.replace(
    needle,
    needle + '\n    <a class="button-secondary" href="/songs/suggest">Suggest</a>',
    1)
p.write_text(src)
print("   ✓ list.html updated")
PYEOF

echo "▸ [16/17] CSS additions"
cat >> internal/web/static/css/app.css <<'CSSEOF'

/* Language picker */
.lang-picker, .lang-switcher { display: flex; flex-wrap: wrap; gap: .4rem; align-items: center; margin: .75rem 0 1.25rem; }
.lang-chip { display: inline-flex; align-items: center; gap: .3rem; padding: .35rem .7rem; background: #fff; border: 1px solid var(--border); border-radius: 999px; text-decoration: none; color: var(--fg); font-size: .9em; }
.lang-chip:hover { border-color: var(--accent); }
.lang-chip.active { border-color: var(--accent); background: #eef2f7; font-weight: 600; }
.lang-chip form { margin: 0; }
.lang-chip .link-btn { padding: 0 .15rem; font-size: 1em; }

/* Primary source disclosure in translation form */
.primary-source { background: #fff; border: 1px solid var(--border); border-radius: 6px; padding: .75rem 1rem; margin-bottom: 1rem; }
.primary-source summary { cursor: pointer; font-weight: 600; }
.primary-source pre { white-space: pre-wrap; word-wrap: break-word; font-size: .85em; margin: .5rem 0 0; }

/* Suggestion cards */
.suggestion-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 1rem; margin: 1rem 0; }
.suggestion-card { background: #fff; border: 1px solid var(--border); border-radius: 8px; padding: 1rem; }
.suggestion-card h3 { margin: 0 0 .25rem; font-size: 1.05em; }
.suggestion-card h3 a { color: var(--accent); text-decoration: none; }
.suggestion-reason { color: #475467; font-size: .9em; margin: .5rem 0; }
CSSEOF

echo "▸ [17/17] Build"
go mod tidy
go build ./... && echo "" && echo "✅ Translations + suggestions complete."
echo ""
echo "Restart:"
echo "  docker compose -f deploy/docker-compose.yml up --build"
echo ""
echo "Verify:"
echo "  1. /songs/suggest                     → pick tags → ranked list"
echo "  2. Arrangement view                   → language chips under 'Language'"
echo "  3. Click '+ Add translation'         → paste FR translation → Save"
echo "  4. Click FR chip                      → chart renders in French"
echo "  5. Transpose +2 while in FR           → chords shift in FR chart"
echo "  6. PDF from FR view                   → 'FR' in subtitle, filename ends _fr.pdf"
echo "  7. Practice link + ?lang=fr           → French chart for volunteer"
echo ""
echo "Optional auto-translate:"
echo "  Set TRANSLATE_API_URL=http://libretranslate:5000 in deploy/.env and add"
echo "  a LibreTranslate container. Then the 'Auto-translate' button appears."
