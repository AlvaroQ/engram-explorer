package ui

import (
	"fmt"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// View helpers of the vendor-neutral agent-session pages.

// asAgentLabel is the display name of an agent kind ("claude-code" → "Claude
// Code"); unknown kinds are shown as-is.
func asAgentLabel(lang, agent string) string {
	key := "agentSessions.agents." + agent
	if v := T(lang, key); v != key {
		return v
	}
	return agent
}

// asTurnText returns the first non-empty text block of a turn.
func asTurnText(t agentsessions.Turn) string {
	for _, b := range t.Blocks {
		if b.Kind == agentsessions.BlockText && strings.TrimSpace(b.Text) != "" {
			return b.Text
		}
	}
	return ""
}

// asFirstUserPrompt returns the first user text without the IDE/harness
// markers of agent, skipping marker-only turns (the page title).
func asFirstUserPrompt(agent string, turns []agentsessions.Turn) string {
	for _, t := range turns {
		if t.Role != "user" {
			continue
		}
		if clean := ccsPlainFromSegments(asSplitMarkers(agent, asTurnText(t))); clean != "" {
			return clean
		}
	}
	return ""
}

// asCountRole counts the turns with role.
func asCountRole(turns []agentsessions.Turn, role string) int {
	n := 0
	for _, t := range turns {
		if t.Role == role {
			n++
		}
	}
	return n
}

// asClock renders the turn time as HH:MM:SS (UTC), or "—".
func asClock(t agentsessions.Turn) string {
	if t.Timestamp.IsZero() {
		return "—"
	}
	return t.Timestamp.UTC().Format("15:04:05")
}

// asTurnPreview summarizes a turn with the Claude Code marker set
// (asTurnPreviewFor).
func asTurnPreview(t agentsessions.Turn) string { return asTurnPreviewFor(claudeAgent, t) }

// asTurnPreviewFor summarizes a turn for its collapsed summary line, hiding
// the IDE/harness markers of agent.
func asTurnPreviewFor(agent string, t agentsessions.Turn) string {
	var parts []string
	for _, b := range t.Blocks {
		switch b.Kind {
		case agentsessions.BlockText:
			if plain := ccsPlainFromSegments(asSplitMarkers(agent, b.Text)); plain != "" {
				parts = append(parts, ccsPromptPreview(plain))
			} else if b.Text != "" {
				parts = append(parts, "📎 attachment")
			}
		case agentsessions.BlockToolCall:
			parts = append(parts, "⚙ "+b.ToolName)
		case agentsessions.BlockToolResult:
			parts = append(parts, "→ result")
		case agentsessions.BlockReasoning:
			parts = append(parts, "💭 reasoning")
		}
	}
	if len(parts) == 0 {
		return "(empty)"
	}
	s := []rune(strings.Join(parts, " · "))
	if len(s) > 100 {
		return string(s[:100]) + "…"
	}
	return string(s)
}

// asTruncate caps s at maxRunes, marking the cut.
func asTruncate(s string, maxRunes int) string {
	if s == "" {
		return "—"
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "… [truncated]"
}

// asUnpriced reports a usage whose models all lack a known price: tokens
// were spent but no cost could be estimated (shown as "Not priced", never
// as a misleading $0).
func asUnpriced(u agentsessions.Usage) bool {
	return u.CostUSD <= 0 && u.UnknownModel
}

// asFormatCost renders the USD cost; a trailing "~" marks a lower bound
// (some tokens were on an unpriced model) and a fully unpriced usage reads
// "Not priced" (localized).
func asFormatCost(u agentsessions.Usage, lang string) string {
	if asUnpriced(u) {
		return T(lang, "agentSessions.cost.notPriced")
	}
	if u.CostUSD <= 0 {
		return "—"
	}
	s := fmt.Sprintf("$%.2f", u.CostUSD)
	if u.CostUSD < 0.01 {
		s = "<$0.01"
	}
	if u.UnknownModel {
		s += "~"
	}
	return s
}

// asCostTitle is the tooltip of an unpriced cost, naming models when known;
// "" for any other usage (no tooltip).
func asCostTitle(u agentsessions.Usage, models []string, lang string) string {
	if !asUnpriced(u) {
		return ""
	}
	if len(models) == 0 {
		return T(lang, "agentSessions.cost.notPricedHint")
	}
	return T(lang, "agentSessions.cost.notPricedHintModels", "models", strings.Join(models, ", "))
}

// asCostCaptionModels is how many model ids a visible caption names before
// folding the rest into "+N".
const asCostCaptionModels = 3

// asCostCaption is the visible explanation under an unpriced cost, naming up
// to three models then "+N"; "" for any other usage.
func asCostCaption(u agentsessions.Usage, models []string, lang string) string {
	if !asUnpriced(u) {
		return ""
	}
	if len(models) == 0 {
		return T(lang, "agentSessions.cost.notPricedHint")
	}
	list := strings.Join(models[:min(len(models), asCostCaptionModels)], ", ")
	if extra := len(models) - asCostCaptionModels; extra > 0 {
		list += fmt.Sprintf(" +%d", extra)
	}
	return T(lang, "agentSessions.cost.notPricedCaption", "models", list)
}

// asUsageBreakdown is the token tooltip: the disjoint buckets, then
// reasoning as part of output.
func asUsageBreakdown(u agentsessions.Usage, lang string) string {
	s := fmt.Sprintf("%s %d · %s %d · %s %d · %s %d",
		T(lang, "agentSessions.usage.input"), u.Input,
		T(lang, "agentSessions.usage.cacheRead"), u.CacheRead,
		T(lang, "agentSessions.usage.cacheWrite"), u.CacheWrite,
		T(lang, "agentSessions.usage.output"), u.Output)
	if u.Reasoning > 0 {
		s += fmt.Sprintf(" · %s %d", T(lang, "agentSessions.usage.reasoning"), u.Reasoning)
	}
	return s
}

// asSortKeys are the sort options of the list, in display order.
var asSortKeys = []string{
	agentsessions.SortEnded, agentsessions.SortStarted, agentsessions.SortCost,
	agentsessions.SortTokens, agentsessions.SortSize, agentsessions.SortProject,
}

// asSortSelected reports whether option is the active sort (default: ended).
func asSortSelected(current, option string) bool {
	if current == "" {
		return option == agentsessions.SortEnded
	}
	return current == option
}
