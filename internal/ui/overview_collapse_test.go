package ui_test

// Collapsing hero (Phase 4, S3/S11): the overview carries the marker the
// overview-collapse island mounts on (no inline script), and styles.css holds
// the artboard-04 geometry the runtime's custom properties drive. The scroll
// math itself is unit-tested in apps/frontend/src/lib/overview-collapse.test.ts.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

func TestOverviewCollapse_Marker(t *testing.T) {
	body := ovGet(t, ui.Deps{RoDB: openProjectsDB(t)}, "/", "en")
	overview := regexp.MustCompile(`(?s)<div class="overview" id="overview">.*`).FindString(body)
	if !strings.Contains(overview, `<div hidden data-island="overview-collapse" data-props="{}"></div>`) {
		t.Error("overview lacks the hidden overview-collapse island marker")
	}
	if n := strings.Count(overview, "data-collapse-fade"); n != 3 {
		t.Errorf("hero controls marked to fade = %d, want 3 (eyebrow, subtitle, CTA)", n)
	}
}

func TestOverviewCollapse_StylesGeometry(t *testing.T) {
	raw, err := ui.StaticFS.ReadFile("static/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	css := strings.Join(strings.Fields(string(raw)), " ")
	for _, want := range []string{
		"--ov-hero-h: 500px;", "--ov-bar-h: 72px;", "--ov-strip-h: 44px;",
		// sticky hero at top: calc(72px - 500px)
		".overview[data-collapse-ready] .ov-hero { position: sticky; top: calc(var(--ov-bar-h) - var(--ov-hero-h));",
		// canvas scale 0.55 and opacity 0.35 at --ov-p = 1, behind the solid bar
		"scale(calc(1 - 0.45 * var(--ov-p)))", "opacity: calc(1 - 0.65 * var(--ov-p));",
		".overview[data-collapse-ready] .ov-hero-solid { opacity: var(--ov-p);",
		// title 20px -> 16px, KPI strip pinned under the bar
		"scale(calc(1 - 0.2 * var(--ov-p)))",
		".overview[data-collapse-ready] .ov-kpis { position: sticky; top: var(--ov-bar-h);",
		// focus is never hidden under the bar + strip
		"scroll-padding-top: calc(72px + 44px + 16px);",
		// reduced motion: the hero scrolls away, the strip pins at the top
		"@media (prefers-reduced-motion: reduce) { .overview[data-collapse-ready] .ov-hero { position: relative; top: auto;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("styles.css lacks %q", want)
		}
	}
}
