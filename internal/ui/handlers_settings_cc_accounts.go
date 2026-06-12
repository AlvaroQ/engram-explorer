package ui

import (
	"net/http"
	"strings"
)

// handleCCAccountAddPost serves POST /settings/cc-accounts.
// Validates and adds a new CC account via the AddCCAccount callback.
// On success: redirects back to /settings (HTMX or 303).
// On error: re-renders the settings page with an inline error in the CC accounts section.
func handleCCAccountAddPost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AddCCAccount == nil {
			http.Error(w, "CC account management not available", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		label := strings.TrimSpace(r.FormValue("label"))
		path := strings.TrimSpace(r.FormValue("path"))
		lang := langForRequest(r)
		theme := themeForRequest(r)

		var errMsg string
		switch {
		case label == "":
			errMsg = T(lang, "settings.ccAccounts.labelEmpty")
		case path == "":
			errMsg = T(lang, "settings.path.empty")
		default:
			if err := d.AddCCAccount(label, path); err != nil {
				errMsg = err.Error()
			}
		}

		if errMsg != "" {
			data := buildSettingsData(d, r, lang, theme)
			data.CCAccountError = errMsg
			renderSettings(w, r, d, data)
			return
		}
		redirectSettings(w, r)
	}
}

// handleCCAccountRemovePost serves POST /settings/cc-accounts/{id}/remove.
// Removes a CC account via the RemoveCCAccount callback.
// On success: redirects back to /settings.
// On error: re-renders the settings page with an inline error.
func handleCCAccountRemovePost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.RemoveCCAccount == nil {
			http.Error(w, "CC account management not available", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		id := r.PathValue("id")
		lang := langForRequest(r)
		theme := themeForRequest(r)

		if err := d.RemoveCCAccount(id); err != nil {
			data := buildSettingsData(d, r, lang, theme)
			data.CCAccountError = err.Error()
			renderSettings(w, r, d, data)
			return
		}
		redirectSettings(w, r)
	}
}

// handleCCAccountTogglePost serves POST /settings/cc-accounts/{id}/toggle.
// Enables or disables a CC account via the UpdateCCAccount callback.
// On success: redirects back to /settings.
// On error: re-renders the settings page with an inline error.
func handleCCAccountTogglePost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.UpdateCCAccount == nil {
			http.Error(w, "CC account management not available", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		id := r.PathValue("id")
		enabled := r.FormValue("enabled") == "true"
		lang := langForRequest(r)
		theme := themeForRequest(r)

		// We need the current label and path to issue an update (label/path are
		// preserved; only enabled is toggled). Look up the account from the callback.
		if d.CCAccounts == nil {
			http.Error(w, "CC account management not available", http.StatusServiceUnavailable)
			return
		}
		accounts := d.CCAccounts()
		var label, path string
		found := false
		for _, acc := range accounts {
			if acc.ID == id {
				label = acc.Label
				path = acc.Path
				found = true
				break
			}
		}
		if !found {
			data := buildSettingsData(d, r, lang, theme)
			data.CCAccountError = T(lang, "settings.ccAccounts.notFound")
			renderSettings(w, r, d, data)
			return
		}

		if err := d.UpdateCCAccount(id, label, path, enabled); err != nil {
			data := buildSettingsData(d, r, lang, theme)
			data.CCAccountError = err.Error()
			renderSettings(w, r, d, data)
			return
		}
		redirectSettings(w, r)
	}
}
