package ui

import (
	"context"
	"net/http"
)

type navPrefsKeyType struct{}

var navPrefsContextKey = navPrefsKeyType{}

// navPrefs controls which sidebar sections are visible. Both default to true.
type navPrefs struct {
	ShowEngram bool
	ShowClaude bool
}

// navPrefsForRequest reads the section-visibility cookies. Both sections are
// shown by default; a cookie value of "0" hides that section.
func navPrefsForRequest(r *http.Request) navPrefs {
	return navPrefs{
		ShowEngram: !cookieHidden(r, "nav_engram"),
		ShowClaude: !cookieHidden(r, "nav_claude"),
	}
}

func cookieHidden(r *http.Request, name string) bool {
	c, err := r.Cookie(name)
	return err == nil && c.Value == "0"
}

// NavContext returns r's context enriched with the resolved nav prefs so the
// shared Sidebar (rendered by both the ui and doctor modules) can read them
// without threading a parameter through every page. Exported for doctor.
func NavContext(r *http.Request) context.Context {
	return context.WithValue(r.Context(), navPrefsContextKey, navPrefsForRequest(r))
}

func navPrefsFromContext(ctx context.Context) navPrefs {
	if v, ok := ctx.Value(navPrefsContextKey).(navPrefs); ok {
		return v
	}
	return navPrefs{ShowEngram: true, ShowClaude: true}
}

// navShowEngram / navShowClaude are templ helpers reading the prefs from ctx.
func navShowEngram(ctx context.Context) bool { return navPrefsFromContext(ctx).ShowEngram }
func navShowClaude(ctx context.Context) bool { return navPrefsFromContext(ctx).ShowClaude }
