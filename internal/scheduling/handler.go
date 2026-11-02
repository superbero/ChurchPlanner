package scheduling

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

type Handler struct {
	svcTypes *ServiceTypeRepository
	plans    *PlanRepository
	assign   *AssignmentRepository
	renderer *httpx.Renderer
}

// ---- Plans list ----

func (h *Handler) PlansIndex(w http.ResponseWriter, r *http.Request) {
	types, err := h.svcTypes.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	selectedType := r.URL.Query().Get("type")
	plans, err := h.plans.ListUpcoming(r.Context(), selectedType, 40)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	typeName := ""
	for _, t := range types {
		if t.ID == selectedType {
			typeName = t.Name
		}
	}
	h.renderer.Render(w, r, http.StatusOK, "scheduling_plans", map[string]any{
		"ServiceTypes": types,
		"Plans":        plans,
		"SelectedType": selectedType,
		"TypeName":     typeName,
	})
}

func (h *Handler) NewPlan(w http.ResponseWriter, r *http.Request) {
	types, err := h.svcTypes.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	defaultStart := time.Now().AddDate(0, 0, 7).Truncate(time.Hour)

	// If the calendar passed ?date=YYYY-MM-DD, use it and default to 10:00 AM
	if dateStr := r.URL.Query().Get("date"); dateStr != "" {
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			defaultStart = t.Add(10 * time.Hour) // 10:00 AM default
		}
	}

	h.renderer.Render(w, r, http.StatusOK, "scheduling_plan_form", map[string]any{
		"ServiceTypes": types,
		"IsNew":        true,
		"Plan":         Plan{ServiceAt: defaultStart, ServiceEndsAt: defaultStart.Add(90 * time.Minute)},
	})
}

func (h *Handler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	in, err := parsePlanInput(r)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	p, err := h.plans.Create(r.Context(), in)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	// Auto-create the primary "Service" time from the plan's service_at.
	// Volunteers never have to re-enter the same date/time.
	_, _ = h.plans.CreateTime(r.Context(), p.ID, "service", "Service",
		p.ServiceAt, p.ServiceAt.Add(90*time.Minute))
	http.Redirect(w, r, "/plans/"+p.ID, http.StatusSeeOther)
}

