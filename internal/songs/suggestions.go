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
