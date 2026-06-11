package ui

import (
	"net/http"
	"net/url"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// ProjectActivityChartIslandProps is the props shape for the project-activity-chart island.
// Fields use json tags that match the TypeScript ActivityDay interface in the island.
type ProjectActivityChartIslandProps struct {
	Activity30d []services.ActivityDay `json:"activity_30d"`
}

// toProjectActivityChartProps converts a services.ActivityDay slice to island props.
func toProjectActivityChartProps(days []services.ActivityDay) ProjectActivityChartIslandProps {
	if days == nil {
		days = []services.ActivityDay{}
	}
	return ProjectActivityChartIslandProps{Activity30d: days}
}

// handleProjectDetailPage serves GET /projects/{project}.
// Full page on direct GET; partial on HX-Request.
// Returns 404 when the project does not exist in the DB or RoDB is nil.
func handleProjectDetailPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, _ := url.PathUnescape(r.PathValue("project"))
		lang := langForRequest(r)
		theme := themeForRequest(r)

		if d.RoDB == nil {
			http.NotFound(w, r)
			return
		}

		overview, err := services.ProjectsGetOverview(d.RoDB, project)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load project: "+err.Error()))
			return
		}
		if overview == nil {
			http.NotFound(w, r)
			return
		}

		// Load sync data for the KPI sync card (best-effort; nil syncRow = not enrolled).
		syncResp, _ := services.SyncListProjects(d.RoDB)
		syncRow := syncByProject(syncResp, project)

		if IsHTMX(r) {
			render(w, r, ProjectDetailPartial(*overview, syncRow, lang))
		} else {
			renderDeps(w, r, d, ProjectDetailPage(*overview, syncRow, lang, theme))
		}
	}
}
