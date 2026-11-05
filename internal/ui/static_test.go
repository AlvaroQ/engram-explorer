package ui_test

// Single source of design tokens (Phase 4, S1): static/tokens.css is the only
// file allowed to define the color tokens, the shared layout links it before
// every other stylesheet, and the doctor stylesheet consumes the shared tokens
// instead of its own hex palette.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// tokenConsumers are the stylesheets that must read, never redefine, tokens.
var tokenConsumers = []string{
	"static/styles.css",
	"../../apps/frontend/src/styles.css",
	"../doctor/static/styles.css",
}

func TestTokensDefinedOnlyInTokensCSS(t *testing.T) {
	themes := loadThemes(t)
	names := map[string]bool{}
	for name, value := range themes["dark"] {
		if hslTriplet.MatchString(value) {
			names[name] = true // color tokens only
		}
	}
	for _, path := range tokenConsumers {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src := cssComment.ReplaceAllString(string(raw), "")
		for _, d := range cssDecl.FindAllStringSubmatch(src, -1) {
			if names[d[1]] {
				t.Errorf("%s redefines --%s; color tokens live only in internal/ui/static/tokens.css", path, d[1])
			}
		}
	}
}

func TestDoctorStylesUseSharedTokens(t *testing.T) {
	raw, err := os.ReadFile("../doctor/static/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	src := cssComment.ReplaceAllString(string(raw), "")
	if m := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`).FindAllString(src, -1); len(m) > 0 {
		t.Errorf("doctor styles.css hard-codes hex colors %v; use hsl(var(--token))", m)
	}
	if strings.Contains(src, "--pico-") {
		t.Error("doctor styles.css still references --pico-* aliases")
	}
	if strings.Contains(src, "--doc-green") || strings.Contains(src, "--doc-amber") ||
		strings.Contains(src, "--doc-red") || strings.Contains(src, "--doc-blue") || strings.Contains(src, "--doc-gray") {
		t.Error("doctor styles.css still defines its own --doc-* status palette")
	}
}

func TestLayoutLinksTokensFirst(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	css := httptest.NewRecorder()
	mux.ServeHTTP(css, httptest.NewRequest(http.MethodGet, "/static/tokens.css", nil))
	if css.Code != http.StatusOK {
		t.Fatalf("GET /static/tokens.css = %d, want 200", css.Code)
	}
	if !strings.Contains(css.Body.String(), "--accent-fg:") {
		t.Fatal("/static/tokens.css does not serve the token definitions")
	}

	var b strings.Builder
	if err := ui.Layout("t", "overview", "en", "dark", "expanded", templ.NopComponent).Render(t.Context(), &b); err != nil {
		t.Fatalf("render layout: %v", err)
	}
	html := b.String()
	tok := strings.Index(html, `href="/static/tokens.css"`)
	app := strings.Index(html, `href="/static/app.css"`)
	sty := strings.Index(html, `href="/static/styles.css"`)
	if tok < 0 || app < 0 || sty < 0 {
		t.Fatalf("layout must link tokens.css, app.css and styles.css (got %d/%d/%d)", tok, app, sty)
	}
	if tok > app || tok > sty {
		t.Fatal("tokens.css must be linked before app.css and styles.css")
	}
}
