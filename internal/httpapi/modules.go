package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// Every route is mounted once at boot, whatever the state of the providers.
// Each request resolves the live instance it needs from the registry through
// a capability (an interface the instance implements); when no Enabled
// instance has it, the route answers 503 MODULE_DISABLED. Nothing here
// switches on a provider ID.

// codeModuleDisabled is the error code of a route whose module is disabled.
const codeModuleDisabled = "MODULE_DISABLED"

// writeModuleDisabled answers 503: the MODULE_DISABLED JSON envelope under
// /api/, the localized HTML message elsewhere.
func writeModuleDisabled(w http.ResponseWriter, r *http.Request, exposeDetails bool) {
	if isAPIPath(r.URL.Path) {
		writeError(w, http.StatusServiceUnavailable, codeModuleDisabled,
			"This module is disabled. Enable it in Settings → Modules.", nil, exposeDetails)
		return
	}
	ui.RenderModuleDisabled(w, r)
}

// gate serves h only while available reports true.
func gate(available func() bool, exposeDetails bool, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available() {
			writeModuleDisabled(w, r, exposeDetails)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// capabilityMux mounts host route families on mux, gated on an Enabled
// instance matching the capability.
type capabilityMux struct {
	mux       *http.ServeMux
	available func() bool
	expose    bool
}

func (c *Container) capabilityMux(mux *http.ServeMux, match func(providers.Instance) bool) capabilityMux {
	return capabilityMux{
		mux:       mux,
		available: func() bool { return c.Registry.Find(match) != nil },
		expose:    c.Config.ExposeDetails,
	}
}

func (m capabilityMux) Handle(pattern string, h http.Handler) {
	m.mux.Handle(pattern, gate(m.available, m.expose, h))
}

func (m capabilityMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(h))
}

// uiLive reports the module state for ui.Deps.Live.
func (c *Container) uiLive() ui.LiveHandles {
	ro, rw := c.storeHandles()
	return ui.LiveHandles{RoDB: ro, RWDB: rw, Claude: c.Registry.Find(isTranscriptSource) != nil}
}
