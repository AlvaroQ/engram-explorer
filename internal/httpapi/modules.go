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
// /api/, the localized HTML page (or htmx fragment) elsewhere.
func (c *Container) writeModuleDisabled(w http.ResponseWriter, r *http.Request) {
	if isAPIPath(r.URL.Path) {
		writeError(w, http.StatusServiceUnavailable, codeModuleDisabled,
			"This module is disabled. Enable it in Settings → Modules.", nil, c.Config.ExposeDetails)
		return
	}
	ui.RenderModuleDisabled(w, r, c.uiShell())
}

// uiShell is the part of ui.Deps the disabled-module page needs for the
// layout: the sidebar groups and the account switcher.
func (c *Container) uiShell() ui.Deps {
	d := ui.Deps{Profiles: buildProfilesFn(c.ProfileStore)}
	if c.Registry != nil {
		d.NavGroups = c.Registry.NavGroups
	}
	return d
}

// withShell serves h with the uiShell data (sidebar NavGroups and profiles)
// in the request context. ui.Render builds on the request context, so a page
// a provider renders with it (the doctor pages) gets the same registry-driven
// sidebar and account switcher as the pages the ui package renders itself.
func (c *Container) withShell(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := c.uiShell()
		ctx := r.Context()
		if d.NavGroups != nil {
			ctx = ui.WithNavGroups(ctx, d.NavGroups())
		}
		if d.Profiles != nil {
			ctx = ui.WithProfiles(ctx, d.Profiles())
		}
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// gate serves h only while available reports true for the request.
func (c *Container) gate(available func(*http.Request) bool, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available(r) {
			c.writeModuleDisabled(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// capabilityMux mounts host route families on mux, gated on an Enabled
// instance matching the capability.
type capabilityMux struct {
	mux       *http.ServeMux
	c         *Container
	available func(*http.Request) bool
}

func (c *Container) capabilityMux(mux *http.ServeMux, match func(providers.Instance) bool) capabilityMux {
	return capabilityMux{
		mux:       mux,
		c:         c,
		available: func(r *http.Request) bool { return c.find(r, match) != nil },
	}
}

func (m capabilityMux) Handle(pattern string, h http.Handler) {
	m.mux.Handle(pattern, m.c.gate(m.available, h))
}

func (m capabilityMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(h))
}

// uiLive reports the module state of r for ui.Deps.Live.
func (c *Container) uiLive(r *http.Request) ui.LiveHandles {
	ro, rw := c.storeHandles(r)
	return ui.LiveHandles{RoDB: ro, RWDB: rw, Claude: c.find(r, isTranscriptSource) != nil}
}
