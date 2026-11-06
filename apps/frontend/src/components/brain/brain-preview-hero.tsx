import { lazy, Suspense, useMemo, type JSX } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../lib/api.ts';
import { isWebGLAvailable } from '../../lib/webgl.ts';
import { ErrorBoundary } from '../ui/error-boundary.tsx';
import { samplePreviewGraph, type PreviewGraph } from './preview-sample.ts';

const GraphScene = lazy(() => import('./graph-scene.tsx').then((m) => ({ default: m.GraphScene })));

interface Props {
  /** True while the hero is collapsed into the bar: no ambient motion. */
  paused: boolean;
}

/**
 * Full-bleed knowledge-graph preview behind the overview hero: no card
 * chrome, no interaction (the hero CTA opens /brain; pointer events pass
 * through so the wheel scrolls the page). Ambient motion follows the T3
 * rules (reduced motion, hidden tab, off-screen) and also stops while the
 * hero is collapsed. Without WebGL or data it renders nothing and the hero
 * keeps its static gradient.
 */
export function BrainPreviewHero({ paused }: Props): JSX.Element | null {
  const { t } = useTranslation();
  const webgl = isWebGLAvailable();
  const { data } = useQuery({ queryKey: ['graph'], queryFn: () => api.graph(), enabled: webgl });
  const { nodes, edges } = useMemo<PreviewGraph>(() => samplePreviewGraph(data), [data]);

  if (!webgl || nodes.length === 0) return null;
  return (
    <div
      className="h-full w-full"
      role="img"
      aria-label={t('brain.heroLabel', { nodes: nodes.length })}
    >
      <ErrorBoundary fallback={null}>
        <Suspense fallback={null}>
          <GraphScene nodes={nodes} edges={edges} colorBy="project" animate={!paused} />
        </Suspense>
      </ErrorBoundary>
    </div>
  );
}
