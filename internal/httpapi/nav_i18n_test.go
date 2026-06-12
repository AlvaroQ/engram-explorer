package httpapi_test

// Regression test: every sidebar LabelKey exposed by the real providers must
// resolve to a translated string in every locale. A key that does not resolve
// is rendered verbatim in the UI (e.g. "nav.ccsessions.list" appearing in the
// sidebar instead of "Claude Code Sessions").

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	"github.com/AlvaroQ/engram-explorer/internal/providers/engram"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

func TestProviderNavLabelKeys_ResolveInAllLocales(t *testing.T) {
	cfg := config.Config{}
	provs := []providers.Provider{
		engram.NewProvider(cfg),
		ccsessions.NewProvider(cfg),
	}
	locales := []string{"en", "es"}

	for _, p := range provs {
		group := p.Nav(nil)
		keys := []string{group.LabelKey}
		for _, link := range group.Links {
			keys = append(keys, link.LabelKey)
		}
		for _, lang := range locales {
			for _, key := range keys {
				if got := ui.T(lang, key); got == key || got == "" {
					t.Errorf("provider %q: LabelKey %q does not resolve in locale %q (got %q)", p.ID(), key, lang, got)
				}
			}
		}
	}
}
