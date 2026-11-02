package scheduling

import (
	"embed"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
		renderer: renderer,
	}}
}

func (m *Module) Routes(r chi.Router) {
	r.Route("/plans", func(r chi.Router) {
		r.Get("/", m.handler.PlansIndex)
		r.Get("/new", m.handler.NewPlan)
		r.Post("/", m.handler.CreatePlan)
		r.Get("/{id}", m.handler.ViewPlan)
		r.Get("/{id}/edit", m.handler.EditPlan)
		r.Post("/{id}", m.handler.UpdatePlan)
		r.Post("/{id}/publish", m.handler.PublishPlan)
		r.Post("/{id}/delete", m.handler.DeletePlan)

		r.Post("/{id}/times", m.handler.AddTime)
		r.Post("/{id}/times/reorder", m.handler.ReorderTimes)
		r.Post("/{id}/times/{tid}/delete", m.handler.DeleteTime)

		r.Post("/{id}/items", m.handler.AddItem)
		r.Get("/{id}/items/{iid}/edit", m.handler.EditItemForm)
		r.Post("/{id}/items/{iid}", m.handler.UpdateItem)
		r.Post("/{id}/items/{iid}/delete", m.handler.DeleteItem)
		r.Post("/{id}/items/reorder", m.handler.ReorderItems)

		r.Post("/{id}/people", m.handler.AssignPerson)
		r.Post("/{id}/people/{pid}/delete", m.handler.RemovePerson)

		r.Post("/{id}/positions", m.handler.AddNeededPosition)
		r.Post("/{id}/positions/{posid}/delete", m.handler.RemoveNeededPosition)
	})


}


// MyScheduleRoutes are accessible to everyone.
func (m *Module) MyScheduleRoutes(r chi.Router) {
	r.Get("/my-schedule", m.handler.MySchedule)
	r.Post("/my-schedule/{ppid}/respond", m.handler.RespondToAssignment)
}
