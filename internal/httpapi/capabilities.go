package httpapi

import (
	"context"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// Search and overview are served by the providers.Searcher and
// providers.StatsSource capabilities of the live instance.

func isSearcher(i providers.Instance) bool    { _, ok := i.(providers.Searcher); return ok }
func isStatsSource(i providers.Instance) bool { _, ok := i.(providers.StatsSource); return ok }

// searcher resolves the Searcher serving r (legacy: the container's own pool).
func (c *Container) searcher(r *http.Request) providers.Searcher {
	if c.Registry == nil {
		return legacySource{c}
	}
	s, _ := c.find(r, isSearcher).(providers.Searcher)
	return s
}

// statsSource resolves the StatsSource serving r (legacy: the container's own pool).
func (c *Container) statsSource(r *http.Request) providers.StatsSource {
	if c.Registry == nil {
		return legacySource{c}
	}
	s, _ := c.find(r, isStatsSource).(providers.StatsSource)
	return s
}

// legacySource gives legacy containers (no registry) the Searcher and
// StatsSource capabilities over their own read-only pool.
type legacySource struct{ c *Container }

func (l legacySource) Search(_ context.Context, q string, limit int) (any, error) {
	return services.ObservationsSearch(l.c.RoDB, q, limit)
}

func (l legacySource) Stats(_ context.Context) (any, error) {
	return services.OverviewBuild(l.c.RoDB)
}
