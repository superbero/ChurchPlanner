package people

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

type UserWithSkills struct {
	auth.User
	Skills    string
	TeamRoles string
}

func (r *Repository) List(ctx context.Context) ([]UserWithSkills, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.name, u.email, u.role, u.phone, u.vocal_low, u.vocal_high,
		       COALESCE((
		           SELECT string_agg(t.name || ' · ' || p.name, ', ' ORDER BY t.name, p.position)
		           FROM user_positions up
		           JOIN positions p ON p.id = up.position_id
		           JOIN teams t ON t.id = p.team_id
		           WHERE up.user_id = u.id
		       ), ''),
		       COALESCE((
		           SELECT string_agg(t.name || ' (' || tm.role || ')', ', ' ORDER BY t.name)
		           FROM team_memberships tm
		           JOIN teams t ON t.id = tm.team_id
		           WHERE tm.user_id = u.id
		       ), '')
		FROM users u
		ORDER BY u.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserWithSkills
	for rows.Next() {
		var u UserWithSkills
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role,
			&u.Phone, &u.VocalLow, &u.VocalHigh, &u.Skills, &u.TeamRoles); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (auth.User, error) {
	var u auth.User
	err := r.db.QueryRow(ctx, `
		SELECT id, name, email, role, phone, vocal_low, vocal_high
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Phone, &u.VocalLow, &u.VocalHigh)
	return u, err
}

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

func (r *Repository) Update(ctx context.Context, id, phone, vocalLow, vocalHigh, role string) error {
	if role == "" {
		role = "volunteer"
	}
	_, err := r.db.Exec(ctx, `
		UPDATE users SET phone = $2, vocal_low = $3, vocal_high = $4, role = $5
		WHERE id = $1
	`, id, phone, vocalLow, vocalHigh, role)
	return err
}

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

type TeamRole struct {
	TeamID   string
	TeamName string
	Role     string
}

func (r *Repository) TeamRolesFor(ctx context.Context, userID string) ([]TeamRole, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.name, tm.role
		FROM team_memberships tm
		JOIN teams t ON t.id = tm.team_id
		WHERE tm.user_id = $1
		ORDER BY t.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamRole
	for rows.Next() {
		var t TeamRole
		if err := rows.Scan(&t.TeamID, &t.TeamName, &t.Role); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
