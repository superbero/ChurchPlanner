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
	svcTypes  *ServiceTypeRepository
	plans     *PlanRepository
	templates *ServiceTemplateRepository
	assign    *AssignmentRepository
	members   *TeamMembershipRepository
	renderer  *httpx.Renderer
}

// ---- Plans list ----

func (h *Handler) PlansIndex(w http.ResponseWriter, r *http.Request) {
	types, err := h.svcTypes.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	selectedType := r.URL.Query().Get("type")
	dateRange := r.URL.Query().Get("date_range")
	if dateRange == "" {
		dateRange = "upcoming"
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 25
	}

	plans, err := h.plans.List(r.Context(), selectedType, limit, dateRange)
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
		"ServiceTypes":       types,
		"Plans":              plans,
		"SelectedType":       selectedType,
		"DateRange":          dateRange,
		"Limit":              limit,
		"TypeName":           typeName,
		"CanManageTemplates": canManageTemplates(r),
	})
}

func (h *Handler) NewPlan(w http.ResponseWriter, r *http.Request) {
	types, err := h.svcTypes.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	templates, err := h.templates.List(r.Context())
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
		"ServiceTypes":     types,
		"ServiceTemplates": templates,
		"IsNew":            true,
		"Plan":             Plan{ServiceAt: defaultStart, ServiceEndsAt: defaultStart.Add(90 * time.Minute)},
	})
}

func (h *Handler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	in, err := parsePlanInput(r)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	templateID := strings.TrimSpace(r.FormValue("service_template_id"))
	var p Plan
	if templateID == "" {
		p, err = h.plans.CreateWithServiceTime(r.Context(), in)
	} else {
		p, err = h.plans.CreateWithServiceTemplate(r.Context(), in, templateID)
		if errors.Is(err, ErrServiceTemplateNotFound) || errors.Is(err, ErrInvalidServiceTemplate) {
			http.Error(w, "The selected service template is invalid or no longer available.", http.StatusUnprocessableEntity)
			return
		}
	}
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+p.ID, http.StatusSeeOther)
}

func (h *Handler) ServiceTemplatesIndex(w http.ResponseWriter, r *http.Request) {
	templates, err := h.templates.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	types, err := h.svcTypes.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "scheduling_service_templates", map[string]any{
		"Templates":    templates,
		"ServiceTypes": types,
	})
}

