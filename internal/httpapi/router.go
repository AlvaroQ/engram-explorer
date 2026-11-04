package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// moduleRouter is the providers.Router handed to Provider.Routes: it gates
// the provider's own routes on that provider being Enabled, and serves them
// with the UI shell data in the request context (see Container.withShell) so
// provider pages render the same sidebar as the host pages.
type moduleRouter struct {
	capabilityMux
	live func(*http.Request) providers.Instance
}

var _ providers.Router = moduleRouter{}

func newModuleRouter(mux *http.ServeMux, c *Container, id string) moduleRouter {
	live := func(r *http.Request) providers.Instance {
		if l := leaseFrom(r); l != nil {
			return l.Instance(id)
		}
		return c.Registry.Instance(id)
	}
	return moduleRouter{
		capabilityMux: capabilityMux{
			mux:       mux,
			c:         c,
			available: func(r *http.Request) bool { return live(r) != nil },
		},
		live: live,
	}
}

// Handle registers h, gated on the provider and served with the shell context.
func (m moduleRouter) Handle(pattern string, h http.Handler) {
	m.capabilityMux.Handle(pattern, m.c.withShell(h))
}

// HandleFunc is Handle for a handler function.
func (m moduleRouter) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(h))
}

// Live returns the instance pinned for r (see withLeases).
func (m moduleRouter) Live(r *http.Request) providers.Instance { return m.live(r) }
