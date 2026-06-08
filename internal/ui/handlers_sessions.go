package ui

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// handleSessionDetailPage serves GET /sessions/{id}.
// Full page on direct GET; partial content when HX-Request: true.
// Returns 404 when RoDB is nil or the session does not exist.
func handleSessionDetailPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.RoDB == nil {
			http.NotFound(w, r)
			return
		}

		id := r.PathValue("id")
		lang := langForRequest(r)
		theme := themeForRequest(r)

		detail, err := services.SessionsGetDetail(d.RoDB, id)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load session: "+err.Error()))
			return
		}
		if detail == nil {
			http.NotFound(w, r)
			return
		}

		if IsHTMX(r) {
			render(w, r, SessionDetailPartial(detail, lang))
		} else {
			render(w, r, SessionDetailPage(detail, lang, theme))
		}
	}
}
