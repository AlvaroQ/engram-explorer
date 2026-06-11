package ui

import (
	"context"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// cloudSyncer is the narrow capability needed to trigger a per-project cloud
// sync. The production *services.CloudControlService satisfies it; test fakes
// (which only implement projectsCloud) do not, so the handler degrades to the
// "no enrolled / unavailable" state instead of breaking those tests.
type cloudSyncer interface {
	Sync(ctx context.Context, project string) (services.CloudResult, error)
}

// syncCloudResult carries the outcome of a sync-all run to the template.
type syncCloudResult struct {
	Lang string
	// State drives which message/variant the result button shows.
	//   "ok"        — all enrolled projects synced
	//   "partial"   — some failed
	//   "failed"    — the run could not start (e.g. DB error) or all failed
	//   "noEnrolled"— no enrolled projects to sync
	State   string
	OK      int
	Failed  int
	Total   int
	Message string
}

// handleSyncCloudPost serves POST /partials/sync-cloud.
// It enumerates enrolled projects (read-only) and triggers a cloud sync for
// each via the real CloudControlService, then swaps a result fragment back into
// the sidebar sync container. This mirrors POST /api/cloud/sync-all but renders
// HTML for the HTMX-driven sidebar instead of JSON.
func handleSyncCloudPost(d Deps, cloud projectsCloud) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)

		syncer, ok := cloud.(cloudSyncer)
		if !ok || d.RoDB == nil {
			// No real trigger available (test fake or read-only mode): degrade.
			render(w, r, syncCloudButtonWithResult(syncCloudResult{
				Lang:    lang,
				State:   "noEnrolled",
				Message: T(lang, "sidebar.sync.noEnrolled"),
			}))
			return
		}

		syncData, err := services.SyncListProjects(d.RoDB)
		if err != nil {
			render(w, r, syncCloudButtonWithResult(syncCloudResult{
				Lang:    lang,
				State:   "failed",
				Message: T(lang, "sidebar.sync.failed", "message", err.Error()),
			}))
			return
		}

		var enrolled []string
		for _, p := range syncData.Projects {
			if p.Enrolled {
				enrolled = append(enrolled, p.Project)
			}
		}
		if len(enrolled) == 0 {
			render(w, r, syncCloudButtonWithResult(syncCloudResult{
				Lang:    lang,
				State:   "noEnrolled",
				Message: T(lang, "sidebar.sync.noEnrolled"),
			}))
			return
		}

		okCount := 0
		for _, proj := range enrolled {
			if _, syncErr := syncer.Sync(r.Context(), proj); syncErr == nil {
				okCount++
			}
		}
		failed := len(enrolled) - okCount

		res := syncCloudResult{Lang: lang, OK: okCount, Failed: failed, Total: len(enrolled)}
		switch {
		case failed == 0:
			res.State = "ok"
			res.Message = T(lang, "sidebar.sync.ok", "ok", itoa(okCount), "total", itoa(len(enrolled)))
		case okCount == 0:
			res.State = "failed"
			res.Message = T(lang, "sidebar.sync.partial", "ok", itoa(okCount), "total", itoa(len(enrolled)), "failed", itoa(failed))
		default:
			res.State = "partial"
			res.Message = T(lang, "sidebar.sync.partial", "ok", itoa(okCount), "total", itoa(len(enrolled)), "failed", itoa(failed))
		}
		render(w, r, syncCloudButtonWithResult(res))
	}
}
