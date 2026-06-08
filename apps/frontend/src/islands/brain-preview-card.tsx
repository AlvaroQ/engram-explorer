/**
 * Island: brain-preview-card
 *
 * Wraps BrainPreviewCard (Three.js + React Query graph preview).
 * Exported mount() is the standard island contract — called by loader.ts.
 *
 * Props:
 *   title   — card heading
 *   colorBy — "project" | "type"  (controls node coloring in the 3D graph)
 *
 * The component fetches /api/graph internally via React Query; no data is
 * passed via props. The island wrapper is intentionally thin — do NOT
 * rewrite BrainPreviewCard here, just mount it.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createRoot } from 'react-dom/client';
// Initialize i18n before rendering — BrainPreviewCard and its sub-components use useTranslation().
import '../lib/i18n.ts';
import { BrainPreviewCard } from '../components/brain/brain-preview-card.tsx';

export interface BrainPreviewCardProps {
  title: string;
  colorBy: 'project' | 'type';
}

// Each island gets its own QueryClient. Graph data fetched by two cards on the
// same page will be re-fetched twice (known cost documented in brain-preview-card.tsx).
// A shared client would require global context wiring outside the island model.
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
export function mount(el: HTMLElement, props: BrainPreviewCardProps): void {
  createRoot(el).render(
    <QueryClientProvider client={queryClient}>
      <BrainPreviewCard title={props.title} colorBy={props.colorBy} />
    </QueryClientProvider>,
  );
}
