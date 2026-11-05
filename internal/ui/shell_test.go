package ui_test

// App shell (Phase 4, S2 + S6): 64px icon rail expandable to 236px with the
// state read server-side from the "sidebar" cookie (collapsed by default),
// a skip link as the first focusable element, aria-current on the active
// link, and an "Agents" group with one entry per live agent.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

var (
	shellAsideRe     = regexp.MustCompile(`(?s)<aside.*?</aside>`)
	shellFocusableRe = regexp.MustCompile(`<(a|button|input|select|textarea)\b[^>]*>`)
	shellToggleRe    = regexp.MustCompile(`<button[^>]*id="sidebar-toggle"[^>]*>`)
)

// shellSource is a minimal agent-session source: only Agent matters here.
type shellSource struct{ id, agent string }

func (s shellSource) ID() string    { return s.id }
func (s shellSource) Agent() string { return s.agent }
func (s shellSource) Fingerprint(context.Context) (string, error) {
	return "fp", nil
}
func (s shellSource) List(context.Context) ([]agentsessions.Session, error) { return nil, nil }
func (s shellSource) Detail(context.Context, string) (*agentsessions.Detail, error) {
	return nil, agentsessions.ErrNotFound
}

func shellGet(t *testing.T, d ui.Deps, target string, cookies ...*http.Cookie) string {
	t.Helper()
	mux := http.NewServeMux()
	ui.Mount(mux, d)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", target, w.Code)
	}
	return w.Body.String()
}

func TestShell_RailStateFromCookie(t *testing.T) {
	cases := []struct {
		name       string
		cookie     *http.Cookie
		collapsed  bool
		toggleName string
	}{
		{"no cookie defaults to the 64px rail", nil, true, "Expand sidebar"},
		{"collapsed cookie", &http.Cookie{Name: "sidebar", Value: "collapsed"}, true, "Expand sidebar"},
		{"expanded cookie", &http.Cookie{Name: "sidebar", Value: "expanded"}, false, "Collapse sidebar"},
		{"invalid cookie falls back to collapsed", &http.Cookie{Name: "sidebar", Value: "wide"}, true, "Expand sidebar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cookies []*http.Cookie
			if tc.cookie != nil {
				cookies = append(cookies, tc.cookie)
			}
			body := shellGet(t, ui.Deps{RoDB: openTestDB(t)}, "/brain", cookies...)
			if got := strings.Contains(body, `class="app-shell sidebar-collapsed"`); got != tc.collapsed {
				t.Errorf("app-shell collapsed = %v, want %v", got, tc.collapsed)
			}
			toggle := shellToggleRe.FindString(body)
			wantExpanded := `aria-expanded="true"`
			if tc.collapsed {
				wantExpanded = `aria-expanded="false"`
			}
			for _, want := range []string{wantExpanded, `aria-controls="sidebar"`, `aria-label="` + tc.toggleName + `"`,
				`data-label-expand="Expand sidebar"`, `data-label-collapse="Collapse sidebar"`} {
				if !strings.Contains(toggle, want) {
					t.Errorf("toggle %s lacks %s", toggle, want)
				}
			}
		})
	}
}

func TestShell_SkipLinkIsFirstFocusable(t *testing.T) {
	for _, tc := range []struct{ lang, text string }{{"en", "Skip to content"}, {"es", "Saltar al contenido"}} {
		for name, layout := range map[string]func(string, string, string, string, string, templ.Component) templ.Component{
			"Layout": ui.Layout, "LayoutFullBleed": ui.LayoutFullBleed,
		} {
			var b strings.Builder
			if err := layout("t", "overview", tc.lang, "dark", "collapsed", templ.NopComponent).Render(t.Context(), &b); err != nil {
				t.Fatal(err)
			}
			html := b.String()
			bodyAt := strings.Index(html, "<body>")
			first := shellFocusableRe.FindString(html[bodyAt:])
			if !strings.Contains(first, `class="skip-link"`) || !strings.Contains(first, `href="#main-content"`) {
				t.Errorf("%s/%s: first focusable element is %s, want the skip link to #main-content", name, tc.lang, first)
			}
			if !strings.Contains(html, `>`+tc.text+`</a>`) {
				t.Errorf("%s/%s: skip link text %q missing", name, tc.lang, tc.text)
			}
			if !regexp.MustCompile(`<main[^>]*id="main-content"[^>]*tabindex="-1"`).MatchString(html) {
				t.Errorf("%s/%s: <main> needs id=main-content and tabindex=-1", name, tc.lang)
			}
		}
	}
}

