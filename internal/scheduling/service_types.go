package scheduling

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ServiceTypeRepository struct{ db *pgxpool.Pool }

func NewServiceTypeRepository(db *pgxpool.Pool) *ServiceTypeRepository {
	return &ServiceTypeRepository{db: db}
}

func (r *ServiceTypeRepository) List(ctx context.Context) ([]ServiceType, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, position FROM service_types ORDER BY position, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceType
	for rows.Next() {
		var s ServiceType
		if err := rows.Scan(&s.ID, &s.Name, &s.Position); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ServiceTypeRepository) Create(ctx context.Context, name string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO service_types (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, strings.TrimSpace(name))
	return err
}
