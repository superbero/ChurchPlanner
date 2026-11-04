package scheduling

import (
	"embed"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

//go:embed templates/*.html
var templatesFS embed.FS

func Templates() fs.FS {
	sub, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}

type Module struct{ handler *Handler }

func New(db *pgxpool.Pool, renderer *httpx.Renderer) *Module {
	return &Module{handler: &Handler{
		svcTypes: NewServiceTypeRepository(db),
		plans:    NewPlanRepository(db),
		assign:   NewAssignmentRepository(db),
		members:  NewTeamMembershipRepository(db),
		renderer: renderer,
	}}
}

func (m *Module) Routes(r chi.Router, authn *auth.Authenticator) {
	r.Route("/plans", func(r chi.Router) {
		r.Get("/", m.handler.PlansIndex)
		r.Get("/{id}", m.handler.ViewPlan)

		r.Group(func(r chi.Router) {
			r.Use(authn.RequireRoles(auth.RoleEditor))

			r.Get("/new", m.handler.NewPlan)
		r.Get("/bulk", m.handler.ShowBulkPlan)
		r.Post("/bulk", m.handler.BulkCreatePlans)
			r.Post("/", m.handler.CreatePlan)
			r.Get("/{id}/edit", m.handler.EditPlan)
			r.Post("/{id}", m.handler.UpdatePlan)
			r.Post("/{id}/publish", m.handler.PublishPlan)
			r.Post("/{id}/delete", m.handler.DeletePlan)

			r.Post("/{id}/times", m.handler.AddTime)
			r.Post("/{id}/times/reorder", m.handler.ReorderTimes)
			r.Post("/{id}/times/{tid}/delete", m.handler.DeleteTime)
		r.Post("/{id}/times/{tid}/duplicate", m.handler.DuplicateTime)

			r.Post("/{id}/items", m.handler.AddItem)
		r.Post("/{id}/items/batch", m.handler.BatchAddItems)
			r.Get("/{id}/items/{iid}/edit", m.handler.EditItemForm)
			r.Post("/{id}/items/{iid}", m.handler.UpdateItem)
			r.Post("/{id}/items/{iid}/delete", m.handler.DeleteItem)
			r.Post("/{id}/items/reorder", m.handler.ReorderItems)

		})

		r.Post("/{id}/people", m.handler.AssignPerson)
		r.Post("/{id}/people/{pid}/delete", m.handler.RemovePerson)
		r.Post("/{id}/positions", m.handler.AddNeededPosition)
		r.Post("/{id}/positions/{posid}/delete", m.handler.RemoveNeededPosition)
	})
}

// MyScheduleRoutes are accessible to everyone.
func (m *Module) MyScheduleRoutes(r chi.Router) {
	r.Get("/my-schedule", m.handler.MySchedule)
	r.Get("/my-schedule/{ppid}", m.handler.ViewAssignedPlan)
	r.Post("/my-schedule/{ppid}/respond", m.handler.RespondToAssignment)
}
