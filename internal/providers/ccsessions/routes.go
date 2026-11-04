package ccsessions

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// Routes registers the routes the Claude Code provider owns. The host gates
// them on this provider being Enabled and hands each request the instance
// pinned for it (r.Live), so a disabled module answers the standard 503:
//
//   - GET /agent-sessions/claude-code/{account}/{key...}: the session detail
//     of one Claude account, rendered by the shared agent-session page. The
//     cross-agent overview and list are host routes (/agent-sessions...).
func (p *ccProvider) Routes(r providers.Router) {
	r.HandleFunc("GET "+DetailPrefix+"{account}/{key...}", func(w http.ResponseWriter, req *http.Request) {
		src := providers.SourceByID(r.Live(req), sourcePrefix+req.PathValue("account"))
		ui.RenderAgentSessionDetail(w, req, src, req.PathValue("key"))
	})
}
