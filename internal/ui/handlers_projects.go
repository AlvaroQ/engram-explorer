package ui

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// capsProbeTimeout bounds the CLI capabilities probe so a hanging subprocess
// never stalls the projects page load.
const capsProbeTimeout = 3 * time.Second

// projectsCloud abstracts cloud-control operations used by the projects handlers.
// The production implementation is *services.CloudControlService; tests inject a fake.
type projectsCloud interface {
	Enroll(ctx context.Context, project string) (services.CloudResult, error)
	Unenroll(ctx context.Context, project string) (services.CloudResult, error)
	Capabilities(ctx context.Context) (services.CloudCapabilities, error)
}

// projectsCaps probes which cloud subcommands are available, with a safe fallback
// of "both enabled" when cloud is nil or the probe fails.
func projectsCaps(ctx context.Context, cloud projectsCloud) services.CloudCapabilities {
	fallback := services.CloudCapabilities{Enroll: true, Unenroll: true}
	if cloud == nil {
		return fallback
	}
	cctx, cancel := context.WithTimeout(ctx, capsProbeTimeout)
	defer cancel()
	caps, err := cloud.Capabilities(cctx)
	if err != nil {
		return fallback
	}
	return caps
}

// projectsData bundles data needed to render the projects page.
type projectsData struct {
	Stats []services.ProjectStats
	Sync  services.SyncProjectsResponse
	Caps  services.CloudCapabilities
}

// loadProjects fetches all projects data; returns empty structs when RoDB is nil.
func loadProjects(ctx context.Context, d Deps, cloud projectsCloud) (projectsData, error) {
	caps := projectsCaps(ctx, cloud)

	if d.RoDB == nil {
		return projectsData{
			Stats: []services.ProjectStats{},
			Sync:  services.SyncProjectsResponse{},
			Caps:  caps,
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

	return projectsData{Stats: stats, Sync: syncResp, Caps: caps}, nil
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
func handleProjectsPage(d Deps, cloud projectsCloud) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		data, err := loadProjects(r.Context(), d, cloud)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load projects: "+err.Error()))
			return
		}

		filtered := filterProjects(data.Stats, q)

		if IsHTMX(r) {
			render(w, r, ProjectsListPartial(filtered, data.Sync, data.Caps, q, lang))
		} else {
			renderDeps(w, r, d, ProjectsListPage(filtered, data.Sync, data.Caps, q, lang, theme))
		}
	}
}

// handleProjectsListPartial serves GET /projects/list.
// Always returns the bare table content (HTMX swap target for the filter input).
func handleProjectsListPartial(d Deps, cloud projectsCloud) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		data, err := loadProjects(r.Context(), d, cloud)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load projects: "+err.Error()))
			return
		}

		filtered := filterProjects(data.Stats, q)
		render(w, r, ProjectsTablePartial(filtered, data.Sync, data.Caps, q, lang))
	}
}

// handleProjectsEnroll serves POST /projects/{project}/enroll.
// Wrapped in requireRW; re-renders the projects table after the operation.
func handleProjectsEnroll(d Deps, cloud projectsCloud) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		lang := langForRequest(r)

		if _, err := cloud.Enroll(r.Context(), project); err != nil {
			render(w, r, ErrorPartial("Enroll failed: "+err.Error()))
			return
		}

		data, err := loadProjects(r.Context(), d, cloud)
		if err != nil {
			render(w, r, ErrorPartial("Failed to reload projects: "+err.Error()))
			return
		}

		render(w, r, ProjectsTablePartial(data.Stats, data.Sync, data.Caps, "", lang))
	}
}

// handleProjectsUnenroll serves POST /projects/{project}/unenroll.
// Wrapped in requireRW; re-renders the projects table after the operation.
func handleProjectsUnenroll(d Deps, cloud projectsCloud) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		lang := langForRequest(r)

		if _, err := cloud.Unenroll(r.Context(), project); err != nil {
			render(w, r, ErrorPartial("Unenroll failed: "+err.Error()))
			return
		}

		data, err := loadProjects(r.Context(), d, cloud)
		if err != nil {
			render(w, r, ErrorPartial("Failed to reload projects: "+err.Error()))
			return
		}

		render(w, r, ProjectsTablePartial(data.Stats, data.Sync, data.Caps, "", lang))
	}
}
