package songs

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TagGroup struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

type Tag struct {
	ID      string `json:"id"`
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
}

type TagGroupWithTags struct {
	Group TagGroup
	Tags  []Tag
}

type TagService struct{ db *pgxpool.Pool }

func NewTagService(db *pgxpool.Pool) *TagService { return &TagService{db: db} }

func (s *TagService) ListGrouped(ctx context.Context) ([]TagGroupWithTags, error) {
	rows, err := s.db.Query(ctx, `
		SELECT g.id, g.name, g.position,
		       t.id, t.group_id, t.name, t.slug
		FROM tag_groups g
		LEFT JOIN tags t ON t.group_id = g.id
		ORDER BY g.position, g.name, t.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TagGroupWithTags
	idx := map[string]int{}
	for rows.Next() {
		var g TagGroup
		var tagID, tagGroupID, tagName, tagSlug *string
		if err := rows.Scan(&g.ID, &g.Name, &g.Position, &tagID, &tagGroupID, &tagName, &tagSlug); err != nil {
			return nil, err
		}
		i, ok := idx[g.ID]
		if !ok {
			out = append(out, TagGroupWithTags{Group: g})
			i = len(out) - 1
			idx[g.ID] = i
		}
		if tagID != nil {
			out[i].Tags = append(out[i].Tags, Tag{
				ID: *tagID, GroupID: *tagGroupID, Name: *tagName, Slug: *tagSlug,
			})
		}
	}
	return out, rows.Err()
}

func (s *TagService) TagsForSong(ctx context.Context, songID string) ([]Tag, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id, t.group_id, t.name, t.slug
		FROM tags t
		JOIN song_tags st ON st.tag_id = t.id
		WHERE st.song_id = $1
		ORDER BY t.name
	`, songID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.GroupID, &t.Name, &t.Slug); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetSongTags replaces all tags for a song. Passing nil clears them.
func (s *TagService) SetSongTags(ctx context.Context, songID string, tagIDs []string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM song_tags WHERE song_id = $1`, songID); err != nil {
		return err
	}
	for _, tagID := range tagIDs {
		if tagID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO song_tags (song_id, tag_id) VALUES ($1, $2)`, songID, tagID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *TagService) CreateGroup(ctx context.Context, name string) (TagGroup, error) {
	var g TagGroup
	err := s.db.QueryRow(ctx, `
		INSERT INTO tag_groups (name)
		VALUES ($1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id, name, position
	`, strings.TrimSpace(name)).Scan(&g.ID, &g.Name, &g.Position)
	return g, err
}

func (s *TagService) CreateTag(ctx context.Context, groupID, name string) (Tag, error) {
	slug := slugify(name)
	var t Tag
	err := s.db.QueryRow(ctx, `
		INSERT INTO tags (group_id, name, slug)
		VALUES ($1, $2, $3)
		ON CONFLICT (group_id, slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id, group_id, name, slug
	`, groupID, strings.TrimSpace(name), slug).Scan(&t.ID, &t.GroupID, &t.Name, &t.Slug)
	return t, err
}

func (s *TagService) DeleteTag(ctx context.Context, tagID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM tags WHERE id = $1`, tagID)
	return err
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
