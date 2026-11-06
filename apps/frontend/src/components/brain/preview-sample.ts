import type { GraphEdge, GraphNode } from '../../lib/api.ts';

// The previews (overview hero and card) run the force layout in a Web Worker
// over a fixed-camera mini view: above ~150 nodes the clusters get too tight,
// so they sample the heaviest nodes. The full graph lives in /brain.
export const PREVIEW_LIMIT = 150;

export interface PreviewGraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

/**
 * Keeps the `limit` heaviest nodes and drops the edges that touch a node left
 * out, so the worker never attracts to a ghost.
 */
export function samplePreviewGraph(
  data: { nodes: GraphNode[]; edges: GraphEdge[] } | undefined,
  limit: number = PREVIEW_LIMIT,
): PreviewGraph {
  if (!data) return { nodes: [], edges: [] };
  if (data.nodes.length <= limit) return { nodes: data.nodes, edges: data.edges };
  const top = [...data.nodes].sort((a, b) => b.weight - a.weight).slice(0, limit);
  const ids = new Set(top.map((n) => n.id));
  return { nodes: top, edges: data.edges.filter((e) => ids.has(e.source) && ids.has(e.target)) };
}
