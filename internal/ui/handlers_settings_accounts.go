package ui

import (
	"net/http"
	"strings"
)

// accountsData carries all data needed to render the Settings → Accounts page.
type accountsData struct {
	Profiles      []ProfileInfo
	ActiveProfile string
	AccountsError string
	Lang          string
	Theme         string
}

// handleAccountsPage serves GET /settings/accounts.
// Renders the accounts management page (profile list, create form, switch).
// Full page on direct GET; partial content when HX-Request: true.
func handleAccountsPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		data := buildAccountsData(d, lang, theme)

		if IsHTMX(r) {
			renderDeps(w, r, d, AccountsPartial(data))
		} else {
			renderDeps(w, r, d, AccountsPage(data))
		}
	}
}

// handleProfileSwitchPost serves POST /settings/profile/switch.
// Switches the active profile via the SwitchProfile callback.
// On success: redirects to /settings/accounts.
// On error: re-renders the accounts page with an inline error.
func handleProfileSwitchPost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.SwitchProfile == nil {
			http.Error(w, "profile switch not available", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		lang := langForRequest(r)
		theme := themeForRequest(r)

		if name == "" {
			data := buildAccountsData(d, lang, theme)
			data.AccountsError = T(lang, "settings.accounts.nameEmpty")
			renderAccounts(w, r, d, data)
			return
		}

		if err := d.SwitchProfile(name); err != nil {
			data := buildAccountsData(d, lang, theme)
			data.AccountsError = err.Error()
			renderAccounts(w, r, d, data)
			return
		}
		redirectAccounts(w, r)
	}
}

// handleProfileCreatePost serves POST /settings/profile/create.
// Creates a new named profile via the CreateProfile callback.
// On success: redirects to /settings/accounts.
// On error: re-renders the accounts page with an inline error.
func handleProfileCreatePost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.CreateProfile == nil {
			http.Error(w, "profile creation not available", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		lang := langForRequest(r)
		theme := themeForRequest(r)

		if name == "" {
			data := buildAccountsData(d, lang, theme)
			data.AccountsError = T(lang, "settings.accounts.nameEmpty")
			renderAccounts(w, r, d, data)
			return
		}

		if err := d.CreateProfile(name); err != nil {
			data := buildAccountsData(d, lang, theme)
			data.AccountsError = err.Error()
			renderAccounts(w, r, d, data)
			return
		}
		redirectAccounts(w, r)
	}
}

// handleProfileDeletePost serves POST /settings/profile/delete.
// Deletes the named profile via the DeleteProfile callback.
// On success: redirects to /settings/accounts.
// On error: re-renders the accounts page with an inline error.
func handleProfileDeletePost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.DeleteProfile == nil {
			http.Error(w, "profile deletion not available", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		lang := langForRequest(r)
		theme := themeForRequest(r)

		if name == "" {
			data := buildAccountsData(d, lang, theme)
			data.AccountsError = T(lang, "settings.accounts.nameEmpty")
			renderAccounts(w, r, d, data)
			return
		}

		if err := d.DeleteProfile(name); err != nil {
			data := buildAccountsData(d, lang, theme)
			data.AccountsError = err.Error()
			renderAccounts(w, r, d, data)
			return
		}
		redirectAccounts(w, r)
	}
}

// buildAccountsData collects the data needed for the accounts page.
func buildAccountsData(d Deps, lang, theme string) accountsData {
	var profiles []ProfileInfo
	if d.Profiles != nil {
		profiles = d.Profiles()
	}
	active := ""
	if d.ActiveProfile != nil {
		active = d.ActiveProfile()
	}
	return accountsData{
		Profiles:      profiles,
		ActiveProfile: active,
		Lang:          lang,
		Theme:         theme,
	}
}

// renderAccounts renders the accounts page (partial for HTMX, full otherwise).
func renderAccounts(w http.ResponseWriter, r *http.Request, d Deps, data accountsData) {
	if IsHTMX(r) {
		renderDeps(w, r, d, AccountsPartial(data))
	} else {
		renderDeps(w, r, d, AccountsPage(data))
	}
}

// redirectAccounts reloads /settings/accounts after a successful change
// (HX-Redirect for HTMX, 303 otherwise).
func redirectAccounts(w http.ResponseWriter, r *http.Request) {
	if IsHTMX(r) {
		w.Header().Set("HX-Redirect", "/settings/accounts")
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, "/settings/accounts", http.StatusSeeOther)
	}
}