func TestShell_AriaCurrentOnActiveLink(t *testing.T) {
	var b strings.Builder
	if err := ui.Layout("t", "brain", "en", "dark", "expanded", templ.NopComponent).Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	aside := shellAsideRe.FindString(b.String())
	if n := strings.Count(aside, `aria-current="page"`); n != 1 {
		t.Fatalf("aria-current count = %d, want 1:\n%s", n, aside)
	}
	if !regexp.MustCompile(`<a href="/brain" class="active" aria-current="page"`).MatchString(aside) {
		t.Errorf("active /brain link lacks class=active + aria-current=page:\n%s", aside)
	}

	// The settings gear is the active entry on /settings.
	body := shellGet(t, ui.Deps{RoDB: openTestDB(t)}, "/settings")
	if !regexp.MustCompile(`<a href="/settings" class="sidebar-brand-settings active" aria-current="page"`).MatchString(body) {
		t.Error("settings gear lacks aria-current=page on /settings")
	}
}

func TestShell_AgentsGroupPerLiveAgent(t *testing.T) {
	both := []agentsessions.Source{shellSource{"claude-code", "claude-code"}, shellSource{"codex", "codex"}}
	aside := shellAsideRe.FindString(shellGet(t, ui.Deps{RoDB: openTestDB(t), AgentSessions: both}, "/settings"))
	for _, want := range []string{
		`sidebar-group-agent-sessions`, `>Agents<`,
		`href="/agent-sessions?agent=claude-code"`, `>Claude Code<`, `nav-icon-claude-code`,
		`href="/agent-sessions?agent=codex"`, `>Codex<`, `nav-icon-codex`,
	} {
		if !strings.Contains(aside, want) {
			t.Errorf("sidebar lacks %s", want)
		}
	}
	for _, gone := range []string{`href="/agent-sessions"`, `href="/agent-sessions/list"`} {
		if strings.Contains(aside, gone) {
			t.Errorf("sidebar still links the neutral page %s", gone)
		}
	}

	// Only the agents with a live source get an entry.
	onlyCodex := []agentsessions.Source{shellSource{"codex", "codex"}}
	aside = shellAsideRe.FindString(shellGet(t, ui.Deps{RoDB: openTestDB(t), AgentSessions: onlyCodex}, "/settings"))
	if strings.Contains(aside, "agent=claude-code") || !strings.Contains(aside, "agent=codex") {
		t.Errorf("with only Codex live the sidebar must list Codex alone:\n%s", aside)
	}
}

func TestShell_AgentLinkActiveByQuery(t *testing.T) {
	both := []agentsessions.Source{shellSource{"claude-code", "claude-code"}, shellSource{"codex", "codex"}}
	d := ui.Deps{RoDB: openTestDB(t), AgentSessions: both}
	current := regexp.MustCompile(`<a href="([^"]+)" class="active" aria-current="page"`)
	for _, tc := range []struct{ target, want string }{
		{"/agent-sessions?agent=codex", "/agent-sessions?agent=codex"},
		{"/agent-sessions/list?agent=claude-code", "/agent-sessions?agent=claude-code"},
		{"/agent-sessions", ""},       // neutral view: no agent is current
		{"/settings?agent=codex", ""}, // ?agent only counts on agent pages
	} {
		aside := shellAsideRe.FindString(shellGet(t, d, tc.target))
		got := ""
		if m := current.FindStringSubmatch(aside); m != nil {
			got = strings.ReplaceAll(m[1], "&amp;", "&")
		}
		if got != tc.want {
			t.Errorf("GET %s: current sidebar link = %q, want %q", tc.target, got, tc.want)
		}
	}
}

// TestShell_SettingsPagesHonorRailCookie: the settings pages render the shell
// state from the "sidebar" cookie like every other page.
func TestShell_SettingsPagesHonorRailCookie(t *testing.T) {
	expanded := &http.Cookie{Name: "sidebar", Value: "expanded"}
	for _, target := range []string{"/settings", "/settings/accounts"} {
		body := shellGet(t, ui.Deps{}, target, expanded)
		if strings.Contains(body, `class="app-shell sidebar-collapsed"`) {
			t.Errorf("GET %s with sidebar=expanded renders the collapsed rail", target)
		}
	}
}
