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

const songColumns = `s.id, s.title, s.author, s.youtube_url, s.ccli_number,
	s.copyright_holder, s.copyright_date, s.publisher, s.scripture, s.notes,
	s.archived, s.created_at, s.updated_at,
	COALESCE(a.original_key, '') AS original_key,
	COALESCE(a.bpm, 0) AS bpm,
	COALESCE((SELECT key FROM song_key_history h WHERE h.song_id = s.id ORDER BY used_on DESC LIMIT 1), '') AS last_used_key,
	NOT EXISTS (SELECT 1 FROM song_key_history h WHERE h.song_id = s.id) AS never_used,
	(s.created_at >= now() - INTERVAL '30 days'
		AND NOT EXISTS (SELECT 1 FROM song_key_history h WHERE h.song_id = s.id)) AS recently_added_unused`

func scanSong(row pgx.Row) (Song, error) {
	var s Song
	err := row.Scan(&s.ID, &s.Title, &s.Author, &s.YouTubeURL, &s.CCLINumber,
		&s.CopyrightHolder, &s.CopyrightDate, &s.Publisher, &s.Scripture,
		&s.Notes, &s.Archived, &s.CreatedAt, &s.UpdatedAt,
		&s.OriginalKey, &s.BPM, &s.LastUsedKey, &s.NeverUsed, &s.RecentlyAddedUnused)
	return s, err
}

func (r *Repository) List(ctx context.Context) ([]Song, error) {
	return r.Search(ctx, Filter{})
}

func (r *Repository) Search(ctx context.Context, f Filter) ([]Song, error) {
	args, where := songSearchConditions(f)
	query := fmt.Sprintf(
		`SELECT %s FROM songs s
		LEFT JOIN song_arrangements a ON a.song_id = s.id AND a.is_default = TRUE
		WHERE %s ORDER BY %s`,
		songColumns, where, songOrderBy(f.Sort),
	)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		song, err := scanSong(rows)
		if err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	return songs, rows.Err()
}

func (r *Repository) SearchPage(ctx context.Context, f Filter, pageNumber, pageSize int) (SongPage, error) {
	pageSize = normalizeSongPageSize(pageSize)
	if pageNumber < 1 {
		pageNumber = 1
	}
	args, where := songSearchConditions(f)

	var total int
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM songs s WHERE "+where, args...).Scan(&total); err != nil {
		return SongPage{}, err
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		pageNumber = 1
	} else if pageNumber > totalPages {
		pageNumber = totalPages
	}

	queryArgs := append(append([]any(nil), args...), pageSize, (pageNumber-1)*pageSize)
	baseQuery := `SELECT %s FROM songs s
		LEFT JOIN song_arrangements a ON a.song_id = s.id AND a.is_default = TRUE
		WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`
	query := fmt.Sprintf(baseQuery, songColumns, where, songOrderBy(f.Sort), len(queryArgs)-1, len(queryArgs))
	rows, err := r.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return SongPage{}, err
	}
	defer rows.Close()

	var out []Song
	for rows.Next() {
		song, err := scanSong(rows)
		if err != nil {
			return SongPage{}, err
		}
		out = append(out, song)
	}
	if err := rows.Err(); err != nil {
		return SongPage{}, err
	}
	return SongPage{Songs: out, Total: total, Page: pageNumber, PageSize: pageSize}, nil
}

func songOrderBy(sort string) string {
	switch sort {
	case "created":
		return "s.created_at DESC, s.title, s.id"
	case "updated":
		return "s.updated_at DESC, s.title, s.id"
	case "unused":
		return `NOT EXISTS (SELECT 1 FROM song_key_history h WHERE h.song_id = s.id) DESC,
			s.title, s.id`
	default:
		return "s.title, s.id"
	}
}

func songSearchConditions(f Filter) ([]any, string) {
	var args []any
	var conds []string

	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, q)
		conds = append(conds, fmt.Sprintf("s.search_vector @@ plainto_tsquery('english', $%d)", len(args)))
	}
	if k := strings.TrimSpace(f.Key); k != "" {
		args = append(args, k)
		conds = append(conds, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM song_arrangements sa
			WHERE sa.song_id = s.id AND sa.original_key = $%d
			UNION
			SELECT 1 FROM song_arrangements sa
			JOIN arrangement_keys ak ON ak.arrangement_id = sa.id
			WHERE sa.song_id = s.id AND ak.key = $%d
		)`, len(args), len(args)))
	}
	if !f.IncludeArchived {
		conds = append(conds, "s.archived = FALSE")
	}
	if len(f.TagSlugs) > 0 {
		args = append(args, f.TagSlugs)
		args = append(args, len(f.TagSlugs))
		conds = append(conds, fmt.Sprintf(
			`(SELECT COUNT(*) FROM song_tags st JOIN tags t ON t.id = st.tag_id
			  WHERE st.song_id = s.id AND t.slug = ANY($%d)) = $%d`,
			len(args)-1, len(args)))
	}

	where := "TRUE"
	if len(conds) > 0 {
		where = strings.Join(conds, " AND ")
	}
	return args, where
}

func (r *Repository) Get(ctx context.Context, id string) (Song, error) {
	s, err := scanSong(r.db.QueryRow(ctx, `SELECT `+songColumns+` FROM songs s LEFT JOIN song_arrangements a ON a.song_id = s.id AND a.is_default = TRUE WHERE s.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	return s, err
}

func (r *Repository) Create(ctx context.Context, in CreateSongInput) (Song, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO songs (
			title, author, youtube_url, ccli_number,
			copyright_holder, copyright_date, publisher, scripture, notes, archived
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id
	`,
		in.Title, in.Author, in.YouTubeURL, in.CCLINumber,
		in.CopyrightHolder, in.CopyrightDate, in.Publisher, in.Scripture,
		in.Notes, in.Archived).Scan(&id)
	if err != nil {
		return Song{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repository) Update(ctx context.Context, id string, in CreateSongInput) (Song, error) {
	var updatedID string
	err := r.db.QueryRow(ctx, `
		UPDATE songs SET
			title=$2, author=$3, youtube_url=$4, ccli_number=$5,
			copyright_holder=$6, copyright_date=$7, publisher=$8,
			scripture=$9, notes=$10, archived=$11, updated_at=now()
		WHERE id=$1
		RETURNING id
	`,
		id, in.Title, in.Author, in.YouTubeURL, in.CCLINumber,
		in.CopyrightHolder, in.CopyrightDate, in.Publisher, in.Scripture,
		in.Notes, in.Archived).Scan(&updatedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	if err != nil {
		return Song{}, err
	}
	return r.Get(ctx, updatedID)
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	return r.DeleteMany(ctx, []string{id})
}

func (r *Repository) DeleteMany(ctx context.Context, ids []string) error {
	uniqueIDs := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `DELETE FROM songs WHERE id::text = ANY($1::text[])`, uniqueIDs)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != len(uniqueIDs) {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}
