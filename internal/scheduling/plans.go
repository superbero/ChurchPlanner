package scheduling

import (
	"fmt"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTimeConflict     = errors.New("time overlaps with an existing time")
	ErrInvalidTimeRange = errors.New("end time must be after start time")
	ErrPlanNotFound     = errors.New("plan not found")
	ErrItemNotFound     = errors.New("plan item not found")
	ErrPersonNotFound   = errors.New("assignment not found")
)

type PlanRepository struct{ db *pgxpool.Pool }

func NewPlanRepository(db *pgxpool.Pool) *PlanRepository { return &PlanRepository{db: db} }

const planColumns = `id, service_type_id, title, service_at, service_ends_at, service_time_id, notes, published, created_at, updated_at`

func scanPlan(row pgx.Row) (Plan, error) {
	var p Plan
	var stID *string
	err := row.Scan(&p.ID, &p.ServiceTypeID, &p.Title, &p.ServiceAt, &p.ServiceEndsAt,
		&stID, &p.Notes, &p.Published, &p.CreatedAt, &p.UpdatedAt)
	if stID != nil {
		p.ServiceTimeID = *stID
	}
	return p, err
}

func (r *PlanRepository) List(ctx context.Context, serviceTypeID string, limit int, dateRange string) ([]Plan, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Base query
	q := `SELECT ` + planColumns + ` FROM plans WHERE 1=1`
	var args []any
	argCount := 1

	if serviceTypeID != "" {
		q += fmt.Sprintf(` AND service_type_id = $%d`, argCount)
		args = append(args, serviceTypeID)
		argCount++
	}

	// Apply date range logic
	switch dateRange {
	case "upcoming":
		q += ` AND service_at >= now() ORDER BY service_at ASC`
	case "past":
		q += ` AND service_at < now() ORDER BY service_at DESC`
	case "this_month":
		q += ` AND date_trunc('month', service_at) = date_trunc('month', now()) ORDER BY service_at ASC`
	case "last_month":
		q += ` AND date_trunc('month', service_at) = date_trunc('month', now() - interval '1 month') ORDER BY service_at DESC`
	default:
		// Default to a reasonable window if no range is provided
		q += ` AND service_at >= now() - interval '30 days' ORDER BY service_at ASC`
	}

	q += fmt.Sprintf(` LIMIT $%d`, argCount)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PlanRepository) Get(ctx context.Context, id string) (Plan, error) {
	p, err := scanPlan(r.db.QueryRow(ctx, `SELECT `+planColumns+` FROM plans WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	return p, err
}

func (r *PlanRepository) Create(ctx context.Context, in CreatePlanInput) (Plan, error) {
	return scanPlan(r.db.QueryRow(ctx, `
		INSERT INTO plans (service_type_id, title, service_at, service_ends_at, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+planColumns,
		in.ServiceTypeID, in.Title, in.ServiceAt, in.ServiceEndsAt, in.Notes))
}

func (r *PlanRepository) CreateWithServiceTime(ctx context.Context, in CreatePlanInput) (Plan, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback(ctx)

	plan, err := scanPlan(tx.QueryRow(ctx, `
		INSERT INTO plans (service_type_id, title, service_at, service_ends_at, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+planColumns,
		in.ServiceTypeID, in.Title, in.ServiceAt, in.ServiceEndsAt, in.Notes))
	if err != nil {
		return Plan{}, err
	}

	var serviceTimeID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO plan_times (plan_id, kind, name, starts_at, ends_at)
		VALUES ($1, 'service', 'Service', $2, $3)
		RETURNING id
	`, plan.ID, in.ServiceAt, in.ServiceEndsAt).Scan(&serviceTimeID); err != nil {
		return Plan{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE plans SET service_time_id = $2 WHERE id = $1
	`, plan.ID, serviceTimeID); err != nil {
		return Plan{}, err
	}
	plan.ServiceTimeID = serviceTimeID
	return plan, tx.Commit(ctx)
}

func (r *PlanRepository) Update(ctx context.Context, id string, in CreatePlanInput) (Plan, error) {
	p, err := scanPlan(r.db.QueryRow(ctx, `
		UPDATE plans SET
			title=$2, service_at=$3, service_ends_at=$4, notes=$5, updated_at=now()
		WHERE id=$1
		RETURNING `+planColumns,
		id, in.Title, in.ServiceAt, in.ServiceEndsAt, in.Notes))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	return p, err
}

func (r *PlanRepository) Publish(ctx context.Context, id string, published bool) error {
	_, err := r.db.Exec(ctx, `UPDATE plans SET published = $2, updated_at = now() WHERE id = $1`, id, published)
	return err
}

func (r *PlanRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM plans WHERE id = $1`, id)
	return err
}

// --- Times ---

func (r *PlanRepository) ListTimes(ctx context.Context, planID string) ([]PlanTime, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, plan_id, kind, name, starts_at, ends_at
		FROM plan_times WHERE plan_id = $1 ORDER BY position, starts_at
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanTime
	for rows.Next() {
		var t PlanTime
		if err := rows.Scan(&t.ID, &t.PlanID, &t.Kind, &t.Name, &t.StartsAt, &t.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateTime inserts a plan time and returns its id.
func (r *PlanRepository) CreateTime(ctx context.Context, planID, kind, name string, starts, ends time.Time) (PlanTime, error) {
	var t PlanTime
	err := r.db.QueryRow(ctx, `
		INSERT INTO plan_times (plan_id, kind, name, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, plan_id, kind, name, starts_at, ends_at
	`, planID, kind, name, starts, ends).Scan(&t.ID, &t.PlanID, &t.Kind, &t.Name, &t.StartsAt, &t.EndsAt)
	return t, err
}

// UpdateTime changes the start/end of an existing plan time.
func (r *PlanRepository) UpdateTime(ctx context.Context, timeID, kind, name string, starts, ends time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE plan_times
		SET kind = $2, name = $3, starts_at = $4, ends_at = $5
		WHERE id = $1
	`, timeID, kind, name, starts, ends)
	return err
}

// SetServiceTimeID records which time slot is the plan's primary "Service".
func (r *PlanRepository) SetServiceTimeID(ctx context.Context, planID, timeID string) error {
	_, err := r.db.Exec(ctx, `UPDATE plans SET service_time_id = $2 WHERE id = $1`, planID, timeID)
	return err
}

func (r *PlanRepository) DeleteTime(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM plan_times WHERE id = $1`, id)
	return err
}

// ReorderTimes renumbers the times of a plan in the given order.
func (r *PlanRepository) ReorderTimes(ctx context.Context, planID string, timeIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, id := range timeIDs {
		_, err := tx.Exec(ctx, `
			UPDATE plan_times SET position = $2
			WHERE id = $1 AND plan_id = $3
		`, id, i, planID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// DB returns the underlying pool (used by the handler for skill lookups).
func (r *PlanRepository) DB() *pgxpool.Pool { return r.db }

// DuplicateTime copies a time block and all its plan items to a new time.
func (r *PlanRepository) DuplicateTime(ctx context.Context, planID, timeID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var kind, name string
	var startsAt, endsAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT kind, name, starts_at, ends_at FROM plan_times WHERE id = $1
	`, timeID).Scan(&kind, &name, &startsAt, &endsAt)
	if err != nil {
		return err
	}

	// Shift the new time to start 1 hour after the original ends
	newStarts := endsAt.Add(1 * time.Hour)
	newEnds := newStarts.Add(endsAt.Sub(startsAt))

	var newTimeID string
	err = tx.QueryRow(ctx, `
		INSERT INTO plan_times (plan_id, kind, name, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, planID, kind, name+" (Copy)", newStarts, newEnds).Scan(&newTimeID)
	if err != nil {
		return err
	}

	// Copy all items from the old time to the new time
	_, err = tx.Exec(ctx, `
		INSERT INTO plan_items (plan_id, plan_time_id, position, kind, title, arrangement_id, key_id, key, duration_minutes, notes)
		SELECT plan_id, $1, position, kind, title, arrangement_id, key_id, key, duration_minutes, notes
		FROM plan_items WHERE plan_time_id = $2
	`, newTimeID, timeID)

	return tx.Commit(ctx)
}

// CheckTimeConflict verifies that the requested time is valid and does not
// overlap with any existing plan time.
func (r *PlanRepository) CheckTimeConflict(ctx context.Context, planID string, starts, ends time.Time, excludeID string) error {
	if !ends.After(starts) {
		return ErrInvalidTimeRange
	}

	var count int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM plan_times
		WHERE plan_id = $1
		  AND starts_at < $3
		  AND ends_at > $2
	`, planID, starts, ends).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return ErrTimeConflict
	}

	return nil
}
