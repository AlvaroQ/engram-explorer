/**
 * Island: agent-usage-charts
 *
 * Renders coding-agent session analytics across every live agent (Claude
 * Code, Codex, ...): totals, cost per agent, project and day, tokens per
 * model and the most expensive sessions. Data is fetched on its own from
 * /api/agent-sessions/stats (server-cached by a source fingerprint), so the
 * heavy transcript scan never blocks the page render.
 *
 * Labels and agent display names are passed in as props from the Go templ
 * side (localized via T()), so this island does not pull in the frontend
 * i18n bundle.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createRoot } from 'react-dom/client';
import {
  AgentUsageCharts,
  type AgentUsageLabels,
} from '../components/charts/agent-usage-charts.tsx';

export interface AgentUsageChartsProps {
  labels: AgentUsageLabels;
  agents: Record<string, string>;
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: 1,
    },
  },
});

export function mount(el: HTMLElement, props: AgentUsageChartsProps): void {
  createRoot(el).render(
    <QueryClientProvider client={queryClient}>
      <AgentUsageCharts labels={props.labels} agents={props.agents} />
    </QueryClientProvider>,
  );
}
