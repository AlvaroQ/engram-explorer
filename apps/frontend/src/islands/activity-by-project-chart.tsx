/**
 * Island: activity-by-project-chart
 *
 * Renders a stacked BarChart (recharts) showing observations per day by project.
 * Exported mount() is the standard island contract — called by loader.ts.
 *
 * This island has NO props: it fetches data itself via /api/activity?range=...
 * so the range toggle (7d / 30d / 90d) can live entirely inside React without
 * any round-trip to the server. Decision: keep the range toggle inside the island
 * (React state) rather than as a query param, because the chart manages its own
 * loading state and the toggle is tightly coupled to the chart's fetch.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createRoot } from 'react-dom/client';
// Initialize i18n before rendering — ActivityByProjectChart uses useTranslation().
import '../lib/i18n.ts';
import { ActivityByProjectChart } from '../components/charts/activity-by-project-chart.tsx';

export interface ActivityByProjectChartProps {
  // No props — the island fetches /api/activity?range=... on its own.
  // Defined here to satisfy the island contract shape.
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: 1,
    },
  },
});

/**
 * Island contract: called by loader.ts with the host element and parsed props.
 */
export function mount(el: HTMLElement, _props: ActivityByProjectChartProps): void {
  createRoot(el).render(
    <QueryClientProvider client={queryClient}>
      <ActivityByProjectChart />
    </QueryClientProvider>,
  );
}
