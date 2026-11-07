package calendar

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// ASSUMED SCHEMA - REPLACE BEFORE USE.
// I have not seen your plans/assignments tables. This guesses:
//   plans(id, title, service_at, published, updated_at)
//   plan_assignments(id, plan_id, user_id, position, status, updated_at)
// Send me the migrations and I'll fix the query. Per the README module rules,
// the cleanest home for this is a read model owned by the plans module,
// implementing FeedSource, instead of SQL inside the calendar module.
// ============================================================================

const defaultServiceLength = 90 * time.Minute // TODO: use the plan's real end/duration

type pgFeedSource struct {
	db      *pgxpool.Pool
	baseURL string
}

func NewPGFeedSource(db *pgxpool.Pool, baseURL string) FeedSource {
	return &pgFeedSource{db: db, baseURL: baseURL}
}

func (s *pgFeedSource) AssignmentsFor(ctx context.Context, userID any, from, to time.Time) ([]Event, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, p.id::text, p.title, a.position, a.status,
		       p.service_at, GREATEST(p.updated_at, a.updated_at)
		FROM plan_assignments a
		JOIN plans p ON p.id = a.plan_id
		WHERE a.user_id = $1
		  AND p.published
		  AND p.service_at >= $2 AND p.service_at < $3
		ORDER BY p.service_at
	`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var assignID, planID, title, position, status string
		var at, modified time.Time
		if err := rows.Scan(&assignID, &planID, &title, &position, &status, &at, &modified); err != nil {
			return nil, err
		}
		out = append(out, Event{
			UID:         "assignment-" + assignID + "@churchplanner", // stable: edits update, not duplicate
			Summary:     title + " - " + position,
			Description: "Role: " + position,
			URL:         s.baseURL + "/plans/" + planID,
			Start:       at,
			End:         at.Add(defaultServiceLength),
			Modified:    modified,
			Tentative:   status != "accepted" && status != "confirmed",
		})
	}
	return out, rows.Err()
}
