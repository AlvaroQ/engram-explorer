package doctor

import (
	"context"
	"net/http"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/daemon"
	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// capsProbeTimeout bounds the CLI capabilities probe so a hanging subprocess
// never stalls the 15s projects poll.
const capsProbeTimeout = 3 * time.Second

// syncCaps probes which cloud subcommands are available, with a safe fallback
// of "both enabled" when cloud is nil or the probe fails — so the Enroll /
// Unenroll buttons are never hidden by a transient probe error.
func syncCaps(ctx context.Context, cloud CloudController) services.CloudCapabilities {
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

// renderSyncProjects loads the projects list + capabilities and renders the
// SyncProjectsPartial. Shared by the polled list endpoint and the enroll /
// unenroll write handlers so the refreshed table always carries caps.
func renderSyncProjects(w http.ResponseWriter, r *http.Request, d Deps, cloud CloudController) {
	caps := syncCaps(r.Context(), cloud)
	if d.RoDB == nil {
		render(w, r, SyncProjectsPartial(services.SyncProjectsResponse{}, caps))
		return
	}
	resp, err := services.SyncListProjects(d.RoDB)
	if err != nil {
		render(w, r, ErrorPartial("Failed to load sync projects: "+err.Error()))
		return
	}
	render(w, r, SyncProjectsPartial(resp, caps))
}

// handleSyncProjectsList serves GET /doctor/sync/projects.
func handleSyncProjectsList(d Deps, cloud CloudController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderSyncProjects(w, r, d, cloud)
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
			render(w, r, ErrorPartial("Failed to compute sync issues: "+err.Error()))
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
			render(w, r, ErrorPartial("Failed to load project detail: "+err.Error()))
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

// handleSyncCloudAction returns a handler that runs a cloud-control op on the
// {project} path value and re-renders the refreshed projects table. Shared by
// enroll and unenroll, which are identical except for the op and the verb used
// in the error message. Wrapped in requireRW, so RWDB is non-nil here.
func handleSyncCloudAction(d Deps, cloud CloudController, verb string, op func(context.Context, string) (services.CloudResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		if _, err := op(r.Context(), project); err != nil {
			render(w, r, ErrorPartial(verb+" failed: "+err.Error()))
			return
		}
		renderSyncProjects(w, r, d, cloud)
	}
}

// handleSyncDeleteProject serves POST /doctor/sync/{project}/delete.
// It permanently removes an unenrolled, observation-free project (its sessions
// and prompts) and re-renders the refreshed projects table. The destructive
// guards live in services.DeleteProject; the UI only offers the button when the
// project is eligible. Wrapped in requireRW, so RWDB is non-nil here.
func handleSyncDeleteProject(d Deps, cloud CloudController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		if _, err := services.DeleteProject(r.Context(), d.RWDB, project); err != nil {
			render(w, r, ErrorPartial("Delete failed: "+err.Error()))
			return
		}
		renderSyncProjects(w, r, d, cloud)
	}
}

// handleSyncTrigger serves POST /doctor/sync/{project}/sync.
// Invoked from the "Sync Now" button on the project detail page; returns the
// refreshed SyncProjectDetailPartial (target #sync-detail-{slug}, which exists
// on that page). Wrapped in requireRW, so RWDB is non-nil here.
func handleSyncTrigger(d Deps, cloud CloudController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")

		// Guard RoDB so the post-sync reload (SyncGetProjectDetail) never
		// dereferences a nil pool — consistent with handleSyncProjectDetail.
		if d.RoDB == nil {
			render(w, r, ErrorPartial("Sync detail is unavailable in this mode."))
			return
		}

		if _, err := cloud.Sync(r.Context(), project); err != nil {
			render(w, r, ErrorPartial("Sync failed: "+err.Error()))
			return
		}

		detail, loadErr := services.SyncGetProjectDetail(d.RoDB, project)
		if loadErr != nil {
			render(w, r, ErrorPartial("Reload failed: "+loadErr.Error()))
			return
		}
		if detail == nil {
			render(w, r, ErrorPartial("Project not found after sync: "+project))
			return
		}
		render(w, r, SyncProjectDetailPartial(detail))
	}
}
