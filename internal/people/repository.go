package people

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

var ErrEmailTaken = errors.New("email address is already in use")
var ErrInvalidResetToken = errors.New("invalid or expired password reset token")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

type UserWithSkills struct {
	auth.User
	Skills    string
	TeamRoles string
	Status    string
}

func (r *Repository) List(ctx context.Context, q string, status string) ([]UserWithSkills, error) {
	query := `
		SELECT u.id, u.name, u.title, u.email, u.role, u.phone, u.vocal_low, u.vocal_high, u.status,
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
		WHERE 1=1
	`
	var args []any
	argCount := 1

	if q != "" {
		query += fmt.Sprintf(` AND (u.name ILIKE $%d OR u.email ILIKE $%d)`, argCount, argCount)
		args = append(args, "%"+q+"%")
		argCount++
	}
	if status != "" {
		query += fmt.Sprintf(` AND u.status = $%d`, argCount)
		args = append(args, status)
		argCount++
	}

	query += ` ORDER BY u.name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserWithSkills
	for rows.Next() {
		var u UserWithSkills
		if err := rows.Scan(&u.ID, &u.Name, &u.Title, &u.Email, &u.Role,
			&u.Phone, &u.VocalLow, &u.VocalHigh, &u.Status, &u.Skills, &u.TeamRoles); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (auth.User, error) {
	var u auth.User
	err := r.db.QueryRow(ctx, `
		SELECT id, name, title, email, role, phone, vocal_low, vocal_high, status
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Title, &u.Email, &u.Role, &u.Phone, &u.VocalLow, &u.VocalHigh, &u.Status)
	return u, err
}

func (r *Repository) Create(ctx context.Context, name, title, email, password, role string) (string, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}
	var id string
	err = r.db.QueryRow(ctx, `
		INSERT INTO users (name, title, email, password_hash, role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, name, title, email, hash, role).Scan(&id)
	return id, err
}

func (r *Repository) Update(ctx context.Context, id, title, email, phone, vocalLow, vocalHigh, role, status string) error {
	if role == "" {
		role = auth.RoleScheduledViewer
	}
	email = strings.ToLower(strings.TrimSpace(email))
	var exists bool
	if err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = $1 AND id <> $2)
	`, email, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrEmailTaken
	}
	_, err := r.db.Exec(ctx, `
		UPDATE users
		SET title = $2, email = $3, phone = $4, vocal_low = $5, vocal_high = $6, role = $7, status = $8,
		    updated_at = now()
		WHERE id = $1
	`, id, title, email, phone, vocalLow, vocalHigh, role, status)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrEmailTaken
	}
	return err
}

func (r *Repository) CreatePasswordReset(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) DeletePasswordReset(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM password_reset_tokens WHERE token_hash = $1`, tokenHash)
	return err
}

func (r *Repository) HasValidPasswordReset(ctx context.Context, tokenHash string) (bool, error) {
	var valid bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM password_reset_tokens
			WHERE token_hash = $1 AND expires_at > now()
		)
	`, tokenHash).Scan(&valid)
	return valid, err
}

func (r *Repository) CompletePasswordReset(ctx context.Context, tokenHash, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `
		DELETE FROM password_reset_tokens
		WHERE token_hash = $1 AND expires_at > now()
		RETURNING user_id
	`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidResetToken
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1
	`, userID, hash); err != nil {
		return err
	}
	return tx.Commit(ctx)
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

func (r *Repository) BulkCreate(ctx context.Context, users []CreateUserInput) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, u := range users {
		hash, _ := auth.HashPassword(u.Password)
		_, err := tx.Exec(ctx, `
			INSERT INTO users (name, email, password_hash, role)
			VALUES ($1, $2, $3, $4)
		`, u.Name, u.Email, hash, u.Role)
		if err != nil {
			return fmt.Errorf("failed to insert %s: %w", u.Email, err)
		}
	}
	return tx.Commit(ctx)
}

type CreateUserInput struct {
	Name     string
	Email    string
	Password string
	Role     string
}
