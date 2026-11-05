package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// TestAsListLoadMore_NoInlineCode: the load-more sentinel and the rows are
// CSP-clean (no inline script/style, no hx-on or eval-only attributes).
func TestAsListLoadMore_NoInlineCode(t *testing.T) {
	v := agentListView{
		Result: agentsessions.ListResult{
			Items: []agentsessions.Session{{Agent: "codex", Source: "codex", Key: "k", FirstPrompt: "p"}},
			Total: 120, Page: 1, PageSize: 50, Pages: 3, HasMore: true,
		},
		Query: agentListQuery{Agent: "codex", Page: 1},
		Lang:  "en",
	}
	var b strings.Builder
	if err := AgentSessionsListPartial(v).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	var rows strings.Builder
	if err := AgentSessionsListRows(v).Render(context.Background(), &rows); err != nil {
		t.Fatal(err)
	}
	for _, html := range []string{b.String(), rows.String()} {
		for _, viol := range InlineCodeViolations(html) {
			t.Error(viol)
		}
		if strings.Contains(html, "hx-on") {
			t.Error("hx-on attributes are not allowed (allowEval=false)")
		}
	}
	if !strings.Contains(rows.String(), "as-load-more-row") {
		t.Error("rows partial lacks the sentinel while HasMore")
	}
}
