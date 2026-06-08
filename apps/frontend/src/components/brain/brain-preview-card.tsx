import { lazy, Suspense, useMemo, useState, type JSX } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type GraphNode, type GraphEdge } from '../../lib/api.ts';
import { Card, CardBody, CardHeader } from '../ui/card.tsx';
import { Skeleton } from '../ui/skeleton.tsx';
import { NodeDetailPanel } from './node-detail-panel.tsx';
import { nodeColor } from './node-colors.ts';

const GraphScene = lazy(() =>
  import('./graph-scene.tsx').then((m) => ({ default: m.GraphScene })),
);

// The preview cards run an O(n²) force layout in a Web Worker. Above ~200 nodes
// it takes seconds to converge AND nodes cluster too tight for a fixed-camera
// mini view, so we cap the sample here. The full graph still lives in /brain.
//
// KNOWN COST: the Overview mounts two cards (colorBy "project" and "type") with
// the SAME graph, so each GraphScene spins up its OWN worker and the identical
// layout is computed twice. Sharing one layout would mean lifting the worker out
// of graph-scene.tsx (the brain feature's core); deferred intentionally. The
// PREVIEW_LIMIT cap keeps the duplicated work bounded.
const PREVIEW_LIMIT = 150;

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
  const navigate = useNavigate();
  const [selected, setSelected] = useState<GraphNode | null>(null);

  const { data, isLoading, error } = useQuery({
    queryKey: ['graph'],
    queryFn: () => api.graph(),
  });

  // Sample top-N nodes by weight for the preview. Drop edges that touch a
  // node we filtered out so the worker doesn't try to attract to a ghost.
  const { nodes, edges } = useMemo<{ nodes: GraphNode[]; edges: GraphEdge[] }>(() => {
    if (!data) return { nodes: [], edges: [] };
    if (data.nodes.length <= PREVIEW_LIMIT) {
      return { nodes: data.nodes, edges: data.edges };
    }
    const top = [...data.nodes].sort((a, b) => b.weight - a.weight).slice(0, PREVIEW_LIMIT);
    const ids = new Set(top.map((n) => n.id));
    const kept = data.edges.filter((e) => ids.has(e.source) && ids.has(e.target));
    return { nodes: top, edges: kept };
  }, [data]);

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
              <Suspense fallback={<Skeleton className="h-full" />}>
                <GraphScene
                  nodes={nodes}
                  edges={edges}
                  colorBy={colorBy}
                  onNodeClick={setSelected}
                  selected={selected}
                  focusNodeId={selected?.id ?? null}
                  focusInsetRight={selected ? PANEL_INSET_PX : null}
                  animate
                />
              </Suspense>

              {selected !== null ? (
                <NodeDetailPanel
                  node={selected}
                  compact
                  color={nodeColor(selected, nodes, colorBy)}
                  onClose={() => setSelected(null)}
                  onNavigate={(path) => { void navigate({ to: path as '/' }); }}
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
