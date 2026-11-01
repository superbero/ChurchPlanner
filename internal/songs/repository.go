package songs

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("song not found")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const columns = `id, title, author, key, tempo, lyrics, chords, created_at, updated_at`

func scan(row pgx.Row) (Song, error) {
	var s Song
	err := row.Scan(&s.ID, &s.Title, &s.Author, &s.Key, &s.Tempo, &s.Lyrics, &s.Chords, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *Repository) List(ctx context.Context) ([]Song, error) {
	rows, err := r.db.Query(ctx, `SELECT `+columns+` FROM songs ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Song
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Song, error) {
	s, err := scan(r.db.QueryRow(ctx, `SELECT `+columns+` FROM songs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	return s, err
}

func (r *Repository) Create(ctx context.Context, in CreateSongInput) (Song, error) {
	return scan(r.db.QueryRow(ctx, `
		INSERT INTO songs (title, author, key, tempo, lyrics, chords)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+columns,
		in.Title, in.Author, in.Key, in.Tempo, in.Lyrics, in.Chords))
}

func (r *Repository) Update(ctx context.Context, id string, in CreateSongInput) (Song, error) {
	s, err := scan(r.db.QueryRow(ctx, `
		UPDATE songs
		SET title=$2, author=$3, key=$4, tempo=$5, lyrics=$6, chords=$7, updated_at=now()
		WHERE id=$1
		RETURNING `+columns,
		id, in.Title, in.Author, in.Key, in.Tempo, in.Lyrics, in.Chords))
	if errors.Is(err, pgx.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	return s, err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM songs WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
