package doctor

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// renderEmptySessions loads the empty-sessions report and renders the section
// body. deleted is forwarded to the partial (non-nil after a bulk delete).
func renderEmptySessions(w http.ResponseWriter, r *http.Request, d Deps, deleted *int64) {
	rep := services.EmptySessionsReport{}
	if d.RoDB != nil {
		var err error
		rep, err = services.ListEmptySessions(d.RoDB, services.EmptySessionsDefaultLimit)
		if err != nil {
			ui.RenderError(w, r, "errors.loadEmptySessions", err)
			return
		}
	}
	ui.Render(w, r, EmptySessionsPartial(rep, d.RWDB != nil, deleted))
}

// handleEmptySessionsList serves GET /doctor/maintenance/empty-sessions.
func handleEmptySessionsList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderEmptySessions(w, r, d, nil)
	}
}

// handleEmptySessionsDelete serves POST /doctor/maintenance/empty-sessions/delete.
// It takes no id list: services.DeleteEmptySessions deletes, in one
// transaction, the sessions that are still empty at that moment. Wrapped in
// writeGuard, so RWDB is non-nil here.
func handleEmptySessionsDelete(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := services.DeleteEmptySessions(r.Context(), d.RWDB)
		if err != nil {
			ui.RenderError(w, r, "errors.deleteFailed", err)
			return
		}
		renderEmptySessions(w, r, d, &res.Deleted)
	}
}
