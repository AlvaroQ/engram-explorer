/**
 * Island: cc-usage-charts
 *
 * Renders Claude Code session analytics (cost by project, tokens by model,
 * totals). Data is fetched on its own from /api/cc-sessions/stats (server-cached
 * by a directory fingerprint), so the heavy full-transcript scan never blocks
 * the page render and the island shows its own loading state.
 *
 * Labels are passed in as props from the Go templ side (localized via T()), so
 * this island does not pull in the frontend i18n bundle.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createRoot } from 'react-dom/client';
import { CCUsageCharts, type CCUsageLabels } from '../components/charts/cc-usage-charts.tsx';

export interface CCUsageChartsProps {
  labels: CCUsageLabels;
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: 1,
    },
  },
});

export function mount(el: HTMLElement, props: CCUsageChartsProps): void {
  createRoot(el).render(
    <QueryClientProvider client={queryClient}>
      <CCUsageCharts labels={props.labels} />
    </QueryClientProvider>,
  );
}
