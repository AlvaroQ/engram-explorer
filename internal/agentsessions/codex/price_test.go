package codex_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
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
		{"gpt-5.6-sol", codex.Price{InputPerMTok: 4, CachedInputPerMTok: 0.40, OutputPerMTok: 20}, true},
		{"gpt-5.6-sol-2026-07-09", codex.Price{InputPerMTok: 4, CachedInputPerMTok: 0.40, OutputPerMTok: 20}, true},
		{"gpt-5.6-terra", codex.Price{InputPerMTok: 2, CachedInputPerMTok: 0.20, OutputPerMTok: 12}, true},
		{"gpt-5.6-luna", codex.Price{InputPerMTok: 0.20, CachedInputPerMTok: 0.02, OutputPerMTok: 1.20}, true},
		{"gpt-5.5", codex.Price{InputPerMTok: 5, CachedInputPerMTok: 0.50, OutputPerMTok: 30}, true},
		{"gpt-5.4", codex.Price{InputPerMTok: 2.50, CachedInputPerMTok: 0.25, OutputPerMTok: 15}, true},
		{"gpt-5.4-mini", codex.Price{InputPerMTok: 0.75, CachedInputPerMTok: 0.075, OutputPerMTok: 4.50}, true},
		{"gpt-5.4-nano", codex.Price{InputPerMTok: 0.20, CachedInputPerMTok: 0.02, OutputPerMTok: 1.25}, true},
		{"gpt-5.3-codex", codex.Price{InputPerMTok: 1.75, CachedInputPerMTok: 0.175, OutputPerMTok: 14}, true},
		{"gpt-5.2", codex.Price{InputPerMTok: 1.75, CachedInputPerMTok: 0.175, OutputPerMTok: 14}, true},
		{"gpt-5.1", codex.Price{InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10}, true},
		{"gpt-5.6-cyber", codex.Price{}, false},
		{"gpt-5.7-sol", codex.Price{}, false},
		{"gpt-5.6", codex.Price{}, false},
		{"gpt-5.6-sol-pro", codex.Price{}, false},
		{"gpt-5.1-codex-max", codex.Price{}, false},
		{"gpt-6-astra", codex.Price{}, false},
		{"codex-auto-review", codex.Price{}, false},
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

// listOne writes one Codex rollout whose turns are the given cumulative
// token_count totals on model and returns the listed session's cost.
func listOne(t *testing.T, model string, totals ...[3]int64) (cost float64, unknown bool) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "11", "06")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"timestamp":"2026-11-06T10:00:00Z","type":"session_meta","payload":{"id":"p","cwd":"/w/p"}}`,
		`{"timestamp":"2026-11-06T10:00:01Z","type":"turn_context","payload":{"cwd":"/w/p","model":"` + model + `"}}`,
	}
	for i, tot := range totals { // {input incl. cached, cached, output}
		lines = append(lines, fmt.Sprintf(`{"timestamp":"2026-11-06T10:00:%02dZ","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":%d,"cached_input_tokens":%d,"output_tokens":%d,"reasoning_output_tokens":0,"total_tokens":%d}}}}`,
			i+2, tot[0], tot[1], tot[2], tot[0]+tot[2]))
	}
	writeFile(t, filepath.Join(dir, "rollout-p.jsonl"), strings.Join(lines, "\n")+"\n")
	ss, err := codex.New("codex", home).List(context.Background())
	if err != nil || len(ss) != 1 {
		t.Fatalf("list = %v, %v", ss, err)
	}
	return ss[0].Usage.CostUSD, ss[0].Usage.UnknownModel
}

// TestCodexCostShortContext prices one turn of 100K input (60K cached) and
// 10K output for every model added from the 2026-11-06 pricing page.
func TestCodexCostShortContext(t *testing.T) {
	// cost = 40K*in + 60K*cached + 10K*out, per 1M tokens.
	cost := func(in, cached, out float64) float64 { return (40_000*in + 60_000*cached + 10_000*out) / 1e6 }
	tests := []struct {
		model string
		want  float64
	}{
		{"gpt-5.6-sol", cost(4, 0.40, 20)},
		{"gpt-5.6-sol-2026-07-09", cost(4, 0.40, 20)},
		{"gpt-5.6-terra", cost(2, 0.20, 12)},
		{"gpt-5.6-luna", cost(0.20, 0.02, 1.20)},
		{"gpt-5.5", cost(5, 0.50, 30)},
		{"gpt-5.4", cost(2.50, 0.25, 15)},
		{"gpt-5.4-mini", cost(0.75, 0.075, 4.50)},
		{"gpt-5.4-nano", cost(0.20, 0.02, 1.25)},
		{"gpt-5.3-codex", cost(1.75, 0.175, 14)},
		{"gpt-5.2", cost(1.75, 0.175, 14)},
		{"gpt-5.1", cost(1.25, 0.125, 10)},
	}
	for _, tt := range tests {
		got, unknown := listOne(t, tt.model, [3]int64{100_000, 60_000, 10_000})
		if unknown || math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("%s: cost = %v unknown=%v, want %v", tt.model, got, unknown, tt.want)
		}
	}
}

// TestCodexCostLongContext applies the long-context rate per turn: only the
// turn whose input exceeds 272K tokens is billed at the higher rate.
func TestCodexCostLongContext(t *testing.T) {
	short := func(in, cached, out float64) float64 { return (40_000*in + 60_000*cached + 10_000*out) / 1e6 }
	long := func(in, cached, out float64) float64 { return (100_000*in + 200_000*cached + 5_000*out) / 1e6 }
	tests := []struct {
		model string
		want  float64
	}{
		{"gpt-5.6-sol", short(4, 0.40, 20) + long(8, 0.80, 30)},
		{"gpt-5.6-terra", short(2, 0.20, 12) + long(4, 0.40, 18)},
		{"gpt-5.6-luna", short(0.20, 0.02, 1.20) + long(0.40, 0.04, 1.80)},
		{"gpt-5.5", short(5, 0.50, 30) + long(10, 1, 45)},
		{"gpt-5.4", short(2.50, 0.25, 15) + long(5, 0.50, 22.50)},
		// No published long-context rate in the table: short-context rate (lower bound).
		{"gpt-5.1", short(1.25, 0.125, 10) + long(1.25, 0.125, 10)},
	}
	for _, tt := range tests {
		// Turn 1: 100K input (60K cached), 10K output. Turn 2 (delta): 300K
		// input (200K cached), 5K output — above the 272K threshold.
		got, unknown := listOne(t, tt.model, [3]int64{100_000, 60_000, 10_000}, [3]int64{400_000, 260_000, 15_000})
		if unknown || math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("%s: cost = %v unknown=%v, want %v", tt.model, got, unknown, tt.want)
		}
	}
	// Exactly 272K input stays on the short-context rate.
	got, _ := listOne(t, "gpt-5.6-sol", [3]int64{272_000, 0, 0})
	if want := 272_000 * 4 / 1e6; math.Abs(got-want) > 1e-9 {
		t.Errorf("272K input: cost = %v, want %v", got, want)
	}
}

func TestCodexCostUnknownVariantsStayUnpriced(t *testing.T) {
	for _, m := range []string{"gpt-5.6-cyber", "gpt-5.7-sol", "gpt-6-astra", "codex-auto-review"} {
		if got, unknown := listOne(t, m, [3]int64{100_000, 60_000, 10_000}); !unknown || got != 0 {
			t.Errorf("%s: cost = %v unknown=%v, want 0 and unknown", m, got, unknown)
		}
	}
}
