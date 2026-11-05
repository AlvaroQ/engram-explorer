package ui_test

// Account switcher (Phase 4, S6): a menu button operable from the keyboard.
// The trigger is a <button> with aria-haspopup/aria-expanded/aria-controls;
// the menu starts hidden with role="menu" and roving-tabindex menu items;
// ui-actions.js (data-menu-button) opens it on click, Enter, Space and the
// arrow keys, and Esc closes it returning focus. The profile-switch form posts
// are unchanged.

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

func TestAccountSwitcher_MenuButton(t *testing.T) {
	profiles := func() []ui.ProfileInfo {
		return []ui.ProfileInfo{{Name: "Default", Active: true}, {Name: "work"}}
	}
	for _, tc := range []struct{ lang, menu string }{
		{"en", "Switch account"},
		{"es", "Cambiar de cuenta"},
	} {
		page := shellGet(t, ui.Deps{RoDB: openTestDB(t), Profiles: profiles}, "/brain",
			&http.Cookie{Name: "lang", Value: tc.lang})
		trigger := regexp.MustCompile(`<button[^>]*id="account-switcher-trigger"[^>]*>`).FindString(page)
		for _, want := range []string{`type="button"`, `aria-haspopup="menu"`, `aria-expanded="false"`,
			`aria-controls="account-switcher-menu"`, `data-menu-button`, `aria-label="` + tc.menu + `: Default"`} {
			if !strings.Contains(trigger, want) {
				t.Errorf("%s: trigger %q lacks %s", tc.lang, trigger, want)
			}
		}
		menu := regexp.MustCompile(`<div[^>]*id="account-switcher-menu"[^>]*>`).FindString(page)
		for _, want := range []string{`role="menu"`, `aria-labelledby="account-switcher-trigger"`, ` hidden`} {
			if !strings.Contains(menu, want) {
				t.Errorf("%s: menu %q lacks %s", tc.lang, menu, want)
			}
		}
		if n := strings.Count(page, `role="menuitem" tabindex="-1"`); n != 2 {
			t.Errorf("%s: menu items = %d, want 2 (switch to work + manage)", tc.lang, n)
		}
		if !strings.Contains(page, `hx-post="/settings/profile/switch"`) || !strings.Contains(page, `name="name" value="work"`) {
			t.Errorf("%s: profile switch form missing", tc.lang)
		}
	}
}
