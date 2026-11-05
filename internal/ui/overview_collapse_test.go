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
