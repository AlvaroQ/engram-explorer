package httpapi

import (
	"net/http"
	"sync"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func ccSessionsRoutes(mux *http.ServeMux, c *Container) {
	cache := &ccStatsCache{}
	mux.HandleFunc("GET /api/cc-sessions/stats", handleCCSessionsStats(c, cache))
}

// ccStatsCache memoizes the heavy full-sweep stats result behind a cheap
// directory fingerprint so a 542MB scan runs once and is reused until a
// transcript changes.
type ccStatsCache struct {
	mu  sync.RWMutex
	key string
	val *services.CCStatsResult
}

// handleCCSessionsStats serves GET /api/cc-sessions/stats — aggregate token/cost
// analytics over all Claude Code transcripts (by project, model, day, top cost).
func handleCCSessionsStats(c *Container, cache *ccStatsCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reader := services.DiskProjectsReader{BaseDir: c.Config.ClaudeProjectsDir}

		fp := services.CCSessionsFingerprint(reader)
		cache.mu.RLock()
		if cache.val != nil && cache.key == fp && fp != "" {
			v := cache.val
			cache.mu.RUnlock()
			writeJSON(w, http.StatusOK, v)
			return
		}
		cache.mu.RUnlock()

		v, err := services.CCSessionsStats(reader)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"failed to compute Claude Code session stats", err.Error(), c.Config.ExposeDetails)
			return
		}

		cache.mu.Lock()
		cache.key = fp
		cache.val = v
		cache.mu.Unlock()

		writeJSON(w, http.StatusOK, v)
	}
}
