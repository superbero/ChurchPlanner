package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKey int

const userKey ctxKey = iota

type User struct {
	ID        string
	Name      string
	Title     string
	Email     string
	Role      string
	Phone     string
	VocalLow  string
	VocalHigh string
	Status    string
}

const (
	RoleAdmin           = "admin"
	RoleManager         = "manager"
	RoleEditor          = "editor"
	RoleViewer          = "viewer"
	RoleScheduledViewer = "scheduled_viewer"
)

func CanManagePeople(role string) bool {
	return role == RoleAdmin || role == RoleManager
}

func CanAssignPrivilegedRoles(role string) bool {
	return role == RoleAdmin
}

func CanEditContent(role string) bool {
	return role == RoleAdmin || role == RoleEditor
}

func CanViewPlans(role string) bool {
	return role == RoleAdmin || role == RoleManager || role == RoleEditor || role == RoleViewer
}

type Authenticator struct {
	DB       *pgxpool.Pool
	Sessions *scs.SessionManager
}

func (a *Authenticator) CurrentUser(r *http.Request) (User, bool) {
	id := a.Sessions.GetString(r.Context(), "user_id")
	if id == "" {
		return User{}, false
	}
	var u User
	err := a.DB.QueryRow(r.Context(),
		`SELECT id, name, email, role, status FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.Sessions.Remove(r.Context(), "user_id")
		}
		return User{}, false
	}
	return u, true
}

func (a *Authenticator) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.CurrentUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if u.Status == "suspended" {
			a.Sessions.Destroy(r.Context())
			http.Redirect(w, r, "/login?error=account_suspended", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

// RequireRoles ensures the logged-in user has one of the allowed roles.
// Organization administrators always pass.
func (a *Authenticator) RequireRoles(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return a.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := UserFrom(r.Context())
			if u.Role == RoleAdmin {
				next.ServeHTTP(w, r)
				return
			}
			for _, role := range allowedRoles {
				if u.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "forbidden", http.StatusForbidden)
		}))
	}
}

func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey).(User)
	return u, ok
}
