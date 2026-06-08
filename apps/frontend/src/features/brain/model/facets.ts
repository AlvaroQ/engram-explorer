/**
 * facets.ts — Stackable facet index and predicate composition.
 *
 * Predicate composition rule:
 *   AND across groups / OR within group
 *
 * A node matches when, for every facet group with at least one selected value,
 * the node's corresponding field matches at least one selected value.
 *
 * Facet availability (C8):
 *   availableFacets() inspects the raw GraphNode array and only returns
 *   groups whose backing field is present and non-empty in at least one node.
 *   Hidden groups never appear in the segmentation panel UI.
 */

import type { GraphNode } from '../../../lib/api.ts';
import type { BrainNode } from './types.ts';
import type { ActiveFacets, FacetGroupId } from './types.ts';

// ---------------------------------------------------------------------------
// Facet index
// ---------------------------------------------------------------------------

/**
 * Build a facet value index from the current BrainNode[] in a level.
 * Returns Map<groupId, Set<string>> of available values per group.
 * Used by the segmentation panel to render facet checkboxes.
 */
export function buildFacetIndex(nodes: BrainNode[]): Map<FacetGroupId, Set<string>> {
  const index = new Map<FacetGroupId, Set<string>>();

  const ensure = (g: FacetGroupId) => {
    if (!index.has(g)) index.set(g, new Set<string>());
    return index.get(g)!;
  };

  for (const n of nodes) {
    const project = n.meta['project'];
    if (typeof project === 'string' && project) ensure('project').add(project);

    const type = n.meta['type'];
    if (typeof type === 'string' && type) ensure('type').add(type);

    const scope = n.meta['scope'];
    if (typeof scope === 'string' && scope) ensure('scope').add(scope);

    const tool = n.meta['tool'];
    if (typeof tool === 'string' && tool) ensure('tool').add(tool);

    const date = n.meta['createdAt'];
    if (typeof date === 'string' && date) {
      // Bucket by year-month for usability
      ensure('date').add(date.slice(0, 7));
    }
  }

  return index;
}

// ---------------------------------------------------------------------------
// Predicate application
// ---------------------------------------------------------------------------

/**
 * Filter BrainNode[] by the active facets and optional search query.
 *
 * - AND across groups: every non-empty group must match.
 * - OR within group: at least one selected value must match.
 * - Search query: case-insensitive substring match on node.label.
 *
 * Empty facets + empty query → all nodes pass.
 */
export function applyFacets(
  nodes: BrainNode[],
  active: ActiveFacets,
  query = '',
): BrainNode[] {
  const q = query.toLowerCase();

  return nodes.filter((n) => {
    // Search filter (AND-composed with facets)
    if (q && !n.label.toLowerCase().includes(q)) return false;

    // Project group
    if (active.project && active.project.length > 0) {
      const nodeProject = n.meta['project'];
      if (!active.project.includes(nodeProject as string)) return false;
    }

    // Type group
    if (active.type && active.type.length > 0) {
      const nodeType = n.meta['type'];
      if (!active.type.includes(nodeType as string)) return false;
    }

    // Scope group
    if (active.scope && active.scope.length > 0) {
      const nodeScope = n.meta['scope'];
      if (!active.scope.includes(nodeScope as string)) return false;
    }

    // Tool group
    if (active.tool && active.tool.length > 0) {
      const nodeTool = n.meta['tool'];
      if (!active.tool.includes(nodeTool as string)) return false;
    }

    // Date group (matches if year-month prefix is in selected dates)
    if (active.date && active.date.length > 0) {
      const nodeDate = n.meta['createdAt'];
      if (typeof nodeDate !== 'string') return false;
      const ym = nodeDate.slice(0, 7);
      if (!active.date.includes(ym)) return false;
    }

    return true;
  });
}

// ---------------------------------------------------------------------------
// Facet availability (C8)
// ---------------------------------------------------------------------------

/**
 * Inspect the raw GraphNode array and return the facet groups whose backing
 * field is present and non-empty in at least one node.
 *
 * Groups backed by fields not in GraphNode (tool_name, session_id) are absent
 * from the result — hidden from the UI until the backend exposes those fields.
 * When those fields appear, this function surfaces them with zero UI changes.
 */
export function availableFacets(
  nodes: ReadonlyArray<GraphNode & { tool_name?: string | null; session_id?: string | null }>,
): FacetGroupId[] {
  const result: FacetGroupId[] = [];

  if (nodes.some((n) => n.project != null && n.project !== '')) {
    result.push('project');
  }
  if (nodes.some((n) => n.type != null && n.type !== '')) {
    result.push('type');
  }
  if (nodes.some((n) => n.scope != null && n.scope !== '')) {
    result.push('scope');
  }
  // Tool: backed by tool_name (not in GraphNode v1 — only in ObservationRow)
  if (nodes.some((n) => n.tool_name != null && n.tool_name !== '')) {
    result.push('tool');
  }
  // Date: backed by createdAt
  if (nodes.some((n) => n.createdAt != null && n.createdAt !== '')) {
    result.push('date');
  }

  return result;
}
