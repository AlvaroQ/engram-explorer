/**
 * use-brain-navigation.ts — Level path stack and breadcrumb state.
 *
 * Design contracts:
 *   S4.1  path: BrainNodeRef[] stack; push appends, pop removes last.
 *   S4.1  pushLevel(ref) is a no-op for leaf nodes (kind === 'observation').
 *   S4.1  popLevel() is a no-op when path is empty.
 *   S4.1  Escape key calls popLevel() when path.length > 0.
 *   M9    resolveNeighbors falls back to parent level when current level has
 *         no incident edges for the selected node.
 *
 * Architecture note:
 *   The navigation state is extracted into a plain `BrainNavigationState` object
 *   (createNavigationState()) to keep it unit-testable without React/RTL.
 *   The useBrainNavigation hook is a thin React wrapper that re-renders on change.
 */

import { useState, useEffect, useCallback } from 'react';
import type { BrainNodeRef, BrainLevel, BrainEdge } from '../model/types';

// ---------------------------------------------------------------------------
// isLeafRef helper (C3.3 / S4.1)
// ---------------------------------------------------------------------------

/**
 * Returns true when a BrainNodeRef represents a leaf observation.
 * Leaf nodes must not be pushed onto the navigation stack.
 */
export function isLeafRef(ref: BrainNodeRef): boolean {
  return ref.kind === 'observation';
}

// ---------------------------------------------------------------------------
// Pure navigation state (unit-testable, no React)
// ---------------------------------------------------------------------------

export interface NavigationState {
  /** Returns a copy of the current path stack. */
  getPath(): BrainNodeRef[];
  /** Append ref to the path. No-op if ref is a leaf (kind=observation). */
  pushLevel(ref: BrainNodeRef): void;
  /** Remove the last entry. No-op if path is empty. */
  popLevel(): void;
  /** Slice the path to include only entries 0..index (inclusive). -1 means root ([]). */
  popTo(index: number): void;
  /** Clear the entire stack. */
  reset(): void;
  /** Subscribe to state changes. Returns an unsubscribe function. */
  subscribe(listener: () => void): () => void;
}

/**
 * Factory for the pure navigation state — no React dependencies.
 * Used directly by unit tests and wrapped by useBrainNavigation.
 */
export function createNavigationState(): NavigationState {
  let path: BrainNodeRef[] = [];
  const listeners = new Set<() => void>();

  function notify() {
    listeners.forEach((l) => l());
  }

  return {
    getPath(): BrainNodeRef[] {
      return [...path];
    },

    pushLevel(ref: BrainNodeRef): void {
      if (isLeafRef(ref)) return; // S4.1: leaf push is no-op
      path = [...path, ref];
      notify();
    },

    popLevel(): void {
      if (path.length === 0) return; // S4.1: pop on empty is no-op
      path = path.slice(0, -1);
      notify();
    },

    popTo(index: number): void {
      if (index < 0) {
        path = [];
      } else if (index < path.length) {
        path = path.slice(0, index + 1);
      }
      // index >= path.length: no-op (popTo beyond top clamps safely)
      notify();
    },

    reset(): void {
      path = [];
      notify();
    },

    subscribe(listener: () => void): () => void {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

// ---------------------------------------------------------------------------
// React hook
// ---------------------------------------------------------------------------

export interface BrainNavigation {
  /** Current navigation path — empty means at root. */
  path: BrainNodeRef[];
  /** Push a node onto the navigation stack. No-op for leaf nodes. */
  pushLevel(ref: BrainNodeRef): void;
  /** Pop the last entry off the stack. No-op when empty. */
  popLevel(): void;
  /**
   * Pop back to the entry at `index` (inclusive).
   * Pass -1 to go to root.
   */
  popTo(index: number): void;
  /** Reset the stack to root. */
  reset(): void;
}

/**
 * React hook wrapping createNavigationState.
 *
 * Manages:
 * - path stack (push/pop/reset)
 * - Escape key handler (S4.1: Escape pops when path is non-empty)
 *
 * The navigation state instance is stable across renders (created once per
 * component mount). Re-renders only on path change.
 */
export function useBrainNavigation(): BrainNavigation {
  // Create stable navigation state instance
  const [navState] = useState(() => createNavigationState());

  // Sync React state with the navigation state
  const [path, setPath] = useState<BrainNodeRef[]>([]);

  useEffect(() => {
    // Subscribe to navigation state changes
    const unsubscribe = navState.subscribe(() => {
      setPath(navState.getPath());
    });
    return unsubscribe;
  }, [navState]);

  // Stable callbacks that delegate to the navigation state
  const pushLevel = useCallback(
    (ref: BrainNodeRef) => navState.pushLevel(ref),
    [navState],
  );

  const popLevel = useCallback(() => navState.popLevel(), [navState]);

  const popTo = useCallback(
    (index: number) => navState.popTo(index),
    [navState],
  );

  const reset = useCallback(() => navState.reset(), [navState]);

  // Escape key handler (S4.1 / S10.3)
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        navState.popLevel();
      }
    }

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [navState]);

  return { path, pushLevel, popLevel, popTo, reset };
}

// ---------------------------------------------------------------------------
// resolveNeighbors — M9 neighbor resolution with parent-level fallback
// ---------------------------------------------------------------------------

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
  const resolveLevel = incidentEdges.length > 0 || !parentLevel
    ? currentLevel
    : parentLevel;

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
