package doctor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// entitySegmentMap maps URL path segments to the canonical EntityKind values
// expected by the services layer.
var entitySegmentMap = map[string]services.EntityKind{
	"observations": services.EntityKindObservation,
	"sessions":     services.EntityKindSession,
	"prompts":      services.EntityKindPrompt,
}

// errUnknownEntity flags a malformed {entity} path segment, which maps to a hard
// 400 (the templates never emit one — it only happens on hand-crafted URLs). An
// invalid {id} is a soft error rendered inline instead.
var errUnknownEntity = errors.New("unknown entity type")

// writeEntityParseError responds to a parseEntityAndID failure: a hard 400 for
// an unknown entity segment, an inline error partial (status 200, so HTMX still
// swaps) for anything else. Both messages are user-safe (usererr).
func writeEntityParseError(w http.ResponseWriter, r *http.Request, err error) {
	msg := ui.ErrorMessage(r, "", err)
	if errors.Is(err, errUnknownEntity) {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	ui.Render(w, r, ui.ErrorPartial(msg))
}

// parseEntityAndID extracts the {entity} and {id} path values and returns the
// canonical EntityKind plus a typed id (string for sessions, int64 otherwise).
// Shared by the assign and delete handlers.
func parseEntityAndID(r *http.Request) (services.EntityKind, any, error) {
	entitySeg := r.PathValue("entity")
	idStr := r.PathValue("id")

	kind, ok := entitySegmentMap[entitySeg]
	if !ok {
		return "", nil, usererr.Wrap(fmt.Sprintf("unknown entity type: %q", entitySeg), errUnknownEntity)
	}
	if kind == services.EntityKindSession {
		return kind, idStr, nil
	}
	n, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return "", nil, usererr.New(fmt.Sprintf("invalid id %q", idStr))
	}
	return kind, n, nil
}

// handleOrphansListPartial serves GET /doctor/orphans/list.
// Always returns the bare OrphansList partial (used by hx-get load trigger).
func handleOrphansListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, projects, err := loadOrphansData(r.Context(), d)
		if err != nil {
			ui.RenderError(w, r, "Failed to load orphans: ", err)
			return
		}
		ui.Render(w, r, OrphansList(data, projects))
	}
}

// handleOrphanObservationDetail serves GET /doctor/orphans/observations/{id}/detail.
// It renders the observation-detail dialog partial so the user can inspect the
// content and session working directory before assigning it to a project.
func handleOrphanObservationDetail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			ui.Render(w, r, ui.ErrorPartial(fmt.Sprintf("invalid observation id %q", idStr)))
			return
		}
		if d.RoDB == nil {
			ui.Render(w, r, ui.ErrorPartial("Database is not available."))
			return
		}

		detail, err := services.LoadOrphanObservationDetail(d.RoDB, id)
		if err != nil {
			ui.RenderError(w, r, "Failed to load observation: ", err)
			return
		}
		if detail == nil {
			ui.Render(w, r, ui.ErrorPartial(fmt.Sprintf("Observation %d not found.", id)))
			return
		}

		projects, err := services.ProjectsList(d.RoDB)
		if err != nil {
			projects = []services.ProjectStats{}
		}
		ui.Render(w, r, OrphanObservationDetailDialog(detail, projects))
	}
}

// handleOrphansAssign serves POST /doctor/orphans/{entity}/{id}/project.
// Wrapped in requireRW, so RWDB is guaranteed non-nil here.
func handleOrphansAssign(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, id, err := parseEntityAndID(r)
		if err != nil {
			writeEntityParseError(w, r, err)
			return
		}

		if err := r.ParseForm(); err != nil {
			ui.RenderError(w, r, "Invalid form data: ", err)
			return
		}
		project := r.FormValue("project")
		if project == "" {
			ui.Render(w, r, ui.ErrorPartial("project field is required"))
			return
		}

		if _, err := services.AssignProject(r.Context(), d.RWDB, kind, id, project); err != nil {
			respondOrphans(w, r, d, ui.ErrorMessage(r, "Assign failed: ", err))
			return
		}
		respondOrphans(w, r, d, "")
	}
}

// handleOrphansDelete serves DELETE /doctor/orphans/{entity}/{id}.
// Wrapped in requireRW, so RWDB is guaranteed non-nil here.
func handleOrphansDelete(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, id, err := parseEntityAndID(r)
		if err != nil {
			writeEntityParseError(w, r, err)
			return
		}

		if _, err := services.DeleteEntity(r.Context(), d.RWDB, kind, id); err != nil {
			respondOrphans(w, r, d, ui.ErrorMessage(r, "Delete failed: ", err))
			return
		}
		respondOrphans(w, r, d, "")
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// respondOrphans reloads the orphans list and renders it. When errMsg is
// non-empty the list is prefixed with an inline error; status stays 200 so HTMX
// still swaps. A reload failure falls back to a bare ErrorPartial.
func respondOrphans(w http.ResponseWriter, r *http.Request, d Deps, errMsg string) {
	data, projects, loadErr := loadOrphansData(r.Context(), d)
	if loadErr != nil || data == nil {
		if errMsg != "" {
			ui.Render(w, r, ui.ErrorPartial(errMsg))
		} else {
			ui.RenderError(w, r, "Reload failed: ", loadErr)
		}
		return
	}
	if errMsg != "" {
		ui.Render(w, r, OrphansListWithError(data, projects, errMsg))
		return
	}
	ui.Render(w, r, OrphansList(data, projects))
}

// loadOrphansData fetches both the orphans list and the projects list from RoDB.
func loadOrphansData(ctx context.Context, d Deps) (*services.OrphansResponse, []services.ProjectStats, error) {
	if d.RoDB == nil {
		empty := &services.OrphansResponse{
			Observations: []services.OrphanObservation{},
			Sessions:     []services.OrphanSession{},
			Prompts:      []services.OrphanPrompt{},
		}
		return empty, nil, nil
	}
	data, err := services.OrphansList(d.RoDB)
	if err != nil {
		return nil, nil, err
	}
	projects, err := services.ProjectsList(d.RoDB)
	if err != nil {
		// Non-fatal: empty project list means the assign dropdown will be empty.
		projects = []services.ProjectStats{}
	}
	return data, projects, nil
}
