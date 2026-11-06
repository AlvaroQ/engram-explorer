package ui_test

// Overview KPIs (Phase 4, S3/S4, owner decision L2): four cards per the
// 02-brain-first artboard — Memories saved, Projects, AI conversations and
// Agent spend this month — each in two DOM versions: the full card and the
// 44px strip item. The strip item is a link with the card's target (S4) and
// starts hidden (inert + aria-hidden); exactly one version per KPI is hidden.
// The agent cards load lazily (the transcript scan never blocks the page):
// the page carries a loader whose response fills both versions out of band.

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

func ovSeed(t *testing.T, db *sql.DB) {
	t.Helper()
	now := time.Now().UTC()
	for _, r := range []struct {
		project string
		days    int
	}{{"alpha", 1}, {"alpha", 2}, {"beta", 3}, {"beta", 20}, {"beta", 21}, {"beta", 22}} {
		created := now.AddDate(0, 0, -r.days).Format("2006-01-02 15:04:05")
		if _, err := db.Exec(
			`INSERT INTO observations (type, title, project, created_at, scope) VALUES ('decision', 'x', ?, ?, 'project')`,
			r.project, created,
		); err != nil {
			t.Fatal(err)
		}
	}
}

var (
	ovCardRe = regexp.MustCompile(`(?s)<div class="ov-kpi glass" id="kpi-([a-z]+)">(.*?)<!-- /kpi -->`)
	ovMiniRe = regexp.MustCompile(`<a class="ov-kpi-mini"[^>]*>`)
	ovHref   = regexp.MustCompile(`href="([^"]*)"`)
)

func TestOverviewKPIs_CardsAndStrip(t *testing.T) {
	db := openProjectsDB(t)
	ovSeed(t, db)
	body := ovGet(t, ui.Deps{RoDB: db}, "/", "en")

	cards := ovCardRe.FindAllStringSubmatch(body, -1)
	if len(cards) != 4 {
		t.Fatalf("KPI cards = %d, want 4", len(cards))
	}
	want := []struct {
		id, label, value, href string
		caption                []string
	}{
		{"memories", "Memories saved", "6", "/observations", []string{"+3 in the last 7 days", ">Browse<"}},
		{"projects", "Projects", "2", "/projects/beta", []string{"Most active: <span class=\"mono\">beta</span>", ">Drill in<"}},
		// No live agent here: both agent cards point to Settings → Data sources.
		{"conversations", "AI conversations", "", "/settings#settings-modules", nil},
		{"spend", "Agent spend this month", "", "/settings#settings-modules", nil},
	}
	for i, w := range want {
		id, card := cards[i][1], cards[i][2]
		if id != w.id {
			t.Errorf("card %d id = %q, want %q", i, id, w.id)
			continue
		}
		if !strings.Contains(card, `<p class="kpi-label">`+w.label+`</p>`) {
			t.Errorf("%s: label %q missing", id, w.label)
		}
		if w.value != "" && !strings.Contains(card, `<p class="kpi-value">`+w.value+`</p>`) {
			t.Errorf("%s: value %q missing", id, w.value)
		}
		for _, c := range w.caption {
			if !strings.Contains(card, c) {
				t.Errorf("%s: caption lacks %q", id, c)
			}
		}
		// Two versions: the full card (exposed) and the strip link (hidden).
		mini := ovMiniRe.FindString(card)
		if mini == "" {
			t.Errorf("%s: no strip link", id)
			continue
		}
		if m := ovHref.FindStringSubmatch(mini); m == nil || m[1] != w.href {
			t.Errorf("%s: strip link %s, want href %q", id, mini, w.href)
		}
		if !strings.Contains(mini, `aria-hidden="true"`) || !strings.Contains(mini, " inert") {
			t.Errorf("%s: strip link must start inert + aria-hidden: %s", id, mini)
		}
		if strings.Count(card, " inert") != 1 || strings.Count(card, `aria-hidden="true" inert`) != 1 {
			t.Errorf("%s: exactly one version must be hidden", id)
		}
		if !strings.Contains(card, `href="`+w.href+`"`) {
			t.Errorf("%s: the full card has no link to %s", id, w.href)
		}
	}
	// The old /sessions target is gone (L2).
	if strings.Contains(body, `href="/sessions"`) {
		t.Error("overview still links /sessions")
	}
}

func TestOverviewKPIs_AgentsNotLive(t *testing.T) {
	body := ovGet(t, ui.Deps{RoDB: openProjectsDB(t)}, "/", "en")
	if strings.Contains(body, "/partials/overview-agents") {
		t.Error("no live agent: the page must not load the agent KPIs")
	}
	for _, id := range []string{"conversations", "spend"} {
		card := regexp.MustCompile(`(?s)id="kpi-` + id + `">(.*?)<!-- /kpi -->`).FindStringSubmatch(body)
		if card == nil || !strings.Contains(card[1], "No coding agent connected yet.") ||
			!strings.Contains(card[1], `href="/settings#settings-modules"`) {
			t.Errorf("%s: want the empty state linking Settings → Data sources", id)
		}
	}
}

func TestOverviewKPIs_AgentsLazyFill(t *testing.T) {
	sources := []agentsessions.Source{
		ovSource{agent: "claude-code", sessions: []agentsessions.Session{ovNow(2, 2.5, false), ovNow(30, 1.25, false)}},
		ovSource{agent: "codex", sessions: []agentsessions.Session{ovNow(5, 0.75, true)}},
	}
	d := ui.Deps{RoDB: openProjectsDB(t), AgentSessions: sources}
	page := ovGet(t, d, "/", "en")
	loader := regexp.MustCompile(`<div[^>]*hx-get="/partials/overview-agents"[^>]*>`).FindString(page)
	for _, want := range []string{`hx-trigger="load"`, `hx-swap="none"`, "hidden"} {
		if !strings.Contains(loader, want) {
			t.Errorf("loader %q lacks %s", loader, want)
		}
	}
	for id, href := range map[string]string{"conversations": "/agent-sessions/list", "spend": "/agent-sessions"} {
		card := regexp.MustCompile(`(?s)id="kpi-` + id + `">(.*?)<!-- /kpi -->`).FindStringSubmatch(page)
		if card == nil || !strings.Contains(ovMiniRe.FindString(card[1]), `href="`+href+`"`) {
			t.Errorf("%s: with live agents the strip link must target %s", id, href)
		}
	}
	for _, id := range []string{"kpi-conversations-body", "kpi-conversations-mini-v", "kpi-spend-body", "kpi-spend-mini-v"} {
		if !strings.Contains(page, `id="`+id+`"`) {
			t.Errorf("page lacks the fill target #%s", id)
		}
	}

}

func TestOverviewKPIs_Localized(t *testing.T) {
	body := ovGet(t, ui.Deps{RoDB: openProjectsDB(t)}, "/", "es")
	for _, want := range []string{"Memorias guardadas", "Proyectos", "Conversaciones con IA", "Gasto en agentes este mes",
		"Aún no hay ningún agente conectado.", "Cifras clave"} {
		if !strings.Contains(body, want) {
			t.Errorf("es overview lacks %q", want)
		}
	}
}
