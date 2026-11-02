package songs

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrArrangementNotFound = errors.New("arrangement not found")
	ErrArrangementKeyNotFound = errors.New("arrangement key not found")
)

type ArrangementRepository struct{ db *pgxpool.Pool }

func NewArrangementRepository(db *pgxpool.Pool) *ArrangementRepository {
	return &ArrangementRepository{db: db}
}

const arrangementColumns = `id, song_id, name, bpm, meter, length_seconds,
	sequence, notes, original_key, source_content, format, primary_language,
	lyrics, chords, is_default, archived, created_at, updated_at`

func scanArrangement(row pgx.Row) (Arrangement, error) {
	var a Arrangement
	err := row.Scan(&a.ID, &a.SongID, &a.Name, &a.BPM, &a.Meter, &a.LengthSeconds,
		&a.Sequence, &a.Notes, &a.OriginalKey, &a.SourceContent, &a.Format,
		&a.PrimaryLanguage,
		&a.Lyrics, &a.Chords, &a.IsDefault, &a.Archived,
		&a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func (r *ArrangementRepository) ListBySong(ctx context.Context, songID string) ([]Arrangement, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+arrangementColumns+` FROM song_arrangements
		WHERE song_id = $1
		ORDER BY is_default DESC, name
	`, songID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Arrangement
	for rows.Next() {
		a, err := scanArrangement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ArrangementRepository) Get(ctx context.Context, id string) (Arrangement, error) {
	a, err := scanArrangement(r.db.QueryRow(ctx,
		`SELECT `+arrangementColumns+` FROM song_arrangements WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Arrangement{}, ErrArrangementNotFound
	}
	return a, err
}

func (r *ArrangementRepository) DefaultForSong(ctx context.Context, songID string) (Arrangement, error) {
	a, err := scanArrangement(r.db.QueryRow(ctx, `
		SELECT `+arrangementColumns+` FROM song_arrangements
		WHERE song_id = $1 AND is_default = TRUE
		LIMIT 1
	`, songID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Arrangement{}, ErrArrangementNotFound
	}
	return a, err
}

// Create inserts an arrangement. If is_default is requested, all siblings are
// demoted first inside a transaction.
func (r *ArrangementRepository) Create(ctx context.Context, songID string, in CreateArrangementInput, isDefault bool) (Arrangement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Arrangement{}, err
	}
	defer tx.Rollback(ctx)

	if isDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE song_arrangements SET is_default = FALSE WHERE song_id = $1`, songID); err != nil {
			return Arrangement{}, err
		}
	}

	format := in.Format
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
		isDefault, in.Archived))
	if err != nil {
		return Arrangement{}, err
	}

	// Auto-create an "Original" key pointing at original_key.
	if a.OriginalKey != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO arrangement_keys (arrangement_id, name, key, is_default)
			VALUES ($1, 'Original', $2, TRUE)
		`, a.ID, a.OriginalKey); err != nil {
			return Arrangement{}, err
		}
	}
	return a, tx.Commit(ctx)
}

func (r *ArrangementRepository) Update(ctx context.Context, id string, in CreateArrangementInput, lyrics, chords string) (Arrangement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Arrangement{}, err
	}
	defer tx.Rollback(ctx)

	// Snapshot the current state IF anything meaningful is changing.
	if _, err := tx.Exec(ctx, `
		INSERT INTO song_arrangement_revisions
			(arrangement_id, name, bpm, meter, length_seconds, sequence, notes,
			 original_key, source_content, format, lyrics, chords, archived)
		SELECT id, name, bpm, meter, length_seconds, sequence, notes,
		       original_key, source_content, format, lyrics, chords, archived
		FROM song_arrangements
		WHERE id = $1
		  AND (source_content  IS DISTINCT FROM $2
		    OR sequence        IS DISTINCT FROM $3
		    OR original_key    IS DISTINCT FROM $4
		    OR name            IS DISTINCT FROM $5
		    OR bpm             IS DISTINCT FROM $6
		    OR meter           IS DISTINCT FROM $7
		    OR length_seconds  IS DISTINCT FROM $8
		    OR notes           IS DISTINCT FROM $9
		    OR lyrics          IS DISTINCT FROM $10
		    OR chords          IS DISTINCT FROM $11)
	`, id, in.SourceContent, in.Sequence, in.OriginalKey, in.Name,
		in.BPM, in.Meter, in.LengthSeconds, in.Notes, lyrics, chords); err != nil {
		return Arrangement{}, err
	}

	updFormat := in.Format
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
		lyrics, chords, in.Archived))
	if errors.Is(err, pgx.ErrNoRows) {
		return Arrangement{}, ErrArrangementNotFound
	}
	return a, tx.Commit(ctx)
}

// RestoreRevision reapplies an old revision as the current state. The Update
// method automatically snapshots the current state first, so restore is safe
// (and reversible).
func (r *ArrangementRepository) RestoreRevision(ctx context.Context, arrangementID, revisionID string) (Arrangement, error) {
	var rev ArrangementRevision
	err := r.db.QueryRow(ctx, `
		SELECT id, arrangement_id, name, bpm, meter, length_seconds, sequence, notes,
		       original_key, source_content, format, lyrics, chords, archived,
		       revision_note, created_at
		FROM song_arrangement_revisions
		WHERE id = $1 AND arrangement_id = $2
	`, revisionID, arrangementID).Scan(&rev.ID, &rev.ArrangementID, &rev.Name, &rev.BPM,
		&rev.Meter, &rev.LengthSeconds, &rev.Sequence, &rev.Notes, &rev.OriginalKey,
		&rev.SourceContent, &rev.Format, &rev.Lyrics, &rev.Chords, &rev.Archived,
		&rev.RevisionNote, &rev.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Arrangement{}, ErrRevisionNotFound
	}
	if err != nil {
		return Arrangement{}, err
	}
	return r.Update(ctx, arrangementID, CreateArrangementInput{
		Name: rev.Name, BPM: rev.BPM, Meter: rev.Meter, LengthSeconds: rev.LengthSeconds,
		Sequence: rev.Sequence, Notes: rev.Notes, OriginalKey: rev.OriginalKey,
		SourceContent: rev.SourceContent, Format: rev.Format, Archived: rev.Archived,
	}, rev.Lyrics, rev.Chords)
}

func (r *ArrangementRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM song_arrangements WHERE id = $1`, id)
	return err
}

