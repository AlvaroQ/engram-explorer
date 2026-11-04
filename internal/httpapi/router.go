package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// moduleRouter is the providers.Router handed to Provider.Routes: it gates
// the provider's own routes on that provider being Enabled.
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

// Live returns the instance pinned for r (see withLeases).
func (m moduleRouter) Live(r *http.Request) providers.Instance { return m.live(r) }
