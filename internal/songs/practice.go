package songs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPracticeTokenNotFound = errors.New("practice token not found, expired, or revoked")

type PracticeToken struct {
	Token         string
	SongID        string
	ArrangementID string
	DisplayKey    string
	ExpiresAt     time.Time
	Revoked       bool
	CreatedAt     time.Time
}

type PracticeRepository struct{ db *pgxpool.Pool }

func NewPracticeRepository(db *pgxpool.Pool) *PracticeRepository {
	return &PracticeRepository{db: db}
}

func (r *PracticeRepository) Create(ctx context.Context, songID, arrangementID, displayKey string, ttl time.Duration) (PracticeToken, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return PracticeToken{}, err
	}
	token := hex.EncodeToString(b)
	expiresAt := time.Now().Add(ttl)

	var arr any
	if arrangementID != "" {
		arr = arrangementID
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO practice_tokens (token, song_id, arrangement_id, display_key, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, token, songID, arr, displayKey, expiresAt); err != nil {
		return PracticeToken{}, err
	}
	return PracticeToken{
		Token: token, SongID: songID, ArrangementID: arrangementID,
		DisplayKey: displayKey, ExpiresAt: expiresAt,
	}, nil
}

func (r *PracticeRepository) Get(ctx context.Context, token string) (PracticeToken, error) {
	var p PracticeToken
	var arrID *string
	err := r.db.QueryRow(ctx, `
		SELECT token, song_id, arrangement_id, display_key, expires_at, revoked, created_at
		FROM practice_tokens WHERE token = $1
	`, token).Scan(&p.Token, &p.SongID, &arrID, &p.DisplayKey, &p.ExpiresAt, &p.Revoked, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PracticeToken{}, ErrPracticeTokenNotFound
	}
	if err != nil {
		return PracticeToken{}, err
	}
	if arrID != nil {
		p.ArrangementID = *arrID
	}
	if p.Revoked || time.Now().After(p.ExpiresAt) {
		return PracticeToken{}, ErrPracticeTokenNotFound
	}
	return p, nil
}

func (r *PracticeRepository) Revoke(ctx context.Context, token string) error {
	_, err := r.db.Exec(ctx, `UPDATE practice_tokens SET revoked = TRUE WHERE token = $1`, token)
	return err
}
