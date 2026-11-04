package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// moduleRouter is the providers.Router handed to Provider.Routes: it gates
// the provider's own routes on that provider being Enabled.
type moduleRouter struct {
	capabilityMux
	live func() providers.Instance
}

var _ providers.Router = moduleRouter{}

func newModuleRouter(mux *http.ServeMux, reg *providers.Registry, id string, expose bool) moduleRouter {
	live := func() providers.Instance { return reg.Instance(id) }
	return moduleRouter{
		capabilityMux: capabilityMux{mux: mux, available: func() bool { return live() != nil }, expose: expose},
		live:          live,
	}
}

func (m moduleRouter) Live() providers.Instance { return m.live() }