func (r *ArrangementRepository) SetDefault(ctx context.Context, songID, arrangementID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE song_arrangements SET is_default = FALSE WHERE song_id = $1`, songID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE song_arrangements SET is_default = TRUE WHERE id = $1 AND song_id = $2`,
		arrangementID, songID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ArrangementSearchResult is a flattened (song, arrangement) row for pickers.
type ArrangementSearchResult struct {
	SongID          string `json:"song_id"`
	SongTitle       string `json:"song_title"`
	SongAuthor      string `json:"song_author"`
	ArrangementID   string `json:"arrangement_id"`
	ArrangementName string `json:"arrangement_name"`
	OriginalKey     string `json:"original_key"`
	PrimaryLanguage string `json:"primary_language"`
}

// SearchArrangements returns (song, arrangement) pairs matching q.
// Empty q returns the first `limit` pairs alphabetically.
func (r *ArrangementRepository) SearchArrangements(ctx context.Context, q string, limit int) ([]ArrangementSearchResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT s.id::text, s.title, s.author,
		       a.id::text, a.name, a.original_key, a.primary_language
		FROM songs s
		JOIN song_arrangements a ON a.song_id = s.id
		WHERE s.archived = FALSE AND a.archived = FALSE
		  AND (
		    $1 = ''
		    OR s.title  ILIKE '%' || $1 || '%'
		    OR s.author ILIKE '%' || $1 || '%'
		    OR s.search_vector @@ plainto_tsquery('english', $1)
		  )
		ORDER BY s.title, a.is_default DESC, a.name
		LIMIT $2
	`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ArrangementSearchResult
	for rows.Next() {
		var x ArrangementSearchResult
		if err := rows.Scan(&x.SongID, &x.SongTitle, &x.SongAuthor,
			&x.ArrangementID, &x.ArrangementName, &x.OriginalKey, &x.PrimaryLanguage); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// --- Keys ---

const keyColumns = `id, arrangement_id, name, key, is_default, created_at, updated_at`

func scanKey(row pgx.Row) (ArrangementKey, error) {
	var k ArrangementKey
	err := row.Scan(&k.ID, &k.ArrangementID, &k.Name, &k.Key, &k.IsDefault, &k.CreatedAt, &k.UpdatedAt)
	return k, err
}

func (r *ArrangementRepository) KeysForArrangement(ctx context.Context, arrangementID string) ([]ArrangementKey, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+keyColumns+` FROM arrangement_keys
		WHERE arrangement_id = $1
		ORDER BY is_default DESC, name
	`, arrangementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArrangementKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *ArrangementRepository) GetKey(ctx context.Context, id string) (ArrangementKey, error) {
	k, err := scanKey(r.db.QueryRow(ctx,
		`SELECT `+keyColumns+` FROM arrangement_keys WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ArrangementKey{}, ErrArrangementKeyNotFound
	}
	return k, err
}

func (r *ArrangementRepository) DefaultKeyForArrangement(ctx context.Context, arrangementID string) (ArrangementKey, error) {
	k, err := scanKey(r.db.QueryRow(ctx, `
		SELECT `+keyColumns+` FROM arrangement_keys
		WHERE arrangement_id = $1 AND is_default = TRUE
		LIMIT 1
	`, arrangementID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ArrangementKey{}, ErrArrangementKeyNotFound
	}
	return k, err
}

func (r *ArrangementRepository) CreateKey(ctx context.Context, arrangementID, name, key string, isDefault bool) (ArrangementKey, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return ArrangementKey{}, err
	}
	defer tx.Rollback(ctx)
	if isDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE arrangement_keys SET is_default = FALSE WHERE arrangement_id = $1`,
			arrangementID); err != nil {
			return ArrangementKey{}, err
		}
	}
	k, err := scanKey(tx.QueryRow(ctx, `
		INSERT INTO arrangement_keys (arrangement_id, name, key, is_default)
		VALUES ($1, $2, $3, $4)
		RETURNING `+keyColumns,
		arrangementID, name, key, isDefault))
	if err != nil {
		return ArrangementKey{}, err
	}
	return k, tx.Commit(ctx)
}

func (r *ArrangementRepository) DeleteKey(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM arrangement_keys WHERE id = $1`, id)
	return err
}

func (r *ArrangementRepository) SetDefaultKey(ctx context.Context, arrangementID, keyID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE arrangement_keys SET is_default = FALSE WHERE arrangement_id = $1`,
		arrangementID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE arrangement_keys SET is_default = TRUE WHERE id = $1 AND arrangement_id = $2`,
		keyID, arrangementID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
