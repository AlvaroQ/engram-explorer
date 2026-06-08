package ui

import (
	"net/http"
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

		data := buildSettingsData(d, lang, theme)

		if IsHTMX(r) {
			render(w, r, SettingsPartial(data))
		} else {
			render(w, r, SettingsPage(data))
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
func buildSettingsData(d Deps, lang, theme string) settingsData {
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

	return settingsData{
		DBPath:      d.Config.EngramDbPath,
		DBOk:        dbOk,
		DaemonURL:   d.Config.DaemonBaseURL,
		DaemonOk:    daemonResult.OK,
		DaemonError: daemonErrMsg,
		Lang:        lang,
		Theme:       theme,
	}
}
