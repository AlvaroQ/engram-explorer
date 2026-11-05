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

// asFormatCost renders the USD cost; a trailing "~" marks a lower bound
// (some tokens were on an unpriced model).
func asFormatCost(u agentsessions.Usage) string {
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
