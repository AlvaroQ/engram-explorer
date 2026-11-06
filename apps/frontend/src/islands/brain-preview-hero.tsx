/**
 * Island: brain-preview-hero
 *
 * The overview hero background (BrainPreviewHero). The collapse runtime
 * dispatches HERO_PAUSE_EVENT on document when the hero collapses into the
 * bar; the island pauses the graph's ambient motion until it expands again.
 */
import { useEffect, useState, type JSX } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createRoot } from 'react-dom/client';
import '../lib/i18n.ts';
import { BrainPreviewHero } from '../components/brain/brain-preview-hero.tsx';
import { HERO_PAUSE_EVENT, heroPausedFromEvent } from '../lib/overview-hero.ts';

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 60_000, retry: 1 } },
});

function Hero(): JSX.Element {
  const [paused, setPaused] = useState(false);
  useEffect(() => {
    const onPause = (e: Event): void => {
      const p = heroPausedFromEvent(e);
      if (p !== null) setPaused(p);
    };
    document.addEventListener(HERO_PAUSE_EVENT, onPause);
    return () => document.removeEventListener(HERO_PAUSE_EVENT, onPause);
  }, []);
  return (
    <QueryClientProvider client={queryClient}>
      <BrainPreviewHero paused={paused} />
    </QueryClientProvider>
  );
}

export function mount(el: HTMLElement): void {
  createRoot(el).render(<Hero />);
}
