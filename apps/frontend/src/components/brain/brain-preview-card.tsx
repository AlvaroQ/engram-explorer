import { lazy, Suspense, useMemo, useState, type JSX } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type GraphNode } from '../../lib/api.ts';
import { Card, CardBody, CardHeader } from '../ui/card.tsx';
import { ErrorBoundary } from '../ui/error-boundary.tsx';
import { Skeleton } from '../ui/skeleton.tsx';
import { NodeDetailPanel } from './node-detail-panel.tsx';
import { nodeColor } from './node-colors.ts';
import { samplePreviewGraph, type PreviewGraph } from './preview-sample.ts';

const GraphScene = lazy(() => import('./graph-scene.tsx').then((m) => ({ default: m.GraphScene })));

// The preview samples the heaviest nodes (samplePreviewGraph): the full graph
// lives in /brain. Each mounted preview runs its own layout worker.

// Horizontal space the open detail panel reserves on the right edge of the card:
// the compact panel is w-72 (288px) anchored with right-3 (12px). The fly-to uses
// this to offset a focused node left of the panel instead of centering under it.
const PANEL_INSET_PX = 288 + 12;

interface Props {
  title: string;
  description?: string;
  colorBy: 'project' | 'type';
}

export function BrainPreviewCard({ title, description, colorBy }: Props): JSX.Element {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<GraphNode | null>(null);

  const { data, isLoading, error } = useQuery({
    queryKey: ['graph'],
    queryFn: () => api.graph(),
  });

  const { nodes, edges } = useMemo<PreviewGraph>(() => samplePreviewGraph(data), [data]);

  return (
    <Card className="flex flex-col">
      <CardHeader title={title} description={description} />
      <CardBody className="flex min-h-0 flex-1 flex-col p-0">
        {/* relative so the NodeDetailPanel (absolute) is scoped to this card */}
        <div className="relative h-[260px] w-full px-1 pb-1">
          {isLoading ? (
            <Skeleton className="h-full" />
          ) : error ? (
            <p className="px-4 pt-4 text-sm text-fail">
              {error instanceof Error ? error.message : String(error)}
            </p>
          ) : nodes.length > 0 ? (
            /* pointer-events-auto ensures canvas clicks and the overlaid panel both work */
            <div className="absolute inset-x-1 inset-y-0 pb-1 pointer-events-auto">
              <ErrorBoundary
                fallback={<p className="px-4 pt-4 text-sm text-fail">{t('brain.graphError')}</p>}
              >
                <Suspense fallback={<Skeleton className="h-full" />}>
                  <GraphScene
                    nodes={nodes}
                    edges={edges}
                    colorBy={colorBy}
                    onNodeClick={setSelected}
                    selected={selected}
                    detailOpen={selected !== null}
                    focusNodeId={selected?.id ?? null}
                    focusInsetRight={selected ? PANEL_INSET_PX : null}
                    animate
                  />
                </Suspense>
              </ErrorBoundary>

              {selected !== null ? (
                <NodeDetailPanel
                  node={selected}
                  compact
                  color={nodeColor(selected, nodes, colorBy)}
                  onClose={() => setSelected(null)}
                  onNavigate={(path) => {
                    window.location.href = path;
                  }}
                />
              ) : null}
            </div>
          ) : (
            <p className="px-4 pt-4 text-sm text-fg-muted">{t('brain.emptyGraph')}</p>
          )}
        </div>
      </CardBody>
    </Card>
  );
}
