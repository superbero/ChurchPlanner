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

- **Organization Administrator (`admin`):** full access, including People,
  permissions, reports, plan and song management, and team assignments. Billing
  and subscription management are not implemented.
- **Manager (`manager`):** manages People profiles and permissions but cannot
  grant administrator or manager access, and has read-only access to published
  plans and songs. A separate team-leader membership grants team-scoped
  assignment management.
- **Editor (`editor`):** creates and edits songs and plans, but cannot manage
  People or system-level access.
- **Viewer (`viewer`):** read-only access to published plans and songs.
- **Scheduled Viewer (`scheduled_viewer`):** sees only plans and assignments
  where they are specifically scheduled, plus songs and their own availability.

Team-leader membership is an additional capability, not a global role. Leaders
can view plans and manage positions/assignments only for their teams. Editors
and administrators can view unpublished plans; team leaders can view drafts
when leading a team on the plan. Legacy `volunteer` accounts migrate to
Scheduled Viewer and `guest` accounts to Viewer.

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
