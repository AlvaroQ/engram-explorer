package ui

import (
	"context"
	"net/http"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// ---------------------------------------------------------------------------
// Nav groups context — carries registry NavGroups per-request for the sidebar
// ---------------------------------------------------------------------------

type navGroupsKeyType struct{}

var navGroupsContextKey = navGroupsKeyType{}

// WithNavGroups returns a context enriched with the given NavGroups slice.
// The shared Sidebar component reads them via navGroupsFromContext.
func WithNavGroups(ctx context.Context, groups []providers.NavGroup) context.Context {
	return context.WithValue(ctx, navGroupsContextKey, groups)
}

// navGroupsFromContext returns the NavGroups stored in ctx, or nil if none.
func navGroupsFromContext(ctx context.Context) []providers.NavGroup {
	v, _ := ctx.Value(navGroupsContextKey).([]providers.NavGroup)
	return v
}

// ---------------------------------------------------------------------------
// Per-provider nav visibility — keyed by provider ID
// ---------------------------------------------------------------------------

type navPrefsKeyType struct{}

var navPrefsContextKey = navPrefsKeyType{}

// navPrefs holds per-group show/hide prefs. True means visible (default).
// Keys are NavGroup IDs.
type navPrefs struct {
	shown map[string]bool
}

// isShown returns true when the provider section with the given ID is visible.
// Defaults to true when no cookie is set (show by default).
func (p navPrefs) isShown(providerID string) bool {
	if p.shown == nil {
		return true
	}
	v, ok := p.shown[providerID]
	if !ok {
		return true // unknown providers default to visible
	}
	return v
}

// navPrefsForRequest reads the per-group section-visibility cookies:
// nav_{groupID}; value "0" means hidden, anything else (or none) shown.
func navPrefsForRequest(r *http.Request) navPrefs {
	shown := map[string]bool{}
	for _, c := range r.Cookies() {
		if id, ok := strings.CutPrefix(c.Name, navCookiePrefix); ok && id != "" {
			shown[id] = c.Value != "0"
		}
	}
	return navPrefs{shown: shown}
}

// navCookiePrefix prefixes the group ID in a section-visibility cookie name.
const navCookiePrefix = "nav_"

// NavContext returns r's context enriched with:
//  1. The resolved nav prefs (per-provider show/hide state from cookies)
//  2. The NavGroups from the Deps.NavGroups func, if provided
//
// The shared Sidebar (rendered by both the ui and doctor modules) reads them
// without threading parameters through every page. Exported for doctor.
func NavContext(r *http.Request) context.Context {
	return context.WithValue(r.Context(), navPrefsContextKey, navPrefsForRequest(r))
}

// NavContextWithGroups returns r's context enriched with nav prefs AND the
// NavGroups obtained by calling the groups func. Used by Render() when a
// NavGroups func is present on Deps.
func NavContextWithGroups(r *http.Request, groupsFn func() []providers.NavGroup) context.Context {
	ctx := context.WithValue(r.Context(), navPrefsContextKey, navPrefsForRequest(r))
	if groupsFn != nil {
		ctx = WithNavGroups(ctx, groupsFn())
	}
	return ctx
}

// NavContextWithGroupsAndProfiles returns r's context enriched with nav prefs,
// NavGroups, and ProfileInfo slice for the account switcher.
func NavContextWithGroupsAndProfiles(r *http.Request, groupsFn func() []providers.NavGroup, profilesFn func() []ProfileInfo) context.Context {
	ctx := NavContextWithGroups(r, groupsFn)
	if profilesFn != nil {
		ctx = WithProfiles(ctx, profilesFn())
	}
	return ctx
}

// ---------------------------------------------------------------------------
// Profiles context — carries profile list per-request for the account switcher
// ---------------------------------------------------------------------------

type profilesKeyType struct{}

var profilesContextKey = profilesKeyType{}

// WithProfiles returns a context enriched with the given ProfileInfo slice.
// The Sidebar component reads them via profilesFromContext.
func WithProfiles(ctx context.Context, profiles []ProfileInfo) context.Context {
	return context.WithValue(ctx, profilesContextKey, profiles)
}

// profilesFromContext returns the ProfileInfo slice stored in ctx, or nil if none.
func profilesFromContext(ctx context.Context) []ProfileInfo {
	v, _ := ctx.Value(profilesContextKey).([]ProfileInfo)
	return v
}

func navPrefsFromContext(ctx context.Context) navPrefs {
	if v, ok := ctx.Value(navPrefsContextKey).(navPrefs); ok {
		return v
	}
	return navPrefs{} // every group visible
}

// navShowGroup returns true when the provider group with the given ID should be
// shown according to the per-request nav prefs stored in ctx.
func navShowGroup(ctx context.Context, providerID string) bool {
	return navPrefsFromContext(ctx).isShown(providerID)
}

// sidebarGroups returns the NavGroups in ctx, or legacyNavGroups when the
// page was rendered without a registry (tests, legacy container).
func sidebarGroups(ctx context.Context) []providers.NavGroup {
	if groups := navGroupsFromContext(ctx); len(groups) > 0 {
		return groups
	}
	return legacyNavGroups()
}

// legacyNavGroups is the sidebar of a UI mounted without a registry: the
// Engram memory pages and the agent-session pages.
func legacyNavGroups() []providers.NavGroup {
	return []providers.NavGroup{
		{
			ID: "engram", LabelKey: "sidebar.groupEngram", Featured: true, Icon: "/static/engram.png",
			Links: []providers.NavLink{
				{Href: "/", Key: "overview", LabelKey: "sidebar.overview", Icon: "overview"},
				{Href: "/brain", Key: "brain", LabelKey: "sidebar.knowledge", Icon: "brain"},
				{Href: "/observations", Key: "observations", LabelKey: "sidebar.observations", Icon: "observations"},
				{Href: "/projects", Key: "projects", LabelKey: "sidebar.projects", Icon: "projects"},
			},
		},
		providers.AgentSessionsNav(),
	}
}
