/**
 * neighbors.ts — resolveNeighbors and supporting types for M9 neighbor resolution.
 *
 * Extracted from features/brain/navigation/use-brain-navigation.ts (now deleted).
 * The hook and navigation state that lived there are no longer used; only this
 * pure function was still referenced by node-detail-panel.tsx.
 *
 * M9: When the selected node has no incident edges in the current level, fall
 * back to the parent level before giving up.
 */

import type { BrainLevel, BrainEdge } from './types.ts';

/** Entry in the resolved neighbors list for a selected node. */
export interface NeighborEntry {
  /** The neighboring node's id. */
  neighborId: string;
  /** The edge connecting the selected node to this neighbor. */
  edge: BrainEdge;
  /**
   * Whether the neighbor is present in the loaded graph.
   * false when the node id cannot be found in either current or parent level (M9-B).
   */
  loaded: boolean;
}

/**
 * Resolve neighbors for `nodeId` from the visible levels.
 *
 * Algorithm (M9):
 * 1. Collect incident edges from `currentLevel`.
 * 2. If none found AND `parentLevel` is provided, fall back to `parentLevel`.
 * 3. For each incident edge, determine the neighbor id (the other endpoint).
 * 4. Mark `loaded: false` if the neighbor id is not present in any level's nodes.
 */
export function resolveNeighbors(
  nodeId: string,
  currentLevel: BrainLevel,
  parentLevel: BrainLevel | null,
): NeighborEntry[] {
  // Collect incident edges from current level
  let incidentEdges = findIncidentEdges(nodeId, currentLevel.edges);

  // M9: fall back to parent level when current level has no incident edges
  const resolveLevel =
    incidentEdges.length > 0 || !parentLevel ? currentLevel : parentLevel;

  if (incidentEdges.length === 0 && parentLevel) {
    incidentEdges = findIncidentEdges(nodeId, parentLevel.edges);
  }

  if (incidentEdges.length === 0) return [];

  // Build a set of all known node ids across both levels for loaded check
  const knownIds = new Set<string>(resolveLevel.nodes.map((n) => n.id));

  return incidentEdges.map((edge) => {
    const neighborId = edge.source === nodeId ? edge.target : edge.source;
    return {
      neighborId,
      edge,
      loaded: knownIds.has(neighborId),
    };
  });
}

/** Returns all edges from `edges` that have `nodeId` as source or target. */
function findIncidentEdges(nodeId: string, edges: BrainEdge[]): BrainEdge[] {
  return edges.filter((e) => e.source === nodeId || e.target === nodeId);
}
