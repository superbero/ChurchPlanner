package scheduling

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

type AssignmentRepository struct{ db *pgxpool.Pool }

func NewAssignmentRepository(db *pgxpool.Pool) *AssignmentRepository {
	return &AssignmentRepository{db: db}
}

// --- Plan items ---

const itemColumns = `id, plan_id, position, kind, title, arrangement_id, key_id, key, duration_minutes, notes, plan_time_id`

func scanItem(row pgx.Row) (PlanItem, error) {
	var i PlanItem
	var arrID, keyID, planTimeID *string
	err := row.Scan(&i.ID, &i.PlanID, &i.Position, &i.Kind, &i.Title, &arrID, &keyID,
		&i.Key, &i.DurationMinutes, &i.Notes, &planTimeID)
	if arrID != nil {
		i.ArrangementID = *arrID
	}
	if keyID != nil {
		i.KeyID = *keyID
	}
	if planTimeID != nil {
		i.PlanTimeID = *planTimeID
	}
	return i, err
}

func (r *AssignmentRepository) ListItems(ctx context.Context, planID string) ([]PlanItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT pi.id, pi.plan_id, pi.position, pi.kind, pi.title,
		       pi.arrangement_id, pi.key_id, pi.key, pi.duration_minutes, pi.notes,
		       pi.plan_time_id,
		       COALESCE(a.song_id::text, ''),
		       COALESCE(s.title, ''),
		       COALESCE(a.name, '')
		FROM plan_items pi
		LEFT JOIN song_arrangements a ON a.id = pi.arrangement_id
		LEFT JOIN songs s ON s.id = a.song_id
		WHERE pi.plan_id = $1
		ORDER BY pi.plan_time_id NULLS LAST, pi.position, pi.created_at
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		var i PlanItem
		var arrID, keyID, planTimeID *string
		if err := rows.Scan(&i.ID, &i.PlanID, &i.Position, &i.Kind, &i.Title,
			&arrID, &keyID, &i.Key, &i.DurationMinutes, &i.Notes, &planTimeID,
			&i.SongID, &i.SongTitle, &i.ArrangementName); err != nil {
			return nil, err
		}
		if arrID != nil {
			i.ArrangementID = *arrID
		}
		if keyID != nil {
			i.KeyID = *keyID
		}
		if planTimeID != nil {
			i.PlanTimeID = *planTimeID
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// CreateItem inserts a song/header/note into a plan. If the item is a song with
// an arrangement + key, it also writes a song_key_history row dated to the plan's
// service date. This is the "key used on a specific date" feature that was a
// placeholder before plans existed.
func (r *AssignmentRepository) CreateItem(ctx context.Context, planID string, in CreatePlanItemInput) (PlanItem, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return PlanItem{}, err
	}
	defer tx.Rollback(ctx)

	var arrID, keyID, planTimeID any
	if in.ArrangementID != "" {
		arrID = in.ArrangementID
	}

	// Resolve plan_time_id: explicit, or auto-pick "service"/only time.
	if in.PlanTimeID != "" {
		planTimeID = in.PlanTimeID
	} else {
		var autoID string
		err := tx.QueryRow(ctx, `
			SELECT id FROM plan_times WHERE plan_id = $1
			ORDER BY CASE WHEN kind = 'service' THEN 0 ELSE 1 END, starts_at
			LIMIT 1
		`, planID).Scan(&autoID)
		if err == nil {
			planTimeID = autoID
		}
		// If no times exist, planTimeID stays nil (allowed)
	}

	// position = max + 1 within the same plan_time (or unassigned bucket)
	var pos int
	if planTimeID != nil {
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(MAX(position), 0) + 1 FROM plan_items
			 WHERE plan_id = $1 AND plan_time_id = $2`,
			planID, planTimeID).Scan(&pos); err != nil {
			return PlanItem{}, err
		}
	} else {
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(MAX(position), 0) + 1 FROM plan_items
			 WHERE plan_id = $1 AND plan_time_id IS NULL`,
			planID).Scan(&pos); err != nil {
			return PlanItem{}, err
		}
	}

	item, err := scanItem(tx.QueryRow(ctx, `
		INSERT INTO plan_items (plan_id, position, kind, title, arrangement_id, key_id, key, duration_minutes, notes, plan_time_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+itemColumns,
		planID, pos, in.Kind, in.Title, arrID, keyID, in.Key, in.DurationMinutes, in.Notes, planTimeID))
	if err != nil {
		return PlanItem{}, err
	}

	// Auto-history for song items
	if in.Kind == "song" && in.ArrangementID != "" && in.Key != "" {
		var serviceAt time.Time
		var songID string
		if err := tx.QueryRow(ctx,
			`SELECT service_at FROM plans WHERE id = $1`, planID).Scan(&serviceAt); err != nil {
			return PlanItem{}, err
		}
		if err := tx.QueryRow(ctx,
			`SELECT song_id FROM song_arrangements WHERE id = $1`, in.ArrangementID).Scan(&songID); err != nil {
			return PlanItem{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO song_key_history (song_id, used_on, key, notes, plan_id)
			VALUES ($1, $2, $3, $4, $5)
		`, songID, serviceAt.Format("2006-01-02"), in.Key, "Scheduled in plan", planID); err != nil {
			return PlanItem{}, err
		}
	}

	return item, tx.Commit(ctx)
}

