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
