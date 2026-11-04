package ui

import (
	"net/http"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// projectsData bundles data needed to render the projects page.
// Sync carries the read-only enrollment status shown per project; cloud
// mutations (enroll / unenroll / sync) are done with the engram CLI directly.
type projectsData struct {
	Stats []services.ProjectStats
	Sync  services.SyncProjectsResponse
}

// loadProjects fetches all projects data; returns empty structs when RoDB is nil.
func loadProjects(d Deps) (projectsData, error) {
	if d.RoDB == nil {
		return projectsData{
			Stats: []services.ProjectStats{},
			Sync:  services.SyncProjectsResponse{},
		}, nil
	}

	stats, err := services.ProjectsList(d.RoDB)
	if err != nil {
		return projectsData{}, err
	}

	syncResp, err := services.SyncListProjects(d.RoDB)
	if err != nil {
		// Sync data is best-effort — render with empty sync if it fails.
		syncResp = services.SyncProjectsResponse{}
	}

	return projectsData{Stats: stats, Sync: syncResp}, nil
}

// filterProjects applies a case-insensitive substring filter to the project list.
func filterProjects(stats []services.ProjectStats, q string) []services.ProjectStats {
	if q == "" {
		return stats
	}
	needle := strings.ToLower(q)
	out := make([]services.ProjectStats, 0, len(stats))
	for _, p := range stats {
		if strings.Contains(strings.ToLower(p.Project), needle) {
			out = append(out, p)
		}
	}
	return out
}

// handleProjectsPage serves GET /projects.
// Full page on direct GET; partial content when HX-Request: true.
// Supports ?q= server-side text filter.
func handleProjectsPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		sidebarState := sidebarStateForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		data, err := loadProjects(d)
		if err != nil {
			RenderError(w, r, "Failed to load projects: ", err)
			return
		}

		filtered := filterProjects(data.Stats, q)

		if IsHTMX(r) {
			Render(w, r, ProjectsListPartial(filtered, data.Sync, q, lang))
		} else {
			renderDeps(w, r, d, ProjectsListPage(filtered, data.Sync, q, lang, theme, sidebarState))
		}
	}
}

// handleProjectsListPartial serves GET /projects/list.
// Always returns the bare table content (HTMX swap target for the filter input).
func handleProjectsListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		data, err := loadProjects(d)
		if err != nil {
			RenderError(w, r, "Failed to load projects: ", err)
			return
		}

		filtered := filterProjects(data.Stats, q)
		Render(w, r, ProjectsTablePartial(filtered, data.Sync, q, lang))
	}
}
