/**
 * segmentation-panel-logic.ts — Pure logic for the segmentation panel.
 *
 * Extracted from the React component so it can be unit-tested without
 * jsdom or RTL.
 *
 * Contracts:
 *   S6.1  AND-across-groups / OR-within-group facet composition.
 *   S6.1  Search query: case-insensitive label.includes, AND-composed with facets.
 *   S6.3  Non-match dimming: computeMatchedIds returns Set<string> (subset of ids).
 *   S9.3  Zero-match state: empty Set (not null) when filters active but 0 nodes pass.
 *         null means "no active filter" (all nodes visible at full opacity).
 */

import type { BrainNode, ActiveFacets } from '../model/types.ts';

// ---------------------------------------------------------------------------
// Legend entry
// ---------------------------------------------------------------------------

export interface LegendEntry {
  label: string;
  color: string;
}

// ---------------------------------------------------------------------------
// computeMatchedIds
// ---------------------------------------------------------------------------

/**
 * Compute the set of BrainNode ids that pass the active facets and search query.
 *
 * Returns:
 *   - null     — no filter active (all nodes pass; render at full opacity)
 *   - Set<string> — explicit match set (may be empty for 0-match state S9.3)
 *
 * The return value drives the dim-overlay in the canvas:
 *   null → no dimming
 *   Set  → nodes NOT in the set are dimmed (S6.3)
 */
export function computeMatchedIds(
  nodes: BrainNode[],
  facets: ActiveFacets,
  query: string,
): Set<string> | null {
  const q = query.toLowerCase().trim();

  const hasProjectFilter = (facets.project?.length ?? 0) > 0;
  const hasTypeFilter = (facets.type?.length ?? 0) > 0;
  const hasScopeFilter = (facets.scope?.length ?? 0) > 0;
  const hasToolFilter = (facets.tool?.length ?? 0) > 0;
  const hasDateFilter = (facets.date?.length ?? 0) > 0;
  const hasQuery = q.length > 0;

  const hasAnyFilter =
    hasProjectFilter ||
    hasTypeFilter ||
    hasScopeFilter ||
    hasToolFilter ||
    hasDateFilter ||
    hasQuery;

  // No filter active → return null (no dimming needed)
  if (!hasAnyFilter) return null;

  const matched = new Set<string>();

  for (const n of nodes) {
    // Search filter (AND-composed with facets)
    if (hasQuery && !n.label.toLowerCase().includes(q)) continue;

    // Project group (OR within group)
    if (hasProjectFilter) {
      const nodeProject = n.meta['project'];
      if (!facets.project!.includes(nodeProject as string)) continue;
    }

    // Type group
    if (hasTypeFilter) {
      const nodeType = n.meta['type'];
      if (!facets.type!.includes(nodeType as string)) continue;
    }

    // Scope group
    if (hasScopeFilter) {
      const nodeScope = n.meta['scope'];
      if (!facets.scope!.includes(nodeScope as string)) continue;
    }

    // Tool group
    if (hasToolFilter) {
      const nodeTool = n.meta['tool'];
      if (!facets.tool!.includes(nodeTool as string)) continue;
    }

    // Date group (year-month prefix match)
    if (hasDateFilter) {
      const nodeDate = n.meta['createdAt'];
      if (typeof nodeDate !== 'string') continue;
      const ym = nodeDate.slice(0, 7);
      if (!facets.date!.includes(ym)) continue;
    }

    matched.add(n.id);
  }

  return matched;
}

// ---------------------------------------------------------------------------
// isZeroMatch
// ---------------------------------------------------------------------------

/**
 * Returns true when an active filter produces 0 matching nodes (S9.3).
 * Returns false when there is no filter (null) or when some nodes match.
 */
export function isZeroMatch(matchedIds: Set<string> | null): boolean {
  return matchedIds !== null && matchedIds.size === 0;
}

// ---------------------------------------------------------------------------
// buildLegendEntries
// ---------------------------------------------------------------------------

/**
 * Build legend entries from the current level's BrainNodes.
 *
 * Legend mode 'color': deduplicate by BrainNode.color and return one entry
 * per unique color. The label is derived from the node's project or type field.
 *
 * Entries are sorted by label for stable ordering.
 */
export function buildLegendEntries(
  nodes: BrainNode[],
  _mode: 'color',
): LegendEntry[] {
  if (nodes.length === 0) return [];

  // Deduplicate by color — map color → first seen project/type label
  const colorToLabel = new Map<string, string>();

  for (const n of nodes) {
    if (!colorToLabel.has(n.color)) {
      // Prefer project label, fallback to type or the node label
      const project = n.meta['project'];
      const type = n.meta['type'];
      const label =
        (typeof project === 'string' && project ? project : null) ??
        (typeof type === 'string' && type ? type : null) ??
        n.label;
      colorToLabel.set(n.color, label);
    }
  }

  return [...colorToLabel.entries()]
    .map(([color, label]) => ({ color, label }))
    .sort((a, b) => a.label.localeCompare(b.label));
}
