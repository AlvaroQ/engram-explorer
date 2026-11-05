package ui_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

type ovSource struct {
	agent    string
	sessions []agentsessions.Session
}

func (s ovSource) ID() string                                  { return s.agent }
func (s ovSource) Agent() string                               { return s.agent }
func (s ovSource) Fingerprint(context.Context) (string, error) { return "fp", nil }
func (s ovSource) List(context.Context) ([]agentsessions.Session, error) {
	return s.sessions, nil
}
func (s ovSource) Detail(context.Context, string) (*agentsessions.Detail, error) {
	return nil, agentsessions.ErrNotFound
}

func ovNow(dayOffset int, cost float64, unknown bool) agentsessions.Session {
	return agentsessions.Session{
		StartedAt: time.Now().UTC().Add(-time.Duration(dayOffset) * time.Hour),
		Usage:     agentsessions.Usage{CostUSD: cost, UnknownModel: unknown},
	}
}

func ovGet(t *testing.T, d ui.Deps, target, lang string) string {
	t.Helper()
	mux := http.NewServeMux()
	ui.Mount(mux, d)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", target, w.Code)
	}
	return w.Body.String()
}

// TestOverviewAgentsPartial: /partials/overview-agents fills both versions
// of the two agent cards out of band (innerHTML targets, so the strip keeps
// its inert state), with the per-agent split as SVG rects and a "~" lower
// bound when a model is unpriced.
func TestOverviewAgentsPartial(t *testing.T) {
	sources := []agentsessions.Source{
		ovSource{agent: "claude-code", sessions: []agentsessions.Session{ovNow(2, 2.5, false), ovNow(30, 1.25, false)}},
		ovSource{agent: "codex", sessions: []agentsessions.Session{ovNow(5, 0.75, true)}},
	}
	d := ui.Deps{RoDB: openProjectsDB(t), AgentSessions: sources}
	frag := ovGet(t, d, "/partials/overview-agents", "en")
	// Spend this month depends on the calendar; conversations are in the 7-day window.
	for _, want := range []string{
		`<div id="kpi-conversations-body" hx-swap-oob="outerHTML">`,
		`<p class="kpi-value">3</p>`,
		`aria-label="Claude Code 2, Codex 1"`,
		`class="kv ov-agent-claude-code">Claude Code 2</span>`,
		`class="kv ov-agent-codex">Codex 1</span>`,
		`<span id="kpi-conversations-mini-v" hx-swap-oob="innerHTML">3</span>`,
		`<div id="kpi-spend-body" hx-swap-oob="outerHTML">`,
		`<span id="kpi-spend-mini-v" hx-swap-oob="innerHTML">`,
	} {
		if !strings.Contains(frag, want) {
			t.Errorf("agent fragment lacks %s\n%s", want, frag)
		}
	}
	if !strings.Contains(frag, "~") {
		t.Error("an unpriced Codex model must mark the spend as a lower bound (~)")
	}
	if strings.Contains(frag, "style=") || strings.Contains(frag, "<script") {
		t.Error("agent fragment carries inline style or script")
	}
	// The bodies are replaced whole (outerHTML), so the loading state ends.
	if strings.Contains(frag, "aria-busy") {
		t.Error("agent fragment must replace the busy bodies, not keep aria-busy")
	}
	if strings.Contains(frag, "Some sessions could not be read") {
		t.Error("no source failed: the partial-read note must not show")
	}
}

type ovFailSource struct{ agent string }

func (s ovFailSource) ID() string                                  { return s.agent }
func (s ovFailSource) Agent() string                               { return s.agent }
func (s ovFailSource) Fingerprint(context.Context) (string, error) { return "fp", nil }
func (s ovFailSource) List(context.Context) ([]agentsessions.Session, error) {
	return nil, errors.New("unreadable transcript folder")
}
func (s ovFailSource) Detail(context.Context, string) (*agentsessions.Detail, error) {
	return nil, agentsessions.ErrNotFound
}

// TestOverviewAgentsPartial_SourceErrorNote: when a source cannot be read the
// totals still show (from the readable sources) and both cards carry a
// localized note, so an undercount is never silent.
func TestOverviewAgentsPartial_SourceErrorNote(t *testing.T) {
	sources := []agentsessions.Source{
		ovSource{agent: "claude-code", sessions: []agentsessions.Session{ovNow(2, 1, false)}},
		ovFailSource{agent: "codex"},
	}
	d := ui.Deps{RoDB: openProjectsDB(t), AgentSessions: sources}
	for lang, note := range map[string]string{
		"en": "Some sessions could not be read",
		"es": "No se pudieron leer algunas sesiones",
	} {
		frag := ovGet(t, d, "/partials/overview-agents", lang)
		if got := strings.Count(frag, note); got != 2 {
			t.Errorf("%s: partial-read note %q appears %d times, want 2 (both cards)\n%s", lang, note, got, frag)
		}
		if !strings.Contains(frag, `<p class="kpi-value">1</p>`) {
			t.Errorf("%s: the readable source must still count", lang)
		}
		if strings.Contains(frag, "unreadable transcript folder") {
			t.Errorf("%s: the source error detail must not reach the page", lang)
		}
	}
}

// TestOverviewAgentsPartial_CollectFailed: when the collect itself fails (a
// cancelled request) the response still replaces both versions of the two
// agent cards with a terminal, localized state: no "…", no aria-busy, and a
// 200 so htmx applies the out-of-band swaps of the hx-swap="none" request.
func TestOverviewAgentsPartial_CollectFailed(t *testing.T) {
	sources := []agentsessions.Source{ovSource{agent: "claude-code"}}
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: openProjectsDB(t), AgentSessions: sources})
	for lang, msg := range map[string]string{
		"en": "Could not load the agent figures.",
		"es": "No se pudieron cargar las cifras de los agentes.",
	} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodGet, "/partials/overview-agents", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200 (out-of-band swaps need a 2xx)", lang, w.Code)
		}
		frag := w.Body.String()
		for _, want := range []string{
			`<div id="kpi-conversations-body" hx-swap-oob="outerHTML">`,
			`<div id="kpi-spend-body" hx-swap-oob="outerHTML">`,
			`<span id="kpi-conversations-mini-v" hx-swap-oob="innerHTML">—</span>`,
			`<span id="kpi-spend-mini-v" hx-swap-oob="innerHTML">—</span>`,
		} {
			if !strings.Contains(frag, want) {
				t.Errorf("%s: error fragment lacks %s\n%s", lang, want, frag)
			}
		}
		if got := strings.Count(frag, msg); got != 2 {
			t.Errorf("%s: error message %q appears %d times, want 2\n%s", lang, msg, got, frag)
		}
		if strings.Contains(frag, "aria-busy") || strings.Contains(frag, "…") {
			t.Errorf("%s: error fragment must end the loading state\n%s", lang, frag)
		}
	}
}