func (r *AssignmentRepository) DeleteItem(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM plan_items WHERE id = $1`, id)
	return err
}

// --- Plan people ---

func (r *AssignmentRepository) ListPeople(ctx context.Context, planID string) ([]PlanPerson, error) {
	rows, err := r.db.Query(ctx, `
		SELECT pp.id, pp.plan_id, pp.person_id, pp.position_id, pp.status, pp.decline_note,
		       u.name, p.name, t.name
		FROM plan_people pp
		JOIN users u ON u.id = pp.person_id
		JOIN positions p ON p.id = pp.position_id
		JOIN teams t ON t.id = p.team_id
		WHERE pp.plan_id = $1
		ORDER BY t.name, p.position, u.name
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanPerson
	for rows.Next() {
		var p PlanPerson
		if err := rows.Scan(&p.ID, &p.PlanID, &p.PersonID, &p.PositionID, &p.Status, &p.DeclineNote,
			&p.PersonName, &p.PositionName, &p.TeamName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *AssignmentRepository) AssignPerson(ctx context.Context, planID, personID, positionID string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO plan_people (plan_id, person_id, position_id, status)
		VALUES ($1, $2, $3, 'U')
		ON CONFLICT (plan_id, person_id, position_id) DO NOTHING
	`, planID, personID, positionID)
	return err
}

func (r *AssignmentRepository) RemovePerson(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM plan_people WHERE id = $1`, id)
	return err
}

func (r *AssignmentRepository) SetStatus(ctx context.Context, id, status, note string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE plan_people SET status = $2, decline_note = $3, updated_at = now()
		WHERE id = $1
	`, id, status, note)
	return err
}

// MySchedule returns the volunteer's upcoming assignments.
func (r *AssignmentRepository) MySchedule(ctx context.Context, personID string) ([]MyAssignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT pp.id, pl.id, pl.title, pl.service_at,
		       st.name, pos.name, tm.name, pp.status
		FROM plan_people pp
		JOIN plans pl ON pl.id = pp.plan_id
		JOIN service_types st ON st.id = pl.service_type_id
		JOIN positions pos ON pos.id = pp.position_id
		JOIN teams tm ON tm.id = pos.team_id
		WHERE pp.person_id = $1 AND pl.service_at >= now() - interval '7 days'
		ORDER BY pl.service_at ASC
		LIMIT 50
	`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MyAssignment
	for rows.Next() {
		var a MyAssignment
		if err := rows.Scan(&a.PlanPersonID, &a.PlanID, &a.PlanTitle, &a.ServiceAt,
			&a.ServiceType, &a.PositionName, &a.TeamName, &a.Status); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetPlanPersonForUser looks up a single assignment scoped to the user (used
// for confirm/decline so a volunteer can't accept someone else's assignment).
func (r *AssignmentRepository) GetPlanPersonForUser(ctx context.Context, planPersonID, personID string) (PlanPerson, error) {
	var p PlanPerson
	err := r.db.QueryRow(ctx, `
		SELECT id, plan_id, person_id, position_id, status, decline_note
		FROM plan_people WHERE id = $1 AND person_id = $2
	`, planPersonID, personID).Scan(&p.ID, &p.PlanID, &p.PersonID, &p.PositionID, &p.Status, &p.DeclineNote)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanPerson{}, ErrPersonNotFound
	}
	return p, err
}

// --- Teams / positions ---

func (r *AssignmentRepository) ListTeams(ctx context.Context) ([]Team, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name FROM teams ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *AssignmentRepository) ListPositions(ctx context.Context) ([]Position, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.team_id, p.name, p.position, t.name
		FROM positions p JOIN teams t ON t.id = p.team_id
		ORDER BY t.name, p.position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Position
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.ID, &p.TeamID, &p.Name, &p.Position, &p.TeamName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}


// ListAllUsers returns everyone who can be assigned, with their profile info.
func (r *AssignmentRepository) ListAllUsers(ctx context.Context) ([]auth.User, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, email, role, phone, vocal_low, vocal_high FROM users ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []auth.User
	for rows.Next() {
		var u auth.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Phone, &u.VocalLow, &u.VocalHigh); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}


// GetItem fetches a single plan item by id.
func (r *AssignmentRepository) GetItem(ctx context.Context, id string) (PlanItem, error) {
	return scanItem(r.db.QueryRow(ctx,
		`SELECT `+itemColumns+` FROM plan_items WHERE id = $1`, id))
}

// UpdateItem edits the mutable fields of an item.
func (r *AssignmentRepository) UpdateItem(ctx context.Context, id string, in UpdatePlanItemInput) error {
	var planTimeID any
	if in.PlanTimeID != "" {
		planTimeID = in.PlanTimeID
	}
	_, err := r.db.Exec(ctx, `
		UPDATE plan_items SET
			title=$2, key=$3, duration_minutes=$4, notes=$5, plan_time_id=$6
		WHERE id = $1
	`, id, in.Title, in.Key, in.DurationMinutes, in.Notes, planTimeID)
	return err
}

// ReorderItems renumbers the items of one time slot in the given order.
// It also reassigns plan_time_id, so dragging an item from one time slot
// to another works via this same endpoint.
func (r *AssignmentRepository) ReorderItems(ctx context.Context, planID, planTimeID string, itemIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, itemID := range itemIDs {
		_, err := tx.Exec(ctx, `
			UPDATE plan_items SET position = $3, plan_time_id = $4
			WHERE id = $1 AND plan_id = $2
		`, itemID, planID, i+1, planTimeID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CountTimes returns how many times exist for a plan.
func (r *AssignmentRepository) CountTimes(ctx context.Context, planID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM plan_times WHERE plan_id = $1`, planID).Scan(&n)
	return n, err
}


// PeopleServingTree returns teams → positions → current assignees + last-served
// data, scoped to a specific plan. `currentPlanDate` is used as the "now" cutoff
// for "last served" queries: only plans strictly before this date count.
func (r *AssignmentRepository) PeopleServingTree(ctx context.Context, planID string, currentPlanDate time.Time) ([]TeamWithPositions, error) {
	// 1. Positions required for THIS plan, joined with their teams.
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.name, p.id, p.name, p.position
		FROM plan_positions pp
		JOIN positions p ON p.id = pp.position_id
		JOIN teams     t ON t.id = p.team_id
		WHERE pp.plan_id = $1
		ORDER BY t.name, p.position
	`, planID)
	if err != nil {
		return nil, err
	}
	var positions []struct {
		TeamID, TeamName, PositionID, PositionName string
		Position                                   int
	}
	for rows.Next() {
		var p struct {
			TeamID, TeamName, PositionID, PositionName string
			Position                                   int
		}
		if err := rows.Scan(&p.TeamID, &p.TeamName, &p.PositionID, &p.PositionName, &p.Position); err != nil {
			rows.Close()
			return nil, err
		}
		positions = append(positions, p)
	}
	rows.Close()

	if len(positions) == 0 {
		return nil, nil
	}

	// 2. Current assignees on this plan
	assignees, err := r.ListPeople(ctx, planID)
	if err != nil {
		return nil, err
	}
	assigneesByPos := map[string][]PersonAssignment{}
	personIDs := make([]string, 0, len(assignees))
	for _, pp := range assignees {
		assigneesByPos[pp.PositionID] = append(assigneesByPos[pp.PositionID], PersonAssignment{
			PlanPersonID: pp.ID,
			PersonID:     pp.PersonID,
			PersonName:   pp.PersonName,
			Status:       pp.Status,
		})
		personIDs = append(personIDs, pp.PersonID)
	}

	// 3. Last-served per person (strictly before this plan)
	lastServedByPerson := map[string]*time.Time{}
	if len(personIDs) > 0 {
		rows, err := r.db.Query(ctx, `
			SELECT pp.person_id, MAX(pl.service_at)
			FROM plan_people pp
			JOIN plans pl ON pl.id = pp.plan_id
			WHERE pp.person_id = ANY($1::uuid[])
			  AND pl.id <> $2
			  AND pl.service_at < $3
			GROUP BY pp.person_id
		`, personIDs, planID, currentPlanDate)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var pid string
			var last time.Time
			if err := rows.Scan(&pid, &last); err != nil {
				return nil, err
			}
			t := last
			lastServedByPerson[pid] = &t
		}
	}
	for posID := range assigneesByPos {
		for i := range assigneesByPos[posID] {
			if last, ok := lastServedByPerson[assigneesByPos[posID][i].PersonID]; ok {
				assigneesByPos[posID][i].LastServed = last
			}
		}
	}

	// 4. Last-filled per position (strictly before this plan, confirmed only)
	lastFilledByPos := map[string]*time.Time{}
	{
		rows, err := r.db.Query(ctx, `
			SELECT pp.position_id, MAX(pl.service_at)
			FROM plan_people pp
			JOIN plans pl ON pl.id = pp.plan_id
			WHERE pl.id <> $1
			  AND pl.service_at < $2
			  AND pp.status = 'C'
			GROUP BY pp.position_id
		`, planID, currentPlanDate)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var posID string
			var last time.Time
			if err := rows.Scan(&posID, &last); err != nil {
				return nil, err
			}
			t := last
			lastFilledByPos[posID] = &t
		}
	}

	// 5. Assemble tree, one team at a time
	teamOrder := []string{}
	teamName := map[string]string{}
	byTeam := map[string][]PositionWithPeople{}
	for _, p := range positions {
		if _, ok := teamName[p.TeamID]; !ok {
			teamName[p.TeamID] = p.TeamName
			teamOrder = append(teamOrder, p.TeamID)
		}
		byTeam[p.TeamID] = append(byTeam[p.TeamID], PositionWithPeople{
			PositionID:   p.PositionID,
			PositionName: p.PositionName,
			Position:     p.Position,
			Assignees:    assigneesByPos[p.PositionID],
			LastFilled:   lastFilledByPos[p.PositionID],
		})
	}

	var out []TeamWithPositions
	for _, tid := range teamOrder {
		out = append(out, TeamWithPositions{
			TeamID:    tid,
			TeamName:  teamName[tid],
			Positions: byTeam[tid],
		})
	}
	return out, nil
}


// AddNeededPosition marks a position as required for this plan.
func (r *AssignmentRepository) AddNeededPosition(ctx context.Context, planID, positionID string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO plan_positions (plan_id, position_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, planID, positionID)
	return err
}

// RemoveNeededPosition marks a position as no longer needed for this plan.
// Any existing assignments for that position on this plan are removed as well.
func (r *AssignmentRepository) RemoveNeededPosition(ctx context.Context, planID, positionID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM plan_people WHERE plan_id = $1 AND position_id = $2
	`, planID, positionID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM plan_positions WHERE plan_id = $1 AND position_id = $2
	`, planID, positionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}


// PendingNotification carries the data needed to send an email.
type PendingNotification struct {
	PlanPersonID string
	Email        string
	Name         string
	PositionName string
	PlanTitle    string
	ServiceAt    time.Time
}

// GetPendingNotifications finds assignments that are unconfirmed and have not
// been emailed yet. Only looks up to 14 days in advance.
func (r *AssignmentRepository) GetPendingNotifications(ctx context.Context) ([]PendingNotification, error) {
	rows, err := r.db.Query(ctx, `
		SELECT pp.id, u.email, u.name, pos.name, pl.title, pl.service_at
		FROM plan_people pp
		JOIN users u ON u.id = pp.person_id
		JOIN plans pl ON pl.id = pp.plan_id
		JOIN positions pos ON pos.id = pp.position_id
		WHERE pp.status = 'U'
		  AND pp.notified_at IS NULL
		  AND pl.service_at > now()
		  AND pl.service_at < now() + interval '14 days'
		ORDER BY pl.service_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingNotification
	for rows.Next() {
		var n PendingNotification
		if err := rows.Scan(&n.PlanPersonID, &n.Email, &n.Name, &n.PositionName, &n.PlanTitle, &n.ServiceAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *AssignmentRepository) MarkNotified(ctx context.Context, planPersonID string) error {
	_, err := r.db.Exec(ctx, `UPDATE plan_people SET notified_at = now() WHERE id = $1`, planPersonID)
	return err
}
