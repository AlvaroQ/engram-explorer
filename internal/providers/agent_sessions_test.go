package providers_test

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// TestAgentSessionsNav_OneEntryPerAgent: the shared "Agents" group lists one
// entry per known agent, linking to the agent-filtered sessions view, and
// each link reports the agent it belongs to.
func TestAgentSessionsNav_OneEntryPerAgent(t *testing.T) {
	g := providers.AgentSessionsNav()
	if g.ID != providers.AgentSessionsNavID || g.LabelKey != "nav.agents" {
		t.Fatalf("group = %q/%q, want %q/nav.agents", g.ID, g.LabelKey, providers.AgentSessionsNavID)
	}
	want := []struct{ agent, href, label, icon string }{
		{"claude-code", "/agent-sessions?agent=claude-code", "nav.agentClaudeCode", "agent-claude-code"},
		{"codex", "/agent-sessions?agent=codex", "nav.agentCodex", "agent-codex"},
	}
	if len(g.Links) != len(want) {
		t.Fatalf("links = %+v, want %d entries", g.Links, len(want))
	}
	for i, w := range want {
		l := g.Links[i]
		if l.Href != w.href || l.LabelKey != w.label || l.Icon != w.icon || l.External {
			t.Errorf("link %d = %+v, want href %s label %s icon %s", i, l, w.href, w.label, w.icon)
		}
		if got := providers.NavLinkAgent(l); got != w.agent {
			t.Errorf("NavLinkAgent(link %d) = %q, want %q", i, got, w.agent)
		}
	}
	if got := providers.NavLinkAgent(providers.NavLink{Href: "/brain", Key: "brain"}); got != "" {
		t.Errorf("NavLinkAgent(non-agent link) = %q, want empty", got)
	}
}
