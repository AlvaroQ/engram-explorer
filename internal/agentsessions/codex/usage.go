package codex

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// tokenUsage mirrors Codex's TokenUsage. cached_input_tokens and
// cache_write_input_tokens are subsets of input_tokens;
// reasoning_output_tokens is a subset of output_tokens.
type tokenUsage struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_output_tokens"`
	Total      int64 `json:"total_tokens"`
}

// key identifies a cumulative total; replayed rows share it.
func (t tokenUsage) key() string {
	return fmt.Sprintf("tot:%d/%d/%d/%d/%d/%d", t.Input, t.Cached, t.CacheWrite, t.Output, t.Reasoning, t.Total)
}

// decreasedFrom reports whether any counter went down (a new baseline).
func (t tokenUsage) decreasedFrom(p tokenUsage) bool {
	return t.Input < p.Input || t.Cached < p.Cached || t.CacheWrite < p.CacheWrite ||
		t.Output < p.Output || t.Reasoning < p.Reasoning || t.Total < p.Total
}

func (t tokenUsage) minus(p tokenUsage) tokenUsage {
	return tokenUsage{
		Input: t.Input - p.Input, Cached: t.Cached - p.Cached, CacheWrite: t.CacheWrite - p.CacheWrite,
		Output: t.Output - p.Output, Reasoning: t.Reasoning - p.Reasoning, Total: t.Total - p.Total,
	}
}

// normalize maps Codex usage into disjoint buckets (L6.4): cached tokens are
// carved out of input_tokens.
func (t tokenUsage) normalize() agentsessions.Usage {
	return agentsessions.Usage{
		Input:      max(t.Input-t.Cached-t.CacheWrite, 0),
		CacheRead:  t.Cached,
		CacheWrite: t.CacheWrite,
		Output:     t.Output,
		Reasoning:  t.Reasoning,
	}
}

// usageEvent is one countable usage increment found in a file. key dedupes
// it across the files of a root session ("" = never deduped).
type usageEvent struct {
	key   string
	model string
	usage tokenUsage
}

// Price is a per-1M-token OpenAI list price.
type Price struct {
	InputPerMTok       float64
	CachedInputPerMTok float64
	OutputPerMTok      float64
}

// prices holds the published OpenAI API list prices (USD per 1M tokens) for
// the gpt-5 family (source: https://openai.com/api/pricing/; values recorded
// for Phase 3 decision L6.7 on 2026-11-04 — re-verify there before relying on
// them). Edit here when rates change; token counts stay exact regardless.
// Models absent from this table are flagged UnknownModel with cost 0.
var prices = map[string]Price{
	"gpt-5":       {InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10},
	"gpt-5-codex": {InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10},
	"gpt-5-mini":  {InputPerMTok: 0.25, CachedInputPerMTok: 0.025, OutputPerMTok: 2},
	"gpt-5-nano":  {InputPerMTok: 0.05, CachedInputPerMTok: 0.005, OutputPerMTok: 0.40},
}

// snapshotSuffix is the only accepted prefix extension: a dated snapshot
// such as "gpt-5-2025-08-07" or "gpt-5-mini-2025-08-07".
var snapshotSuffix = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)

// LookupPrice returns the price for model, matched case-insensitively by
// exact name or by exact name plus a dated snapshot suffix. Other variants
// (gpt-5.1, gpt-5-pro, gpt-5-codex-mini, ...) are not guessed.
func LookupPrice(model string) (Price, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if p, ok := prices[m]; ok {
		return p, true
	}
	if loc := snapshotSuffix.FindStringIndex(m); loc != nil {
		p, ok := prices[m[:loc[0]]]
		return p, ok
	}
	return Price{}, false
}

// priced returns the normalized usage of t with its cost. Cache writes are
// billed at the input rate (OpenAI does not price them separately).
func priced(t tokenUsage, model string) agentsessions.Usage {
	u := t.normalize()
	p, ok := LookupPrice(model)
	if !ok {
		u.UnknownModel = u.Total() > 0
		return u
	}
	const m = 1_000_000.0
	u.CostUSD = float64(u.Input+u.CacheWrite)/m*p.InputPerMTok +
		float64(u.CacheRead)/m*p.CachedInputPerMTok +
		float64(u.Output)/m*p.OutputPerMTok
	return u
}
