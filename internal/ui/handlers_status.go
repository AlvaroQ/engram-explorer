package ui

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/daemon"
)

// statusState is the resolved backend health state shown by the sidebar status
// pill. The order encodes severity precedence (worst first) used when mapping
// the live DB/daemon checks to a single pill state.
type statusState string

const (
	statusChecking      statusState = "checking"
	statusAllOk         statusState = "allOk"
	statusDBError       statusState = "dbError"
	statusDaemonOffline statusState = "daemonOffline"
	// statusBackendUnreachable is only reachable client-side (the HTMX request
	// itself fails). It is defined here so the i18n key has a server-side home,
	// but the server never emits it — when the server responds, the backend is
	// by definition reachable.
	statusBackendUnreachable statusState = "backendUnreachable"
)

// statusPillData carries the resolved health state to the statusPill template.
type statusPillData struct {
	Lang  string
	State statusState
}

// statusLabelKey maps a statusState to its sidebar.status.* i18n key.
func statusLabelKey(s statusState) string {
	switch s {
	case statusChecking:
		return "sidebar.status.checking"
	case statusDBError:
		return "sidebar.status.dbError"
	case statusDaemonOffline:
		return "sidebar.status.daemonOffline"
	case statusBackendUnreachable:
		return "sidebar.status.backendUnreachable"
	default:
		return "sidebar.status.allOk"
	}
}

// resolveStatusState performs best-effort live health checks (DB ping + daemon
// ping) and maps them to a single pill state. DB errors take precedence over a
// daemon being offline, mirroring the /api/health semantics in
// internal/httpapi/server.go (db.ok and daemon.ok).
func resolveStatusState(d Deps) statusState {
	// DB health: a nil pool (read-only / no-provider mode) or a failing SELECT 1
	// both count as a DB error for the pill.
	dbOk := false
	if d.RoDB != nil {
		var n int
		if err := d.RoDB.QueryRow("SELECT 1").Scan(&n); err == nil {
			dbOk = n == 1
		}
	}
	if !dbOk {
		return statusDBError
	}

	dc := daemon.New(d.Config.DaemonBaseURL, d.Config.DaemonTimeoutMs)
	if !dc.FetchJSON("/health").OK {
		return statusDaemonOffline
	}

	return statusAllOk
}

// handleStatusPillPartial serves GET /partials/status-pill.
// It always returns the pill fragment (200) so the sidebar HTMX poll can swap
// it in place. The fragment carries its own hx-get + hx-trigger so it keeps
// polling after each swap.
func handleStatusPillPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		data := statusPillData{
			Lang:  lang,
			State: resolveStatusState(d),
		}
		render(w, r, statusPill(data))
	}
}
