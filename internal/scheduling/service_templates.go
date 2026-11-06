package scheduling

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrServiceTemplateNotFound = errors.New("service template not found")
var ErrServiceTemplateExists = errors.New("service template already exists")
var ErrInvalidServiceTemplate = errors.New("service template must contain exactly one service section at offset zero")

type ServiceTemplateRepository struct {
	db *pgxpool.Pool
}

func NewServiceTemplateRepository(db *pgxpool.Pool) *ServiceTemplateRepository {
	return &ServiceTemplateRepository{db: db}
}

func (r *ServiceTemplateRepository) List(ctx context.Context) ([]ServiceTemplate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT st.id, st.service_type_id, ty.name, st.name
		FROM service_templates st
		JOIN service_types ty ON ty.id = st.service_type_id
		ORDER BY ty.position, ty.name, st.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []ServiceTemplate
	for rows.Next() {
		var template ServiceTemplate
		if err := rows.Scan(&template.ID, &template.ServiceTypeID, &template.ServiceType, &template.Name); err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range templates {
		templates[i].Times, err = r.ListTimes(ctx, templates[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return templates, nil
}

func (r *ServiceTemplateRepository) ListTimes(ctx context.Context, templateID string) ([]ServiceTemplateTime, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, template_id, position, kind, name, start_offset_minutes, duration_minutes
		FROM service_template_times
		WHERE template_id = $1
		ORDER BY position
	`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var times []ServiceTemplateTime
	for rows.Next() {
		var item ServiceTemplateTime
		if err := rows.Scan(&item.ID, &item.TemplateID, &item.Position, &item.Kind, &item.Name,
			&item.StartOffsetMinutes, &item.DurationMinutes); err != nil {
			return nil, err
		}
		times = append(times, item)
	}
	return times, rows.Err()
}

func (r *ServiceTemplateRepository) Create(ctx context.Context, template ServiceTemplate) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var templateID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO service_templates (service_type_id, name)
		VALUES ($1, $2)
		RETURNING id
	`, template.ServiceTypeID, strings.TrimSpace(template.Name)).Scan(&templateID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrServiceTemplateExists
		}
		return err
	}
	for i, item := range template.Times {
		if _, err := tx.Exec(ctx, `
			INSERT INTO service_template_times
				(template_id, position, kind, name, start_offset_minutes, duration_minutes)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, templateID, i, item.Kind, strings.TrimSpace(item.Name), item.StartOffsetMinutes, item.DurationMinutes); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *ServiceTemplateRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM service_templates WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrServiceTemplateNotFound
	}
	return nil
}

func (r *PlanRepository) CreateWithServiceTemplate(ctx context.Context, in CreatePlanInput, templateID string) (Plan, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback(ctx)

	var templateTypeID string
	if err := tx.QueryRow(ctx, `
		SELECT service_type_id FROM service_templates WHERE id = $1
	`, templateID).Scan(&templateTypeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Plan{}, ErrServiceTemplateNotFound
		}
		return Plan{}, err
	}
	if templateTypeID != in.ServiceTypeID {
		return Plan{}, ErrServiceTemplateNotFound
	}

	rows, err := tx.Query(ctx, `
		SELECT kind, name, start_offset_minutes, duration_minutes
		FROM service_template_times
		WHERE template_id = $1
		ORDER BY position
	`, templateID)
	if err != nil {
		return Plan{}, err
	}
	var times []ServiceTemplateTime
	serviceCount := 0
	invalid := false
	for rows.Next() {
		var item ServiceTemplateTime
		if err := rows.Scan(&item.Kind, &item.Name, &item.StartOffsetMinutes, &item.DurationMinutes); err != nil {
			rows.Close()
			return Plan{}, err
		}
		if item.Kind == "service" {
			serviceCount++
			if item.StartOffsetMinutes != 0 || item.DurationMinutes != 0 {
				invalid = true
			}
		} else if item.StartOffsetMinutes < -1440 || item.StartOffsetMinutes > 1440 ||
			item.DurationMinutes < 1 || item.DurationMinutes > 720 {
			invalid = true
		}
		if item.Kind != "service" && item.Kind != "rehearsal" && item.Kind != "other" {
			invalid = true
		}
		times = append(times, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Plan{}, err
	}
	rows.Close()
	if invalid || serviceCount != 1 || len(times) == 0 {
		return Plan{}, ErrInvalidServiceTemplate
	}

	plan, err := scanPlan(tx.QueryRow(ctx, `
		INSERT INTO plans (service_type_id, title, service_at, service_ends_at, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+planColumns,
		in.ServiceTypeID, in.Title, in.ServiceAt, in.ServiceEndsAt, in.Notes))
	if err != nil {
		return Plan{}, err
	}

	for i, item := range times {
		start, end := in.ServiceAt, in.ServiceEndsAt
		if item.Kind != "service" {
			start = in.ServiceAt.Add(time.Duration(item.StartOffsetMinutes) * time.Minute)
			end = start.Add(time.Duration(item.DurationMinutes) * time.Minute)
		}
		var timeID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO plan_times (plan_id, kind, name, starts_at, ends_at, position)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, plan.ID, item.Kind, item.Name, start, end, i).Scan(&timeID); err != nil {
			return Plan{}, err
		}
		if item.Kind == "service" {
			plan.ServiceTimeID = timeID
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE plans SET service_time_id = $2 WHERE id = $1
	`, plan.ID, plan.ServiceTimeID); err != nil {
		return Plan{}, err
	}
	return plan, tx.Commit(ctx)
}
