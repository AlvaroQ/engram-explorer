package providers

import (
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// AgentSessionsNavID is the sidebar group every agent-session provider
// contributes to. Registry.NavGroups merges groups sharing an ID, so enabling
// several agent-session providers renders a single "Agents" group.
const AgentSessionsNavID = "agent-sessions"

// agentNavKeyPrefix prefixes the agent kind in the Key of an agent nav link.
const agentNavKeyPrefix = "agent:"

// agentNavEntries are the agent kinds with a sidebar entry, in display order.
var agentNavEntries = []struct{ agent, labelKey string }{
	{"claude-code", "nav.agentClaudeCode"},
	{"codex", "nav.agentCodex"},
}

// AgentSessionsNav returns the shared "Agents" sidebar group: one entry per
// known agent kind, linking to the sessions view filtered by that agent. The
// host shows only the entries whose agent has a live source (NavLinkAgent);
// the cross-agent view stays reachable as a tab of that page.
func AgentSessionsNav() NavGroup {
	links := make([]NavLink, 0, len(agentNavEntries))
	for _, e := range agentNavEntries {
		links = append(links, NavLink{
			Href:     agentsessions.RoutePrefix + "?agent=" + e.agent,
			Key:      agentNavKeyPrefix + e.agent,
			LabelKey: e.labelKey,
			Icon:     "agent-" + e.agent,
		})
	}
	return NavGroup{ID: AgentSessionsNavID, LabelKey: "nav.agents", Links: links}
}

// NavLinkAgent returns the agent kind of an AgentSessionsNav link, or "" for
// any other link.
func NavLinkAgent(l NavLink) string {
	agent, _ := strings.CutPrefix(l.Key, agentNavKeyPrefix)
	if agent == l.Key {
		return ""
	}
	return agent
}

// SourceByID returns the agent-session source with the given ID among those
// inst serves (AgentSessionSource), or nil. Providers use it in their detail
// routes to resolve the {source} of the URL among their live sources.
func SourceByID(inst Instance, id string) agentsessions.Source {
	as, ok := inst.(AgentSessionSource)
	if !ok {
		return nil
	}
	for _, s := range as.AgentSessionSources() {
		if s.ID() == id {
			return s
		}
	}
	return nil
}