func (h *Handler) CreateServiceTemplate(w http.ResponseWriter, r *http.Request) {
	template, err := parseServiceTemplateInput(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	serviceTypes, err := h.svcTypes.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	validServiceType := false
	for _, serviceType := range serviceTypes {
		if serviceType.ID == template.ServiceTypeID {
			validServiceType = true
			break
		}
	}
	if !validServiceType {
		http.Error(w, "The selected service type is invalid.", http.StatusUnprocessableEntity)
		return
	}
	if err := h.templates.Create(r.Context(), template); err != nil {
		if errors.Is(err, ErrServiceTemplateExists) {
			http.Error(w, "A template with this name already exists for the selected service type.", http.StatusConflict)
			return
		}
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/templates", http.StatusSeeOther)
}

func (h *Handler) DeleteServiceTemplate(w http.ResponseWriter, r *http.Request) {
	err := h.templates.Delete(r.Context(), chi.URLParam(r, "templateID"))
	if errors.Is(err, ErrServiceTemplateNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/templates", http.StatusSeeOther)
}

func canManageTemplates(r *http.Request) bool {
	u, ok := auth.UserFrom(r.Context())
	return ok && auth.CanEditContent(u.Role)
}

func parseServiceTemplateInput(r *http.Request) (ServiceTemplate, error) {
	if err := r.ParseForm(); err != nil {
		return ServiceTemplate{}, errors.New("invalid template form")
	}
	template := ServiceTemplate{
		ServiceTypeID: strings.TrimSpace(r.FormValue("service_type_id")),
		Name:          strings.TrimSpace(r.FormValue("name")),
	}
	if template.ServiceTypeID == "" || template.Name == "" || len(template.Name) > 100 {
		return ServiceTemplate{}, errors.New("select a service type and enter a template name of 1 to 100 characters")
	}
	kinds := r.Form["section_kind"]
	names := r.Form["section_name"]
	offsets := r.Form["section_offset"]
	durations := r.Form["section_duration"]
	if len(kinds) == 0 || len(kinds) > 12 || len(names) != len(kinds) ||
		len(offsets) != len(kinds) || len(durations) != len(kinds) {
		return ServiceTemplate{}, errors.New("a template must contain between 1 and 12 complete sections")
	}

	serviceCount := 0
	for i := range kinds {
		kind := strings.TrimSpace(kinds[i])
		name := strings.TrimSpace(names[i])
		if name == "" || len(name) > 100 {
			return ServiceTemplate{}, errors.New("each section needs a name of 1 to 100 characters")
		}
		offset, err := strconv.Atoi(strings.TrimSpace(offsets[i]))
		if err != nil || offset < -1440 || offset > 1440 {
			return ServiceTemplate{}, errors.New("section offsets must be between -1440 and 1440 minutes")
		}
		duration, err := strconv.Atoi(strings.TrimSpace(durations[i]))
		if err != nil {
			return ServiceTemplate{}, errors.New("enter a valid section duration")
		}
		switch kind {
		case "service":
			serviceCount++
			if offset != 0 || duration != 0 {
				return ServiceTemplate{}, errors.New("the service section uses the plan's service time and must have offset and duration set to zero")
			}
		case "rehearsal", "other":
			if duration < 1 || duration > 720 {
				return ServiceTemplate{}, errors.New("non-service sections must last between 1 and 720 minutes")
			}
		default:
			return ServiceTemplate{}, errors.New("invalid section type")
		}
		template.Times = append(template.Times, ServiceTemplateTime{
			Position:           i,
			Kind:               kind,
			Name:               name,
			StartOffsetMinutes: offset,
			DurationMinutes:    duration,
		})
	}
	if serviceCount != 1 {
		return ServiceTemplate{}, errors.New("a template must contain exactly one service section")
	}
	return template, nil
}

func (h *Handler) renderPlan(w http.ResponseWriter, r *http.Request, planID string, errMsg string) {
	plan, err := h.plans.Get(r.Context(), planID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if !auth.CanEditContent(u.Role) && !plan.Published {
		isLeader, err := h.members.IsLeaderOfAnyTeam(r.Context(), u.ID)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
		if !isLeader {
			http.NotFound(w, r)
			return
		}
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
	isAdmin := u.Role == auth.RoleAdmin
	leaderTeamSet := make(map[string]bool)
	memberSet := make(map[string]bool)
	if !isAdmin {
		leaderTeams, err := h.members.LeaderTeams(r.Context(), u.ID)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
		for _, teamID := range leaderTeams {
			leaderTeamSet[teamID] = true
		}
		memberSet, err = h.members.MemberIDsForTeams(r.Context(), leaderTeams)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
		filteredTeams := tree[:0]
		for _, team := range tree {
			if leaderTeamSet[team.TeamID] {
				filteredTeams = append(filteredTeams, team)
			}
		}
		tree = filteredTeams

		filteredPositions := allPositions[:0]
		for _, position := range allPositions {
			if leaderTeamSet[position.TeamID] {
				filteredPositions = append(filteredPositions, position)
			}
		}
		allPositions = filteredPositions

		filteredUsers := users[:0]
		for _, user := range users {
			if memberSet[user.ID] {
				filteredUsers = append(filteredUsers, user)
			}
		}
		users = filteredUsers
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
	if len(times) > 0 {
		latest := times[0]
		for _, planTime := range times[1:] {
			if planTime.EndsAt.After(latest.EndsAt) {
				latest = planTime
			}
		}
		nextStart = latest.EndsAt
	}

	// Fetch users who are blocked out on the plan's date
	blockedMap := make(map[string]bool)
	brows, err := h.plans.DB().Query(r.Context(), `
		SELECT user_id FROM user_blockouts
		WHERE starts_on <= $1 AND ends_on >= $1
	`, plan.ServiceAt.Format("2006-01-02"))
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	for brows.Next() {
		var uid string
		if err := brows.Scan(&uid); err != nil {
			brows.Close()
			httpx.ServerError(w, err)
			return
		}
		blockedMap[uid] = true
	}
	if err := brows.Err(); err != nil {
		brows.Close()
		httpx.ServerError(w, err)
		return
	}
	brows.Close()

	// Build a map of PositionID -> []User (only those with the skill)
	userPositions, err := h.plans.DB().Query(r.Context(), `
		SELECT user_id, position_id FROM user_positions
	`)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	skillsByPosition := make(map[string]map[string]bool)
	for userPositions.Next() {
		var userID, positionID string
		if err := userPositions.Scan(&userID, &positionID); err != nil {
			userPositions.Close()
			httpx.ServerError(w, err)
			return
		}
		if skillsByPosition[positionID] == nil {
			skillsByPosition[positionID] = make(map[string]bool)
		}
		skillsByPosition[positionID][userID] = true
	}
	if err := userPositions.Err(); err != nil {
		userPositions.Close()
		httpx.ServerError(w, err)
		return
	}
	userPositions.Close()

	usersByPos := make(map[string][]auth.User)
	for _, pos := range allPositions {
		var eligible []auth.User
		for _, u := range users {
			if skillsByPosition[pos.ID][u.ID] {
				eligible = append(eligible, u)
			}
		}
		// If no one has the skill, fall back to all users (so you aren't blocked)
		if len(eligible) == 0 {
			eligible = users
		}
		usersByPos[pos.ID] = eligible
	}

	// Compute which positions the current user may edit.
	var manageableIDs = map[string]bool{}
	var manageablePositions []Position
	if isAdmin {
		for _, pos := range allPositions {
			manageableIDs[pos.ID] = true
		}
		manageablePositions = allPositions
	} else {
		for _, pos := range allPositions {
			if leaderTeamSet[pos.TeamID] {
				manageableIDs[pos.ID] = true
				manageablePositions = append(manageablePositions, pos)
			}
		}
	}

	h.renderer.Render(w, r, http.StatusOK, "scheduling_plan_view", map[string]any{
		"Plan":                  plan,
		"Times":                 timeGroups,
		"Unassigned":            unassigned,
		"Teams":                 tree,
		"Users":                 users,
		"UsersByPos":            usersByPos,
		"AllPositions":          allPositions,
		"ManageablePositions":   manageablePositions,
		"ManageablePositionIDs": manageableIDs,
		"BlockedUsers":          blockedMap,
		"NextStart":             nextStart,
		"HasMultiple":           len(times) > 1,
		"Error":                 errMsg,
	})
}

func (h *Handler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	if u, ok := auth.UserFrom(r.Context()); !ok || !auth.CanEditContent(u.Role) {
		http.Error(w, "editor access required", http.StatusForbidden)
		return
	}
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
		_ = h.plans.UpdateTime(r.Context(), before.ServiceTimeID, "service", "Service", p.ServiceAt, p.ServiceEndsAt)
	}
	http.Redirect(w, r, "/plans/"+id, http.StatusSeeOther)
}

func (h *Handler) PublishPlan(w http.ResponseWriter, r *http.Request) {
	if u, ok := auth.UserFrom(r.Context()); !ok || !auth.CanEditContent(u.Role) {
		http.Error(w, "editor access required", http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	published := r.FormValue("published") == "1"
	if err := h.plans.Publish(r.Context(), id, published); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+id, http.StatusSeeOther)
}

func (h *Handler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	if u, ok := auth.UserFrom(r.Context()); !ok || !auth.CanEditContent(u.Role) {
		http.Error(w, "editor access required", http.StatusForbidden)
		return
	}
	if err := h.plans.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}

// ---- Times ----

func (h *Handler) DeleteTime(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
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
	allowed, err := h.canManagePosition(r, positionID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if !allowed {
		forbid(w)
		return
	}
	if user, ok := auth.UserFrom(r.Context()); ok && user.Role != auth.RoleAdmin {
		eligible, err := h.members.IsMemberOfPositionTeam(r.Context(), personID, positionID)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
		if !eligible {
			forbid(w)
			return
		}
	}
	if err := h.assign.AssignPerson(r.Context(), planID, personID, positionID); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) RemovePerson(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	ppID := chi.URLParam(r, "pid")
	posID, err := h.members.AssignmentPositionForPlan(r.Context(), ppID, planID)
	if errors.Is(err, ErrPersonNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	allowed, err := h.canManagePosition(r, posID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if !allowed {
		forbid(w)
		return
	}
	if err := h.assign.RemovePerson(r.Context(), ppID); err != nil {
		httpx.ServerError(w, err)
		return
	}
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

// ViewAssignedPlan exposes a read-only service plan only through one of the
// current user's own assignments. Management controls remain on /plans.
func (h *Handler) ViewAssignedPlan(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	assignment, err := h.assign.GetPlanPersonForUser(
		r.Context(), chi.URLParam(r, "ppid"), u.ID)
	if err != nil {
		if errors.Is(err, ErrPersonNotFound) {
			http.NotFound(w, r)
			return
		}
		httpx.ServerError(w, err)
		return
	}

	plan, err := h.plans.Get(r.Context(), assignment.PlanID)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) {
			http.NotFound(w, r)
			return
		}
		httpx.ServerError(w, err)
		return
	}
	times, err := h.plans.ListTimes(r.Context(), plan.ID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	items, err := h.assign.ListItems(r.Context(), plan.ID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	byTime := make(map[string][]PlanItem)
	var unassigned []PlanItem
	for _, item := range items {
		if item.PlanTimeID == "" {
			unassigned = append(unassigned, item)
		} else {
			byTime[item.PlanTimeID] = append(byTime[item.PlanTimeID], item)
		}
	}
	groups := make([]PlanTimeWithItems, 0, len(times))
	for _, planTime := range times {
		groups = append(groups, PlanTimeWithItems{
			Time: planTime, Items: byTime[planTime.ID],
		})
	}
	h.renderer.Render(w, r, http.StatusOK, "scheduling_assigned_plan",
		map[string]any{
			"Plan":       plan,
			"Assignment": assignment,
			"Times":      groups,
			"Unassigned": unassigned,
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
		http.Error(w, "status must be C (confirm) or D (decline)",
			http.StatusBadRequest)
		return
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
		allowed, err := h.canManagePosition(r, positionID)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
		if !allowed {
			forbid(w)
			return
		}
		if err := h.assign.AddNeededPosition(r.Context(), planID, positionID); err != nil {
			httpx.ServerError(w, err)
			return
		}
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) RemoveNeededPosition(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	posID := chi.URLParam(r, "posid")
	allowed, err := h.canManagePosition(r, posID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if !allowed {
		forbid(w)
		return
	}
	if err := h.assign.RemoveNeededPosition(r.Context(), planID, posID); err != nil {
		httpx.ServerError(w, err)
		return
	}
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

// canManagePosition returns true if the current user can manage assignments
// and position mutations for this position. Admins always pass.
func (h *Handler) canManagePosition(r *http.Request, positionID string) (bool, error) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		return false, nil
	}
	if auth.CanEditContent(u.Role) {
		return true, nil
	}
	teamID, err := h.members.PositionTeamID(r.Context(), positionID)
	if err != nil {
		return false, err
	}
	return h.members.CanManageTeam(r.Context(), u.ID, teamID)
}

func forbid(w http.ResponseWriter) {
	http.Error(w, "forbidden: not a leader of this team", http.StatusForbidden)
}

func (h *Handler) DuplicateTime(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	timeID := chi.URLParam(r, "tid")
	if err := h.plans.DuplicateTime(r.Context(), planID, timeID); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) DuplicateItem(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "iid")
	if _, err := h.assign.DuplicateItem(r.Context(), planID, itemID); err != nil {
		if errors.Is(err, ErrItemNotFound) {
			http.NotFound(w, r)
			return
		}
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) BatchAddItems(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}

	planTimeID := strings.TrimSpace(r.FormValue("plan_time_id"))
	arrIDs := r.Form["arrangement_id"]
	keys := r.Form["key"]
	durations := r.Form["duration"]

	for i, arrID := range arrIDs {
		if arrID == "" {
			continue
		}

		// Safety check: does this song already exist in this specific time block?
		var exists bool
		err := h.plans.DB().QueryRow(r.Context(), `
			SELECT EXISTS(
				SELECT 1 FROM plan_items
				WHERE plan_id = $1 AND plan_time_id = $2 AND arrangement_id = $3
			)
		`, planID, planTimeID, arrID).Scan(&exists)

		if err != nil || exists {
			continue // Skip if already exists or on error
		}

		key := ""
		if i < len(keys) {
			key = keys[i]
		}
		duration := 0
		if i < len(durations) {
			duration, _ = strconv.Atoi(durations[i])
		}

		in := CreatePlanItemInput{
			Kind:            "song",
			ArrangementID:   arrID,
			Key:             key,
			DurationMinutes: duration,
			PlanTimeID:      planTimeID,
		}
		if _, err := h.assign.CreateItem(r.Context(), planID, in); err != nil {
			httpx.ServerError(w, err)
			return
		}
	}

	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) ViewPlan(w http.ResponseWriter, r *http.Request) {
	h.renderPlan(w, r, chi.URLParam(r, "id"), "")
}

func (h *Handler) EditPlan(w http.ResponseWriter, r *http.Request) {
	if u, ok := auth.UserFrom(r.Context()); !ok || !auth.CanEditContent(u.Role) {
		http.Error(w, "editor access required", http.StatusForbidden)
		return
	}
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

func (h *Handler) AddTime(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	starts, err := time.Parse("2006-01-02T15:04", r.FormValue("starts_at"))
	if err != nil {
		h.renderPlan(w, r, planID, "Invalid start time format.")
		return
	}

	var ends time.Time
	if rawEnds := r.FormValue("ends_at"); rawEnds != "" {
		ends, err = time.Parse("2006-01-02T15:04", rawEnds)
		if err != nil {
			h.renderPlan(w, r, planID, "Invalid end time format.")
			return
		}
	} else {
		duration, _ := strconv.Atoi(r.FormValue("duration_minutes"))
		if duration <= 0 {
			duration = 60
		}
		ends = starts.Add(time.Duration(duration) * time.Minute)
	}

	if err := h.plans.CheckTimeConflict(r.Context(), planID, starts, ends, ""); err != nil {
		h.renderPlan(w, r, planID, err.Error())
		return
	}

	if _, err := h.plans.CreateTime(r.Context(), planID,
		r.FormValue("kind"), strings.TrimSpace(r.FormValue("name")), starts, ends); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}

func (h *Handler) ShowBulkPlan(w http.ResponseWriter, r *http.Request) {
	types, err := h.svcTypes.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "scheduling_plan_bulk", map[string]any{
		"ServiceTypes": types,
	})
}

func (h *Handler) BulkCreatePlans(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}

	typeID := r.FormValue("service_type_id")
	titleBase := strings.TrimSpace(r.FormValue("title_base"))
	startStr := r.FormValue("start_date")
	endStr := r.FormValue("end_date")
	frequency := r.FormValue("frequency")

	startDate, _ := time.Parse("2006-01-02", startStr)
	endDate, _ := time.Parse("2006-01-02", endStr)

	if startDate.IsZero() || endDate.IsZero() || endDate.Before(startDate) {
		http.Error(w, "Invalid date range", http.StatusBadRequest)
		return
	}

	startHour, startMinute := 10, 0
	endHour, endMinute := 11, 30

	created := 0
	current := startDate

	for !current.After(endDate) {
		var exists bool
		h.plans.DB().QueryRow(r.Context(), `
			SELECT EXISTS(
				SELECT 1 FROM plans
				WHERE service_type_id = $1 AND service_at::date = $2::date
			)
		`, typeID, current.Format("2006-01-02")).Scan(&exists)

		if !exists {
			serviceAt := time.Date(current.Year(), current.Month(), current.Day(), startHour, startMinute, 0, 0, time.Local)
			serviceEnds := time.Date(current.Year(), current.Month(), current.Day(), endHour, endMinute, 0, 0, time.Local)

			var planID string
			err := h.plans.DB().QueryRow(r.Context(), `
				INSERT INTO plans (service_type_id, title, service_at, service_ends_at)
				VALUES ($1, $2, $3, $4)
				RETURNING id
			`, typeID, titleBase+" - "+current.Format("Jan 2"), serviceAt, serviceEnds).Scan(&planID)

			if err == nil {
				h.plans.CreateTime(r.Context(), planID, "service", "Service", serviceAt, serviceEnds)
				created++
			}
		}

		switch frequency {
		case "weekly":
			current = current.AddDate(0, 0, 7)
		case "biweekly":
			current = current.AddDate(0, 0, 14)
		case "monthly":
			current = current.AddDate(0, 1, 0)
		default:
			current = current.AddDate(0, 0, 7)
		}
	}

	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}

func (h *Handler) UpdateTime(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	timeID := chi.URLParam(r, "tid")

	starts, err := time.Parse("2006-01-02T15:04", r.FormValue("starts_at"))
	if err != nil {
		h.renderPlan(w, r, planID, "Invalid start time format.")
		return
	}
	ends, err := time.Parse("2006-01-02T15:04", r.FormValue("ends_at"))
	if err != nil {
		h.renderPlan(w, r, planID, "Invalid end time format.")
		return
	}

	if err := h.plans.CheckTimeConflict(r.Context(), planID, starts, ends, timeID); err != nil {
		h.renderPlan(w, r, planID, err.Error())
		return
	}

	kind := r.FormValue("kind")
	if kind == "" {
		kind = "service"
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "Service"
	}

	if err := h.plans.UpdateTime(r.Context(), timeID, kind, name, starts, ends); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/plans/"+planID, http.StatusSeeOther)
}
