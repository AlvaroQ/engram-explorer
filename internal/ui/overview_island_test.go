package ui_test

import (
	"regexp"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestOverviewHero_GraphIsland (S3): the hero background is the
// brain-preview-hero island in the back layer (it labels itself as role="img").
func TestOverviewHero_GraphIsland(t *testing.T) {
	body := ovGet(t, ui.Deps{RoDB: openProjectsDB(t)}, "/", "en")
	back := regexp.MustCompile(`(?s)<div class="ov-hero-back">.*?<div class="ov-hero-ui"`).FindString(body)
	if !regexp.MustCompile(`<div class="ov-hero-canvas"><div data-island="brain-preview-hero"`).MatchString(back) {
		t.Errorf("hero back layer lacks the brain-preview-hero island: %s", back)
	}
}
