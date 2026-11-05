package codex_test

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
)

func TestLookupPrice(t *testing.T) {
	gpt5 := codex.Price{InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10}
	mini := codex.Price{InputPerMTok: 0.25, CachedInputPerMTok: 0.025, OutputPerMTok: 2}
	nano := codex.Price{InputPerMTok: 0.05, CachedInputPerMTok: 0.005, OutputPerMTok: 0.40}
	tests := []struct {
		model string
		want  codex.Price
		ok    bool
	}{
		{"gpt-5", gpt5, true},
		{"GPT-5", gpt5, true},
		{"gpt-5-2025-08-07", gpt5, true},
		{"gpt-5-codex", gpt5, true},
		{"gpt-5-mini", mini, true},
		{"gpt-5-mini-2025-08-07", mini, true},
		{"gpt-5-nano", nano, true},
		{"gpt-5.1", codex.Price{}, false},
		{"gpt-5-codex-mini", codex.Price{}, false},
		{"gpt-5-pro", codex.Price{}, false},
		{"o3", codex.Price{}, false},
		{"", codex.Price{}, false},
	}
	for _, tt := range tests {
		got, ok := codex.LookupPrice(tt.model)
		if ok != tt.ok || got != tt.want {
			t.Errorf("LookupPrice(%q) = %+v, %v; want %+v, %v", tt.model, got, ok, tt.want, tt.ok)
		}
	}
}
