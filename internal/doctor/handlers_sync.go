package doctor

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/daemon"
	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// renderSyncProjects loads the projects list and renders the read-only
// SyncProjectsPartial. Shared by the polled list endpoint and the delete
// handler so the refreshed table is always identical.
func renderSyncProjects(w http.ResponseWriter, r *http.Request, d Deps) {
	if d.RoDB == nil {
		render(w, r, SyncProjectsPartial(services.SyncProjectsResponse{}))
		return
	}
	resp, err := services.SyncListProjects(d.RoDB)
	if err != nil {
		renderError(w, r, "Failed to load sync projects: ", err)
		return
	}
	render(w, r, SyncProjectsPartial(resp))
}

// handleSyncProjectsList serves GET /doctor/sync/projects.
func handleSyncProjectsList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderSyncProjects(w, r, d)
	}
}

// handleSyncIssues serves GET /doctor/sync/issues.
// Pings the daemon (bounded by Config.DaemonTimeoutMs), then calls
// services.SyncComputeIssues(roDB, daemonAvailable).
// This replicates the pattern from internal/httpapi/routes_sync.go without
// importing that package.
func handleSyncIssues(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Determine daemon availability.  Use the injected DaemonPing when
		// present (test path); otherwise make a real HTTP round-trip (production).
		var daemonAvailable bool
		if d.DaemonPing != nil {
			daemonAvailable = d.DaemonPing()
		} else {
			dc := daemon.New(d.Config.DaemonBaseURL, d.Config.DaemonTimeoutMs)
			daemonAvailable = dc.FetchJSON("/health").OK
		}

		if d.RoDB == nil {
			resp := services.SyncIssuesResponse{}
			if !daemonAvailable {
				hint := "Start the daemon with `engram serve` and reload"
				resp.Issues = []services.SyncIssue{
					{
						Code:     "DAEMON_DOWN",
						Severity: services.SyncIssueSeverityHigh,
						Message:  "engram serve is not reachable — telemetry unavailable",
						Hint:     &hint,
					},
				}
			}
			render(w, r, SyncIssuesPartial(resp))
			return
		}

		resp, err := services.SyncComputeIssues(d.RoDB, daemonAvailable)
		if err != nil {
			renderError(w, r, "Failed to compute sync issues: ", err)
			return
		}
		render(w, r, SyncIssuesPartial(resp))
	}
}

// handleSyncProjectDetail serves GET /doctor/sync/{project}.
// Calls services.SyncGetProjectDetail(roDB, project) directly, fixing the
// latent route mismatch where the React frontend called
// /api/sync/projects/{project} but the backend registered /api/sync/{project}.
func handleSyncProjectDetail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")

		if d.RoDB == nil {
			http.NotFound(w, r)
			return
		}

		detail, err := services.SyncGetProjectDetail(d.RoDB, project)
		if err != nil {
			renderError(w, r, "Failed to load project detail: ", err)
			return
		}
		if detail == nil {
			http.NotFound(w, r)
			return
		}

		if IsHTMX(r) {
			render(w, r, SyncProjectDetailPartial(detail))
		} else {
			// Use the shared ui.Layout with activeNav="settings" so the Settings
			// gear is highlighted (doctor layout is no longer used).
			render(w, r, ui.Layout("Sync: "+project, "settings", ui.LangForRequest(r), ui.ThemeForRequest(r), ui.SidebarStateForRequest(r), SyncProjectDetailPartial(detail)))
		}
	}
}

// handleSyncDeleteProject serves POST /doctor/sync/{project}/delete.
// It permanently removes an unenrolled, observation-free project (its sessions
// and prompts) and re-renders the refreshed projects table. The destructive
// guards live in services.DeleteProject; the UI only offers the button when the
// project is eligible. Wrapped in requireRW, so RWDB is non-nil here.
func handleSyncDeleteProject(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		if _, err := services.DeleteProject(r.Context(), d.RWDB, project); err != nil {
			renderError(w, r, "Delete failed: ", err)
			return
		}
		renderSyncProjects(w, r, d)
	}
}
