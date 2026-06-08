/**
 * shared.ts — Shared helpers used across multiple view modules.
 *
 * Extracted to avoid duplication between lobulos.ts and temas.ts (I18).
 * Zero side-effects — pure functions only.
 */

import type { GraphNode } from '../../../../lib/api.ts';
import type { BrainNode } from '../types.ts';
import { buildHexPalette } from '../../../../components/brain/node-colors.ts';

// ---------------------------------------------------------------------------
// Project grouping — shared between lobulos and temas buildLobes
// ---------------------------------------------------------------------------

/**
 * Group a flat GraphNode array by project key.
 * Null project is mapped to '∅'.
 */
export function groupByProject(nodes: GraphNode[]): Map<string, GraphNode[]> {
  const byProject = new Map<string, GraphNode[]>();
  for (const n of nodes) {
    const key = n.project ?? '∅';
    const bucket = byProject.get(key);
    if (bucket) bucket.push(n);
    else byProject.set(key, [n]);
  }
  return byProject;
}

/**
 * Build lobe-level aggregate BrainNode[] from the flat GraphNode array.
 * One lobe per distinct project (null project → "∅").
 *
 * Shared implementation used by both lobulos.ts and temas.ts — their
 * only difference was an extra `dominantType` field in the lobe meta,
 * which is now handled by the `includeDominantType` flag.
 */
export function buildLobesShared(
  nodes: GraphNode[],
  includeDominantType = false,
): BrainNode[] {
  const byProject = groupByProject(nodes);
  const palette = buildHexPalette(nodes.map((n) => n.project));

  return Array.from(byProject.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([project, members]) => {
      const meta: Record<string, unknown> = {
        project,
        observationCount: members.length,
      };
      if (includeDominantType) {
        const dt = dominantValue(members, (m) => m.type ?? null);
        if (dt !== undefined) meta['dominantType'] = dt;
      }
      return {
        id: `lobe:${project}`,
        kind: 'lobe' as const,
        label: project,
        color: palette.get(project) ?? '#4b5563',
        weight: members.length,
        childCount: members.length,
        meta,
        sourceIds: members.map((m) => m.id),
      };
    });
}

// ---------------------------------------------------------------------------
// Dominant value helpers
// ---------------------------------------------------------------------------

/**
 * Return the most frequent non-null value extracted by `accessor` from `items`.
 * Ties broken by string sort ascending. Returns undefined when no values.
 */
export function dominantValue<T>(
  items: T[],
  accessor: (item: T) => string | null | undefined,
): string | undefined {
  const counts = new Map<string, number>();
  for (const item of items) {
    const v = accessor(item);
    if (!v) continue;
    counts.set(v, (counts.get(v) ?? 0) + 1);
  }
  if (counts.size === 0) return undefined;

  let best: string | undefined;
  let bestCount = 0;
  for (const [v, c] of [...counts.entries()].sort(([a], [b]) => a.localeCompare(b))) {
    if (c > bestCount) { best = v; bestCount = c; }
  }
  return best;
}

/**
 * Convenience wrapper: dominant `type` field across GraphNode members.
 */
export function dominantType(members: GraphNode[]): string | undefined {
  return dominantValue(members, (m) => m.type ?? null);
}
