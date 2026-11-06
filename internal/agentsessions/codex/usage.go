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

// prices holds the published OpenAI API standard list prices (USD per 1M
// tokens, short context: at most LongContextThreshold input tokens per
// request). Edit here when rates change; token counts stay exact regardless.
// Models absent from this table are flagged UnknownModel with cost 0: no
// variant is guessed from a family name.
//
// Sources:
//   - gpt-5.1 … gpt-5.6-*: https://developers.openai.com/api/docs/pricing
//     (checked 2026-11-06). The page notes: "GPT-5.6 Sol's promotional
//     pricing is available until November 30, 2026." — re-check gpt-5.6-sol
//     after that date.
//   - gpt-5, gpt-5-codex, gpt-5-mini, gpt-5-nano: https://openai.com/api/pricing/
//     (recorded for Phase 3 decision L6.7 on 2026-11-04).
var prices = map[string]Price{
	"gpt-5":         {InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10},
	"gpt-5-codex":   {InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10},
	"gpt-5-mini":    {InputPerMTok: 0.25, CachedInputPerMTok: 0.025, OutputPerMTok: 2},
	"gpt-5-nano":    {InputPerMTok: 0.05, CachedInputPerMTok: 0.005, OutputPerMTok: 0.40},
	"gpt-5.1":       {InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10},
	"gpt-5.2":       {InputPerMTok: 1.75, CachedInputPerMTok: 0.175, OutputPerMTok: 14},
	"gpt-5.3-codex": {InputPerMTok: 1.75, CachedInputPerMTok: 0.175, OutputPerMTok: 14},
	"gpt-5.4":       {InputPerMTok: 2.50, CachedInputPerMTok: 0.25, OutputPerMTok: 15},
	"gpt-5.4-mini":  {InputPerMTok: 0.75, CachedInputPerMTok: 0.075, OutputPerMTok: 4.50},
	"gpt-5.4-nano":  {InputPerMTok: 0.20, CachedInputPerMTok: 0.02, OutputPerMTok: 1.25},
	"gpt-5.5":       {InputPerMTok: 5, CachedInputPerMTok: 0.50, OutputPerMTok: 30},
	"gpt-5.6-sol":   {InputPerMTok: 4, CachedInputPerMTok: 0.40, OutputPerMTok: 20},
	"gpt-6-astra":   {InputPerMTok: 10, CachedInputPerMTok: 1, OutputPerMTok: 50},
	"gpt-5.6-terra": {InputPerMTok: 2, CachedInputPerMTok: 0.20, OutputPerMTok: 12},
	"gpt-5.6-luna":  {InputPerMTok: 0.20, CachedInputPerMTok: 0.02, OutputPerMTok: 1.20},
}

// LongContextThreshold is the per-request input size (cached tokens
// included) above which OpenAI bills the long-context rate.
const LongContextThreshold = 272_000

// longContextPrices are the published long-context rates (same source and
// date as prices). A priced model without an entry here keeps its
// short-context rate on long requests, so its cost is a lower bound.
var longContextPrices = map[string]Price{
	"gpt-5.4":       {InputPerMTok: 5, CachedInputPerMTok: 0.50, OutputPerMTok: 22.50},
	"gpt-5.5":       {InputPerMTok: 10, CachedInputPerMTok: 1, OutputPerMTok: 45},
	"gpt-5.6-sol":   {InputPerMTok: 8, CachedInputPerMTok: 0.80, OutputPerMTok: 30},
	"gpt-6-astra":   {InputPerMTok: 20, CachedInputPerMTok: 2, OutputPerMTok: 75},
	"gpt-5.6-terra": {InputPerMTok: 4, CachedInputPerMTok: 0.40, OutputPerMTok: 18},
	"gpt-5.6-luna":  {InputPerMTok: 0.40, CachedInputPerMTok: 0.04, OutputPerMTok: 1.80},
}

// snapshotSuffix is the only accepted prefix extension: a dated snapshot
// such as "gpt-5-2025-08-07", "gpt-5-mini-2025-08-07" or
// "gpt-5.6-sol-2026-07-09".
var snapshotSuffix = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)

// LookupPrice returns the price for model, matched case-insensitively by
// exact name or by exact name plus a dated snapshot suffix. Other variants
// (gpt-5-pro, gpt-5-codex-mini, gpt-5.6-cyber, ...) are not guessed.
func LookupPrice(model string) (Price, bool) {
	p, ok := prices[baseModel(model)]
	return p, ok
}

// baseModel lowercases model and strips a dated snapshot suffix unless the
// full name is itself a table key.
func baseModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if _, ok := prices[m]; ok {
		return m
	}
	if loc := snapshotSuffix.FindStringIndex(m); loc != nil {
		return m[:loc[0]]
	}
	return m
}

// requestPrice is the rate for one request of inputTokens (cached included):
// the long-context rate above LongContextThreshold when one is published.
func requestPrice(model string, inputTokens int64) (Price, bool) {
	base := baseModel(model)
	p, ok := prices[base]
	if ok && inputTokens > LongContextThreshold {
		if lp, has := longContextPrices[base]; has {
			p = lp
		}
	}
	return p, ok
}

// priced returns the normalized usage of t with its cost. t is one usage
// increment (normally one request), so the long-context rate is chosen from
// its own input. Cache writes are billed at the input rate: Codex rollouts
// carry no separate cache-write count to apply the 1.25x write rate to.
func priced(t tokenUsage, model string) agentsessions.Usage {
	u := t.normalize()
	p, ok := requestPrice(model, t.Input)
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
