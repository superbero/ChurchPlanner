package songs

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRevisionNotFound = errors.New("revision not found")

type ArrangementRevision struct {
	ID            string
	ArrangementID string
	Name          string
	BPM           int
	Meter         string
	LengthSeconds int
	Sequence      string
	Notes         string
	OriginalKey   string
	SourceContent string
	Format        string
	Lyrics        string
	Chords        string
	Archived      bool
	RevisionNote  string
	CreatedAt     time.Time
}

type RevisionRepository struct{ db *pgxpool.Pool }

func NewRevisionRepository(db *pgxpool.Pool) *RevisionRepository {
	return &RevisionRepository{db: db}
}

func (r *RevisionRepository) List(ctx context.Context, arrangementID string) ([]ArrangementRevision, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, arrangement_id, name, bpm, meter, length_seconds, sequence, notes,
		       original_key, source_content, format, lyrics, chords, archived,
		       revision_note, created_at
		FROM song_arrangement_revisions
		WHERE arrangement_id = $1
		ORDER BY created_at DESC
	`, arrangementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ArrangementRevision
	for rows.Next() {
		var rev ArrangementRevision
		if err := rows.Scan(&rev.ID, &rev.ArrangementID, &rev.Name, &rev.BPM, &rev.Meter,
			&rev.LengthSeconds, &rev.Sequence, &rev.Notes, &rev.OriginalKey,
			&rev.SourceContent, &rev.Format, &rev.Lyrics, &rev.Chords,
			&rev.Archived, &rev.RevisionNote, &rev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

func (r *RevisionRepository) Get(ctx context.Context, id string) (ArrangementRevision, error) {
	var rev ArrangementRevision
	err := r.db.QueryRow(ctx, `
		SELECT id, arrangement_id, name, bpm, meter, length_seconds, sequence, notes,
		       original_key, source_content, format, lyrics, chords, archived,
		       revision_note, created_at
		FROM song_arrangement_revisions WHERE id = $1
	`, id).Scan(&rev.ID, &rev.ArrangementID, &rev.Name, &rev.BPM, &rev.Meter,
		&rev.LengthSeconds, &rev.Sequence, &rev.Notes, &rev.OriginalKey,
		&rev.SourceContent, &rev.Format, &rev.Lyrics, &rev.Chords,
		&rev.Archived, &rev.RevisionNote, &rev.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ArrangementRevision{}, ErrRevisionNotFound
	}
	return rev, err
}
