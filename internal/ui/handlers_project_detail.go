package ui

import (
	"errors"
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

// otherProjectNames returns all project names from the sync list excluding current.
// Returns an empty slice when the list cannot be loaded (best-effort).
func otherProjectNames(d Deps, current string) []string {
	if d.RoDB == nil {
		return nil
	}
	stats, err := services.ProjectsList(d.RoDB)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(stats))
	for _, s := range stats {
		if s.Project != current && s.Project != "" {
			names = append(names, s.Project)
		}
	}
	return names
}

// handleProjectDetailPage serves GET /projects/{project}.
// Full page on direct GET; partial on HX-Request.
// Returns 404 when the project does not exist in the DB or RoDB is nil.
func handleProjectDetailPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, _ := url.PathUnescape(r.PathValue("project"))
		lang := langForRequest(r)
		theme := themeForRequest(r)
		sidebarState := sidebarStateForRequest(r)

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

		// Load other project names for the combine form (best-effort).
		others := otherProjectNames(d, project)

		if IsHTMX(r) {
			render(w, r, ProjectDetailPartial(*overview, syncRow, lang, others))
		} else {
			renderDeps(w, r, d, ProjectDetailPage(*overview, syncRow, lang, theme, sidebarState, others))
		}
	}
}

// mergeErrorMessage maps WriteError codes returned by services.RenameProject
// (mode="merge") to plain-language i18n keys in the projectDetail.combine.error
// namespace. Falls back to projectDetail.combine.error.generic for unknown codes.
func mergeErrorMessage(lang string, we *services.WriteError) string {
	switch we.Code {
	case "SAME_NAME":
		return T(lang, "projectDetail.combine.error.sameName")
	case "SOURCE_NOT_FOUND":
		return T(lang, "projectDetail.combine.error.sourceNotFound")
	case "TARGET_NOT_FOUND":
		return T(lang, "projectDetail.combine.error.targetNotFound")
	case "ENROLLED_SOURCE_UNSUPPORTED":
		return T(lang, "projectDetail.combine.error.enrolledSource")
	case "ENROLLED_TARGET_UNSUPPORTED":
		return T(lang, "projectDetail.combine.error.enrolledTarget")
	default:
		return T(lang, "projectDetail.combine.error.generic", "message", we.Message)
	}
}

// handleProjectMergePost serves POST /projects/{project}/merge.
// It merges the current project into the selected target using
// services.RenameProject with Mode="merge". On success it redirects the
// browser (or HTMX) to the surviving target project detail page.
func handleProjectMergePost(d Deps) http.HandlerFunc {
	return requireRW(d, func(w http.ResponseWriter, r *http.Request) {
		source, _ := url.PathUnescape(r.PathValue("project"))
		lang := langForRequest(r)

		if err := r.ParseForm(); err != nil {
			render(w, r, ErrorPartial("Bad request: "+err.Error()))
			return
		}
		target := r.FormValue("target")
		if target == "" {
			render(w, r, ErrorPartial(T(lang, "projectDetail.combine.error.targetNotFound")))
			return
		}

		_, err := services.RenameProject(r.Context(), d.RWDB, services.RenameProjectParams{
			Source: source,
			Target: target,
			Mode:   "merge",
		})
		if err != nil {
			var we *services.WriteError
			if errors.As(err, &we) {
				render(w, r, ErrorPartial(mergeErrorMessage(lang, we)))
				return
			}
			render(w, r, ErrorPartial(T(lang, "projectDetail.combine.error.generic", "message", err.Error())))
			return
		}

		// Success: redirect to the surviving project.
		redirectURL := "/projects/" + url.PathEscape(target)
		if IsHTMX(r) {
			w.Header().Set("HX-Redirect", redirectURL)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
	})
}
