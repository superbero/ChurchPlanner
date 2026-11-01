package auth

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewSessionManager(db *pgxpool.Pool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = pgxstore.New(db)
	sm.Lifetime = 7 * 24 * time.Hour
	sm.Cookie.Name = "churchplanner_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Path = "/"
	return sm
}
