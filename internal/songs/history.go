package songs

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type KeyHistory struct {
	ID        string    `json:"id"`
	SongID    string    `json:"song_id"`
	UsedOn    time.Time `json:"used_on"`
	Key       string    `json:"key"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

type HistoryRepository struct{ db *pgxpool.Pool }

func NewHistoryRepository(db *pgxpool.Pool) *HistoryRepository {
	return &HistoryRepository{db: db}
}

func (r *HistoryRepository) List(ctx context.Context, songID string) ([]KeyHistory, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, song_id, used_on, key, notes, created_at
		FROM song_key_history
		WHERE song_id = $1
		ORDER BY used_on DESC, created_at DESC
	`, songID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KeyHistory
	for rows.Next() {
		var h KeyHistory
		if err := rows.Scan(&h.ID, &h.SongID, &h.UsedOn, &h.Key, &h.Notes, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *HistoryRepository) Add(ctx context.Context, h KeyHistory) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO song_key_history (song_id, used_on, key, notes)
		VALUES ($1, $2, $3, $4)
	`, h.SongID, h.UsedOn, h.Key, h.Notes)
	return err
}
