package httpapi

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// agentSessionsRoutes mounts the cross-agent JSON API. The route is gated on
// a live AgentSessionSource (503 MODULE_DISABLED otherwise) and reads the
// sources of every live one pinned for the request.
func agentSessionsRoutes(mux routeMux, c *Container) {
	cache := &agentStatsCache{}
	mux.HandleFunc("GET /api/agent-sessions/stats", handleAgentSessionsStats(c, cache))
}

// agentStatsCache memoizes the last stats result behind the combined source
// fingerprint (stat-only) plus the filters, so a full transcript sweep runs
// once and is reused until a session file changes.
type agentStatsCache struct {
	mu  sync.Mutex
	key string
	val *agentsessions.StatsResult
}

// handleAgentSessionsStats serves GET /api/agent-sessions/stats: the neutral
// StatsResult (disjoint buckets input / cache_read / cache_write / output,
// reasoning reported apart, cost, unknown-model flag, per agent / project /
// model / day and top sessions). Optional filters: agent, source, project,
// model, q, from, to (YYYY-MM-DD, UTC; to inclusive).
func handleAgentSessionsStats(c *Container, cache *agentStatsCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sources := c.agentSessionSources(r)
		q := r.URL.Query()
		p := statsParams(q)
		key := agentsessions.Fingerprint(r.Context(), sources) + "?" + q.Encode()

		cache.mu.Lock()
		if cache.val != nil && cache.key == key {
			v := cache.val
			cache.mu.Unlock()
			writeJSON(w, http.StatusOK, v)
			return
		}
		cache.mu.Unlock()

		res, err := agentsessions.Stats(r.Context(), sources, p)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"failed to compute agent session stats", err.Error(), c.Config.ExposeDetails)
			return
		}
		cache.mu.Lock()
		cache.key, cache.val = key, res
		cache.mu.Unlock()
		writeJSON(w, http.StatusOK, res)
	}
}

func statsParams(q url.Values) agentsessions.ListParams {
	p := agentsessions.ListParams{
		Query: q.Get("q"), Agent: q.Get("agent"), Source: q.Get("source"),
		Project: q.Get("project"), Model: q.Get("model"),
	}
	if t, err := time.Parse(time.DateOnly, q.Get("from")); err == nil {
		p.From = t
	}
	if t, err := time.Parse(time.DateOnly, q.Get("to")); err == nil {
		p.To = t.AddDate(0, 0, 1)
	}
	return p
}
