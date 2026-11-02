package people

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

type UserWithSkills struct {
	auth.User
	Skills string
}

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// List returns all users with their profiles and a comma-separated list of skills.
func (r *Repository) List(ctx context.Context) ([]UserWithSkills, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.name, u.email, u.role, u.phone, u.vocal_low, u.vocal_high,
		       COALESCE(string_agg(t.name || ' · ' || p.name, ', ' ORDER BY t.name, p.position), '') AS skills
		FROM users u
		LEFT JOIN user_positions up ON up.user_id = u.id
		LEFT JOIN positions p ON p.id = up.position_id
		LEFT JOIN teams t ON t.id = p.team_id
		GROUP BY u.id
		ORDER BY u.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserWithSkills
	for rows.Next() {
		var u UserWithSkills
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Phone, &u.VocalLow, &u.VocalHigh, &u.Skills); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Get returns a single user.
func (r *Repository) Get(ctx context.Context, id string) (auth.User, error) {
	var u auth.User
	err := r.db.QueryRow(ctx, `
		SELECT id, name, email, role, phone, vocal_low, vocal_high
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Phone, &u.VocalLow, &u.VocalHigh)
	return u, err
}

// Update saves profile info.
func (r *Repository) Update(ctx context.Context, id, phone, vocalLow, vocalHigh string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET phone = $2, vocal_low = $3, vocal_high = $4 WHERE id = $1
	`, id, phone, vocalLow, vocalHigh)
	return err
}

// PositionsFor returns the position IDs a user is skilled in.
func (r *Repository) PositionsFor(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT position_id FROM user_positions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetPositions replaces all skills for a user.
func (r *Repository) SetPositions(ctx context.Context, userID string, positionIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM user_positions WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, posID := range positionIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO user_positions (user_id, position_id) VALUES ($1, $2)`, userID, posID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}


// Create inserts a new user, hashing the password.
func (r *Repository) Create(ctx context.Context, name, email, password, role string) (string, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}
	var id string
	err = r.db.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, name, email, hash, role).Scan(&id)
	return id, err
}
