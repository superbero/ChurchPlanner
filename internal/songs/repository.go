package songs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("song not found")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const songColumns = `id, title, author, youtube_url, ccli_number,
	copyright_holder, copyright_date, publisher, scripture, notes,
	archived, created_at, updated_at`

func scanSong(row pgx.Row) (Song, error) {
	var s Song
	err := row.Scan(&s.ID, &s.Title, &s.Author, &s.YouTubeURL, &s.CCLINumber,
		&s.CopyrightHolder, &s.CopyrightDate, &s.Publisher, &s.Scripture,
		&s.Notes, &s.Archived, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *Repository) List(ctx context.Context) ([]Song, error) {
	return r.Search(ctx, Filter{})
}

func (r *Repository) Search(ctx context.Context, f Filter) ([]Song, error) {
	var args []any
	var conds []string

	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, q)
		conds = append(conds, fmt.Sprintf("search_vector @@ plainto_tsquery('english', $%d)", len(args)))
	}
	if k := strings.TrimSpace(f.Key); k != "" {
		args = append(args, k)
		conds = append(conds, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM song_arrangements a
			WHERE a.song_id = songs.id AND a.original_key = $%d
			UNION
			SELECT 1 FROM song_arrangements a
			JOIN arrangement_keys ak ON ak.arrangement_id = a.id
			WHERE a.song_id = songs.id AND ak.key = $%d
		)`, len(args), len(args)))
	}
	if !f.IncludeArchived {
		conds = append(conds, "archived = FALSE")
	}
	if len(f.TagSlugs) > 0 {
		args = append(args, f.TagSlugs)
		args = append(args, len(f.TagSlugs))
		conds = append(conds, fmt.Sprintf(
			`(SELECT COUNT(*) FROM song_tags st JOIN tags t ON t.id = st.tag_id
			  WHERE st.song_id = songs.id AND t.slug = ANY($%d)) = $%d`,
			len(args)-1, len(args)))
	}

	where := "TRUE"
	if len(conds) > 0 {
		where = strings.Join(conds, " AND ")
	}
	orderBy := "title"
	switch f.Sort {
	case "created":
		orderBy = "created_at DESC"
	case "updated":
		orderBy = "updated_at DESC"
	}

	q := fmt.Sprintf(`SELECT %s FROM songs WHERE %s ORDER BY %s`, songColumns, where, orderBy)
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Song
	for rows.Next() {
		s, err := scanSong(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Song, error) {
	s, err := scanSong(r.db.QueryRow(ctx, `SELECT `+songColumns+` FROM songs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	return s, err
}

func (r *Repository) Create(ctx context.Context, in CreateSongInput) (Song, error) {
	return scanSong(r.db.QueryRow(ctx, `
		INSERT INTO songs (
			title, author, youtube_url, ccli_number,
			copyright_holder, copyright_date, publisher, scripture, notes, archived
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING `+songColumns,
		in.Title, in.Author, in.YouTubeURL, in.CCLINumber,
		in.CopyrightHolder, in.CopyrightDate, in.Publisher, in.Scripture,
		in.Notes, in.Archived))
}

func (r *Repository) Update(ctx context.Context, id string, in CreateSongInput) (Song, error) {
	s, err := scanSong(r.db.QueryRow(ctx, `
		UPDATE songs SET
			title=$2, author=$3, youtube_url=$4, ccli_number=$5,
			copyright_holder=$6, copyright_date=$7, publisher=$8,
			scripture=$9, notes=$10, archived=$11, updated_at=now()
		WHERE id=$1
		RETURNING `+songColumns,
		id, in.Title, in.Author, in.YouTubeURL, in.CCLINumber,
		in.CopyrightHolder, in.CopyrightDate, in.Publisher, in.Scripture,
		in.Notes, in.Archived))
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
