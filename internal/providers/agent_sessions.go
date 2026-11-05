package providers

import "github.com/AlvaroQ/engram-explorer/internal/agentsessions"

// AgentSessionsNavID is the sidebar group every agent-session provider
// contributes to. Registry.NavGroups merges groups sharing an ID, so enabling
// several agent-session providers renders a single "Agent sessions" group.
const AgentSessionsNavID = "agent-sessions"

// AgentSessionsNav returns the shared sidebar group of the agent-session
// providers: the cross-agent overview and session list served by the host.
func AgentSessionsNav() NavGroup {
	return NavGroup{
		ID:       AgentSessionsNavID,
		LabelKey: "nav.agentSessions",
		Links: []NavLink{
			{Href: "/agent-sessions", Key: "agent-sessions", LabelKey: "nav.agentSessionsOverview", Icon: "agent-sessions-overview"},
			{Href: "/agent-sessions/list", Key: "agent-sessions-list", LabelKey: "nav.agentSessionsList", Icon: "agent-sessions"},
		},
	}
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
