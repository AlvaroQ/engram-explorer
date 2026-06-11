package ui

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/daemon"
)

// handleSettingsPage serves GET /settings.
// Collects live system info (DB ping, daemon reachability) and renders the page.
// Full page on direct GET; partial content when HX-Request: true.
func handleSettingsPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)

		data := buildSettingsData(d, r, lang, theme)

		if IsHTMX(r) {
			renderDeps(w, r, d, SettingsPartial(data))
		} else {
			renderDeps(w, r, d, SettingsPage(data))
		}
	}
}

// handleNavVisibilityPost serves POST /settings/nav-visibility. It toggles
// whether a sidebar section (engram|claude) is shown, via a cookie, then
// reloads /settings so the sidebar re-renders.
func handleNavVisibilityPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		// Section values are provider IDs: "engram", "cc-sessions".
		// The legacy "claude" value is also accepted for backward compat.
		section := r.FormValue("section")
		var cookieName string
		switch section {
		case "engram":
			cookieName = "nav_engram"
		case "cc-sessions", "claude":
			// "claude" is the legacy value; map to the provider-ID cookie.
			cookieName = "nav_cc-sessions"
		default:
			http.Error(w, "invalid section", http.StatusBadRequest)
			return
		}

		cookieVal := "1"
		if r.FormValue("value") == "hide" {
			cookieVal = "0"
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    cookieVal,
			Path:     "/",
			MaxAge:   int(365 * 24 * time.Hour / time.Second),
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})

		if IsHTMX(r) {
			w.Header().Set("HX-Redirect", "/settings")
			w.WriteHeader(http.StatusOK)
		} else {
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
		}
	}
}

// handleThemePost serves POST /theme.
// Sets the "theme" cookie (dark|light, Path=/, 1 year) and redirects back to
// /settings so the full page reloads with the new theme applied.
// requireRW does NOT apply — this is a user preference, not a DB write.
func handleThemePost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		value := r.FormValue("value")
		if value != "dark" && value != "light" {
			http.Error(w, "invalid theme value", http.StatusBadRequest)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "theme",
			Value:    value,
			Path:     "/",
			MaxAge:   int(365 * 24 * time.Hour / time.Second),
			HttpOnly: false, // read by server only — JS doesn't need it
			SameSite: http.SameSiteLaxMode,
		})

		// HX-Redirect triggers a full-page reload by HTMX; plain requests use 303.
		if IsHTMX(r) {
			w.Header().Set("HX-Redirect", "/settings")
			w.WriteHeader(http.StatusOK)
		} else {
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
		}
	}
}

// handleLangPost serves POST /lang.
// Sets the "lang" cookie (en|es, Path=/, 1 year) and redirects back to
// /settings so the full page reloads with the new language applied.
// requireRW does NOT apply — this is a user preference, not a DB write.
func handleLangPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		value := r.FormValue("value")
		if value != "en" && value != "es" {
			http.Error(w, "invalid lang value", http.StatusBadRequest)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "lang",
			Value:    value,
			Path:     "/",
			MaxAge:   int(365 * 24 * time.Hour / time.Second),
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})

		if IsHTMX(r) {
			w.Header().Set("HX-Redirect", "/settings")
			w.WriteHeader(http.StatusOK)
		} else {
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
		}
	}
}

// buildSettingsData collects live system data for the settings page.
// It performs a best-effort daemon ping — failures are reflected in the returned
// struct rather than propagated as handler errors.
func buildSettingsData(d Deps, r *http.Request, lang, theme string) settingsData {
	// DB health check.
	dbOk := false
	if d.RoDB != nil {
		var n int
		if err := d.RoDB.QueryRow("SELECT 1").Scan(&n); err == nil {
			dbOk = n == 1
		}
	}

	// Daemon reachability check.
	dc := daemon.New(d.Config.DaemonBaseURL, d.Config.DaemonTimeoutMs)
	daemonResult := dc.FetchJSON("/health")

	daemonErrMsg := ""
	if !daemonResult.OK && daemonResult.Error != nil {
		daemonErrMsg = daemonResult.Error.Message
	}

	// Claude Code projects directory check (it is scanned, not connected).
	claudePath := d.Paths.ClaudeDir()
	claudeOk := false
	if claudePath != "" {
		if info, err := os.Stat(claudePath); err == nil && info.IsDir() {
			claudeOk = true
		}
	}

	prefs := navPrefsForRequest(r)

	return settingsData{
		DBPath:      d.Paths.EngramDB(),
		DBOk:        dbOk,
		ClaudePath:  claudePath,
		ClaudeOk:    claudeOk,
		DaemonURL:   d.Config.DaemonBaseURL,
		DaemonOk:    daemonResult.OK,
		DaemonError: daemonErrMsg,
		ShowNavGroups: map[string]bool{
			"engram":      prefs.isShown("engram"),
			"cc-sessions": prefs.isShown("cc-sessions"),
		},
		Lang:  lang,
		Theme: theme,
	}
}

// handleEngramDBPost serves POST /settings/engram-db. It hot-swaps the SQLite
// pools to a new database file via the ReloadEngramDB callback and persists the
// override. On error it re-renders the settings page with an inline message.
func handleEngramDBPost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		path := strings.TrimSpace(r.FormValue("path"))
		lang := langForRequest(r)
		theme := themeForRequest(r)

		errMsg := ""
		switch {
		case path == "":
			errMsg = T(lang, "settings.path.empty")
		case d.ReloadEngramDB == nil:
			errMsg = T(lang, "settings.path.unavailable")
		default:
			if err := d.ReloadEngramDB(path); err != nil {
				errMsg = err.Error()
			}
		}

		if errMsg != "" {
			data := buildSettingsData(d, r, lang, theme)
			data.EngramPathError = errMsg
			renderSettings(w, r, d, data)
			return
		}
		redirectSettings(w, r)
	}
}

// handleClaudeDirPost serves POST /settings/claude-dir. It applies a new Claude
// Code projects directory live via the SetClaudeDir callback.
func handleClaudeDirPost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		path := strings.TrimSpace(r.FormValue("path"))
		lang := langForRequest(r)
		theme := themeForRequest(r)

		errMsg := ""
		switch {
		case path == "":
			errMsg = T(lang, "settings.path.empty")
		case d.SetClaudeDir == nil:
			errMsg = T(lang, "settings.path.unavailable")
		default:
			if err := d.SetClaudeDir(path); err != nil {
				errMsg = err.Error()
			}
		}

		if errMsg != "" {
			data := buildSettingsData(d, r, lang, theme)
			data.ClaudePathError = errMsg
			renderSettings(w, r, d, data)
			return
		}
		redirectSettings(w, r)
	}
}

// renderSettings renders the settings page (partial for HTMX, full otherwise).
// d is used to pass NavGroups into the render context.
func renderSettings(w http.ResponseWriter, r *http.Request, d Deps, data settingsData) {
	if IsHTMX(r) {
		renderDeps(w, r, d, SettingsPartial(data))
	} else {
		renderDeps(w, r, d, SettingsPage(data))
	}
}

// redirectSettings reloads /settings after a successful change (HX-Redirect for
// HTMX, 303 otherwise).
func redirectSettings(w http.ResponseWriter, r *http.Request) {
	if IsHTMX(r) {
		w.Header().Set("HX-Redirect", "/settings")
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
	}
}
