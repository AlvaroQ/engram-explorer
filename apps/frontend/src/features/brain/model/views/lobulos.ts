/**
 * lobulos.ts — Lóbulos view grouping.
 *
 * Level 0 (root): one "lobe" node per distinct project.
 * Level 1 (children): observation-kind leaf nodes in that lobe.
 * Level 2: leaf — no further drill.
 *
 * Default color-by: 'project'.
 */

import type { GraphNode } from '../../../../lib/api.ts';
import type { BrainNode, BrainNodeRef } from '../types.ts';
import { buildLobesShared } from './shared.ts';

export const defaultColorBy = 'project' as const;

/**
 * Build lobe-level aggregate BrainNode[] from the flat GraphNode array.
 * One lobe per distinct project (null project → "∅").
 * Delegates to buildLobesShared with dominantType included (I18).
 */
export function buildLobes(nodes: GraphNode[]): BrainNode[] {
  return buildLobesShared(nodes, /* includeDominantType */ true);
}

/**
 * Build observation-leaf BrainNode[] for a given project.
 */
export function buildLobeChildren(
  project: string,
  nodes: GraphNode[],
  palette: Map<string, string>,
): BrainNode[] {
  const members = nodes.filter((n) => (n.project ?? '∅') === project);
  return members.map((n) => obsNode(n, palette));
}

/** Synthetic BrainNodeRef for a lobe. */
export function lobeRef(project: string): BrainNodeRef {
  return { id: `lobe:${project}`, kind: 'lobe', label: project };
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/** Convert a GraphNode to a leaf observation BrainNode. */
export function obsNode(n: GraphNode, palette: Map<string, string>): BrainNode {
  return {
    id: `obs:${n.id}`,
    kind: 'observation',
    label: n.label ?? `obs:${n.id}`,
    color: palette.get(n.project ?? '∅') ?? '#4b5563',
    weight: n.weight,
    childCount: 0,
    meta: {
      project: n.project,
      type: n.type,
      scope: n.scope,
      topicKey: n.topicKey,
      createdAt: n.createdAt,
    },
    sourceIds: [n.id],
  };
}

