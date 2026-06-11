package ui

import (
	"context"
	"net/http"

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

// navPrefs holds per-provider show/hide prefs. True means visible (default).
// Keys are provider IDs (e.g. "engram", "cc-sessions").
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

// navPrefsForRequest reads per-provider section-visibility cookies.
// Cookie name format: nav_{providerID}; value "0" means hidden.
// Supported provider IDs: "engram", "cc-sessions" (and any future providers).
func navPrefsForRequest(r *http.Request) navPrefs {
	return navPrefs{
		shown: map[string]bool{
			"engram":      !cookieHidden(r, "nav_engram"),
			"cc-sessions": !cookieHidden(r, "nav_cc-sessions"),
		},
	}
}

func cookieHidden(r *http.Request, name string) bool {
	c, err := r.Cookie(name)
	return err == nil && c.Value == "0"
}

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
// NavGroups obtained by calling the groups func. Used by render() when a
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
	// Default: all providers visible.
	return navPrefs{shown: map[string]bool{
		"engram":      true,
		"cc-sessions": true,
	}}
}

// navShowGroup returns true when the provider group with the given ID should be
// shown according to the per-request nav prefs stored in ctx.
func navShowGroup(ctx context.Context, providerID string) bool {
	return navPrefsFromContext(ctx).isShown(providerID)
}

// navShowEngram / navShowClaude are kept for backward compat during the
// transition: legacy templ files that call these helpers directly still work.
// They are NOT removed in WU-7 to avoid breaking unrelated templ files.
func navShowEngram(ctx context.Context) bool { return navShowGroup(ctx, "engram") }
func navShowClaude(ctx context.Context) bool { return navShowGroup(ctx, "cc-sessions") }
