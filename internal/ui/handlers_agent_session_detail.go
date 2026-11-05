package ui

import (
	"errors"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// RenderAgentSessionDetail serves the detail page of the session key of src.
// Agent-session providers call it from the source-scoped detail route they
// mount (Provider.Routes), so every agent renders through the same page. A
// nil source, an invalid key (path traversal included) or an unknown session
// answers 404; read errors render the generic error partial.
func RenderAgentSessionDetail(w http.ResponseWriter, r *http.Request, src agentsessions.Source, key string) {
	if src == nil {
		http.NotFound(w, r)
		return
	}
	d, err := src.Detail(r.Context(), key)
	if errors.Is(err, agentsessions.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		RenderError(w, r, "errors.loadSession", err)
		return
	}
	d.Session.Source, d.Session.Agent = src.ID(), src.Agent()
	lang := langForRequest(r)
	if IsHTMX(r) {
		Render(w, r, AgentSessionDetailPartial(d, lang))
		return
	}
	Render(w, r, AgentSessionDetailPage(d, lang, themeForRequest(r), sidebarStateForRequest(r)))
}
