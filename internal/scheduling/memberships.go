package scheduling

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TeamMembership struct {
	ID     string
	UserID string
	TeamID string
	Role   string // "member" | "leader"
}

type TeamMembershipRepository struct{ db *pgxpool.Pool }

func NewTeamMembershipRepository(db *pgxpool.Pool) *TeamMembershipRepository {
	return &TeamMembershipRepository{db: db}
}

func (r *TeamMembershipRepository) ListForUser(ctx context.Context, userID string) ([]TeamMembership, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, team_id, role FROM team_memberships WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamMembership
	for rows.Next() {
		var m TeamMembership
		if err := rows.Scan(&m.ID, &m.UserID, &m.TeamID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *TeamMembershipRepository) LeaderTeams(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT team_id FROM team_memberships WHERE user_id = $1 AND role = 'leader'
	`, userID)
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

func (r *TeamMembershipRepository) IsLeaderOfAnyTeam(ctx context.Context, userID string) (bool, error) {
	var b bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM team_memberships WHERE user_id = $1 AND role = 'leader')
	`, userID).Scan(&b)
	return b, err
}

func (r *TeamMembershipRepository) CanManageTeam(ctx context.Context, userID, teamID string) (bool, error) {
	var b bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM team_memberships
		              WHERE user_id = $1 AND team_id = $2 AND role = 'leader')
	`, userID, teamID).Scan(&b)
	return b, err
}

// SetForUser replaces all memberships for the given user.
// `memberships` maps team_id -> role ("leader" or "member"). Empty map clears all.
func (r *TeamMembershipRepository) SetForUser(ctx context.Context, userID string, memberships map[string]string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM team_memberships WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for teamID, role := range memberships {
		if role != "leader" && role != "member" {
			role = "member"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO team_memberships (user_id, team_id, role) VALUES ($1, $2, $3)
		`, userID, teamID, role); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// PositionTeamID looks up the team that a position belongs to.
func (r *TeamMembershipRepository) PositionTeamID(ctx context.Context, positionID string) (string, error) {
	var teamID string
	err := r.db.QueryRow(ctx, `SELECT team_id FROM positions WHERE id = $1`, positionID).Scan(&teamID)
	return teamID, err
}

// AssignmentPosition returns the position_id of a plan_people row.
func (r *TeamMembershipRepository) AssignmentPosition(ctx context.Context, planPersonID string) (string, error) {
	var posID string
	err := r.db.QueryRow(ctx, `SELECT position_id FROM plan_people WHERE id = $1`, planPersonID).Scan(&posID)
	return posID, err
}
