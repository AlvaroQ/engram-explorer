package codexsessions

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// DetailPrefix is the URL prefix of the detail route this provider mounts:
// agentsessions.DetailPath of a Codex session starts with it.
const DetailPrefix = agentsessions.RoutePrefix + "/" + SourceID + "/"

// Routes registers the routes this provider owns, gated by the host on the
// provider being Enabled (a disabled module answers the standard 503):
//
//   - GET /agent-sessions/codex/{key...}: the session detail, rendered by the
//     shared agent-session page. The key is the rollout path relative to
//     <home>/sessions; the source rejects anything else (traversal included).
func (p *codexProvider) Routes(r providers.Router) {
	r.HandleFunc("GET "+DetailPrefix+"{key...}", func(w http.ResponseWriter, req *http.Request) {
		ui.RenderAgentSessionDetail(w, req, providers.SourceByID(r.Live(req), SourceID), req.PathValue("key"))
	})
}
