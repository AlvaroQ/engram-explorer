package httpapi

import (
	"net/http"
	"sync"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func ccSessionsRoutes(mux *http.ServeMux, c *Container) {
	cache := &ccMultiStatsCache{}
	mux.HandleFunc("GET /api/cc-sessions/stats", handleCCSessionsStats(c, cache))
}

// ccMultiStatsCache memoizes the heavy full-sweep stats result behind a cheap
// directory fingerprint so a 542MB scan runs once and is reused until a
// transcript changes.
type ccMultiStatsCache struct {
	mu  sync.RWMutex
	key string
	val *services.CCStatsMultiResult
}

// handleCCSessionsStats serves GET /api/cc-sessions/stats — aggregate token/cost
// analytics over all Claude Code transcripts (by project, model, day, top cost),
// aggregated across all enabled CC accounts.
func handleCCSessionsStats(c *Container, cache *ccMultiStatsCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sources := buildCCAccountSourcesFn(c.ProfileStore, c.Paths.ClaudeDir(), c.Config.ConfigHome)()

		// Fingerprint: concatenate per-account fingerprints so any transcript
		// change in any account invalidates the cache.
		fp := ""
		for _, src := range sources {
			fp += "|" + services.CCSessionsFingerprint(src.Reader)
		}

		cache.mu.RLock()
		if cache.val != nil && cache.key == fp && fp != "" {
			v := cache.val
			cache.mu.RUnlock()
			writeJSON(w, http.StatusOK, v.Total)
			return
		}
		cache.mu.RUnlock()

		multi, err := services.CCSessionsStatsMulti(sources)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"failed to compute Claude Code session stats", err.Error(), c.Config.ExposeDetails)
			return
		}

		cache.mu.Lock()
		cache.key = fp
		cache.val = multi
		cache.mu.Unlock()

		// The existing cc-usage-charts island expects a CCStatsResult shape,
		// so we serve multi.Total to stay backward compatible with the island.
		writeJSON(w, http.StatusOK, multi.Total)
	}
}
