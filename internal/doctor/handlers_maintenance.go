package doctor

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// handleMaintenancePage serves GET /settings/maintenance.
// It renders the consolidated maintenance page using the shared ui.Layout shell
// with activeNav="settings" so the gear icon stays highlighted. The page body
// contains two subsections:
//   - #unassigned — orphaned items (deferred via hx-get=/doctor/orphans/list)
//   - #cloud      — cloud backup / sync shell (inline syncShellContent)
//
// This handler lives in internal/doctor (not internal/ui) because doctor imports
// internal/ui for the layout — the reverse import would create a cycle.
func handleMaintenancePage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := ui.LangForRequest(r)
		// HTMX partial swap (e.g. hx-boost navigation) gets just the body;
		// a direct browser load gets the full page shell. Mirrors the
		// IsHTMX(r) pattern used by the other page handlers in this package.
		if IsHTMX(r) {
			render(w, r, maintenanceContent(lang))
			return
		}
		theme := ui.ThemeForRequest(r)
		sidebarState := ui.SidebarStateForRequest(r)
		render(w, r, MaintenancePage(lang, theme, sidebarState))
	}
}
