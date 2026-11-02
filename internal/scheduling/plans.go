package scheduling

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPlanNotFound   = errors.New("plan not found")
	ErrItemNotFound   = errors.New("plan item not found")
	ErrPersonNotFound = errors.New("assignment not found")
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

func (r *PlanRepository) ListUpcoming(ctx context.Context, serviceTypeID string, limit int) ([]Plan, error) {
	if limit <= 0 {
		limit = 30
	}
	var rows pgx.Rows
	var err error
	if serviceTypeID == "" {
		rows, err = r.db.Query(ctx, `
			SELECT `+planColumns+` FROM plans
			WHERE service_at >= now() - interval '30 days'
			ORDER BY service_at ASC LIMIT $1
		`, limit)
	} else {
		rows, err = r.db.Query(ctx, `
			SELECT `+planColumns+` FROM plans
			WHERE service_type_id = $1 AND service_at >= now() - interval '30 days'
			ORDER BY service_at ASC LIMIT $2
		`, serviceTypeID, limit)
	}
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
func (r *PlanRepository) UpdateTime(ctx context.Context, id string, starts, ends time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE plan_times SET starts_at = $2, ends_at = $3 WHERE id = $1
	`, id, starts, ends)
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
