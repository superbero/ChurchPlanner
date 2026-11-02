package scheduling

import "time"

type ServiceType struct {
	ID       string
	Name     string
	Position int
}

type Plan struct {
	ID            string
	ServiceTypeID string
	Title         string
	ServiceAt     time.Time
	ServiceEndsAt time.Time
	ServiceTimeID string
	Notes         string
	Published     bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PlanTime struct {
	ID       string
	PlanID   string
	Kind     string
	Name     string
	StartsAt time.Time
	EndsAt   time.Time
}

type Team struct {
	ID   string
	Name string
}

type Position struct {
	ID       string
	TeamID   string
	Name     string
	Position int
	TeamName string // populated on join
}

type PlanPerson struct {
	ID          string
	PlanID      string
	PersonID    string
	PositionID  string
	Status      string
	DeclineNote string
	// joined
	PersonName   string
	PositionName string
	TeamName     string
}

type PlanItem struct {
	ID              string
	PlanID          string
	PlanTimeID      string
	Position        int
	Kind            string
	Title           string
	ArrangementID   string
	KeyID           string
	Key             string
	DurationMinutes int
	Notes           string
	// joined for display
	SongID          string
	SongTitle       string
	ArrangementName string
}

// PlanTimeWithItems is one time slot with its own order of service.
type PlanTimeWithItems struct {
	Time  PlanTime
	Items []PlanItem
}

// UpdatePlanItemInput carries the fields editable on an existing item.
type UpdatePlanItemInput struct {
	Title           string
	Key             string
	DurationMinutes int
	Notes           string
	PlanTimeID      string
}

type CreatePlanInput struct {
	ServiceTypeID string
	Title         string
	ServiceAt     time.Time
	ServiceEndsAt time.Time
	Notes         string
}

type CreatePlanItemInput struct {
	Kind            string
	Title           string
	ArrangementID   string
	Key             string
	DurationMinutes int
	Notes           string
	PlanTimeID      string
}

// MyAssignment is one row in the volunteer's own schedule.
type MyAssignment struct {
	PlanPersonID   string
	PlanID         string
	PlanTitle      string
	ServiceAt      time.Time
	ServiceType    string
	PositionName   string
	TeamName       string
	Status         string
}


// PersonAssignment is one person on a plan for a given position.
type PersonAssignment struct {
	PlanPersonID string
	PersonID     string
	PersonName   string
	PersonEmail  string
	Status       string // U | C | D
	LastServed   *time.Time
}

// PositionWithPeople is one position (e.g. "Worship Leader") with its current
// assignees and last-filled metadata.
type PositionWithPeople struct {
	PositionID   string
	PositionName string
	Position     int
	Assignees    []PersonAssignment
	LastFilled   *time.Time // when this position was last filled (any plan before this one)
}

// TeamWithPositions groups positions under their team.
type TeamWithPositions struct {
	TeamID    string
	TeamName  string
	Positions []PositionWithPeople
}
