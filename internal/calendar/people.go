package calendar

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Person struct{ ID, Name string }

// PeopleLister feeds the "Person" dropdown for admins/managers.
// It is an interface so the calendar module does not own knowledge of the users
// table (README module rules). Ideally the people module implements it; until
// then pgPeople below does the one small query.
type PeopleLister interface {
	ListActive(ctx context.Context) ([]Person, error)
}

type pgPeople struct{ db *pgxpool.Pool }

func NewPGPeople(db *pgxpool.Pool) PeopleLister { return &pgPeople{db: db} }

func (p *pgPeople) ListActive(ctx context.Context) ([]Person, error) {
	rows, err := p.db.Query(ctx,
		`SELECT id::text, name FROM users WHERE status <> 'suspended' ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Person
	for rows.Next() {
		var x Person
		if err := rows.Scan(&x.ID, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
