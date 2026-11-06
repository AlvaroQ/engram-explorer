package ui_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestOverviewHero_TitleCTAAndSearch (S3, S5): the hero carries the page
// title, the CTA to the Knowledge Graph and the memory search, a plain GET
// form on /observations?q= (no new endpoint, no script).
func TestOverviewHero_TitleCTAAndSearch(t *testing.T) {
	for _, tc := range []struct{ lang, title, cta, search string }{
		{"en", "Overview", "Open Knowledge Graph", "Search memories"},
		{"es", "Resumen", "Abrir el grafo de conocimiento", "Buscar memorias"},
	} {
		body := ovGet(t, ui.Deps{RoDB: openProjectsDB(t)}, "/", tc.lang)
		hero := regexp.MustCompile(`(?s)<section class="ov-hero" id="ov-hero"[^>]*>.*?</section>`).FindString(body)
		if hero == "" {
			t.Fatalf("%s: no hero section", tc.lang)
		}
		for _, want := range []string{
			`<h1 id="ov-title">` + tc.title + `</h1>`,
			`<a class="btn btn-tonal" href="/brain">` + tc.cta + `</a>`,
			`<form class="ov-search glass" id="ov-search" role="search" action="/observations" method="get">`,
			`<label class="sr-only" for="ov-q">` + tc.search + `</label>`,
			`name="q"`,
		} {
			if !strings.Contains(hero, want) {
				t.Errorf("%s: hero lacks %s", tc.lang, want)
			}
		}
	}
}
