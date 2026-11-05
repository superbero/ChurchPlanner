package people

import (
	"encoding/csv"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
	"github.com/yourchurch/churchplanner/internal/scheduling"
)

var phoneInputCharacters = regexp.MustCompile(`^[+0-9().\-\s]+$`)

type Handler struct {
	repo        *Repository
	schedRepo   *scheduling.AssignmentRepository
	memberships *scheduling.TeamMembershipRepository
	renderer    *httpx.Renderer
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	users, err := h.repo.List(r.Context(), q, status)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "people_list", map[string]any{
		"Users":        users,
		"Query":        q,
		"StatusFilter": status,
	})
}

func (h *Handler) View(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.repo.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	requester, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if requester.Role == auth.RoleManager &&
		(user.Role == auth.RoleAdmin || user.Role == auth.RoleManager) {
		http.Error(w, "only an organization administrator can view administrator or manager profiles", http.StatusForbidden)
		return
	}

	positions, err := h.schedRepo.ListPositions(r.Context())
	if err != nil {
		slog.Error("ListPositions", "err", err)
		httpx.ServerError(w, err)
		return
	}
	teams, err := h.schedRepo.ListTeams(r.Context())
	if err != nil {
		slog.Error("ListTeams", "err", err)
		httpx.ServerError(w, err)
		return
	}

	userSkills, err := h.repo.PositionsFor(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	skillMap := map[string]bool{}
	for _, s := range userSkills {
		skillMap[s] = true
	}

	teamRoles, err := h.repo.TeamRolesFor(r.Context(), id)
	if err != nil {
		slog.Error("TeamRolesFor", "err", err)
		httpx.ServerError(w, err)
		return
	}
	membershipRoles := map[string]string{}
	for _, tr := range teamRoles {
		membershipRoles[tr.TeamID] = tr.Role
	}

	h.renderer.Render(w, r, http.StatusOK, "people_view", map[string]any{
		"User":                     user,
		"Positions":                positions,
		"SkillMap":                 skillMap,
		"Teams":                    teams,
		"MembershipRoles":          membershipRoles,
		"CanAssignPrivilegedRoles": canAssignPrivilegedRoles(r),
	})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	requester, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	target, err := h.repo.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	role := strings.TrimSpace(r.FormValue("role"))
	if !validUserRole(role) {
		http.Error(w, "invalid user role", http.StatusBadRequest)
		return
	}
	if requester.Role != auth.RoleAdmin &&
		(target.Role == auth.RoleAdmin || target.Role == auth.RoleManager ||
			role == auth.RoleAdmin || role == auth.RoleManager) {
		http.Error(w, "only an organization administrator can manage administrator or manager roles", http.StatusForbidden)
		return
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status != "active" && status != "suspended" {
		http.Error(w, "invalid account status", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	parsedEmail, emailErr := mail.ParseAddress(email)
	if emailErr != nil || parsedEmail.Address != email || len(email) > 254 {
		http.Error(w, "Enter a valid email address.", http.StatusUnprocessableEntity)
		return
	}
	if email != strings.ToLower(strings.TrimSpace(target.Email)) && requester.Role != auth.RoleAdmin {
		http.Error(w, "only an organization administrator can change a user's email address", http.StatusForbidden)
		return
	}
	phone, err := normalizePhone(r.FormValue("phone"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	if err := h.repo.Update(
		r.Context(),
		id,
		strings.TrimSpace(r.FormValue("title")),
		email,
		phone,
		strings.TrimSpace(r.FormValue("vocal_low")),
		strings.TrimSpace(r.FormValue("vocal_high")),
		role,
		status,
	); err != nil {
		if errors.Is(err, ErrEmailTaken) {
			http.Error(w, "That email address is already associated with another account.", http.StatusConflict)
			return
		}
		httpx.ServerError(w, err)
		return
	}

	positionIDs := r.Form["position_ids"]
	if err := h.repo.SetPositions(r.Context(), id, positionIDs); err != nil {
		httpx.ServerError(w, err)
		return
	}

	memberships := map[string]string{}
	for key, values := range r.Form {
		if !strings.HasPrefix(key, "team_role_") || len(values) == 0 {
			continue
		}
		teamRole := strings.TrimSpace(values[0])
		if teamRole == "" {
			continue
		}
		if teamRole != "leader" && teamRole != "member" {
			http.Error(w, "invalid team membership role", http.StatusBadRequest)
			return
		}
		teamID := strings.TrimPrefix(key, "team_role_")
		memberships[teamID] = teamRole
	}
	if err := h.memberships.SetForUser(r.Context(), id, memberships); err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/people/"+id, http.StatusSeeOther)
}

func normalizePhone(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "+") || strings.Count(value, "+") != 1 ||
		!phoneInputCharacters.MatchString(value) {
		return "", errors.New("Enter a phone number with its country code, for example +1 555-0100.")
	}

	var digits strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
		}
	}
	number := digits.String()
	if len(number) < 7 || len(number) > 15 || number[0] == '0' {
		return "", errors.New("Phone number must include a valid country code and contain 7 to 15 digits.")
	}
	return "+" + number, nil
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, r, http.StatusOK, "people_form", map[string]any{
		"Role":                     auth.RoleScheduledViewer,
		"CanAssignPrivilegedRoles": canAssignPrivilegedRoles(r),
	})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	title := strings.TrimSpace(r.FormValue("title"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		role = auth.RoleScheduledViewer
	}
	if !validUserRole(role) {
		h.renderer.Render(w, r, http.StatusUnprocessableEntity, "people_form", map[string]any{
			"Error": "invalid user role", "Name": name, "Title": title, "Email": email,
		})
		return
	}
	requester, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if requester.Role != auth.RoleAdmin &&
		(role == auth.RoleAdmin || role == auth.RoleManager) {
		http.Error(w, "only an organization administrator can create administrator or manager accounts", http.StatusForbidden)
		return
	}

	if _, err := h.repo.Create(r.Context(), name, title, email, password, role); err != nil {
		h.renderer.Render(w, r, http.StatusUnprocessableEntity, "people_form", map[string]any{
			"Error": err.Error(), "Name": name, "Title": title, "Email": email, "Role": role,
		})
		return
	}
	http.Redirect(w, r, "/people", http.StatusSeeOther)
}

func validUserRole(role string) bool {
	switch role {
	case auth.RoleAdmin, auth.RoleManager, auth.RoleEditor,
		auth.RoleViewer, auth.RoleScheduledViewer:
		return true
	default:
		return false
	}
}

func canAssignPrivilegedRoles(r *http.Request) bool {
	u, ok := auth.UserFrom(r.Context())
	return ok && auth.CanAssignPrivilegedRoles(u.Role)
}

func (h *Handler) ShowBulkImport(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, r, http.StatusOK, "people_bulk", nil)
}

func (h *Handler) BulkImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httpx.ServerError(w, err)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		http.Error(w, "Invalid CSV", http.StatusBadRequest)
		return
	}

	var users []CreateUserInput
	for i, row := range records {
		if i == 0 {
			continue
		} // skip header
		if len(row) < 3 {
			continue
		}
		users = append(users, CreateUserInput{
			Name:     row[0],
			Email:    row[1],
			Role:     row[2],
			Password: "changeme123", // Temporary password
		})
	}

	if err := h.repo.BulkCreate(r.Context(), users); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/people", http.StatusSeeOther)
}
