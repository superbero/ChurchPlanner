package songs

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SongHealth struct {
	SongID       string
	Title        string
	Author       string
	LastUsed     *time.Time
	DaysSinceUse int
	Uses12m      int
	Uses3m       int
	OriginalKey  string
	AvgDrift     float64
	DriftSamples int
	Category     string
}

type HealthService struct{ db *pgxpool.Pool }

func NewHealthService(db *pgxpool.Pool) *HealthService { return &HealthService{db: db} }

func (s *HealthService) Compute(ctx context.Context) ([]SongHealth, error) {
	// 1. Song-level aggregates
	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.title, s.author,
		       MAX(h.used_on) AS last_used,
		       COUNT(h.id) FILTER (WHERE h.used_on >= now() - interval '12 months') AS uses_12m,
		       COUNT(h.id) FILTER (WHERE h.used_on >= now() - interval '3 months')  AS uses_3m,
		       COALESCE((SELECT original_key FROM song_arrangements
		                 WHERE song_id = s.id AND is_default AND original_key <> ''
		                 LIMIT 1), '') AS orig_key
		FROM songs s
		LEFT JOIN song_key_history h ON h.song_id = s.id
		WHERE s.archived = FALSE
		GROUP BY s.id
		ORDER BY s.title
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		h SongHealth
	}
	var list []row
	byID := map[string]int{}
	for rows.Next() {
		var r SongHealth
		if err := rows.Scan(&r.SongID, &r.Title, &r.Author, &r.LastUsed,
			&r.Uses12m, &r.Uses3m, &r.OriginalKey); err != nil {
			return nil, err
		}
		byID[r.SongID] = len(list)
		list = append(list, row{h: r})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. History keys, compute drift in Go (Postgres can't do semitone math)
	histRows, err := s.db.Query(ctx, `
		SELECT h.song_id, h.key,
		       COALESCE((SELECT original_key FROM song_arrangements
		                 WHERE song_id = h.song_id AND is_default AND original_key <> ''
		                 LIMIT 1), '') AS orig_key
		FROM song_key_history h
	`)
	if err != nil {
		return nil, err
	}
	defer histRows.Close()

	type driftAcc struct {
		sum int
		n   int
	}
	drifts := map[string]*driftAcc{}
	for histRows.Next() {
		var songID, key, origKey string
		if err := histRows.Scan(&songID, &key, &origKey); err != nil {
			return nil, err
		}
		if origKey == "" || key == "" {
			continue
		}
		oi, ok1 := KeyIndex(origKey)
		ci, ok2 := KeyIndex(key)
		if !ok1 || !ok2 {
			continue
		}
		d := ci - oi
		if d > 6 {
			d -= 12
		}
		if d < -6 {
			d += 12
		}
		if d < 0 {
			d = -d
		}
		acc := drifts[songID]
		if acc == nil {
			acc = &driftAcc{}
			drifts[songID] = acc
		}
		acc.sum += d
		acc.n++
	}

	now := time.Now()
	for i := range list {
		h := &list[i].h
		if h.LastUsed != nil {
			h.DaysSinceUse = int(now.Sub(*h.LastUsed).Hours() / 24)
		} else {
			h.DaysSinceUse = -1 // never used
		}
		if acc := drifts[h.SongID]; acc != nil && acc.n > 0 {
			h.AvgDrift = float64(acc.sum) / float64(acc.n)
			h.DriftSamples = acc.n
		}
		h.Category = categorize(h)
	}

	out := make([]SongHealth, len(list))
	for i, r := range list {
		out[i] = r.h
	}
	return out, nil
}

func categorize(h *SongHealth) string {
	switch {
	case h.DaysSinceUse == -1 || h.DaysSinceUse > 365:
		return "stale"
	case h.Uses3m >= 5:
		return "overused"
	case h.AvgDrift >= 2 && h.DriftSamples >= 3:
		return "drifted"
	case h.DaysSinceUse >= 0 && h.DaysSinceUse < 30:
		return "fresh"
	}
	return "ok"
}
