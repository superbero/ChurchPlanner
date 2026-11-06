# Church Planner

Modular monolith in Go for church service planning, with song and people
libraries, service plans, team scheduling, calendar, check-in, and reports.

## First run (local)

    go mod tidy           # resolves dependencies and writes go.sum
    # Create deploy/.env if needed; at minimum set:
    # DATABASE_URL=postgres://church:church@db:5432/churchplanner?sslmode=disable
    docker compose -f deploy/docker-compose.yml up --build

Open http://localhost:8080 — the initial migration seeds
`onesimeking@gmail.com` / `changeme`. Change this password immediately after
the first sign-in. Set `DATABASE_URL` and the other deployment settings in the
environment where the server runs; Docker is optional outside this local
development setup.

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
  grant administrator or manager access, change account email addresses, or
  reset passwords. Managers can edit standard People profile details and have
  read-only access to published plans and songs. A separate team-leader
  membership grants team-scoped assignment management.
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

## Password resets

Organization administrators can send a password reset link from the People
directory. The link is single-use and expires after 30 minutes; users choose
their own password on the reset page. Managers cannot send reset links. Email
delivery must be configured for this feature to work.

## Email delivery

Only organization administrators see and can access **Email settings** in the
app menu. They can configure SMTP and send a test message. This works whether
the app is run in Docker or deployed directly; settings are stored in the
database, not in Docker configuration.

Before saving SMTP credentials in the app, set `APP_ENCRYPTION_KEY` in the
application's runtime environment. Generate a strong key (for example, with
`openssl rand -base64 32`), keep it persistent across restarts and deployments,
and keep a protected backup. SMTP passwords are encrypted in the database
with this key. Do not rotate or lose it without first updating the stored SMTP
password, or the app will no longer be able to decrypt the credentials.

SMTP can alternatively be configured through the runtime environment using
`SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, and `SMTP_FROM`. Set
`PUBLIC_BASE_URL` to the app's public HTTPS URL so emailed password-reset links
point to the correct site. For example, Gmail commonly uses `smtp.gmail.com`,
port `587`, and a Google App Password (not the Google account password).

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
