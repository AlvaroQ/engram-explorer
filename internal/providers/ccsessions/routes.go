package ccsessions

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// Routes registers the routes the Claude Code provider owns. The host gates
// them on this provider being Enabled and hands each request the instance
// pinned for it (r.Live), so a disabled module answers the standard 503:
//
//   - GET /agent-sessions/claude-code/{account}/{key...}: the session detail
//     of one Claude account, rendered by the shared agent-session page. The
//     cross-agent overview and list are host routes (/agent-sessions...).
//   - GET /cc-sessions/{project}/{id}: the pre-Phase-3 detail URL, 301 to the
//     detail above (account from ?account=, else the first enabled one).
//   - GET /api/cc-sessions/stats: DEPRECATED Claude-only stats in their
//     original shape; use GET /api/agent-sessions/stats.
func (p *ccProvider) Routes(r providers.Router) {
	r.HandleFunc("GET "+DetailPrefix+"{account}/{key...}", func(w http.ResponseWriter, req *http.Request) {
		src := providers.SourceByID(r.Live(req), sourcePrefix+req.PathValue("account"))
		ui.RenderAgentSessionDetail(w, req, src, req.PathValue("key"))
	})
	r.HandleFunc("GET /cc-sessions/{project}/{id}", func(w http.ResponseWriter, req *http.Request) {
		inst, _ := r.Live(req).(*ccInstance)
		project, id := req.PathValue("project"), req.PathValue("id")
		if inst == nil || services.ValidateCCSessionPath(project, id) != nil {
			http.NotFound(w, req)
			return
		}
		account := req.URL.Query().Get("account")
		if account == "" {
			account = inst.liveAccounts()[0].id
		}
		http.Redirect(w, req, agentsessions.DetailPath(sourcePrefix+account, project+"/"+id), http.StatusMovedPermanently)
	})
	r.HandleFunc("GET /api/cc-sessions/stats", func(w http.ResponseWriter, req *http.Request) {
		inst, _ := r.Live(req).(*ccInstance)
		if inst == nil {
			http.NotFound(w, req)
			return
		}
		p.serveDeprecatedStats(w, inst)
	})
}

// serveDeprecatedStats answers GET /api/cc-sessions/stats with the exact
// pre-Phase-3 shape (services.CCStatsResult over every enabled Claude
// account), cached by the accounts' fingerprints, plus the RFC 9745
// Deprecation header and a successor-version link.
func (p *ccProvider) serveDeprecatedStats(w http.ResponseWriter, inst *ccInstance) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", `</api/agent-sessions/stats>; rel="successor-version"`)

	sources := inst.accountSources()
	fp := ""
	for _, src := range sources {
		fp += "|" + url.PathEscape(src.ID) + ":" + services.CCSessionsFingerprint(src.Reader)
	}
	p.stats.mu.Lock()
	defer p.stats.mu.Unlock()
	if p.stats.val == nil || p.stats.key != fp {
		multi, err := services.CCSessionsStatsMulti(sources)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]string{
				"code": "INTERNAL", "message": "failed to compute Claude Code session stats",
			}})
			return
		}
		p.stats.key, p.stats.val = fp, multi.Total
	}
	writeJSON(w, http.StatusOK, p.stats.val)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
