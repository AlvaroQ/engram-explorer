package httpapi_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

var sidebarRe = regexp.MustCompile(`(?s)<aside[^>]*id="sidebar".*?</aside>`)

// TestDoctorPages_RenderTheSameSidebarAsUIPages covers S16 (Phase 2 follow-up
// C6): the provider-owned doctor pages (Settings → Maintenance and the sync
// project detail) render the sidebar from the registry NavGroups and the
// profiles, exactly like the host UI pages — not the legacy hardcoded layout.
// With Claude Code disabled and two profiles, that means no Claude Code link
// and an account switcher.
func TestDoctorPages_RenderTheSameSidebarAsUIPages(t *testing.T) {
	f := newLifecycleFixtureWith(t, true, false, func(dbPath, _ string) map[string]config.Profile {
		return map[string]config.Profile{"work": {Providers: map[string]config.ProviderConfig{
			"engram": {Enabled: true, Path: dbPath},
		}}}
	})

	ref := f.fetch(t, "/settings")
	if ref.Code != 200 {
		t.Fatalf("GET /settings: status %d", ref.Code)
	}
	want := sidebarRe.FindString(ref.Body.String())
	if want == "" {
		t.Fatal("GET /settings: no sidebar")
	}
	if !strings.Contains(want, `class="account-switcher"`) || strings.Contains(want, `href="/cc-overview"`) {
		t.Fatalf("reference sidebar is not the registry-driven one:\n%s", want)
	}

	for _, route := range []string{"/settings/maintenance", "/doctor/sync/myproject"} {
		w := f.fetch(t, route)
		if w.Code != 200 {
			t.Errorf("GET %s: status %d, want 200 (body %.200s)", route, w.Code, w.Body.String())
			continue
		}
		if got := sidebarRe.FindString(w.Body.String()); got != want {
			t.Errorf("GET %s: sidebar differs from the UI pages'\n got: %.600s\nwant: %.600s", route, got, want)
		}
	}
}