func (h *Handler) ViewPlan(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	plan, err := h.plans.Get(r.Context(), planID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	times, err := h.plans.ListTimes(r.Context(), planID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	items, err := h.assign.ListItems(r.Context(), planID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	users, err := h.assign.ListAllUsers(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	allPositions, err := h.assign.ListPositions(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	tree, err := h.assign.PeopleServingTree(r.Context(), planID, plan.ServiceAt)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	// Group items by plan_time_id
	byTime := map[string][]PlanItem{}
	var unassigned []PlanItem
	for _, it := range items {
		if it.PlanTimeID == "" {
			unassigned = append(unassigned, it)
		} else {
			byTime[it.PlanTimeID] = append(byTime[it.PlanTimeID], it)
		}
	}
	var timeGroups []PlanTimeWithItems
	for _, t := range times {
		timeGroups = append(timeGroups, PlanTimeWithItems{
			Time:  t,
			Items: byTime[t.ID],
		})
	}

	// NextStart: the plan's own service_at. The user adds a rehearsal
	// separately if they want one — it will usually be earlier than this,
	// not later, so defaulting to "end of the latest time" was confusing.
	nextStart := plan.ServiceAt

	// Fetch users who are blocked out on the plan's date
	blockedMap := make(map[string]bool)
	brows, err := h.plans.DB().Query(r.Context(), `
		SELECT user_id FROM user_blockouts
		WHERE starts_on <= $1 AND ends_on >= $1
	`, plan.ServiceAt.Format("2006-01-02"))
	if err == nil {
		for brows.Next() {
			var uid string
			brows.Scan(&uid)
			blockedMap[uid] = true
		}
		brows.Close()
	}

	// Build a map of PositionID -> []User (only those with the skill)
	usersByPos := make(map[string][]auth.User)
	for _, pos := range allPositions {
		var eligible []auth.User
		for _, u := range users {
			// Check if user has this skill
			var hasSkill bool
			err := h.plans.DB().QueryRow(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM user_positions WHERE user_id = $1 AND position_id = $2)`,
				u.ID, pos.ID).Scan(&hasSkill)
			if err == nil && hasSkill {
				eligible = append(eligible, u)
			}
		}
		// If no one has the skill, fall back to all users (so you aren't blocked)
		if len(eligible) == 0 {
			eligible = users
		}
		usersByPos[pos.ID] = eligible
	}

	h.renderer.Render(w, r, http.StatusOK, "scheduling_plan_view", map[string]any{
		"Plan":         plan,
		"Times":        timeGroups,
		"Unassigned":   unassigned,
		"Teams":        tree,
		"Users":        users,
		"UsersByPos":   usersByPos,
		"AllPositions": allPositions,
		"BlockedUsers": blockedMap,
		"NextStart":    nextStart,
		"HasMultiple":  len(times) > 1,
	})
}

func (h *Handler) EditPlan(w http.ResponseWriter, r *http.Request) {
	plan, err := h.plans.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	types, _ := h.svcTypes.List(r.Context())
	h.renderer.Render(w, r, http.StatusOK, "scheduling_plan_form", map[string]any{
		"ServiceTypes": types,
		"IsNew":        false,
		"Plan":         plan,
	})
}

func (h *Handler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	in, err := parsePlanInput(r)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	before, err := h.plans.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.plans.Update(r.Context(), id, in)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	// If the plan has a tracked Service time and the date/time actually moved,
	// sync that specific slot so the plan view stays consistent.
	if before.ServiceTimeID != "" &&
		(!before.ServiceAt.Equal(p.ServiceAt) || !before.ServiceEndsAt.Equal(p.ServiceEndsAt)) {
		_ = h.plans.UpdateTime(r.Context(), before.ServiceTimeID, p.ServiceAt, p.ServiceEndsAt)
	}
	http.Redirect(w, r, "/plans/"+id, http.StatusSeeOther)
}

func (h *Handler) PublishPlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	published := r.FormValue("published") == "1"
	if err := h.plans.Publish(r.Context(), id, published); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+id, http.StatusSeeOther)
}

func (h *Handler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	if err := h.plans.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}

// ---- Times ----

func (h *Handler) AddTime(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	starts, err := time.Parse("2006-01-02T15:04", r.FormValue("starts_at"))
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	// Duration-first: accept duration_minutes; fall back to ends_at for older clients.
	var ends time.Time
	if rawEnds := r.FormValue("ends_at"); rawEnds != "" {
		ends, err = time.Parse("2006-01-02T15:04", rawEnds)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
	} else {
		duration, _ := strconv.Atoi(r.FormValue("duration_minutes"))
		if duration <= 0 {
			duration = 60
		}
		ends = starts.Add(time.Duration(duration) * time.Minute)
	}
	if _, err := h.plans.CreateTime(r.Context(), planID,
		r.FormValue("kind"), strings.TrimSpace(r.FormValue("name")), starts, ends); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) DeleteTime(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	n, err := h.assign.CountTimes(r.Context(), planID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if n <= 1 {
		http.Error(w, "Cannot delete the only time on a plan — items would be lost.", http.StatusBadRequest)
		return
	}
	_ = h.plans.DeleteTime(r.Context(), chi.URLParam(r, "tid"))
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

// ---- Items ----

func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	duration, _ := strconv.Atoi(r.FormValue("duration_minutes"))
	in := CreatePlanItemInput{
		Kind:            r.FormValue("kind"),
		Title:           strings.TrimSpace(r.FormValue("title")),
		ArrangementID:   strings.TrimSpace(r.FormValue("arrangement_id")),
		Key:             strings.TrimSpace(r.FormValue("key")),
		DurationMinutes: duration,
		Notes:           strings.TrimSpace(r.FormValue("notes")),
		PlanTimeID:      strings.TrimSpace(r.FormValue("plan_time_id")),
	}
	if in.Kind == "" {
		in.Kind = "song"
	}
	if _, err := h.assign.CreateItem(r.Context(), planID, in); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	_ = h.assign.DeleteItem(r.Context(), chi.URLParam(r, "iid"))
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

// ---- Assignments ----

func (h *Handler) AssignPerson(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	personID := strings.TrimSpace(r.FormValue("person_id"))
	positionID := strings.TrimSpace(r.FormValue("position_id"))
	if personID == "" || positionID == "" {
		http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
		return
	}
	if err := h.assign.AssignPerson(r.Context(), planID, personID, positionID); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) RemovePerson(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	_ = h.assign.RemovePerson(r.Context(), chi.URLParam(r, "pid"))
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

// ---- Volunteer self-service ----

func (h *Handler) MySchedule(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	assignments, err := h.assign.MySchedule(r.Context(), u.ID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "scheduling_my_schedule", map[string]any{
		"User":        u,
		"Assignments": assignments,
	})
}

func (h *Handler) RespondToAssignment(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	ppID := chi.URLParam(r, "ppid")
	status := r.FormValue("status")
	note := strings.TrimSpace(r.FormValue("decline_note"))

	// Authorisation: the assignment must belong to the logged-in user.
	if _, err := h.assign.GetPlanPersonForUser(r.Context(), ppID, u.ID); err != nil {
		http.Error(w, "not your assignment", http.StatusForbidden)
		return
	}
	if status != "C" && status != "D" {
		status = "U"
	}
	if err := h.assign.SetStatus(r.Context(), ppID, status, note); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/my-schedule", http.StatusSeeOther)
}

// ---- helpers ----

func parsePlanInput(r *http.Request) (CreatePlanInput, error) {
	dateStr := r.FormValue("service_date")
	startStr := r.FormValue("service_start")
	endStr := r.FormValue("service_end")

	// Backward compat: accept a single service_at datetime-local if no
	// three-field submission is present.
	if dateStr == "" && startStr == "" && endStr == "" {
		raw := r.FormValue("service_at")
		if raw == "" {
			return CreatePlanInput{}, errors.New("service_date, service_start, and service_end are required")
		}
		var t time.Time
		var err error
		if len(raw) > 10 {
			t, err = time.Parse("2006-01-02T15:04", raw)
		} else {
			t, err = time.Parse("2006-01-02", raw)
		}
		if err != nil {
			return CreatePlanInput{}, err
		}
		return CreatePlanInput{
			ServiceTypeID: strings.TrimSpace(r.FormValue("service_type_id")),
			Title:         strings.TrimSpace(r.FormValue("title")),
			ServiceAt:     t,
			ServiceEndsAt: t.Add(90 * time.Minute),
			Notes:         strings.TrimSpace(r.FormValue("notes")),
		}, nil
	}

	if dateStr == "" || startStr == "" || endStr == "" {
		return CreatePlanInput{}, errors.New("date, start time, and end time are required")
	}

	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return CreatePlanInput{}, err
	}
	start, err := time.Parse("15:04", startStr)
	if err != nil {
		return CreatePlanInput{}, err
	}
	end, err := time.Parse("15:04", endStr)
	if err != nil {
		return CreatePlanInput{}, err
	}

	startDt := time.Date(d.Year(), d.Month(), d.Day(), start.Hour(), start.Minute(), 0, 0, time.UTC)
	endDt := time.Date(d.Year(), d.Month(), d.Day(), end.Hour(), end.Minute(), 0, 0, time.UTC)
	if !endDt.After(startDt) {
		// Treat as crossing midnight
		endDt = endDt.Add(24 * time.Hour)
	}

	return CreatePlanInput{
		ServiceTypeID: strings.TrimSpace(r.FormValue("service_type_id")),
		Title:         strings.TrimSpace(r.FormValue("title")),
		ServiceAt:     startDt,
		ServiceEndsAt: endDt,
		Notes:         strings.TrimSpace(r.FormValue("notes")),
	}, nil
}


// ---- Item edit ----

func (h *Handler) EditItemForm(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "iid")
	item, err := h.assign.GetItem(r.Context(), itemID)
	if err != nil || item.PlanID != planID {
		http.NotFound(w, r)
		return
	}
	plan, err := h.plans.Get(r.Context(), planID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	times, err := h.plans.ListTimes(r.Context(), planID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "scheduling_item_form", map[string]any{
		"Plan":  plan,
		"Item":  item,
		"Times": times,
	})
}

func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "iid")
	duration, _ := strconv.Atoi(r.FormValue("duration_minutes"))
	err := h.assign.UpdateItem(r.Context(), itemID, UpdatePlanItemInput{
		Title:           strings.TrimSpace(r.FormValue("title")),
		Key:             strings.TrimSpace(r.FormValue("key")),
		DurationMinutes: duration,
		Notes:           strings.TrimSpace(r.FormValue("notes")),
		PlanTimeID:      strings.TrimSpace(r.FormValue("plan_time_id")),
	})
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

// ---- Reorder ----

func (h *Handler) ReorderItems(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}
	timeID := r.FormValue("plan_time_id")
	ids := r.Form["item_ids"]
	if err := h.assign.ReorderItems(r.Context(), planID, timeID, ids); err != nil {
		httpx.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


// nextSunday10AM returns the next Sunday at 10:00 local time.
func nextSunday10AM() time.Time {
	now := time.Now()
	daysAhead := (int(time.Sunday) - int(now.Weekday()) + 7) % 7
	if daysAhead == 0 {
		daysAhead = 7
	}
	d := now.AddDate(0, 0, daysAhead)
	return time.Date(d.Year(), d.Month(), d.Day(), 10, 0, 0, 0, d.Location())
}


func (h *Handler) AddNeededPosition(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	positionID := strings.TrimSpace(r.FormValue("position_id"))
	if positionID != "" {
		_ = h.assign.AddNeededPosition(r.Context(), planID, positionID)
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) RemoveNeededPosition(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	posID := chi.URLParam(r, "posid")
	_ = h.assign.RemoveNeededPosition(r.Context(), planID, posID)
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}


func (h *Handler) ReorderTimes(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}
	ids := r.Form["time_ids"]
	if err := h.plans.ReorderTimes(r.Context(), planID, ids); err != nil {
		httpx.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
