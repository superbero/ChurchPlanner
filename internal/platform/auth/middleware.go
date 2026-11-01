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
	ID    string
	Name  string
	Email string
	Role  string
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
		`SELECT id, name, email, role FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role)
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
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (a *Authenticator) RequireRole(role string, next http.Handler) http.Handler {
	return a.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		if u.Role != role && u.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey).(User)
	return u, ok
}
