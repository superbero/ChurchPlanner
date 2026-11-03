# Church Planner

Modular monolith in Go for church service planning: songs, people, scheduling, calendar.

## First run (local)

    go mod tidy           # resolves dependencies and writes go.sum
    cd deploy
    cp .env.example .env
    cd ..
    docker compose -f deploy/docker-compose.yml up --build

Open http://localhost:8080 — sign in with `admin@example.com` / `changeme`.
**Change that password immediately** (edit `migrations/00001_init.sql` before first run, or
delete the seeded row and create your own).

## Tests

Install Go 1.26 or newer, then run:

    make test        # all unit and template tests
    make test-race   # run tests with the race detector
    make test-cover  # report package and total coverage
    make vet         # run go vet

GitHub Actions runs the race-enabled test suite and `go vet` on pushes and
pull requests. Tests do not require a running database.

## Roles and access

- **Admin:** full access to plans, people, reports, songs, and team assignments.
- **Team leader:** can view plans and manage positions/assignments only for teams
  where they have the leader membership. They can assign only members of those
  teams; editing plan details, times, and service order remains admin-only.
- **Editor:** can manage songs and arrangements; plan management requires a
  separate team-leader membership.
- **Volunteer:** can view published calendar plans, manage their own unavailable
  dates, see/respond to their own assignments, use songs, and access check-in
  staff tools.
- **Guest:** read-only calendar/song access and their own assigned-plan access.

People, team memberships, and reports are admin-only. Unpublished plans are
visible only to admins and team leaders.

## Module rules (enforced by `internal/`)

- A module may import `internal/platform/*` and its own subpackages only.
- A module must NOT import another module's packages. If scheduling needs song data,
  add a small interface in `internal/platform` or an explicit read model.
- Each module owns its tables. No cross-module joins in SQL.
- Each module exposes `New(db, renderer) *Module` and `(m) Routes(chi.Router)`.
- Templates live under `internal/<module>/templates/` and define `{{define "content"}}`.

## Adding a module

    ./scripts/new-module.sh events

Then wire it into `internal/web/router.go`:

    renderer.Add("events", events.Templates(), "*.html")
    ...
    events.New(d.DB, renderer).Routes(r)

## Safe update procedure

1. Write migration as **additive** (add column / table, never rename/drop in the same deploy).
2. Add module code behind new routes; keep old routes working.
3. `go test ./...`.
4. `docker compose up --build -d`.
5. Once stable, ship a second migration that removes the old column.
