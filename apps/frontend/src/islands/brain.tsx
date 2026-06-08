/**
 * Island: brain
 *
 * Wraps the full BrainPage component as a React island mounted by loader.ts.
 * The brain page fetches its own data via /api/graph and manages all state
 * internally — no props required.
 *
 * Navigation in BrainPage uses window.location.href (full-page navigation to
 * /ui/* templ pages), so no TanStack Router context is required here.
 *
 * Props: {} (empty — the brain page self-contains all state and data fetching)
 *
 * Exported mount() is the standard island contract — called by loader.ts.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createRoot } from 'react-dom/client';
// Initialize i18n before rendering — BrainPage uses useTranslation().
// The side-effect import calls i18n.init() via sync.Once equivalent.
import '../lib/i18n.ts';
import { BrainPage } from '../pages/brain-page.tsx';

export interface BrainIslandProps {
  // empty — brain page manages its own state
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: 1,
    },
  },
});

export function mount(el: HTMLElement, _props: BrainIslandProps): void {
  createRoot(el).render(
    <QueryClientProvider client={queryClient}>
      <BrainPage />
    </QueryClientProvider>,
  );
}
