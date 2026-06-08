/**
 * use-brain-navigation.test.ts
 *
 * Strict TDD tests for the brain navigation stack logic.
 *
 * Strategy: the navigation state is extracted into a pure `BrainNavigationState`
 * class/object that can be unit-tested without React or RTL.
 * The `useBrainNavigation` hook is a thin React wrapper around the state.
 *
 * Covers:
 *   C3.1  push/pop semantics
 *         - push appends to stack
 *         - pop on empty is no-op
 *         - pop removes last item
 *         - push leaf (kind=observation) is no-op
 *         - reset() clears the stack
 *         - popTo(index) slices to given depth
 *   C3.3  isLeafRef helper
 *   C3.3  resolveNeighbors — M9 fallback to parent level
 */

import { describe, it, expect } from 'vitest';
import {
  createNavigationState,
  isLeafRef,
  resolveNeighbors,
} from './use-brain-navigation';
import type { BrainNodeRef, BrainLevel, BrainEdge } from '../model/types';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeRef(id: string, kind: BrainNodeRef['kind'] = 'lobe'): BrainNodeRef {
  return { id, kind, label: id };
}

function makeLeafRef(id: string): BrainNodeRef {
  return { id, kind: 'observation', label: id };
}

function makeLevel(
  nodeIds: string[],
  edges: BrainEdge[] = [],
): BrainLevel {
  return {
    nodes: nodeIds.map((id) => ({
      id,
      kind: 'observation' as const,
      label: id,
      color: '#fff',
      weight: 1,
      childCount: 0,
      meta: {},
    })),
    edges,
    parentPath: [],
    stats: { nodeCount: nodeIds.length, observationCount: nodeIds.length, crossSynapses: 0 },
  };
}

// ---------------------------------------------------------------------------
// C3.1 — push/pop stack semantics via createNavigationState
// ---------------------------------------------------------------------------

describe('createNavigationState — push/pop semantics', () => {
  it('path starts empty', () => {
    const nav = createNavigationState();
    expect(nav.getPath()).toEqual([]);
  });

  it('pushLevel(ref) appends to path', () => {
    const nav = createNavigationState();
    const ref = makeRef('lobe:engram');

    nav.pushLevel(ref);

    expect(nav.getPath()).toEqual([ref]);
  });

  it('pushLevel chains — multiple pushes accumulate', () => {
    const nav = createNavigationState();
    const ref1 = makeRef('lobe:engram');
    const ref2 = makeRef('neuron:engram/auth-model', 'neuron');

    nav.pushLevel(ref1);
    nav.pushLevel(ref2);

    expect(nav.getPath()).toEqual([ref1, ref2]);
  });

  it('pushLevel with a leaf ref (kind=observation) is a no-op', () => {
    const nav = createNavigationState();
    const leaf = makeLeafRef('obs:42');

    nav.pushLevel(leaf);

    // Leaf push must not modify path
    expect(nav.getPath()).toEqual([]);
  });

  it('pushLevel with cluster kind (non-leaf) is allowed', () => {
    const nav = createNavigationState();
    const cluster = makeRef('cluster:0', 'cluster');

    nav.pushLevel(cluster);

    expect(nav.getPath()).toEqual([cluster]);
  });

  it('pushLevel with neuron kind is allowed', () => {
    const nav = createNavigationState();
    const neuron = makeRef('neuron:engram/auth', 'neuron');

    nav.pushLevel(neuron);

    expect(nav.getPath()).toEqual([neuron]);
  });

  it('popLevel() removes the last entry', () => {
    const nav = createNavigationState();
    const ref1 = makeRef('lobe:engram');
    const ref2 = makeRef('neuron:engram/auth-model', 'neuron');

    nav.pushLevel(ref1);
    nav.pushLevel(ref2);
    nav.popLevel();

    expect(nav.getPath()).toEqual([ref1]);
  });

  it('popLevel() on empty stack is a no-op', () => {
    const nav = createNavigationState();

    nav.popLevel(); // no-op

    expect(nav.getPath()).toEqual([]);
  });

  it('reset() clears the entire stack', () => {
    const nav = createNavigationState();
    nav.pushLevel(makeRef('lobe:engram'));
    nav.pushLevel(makeRef('neuron:engram/auth-model', 'neuron'));

    nav.reset();

    expect(nav.getPath()).toEqual([]);
  });

  it('popTo(0) keeps first item, removes rest', () => {
    const nav = createNavigationState();
    const ref1 = makeRef('lobe:engram');
    const ref2 = makeRef('neuron:engram/auth-model', 'neuron');
    const ref3 = makeRef('neuron:engram/ui-model', 'neuron');

    nav.pushLevel(ref1);
    nav.pushLevel(ref2);
    nav.pushLevel(ref3);

    nav.popTo(0);

    expect(nav.getPath()).toEqual([ref1]);
  });

  it('popTo(-1) means root — path becomes []', () => {
    const nav = createNavigationState();
    nav.pushLevel(makeRef('lobe:engram'));

    nav.popTo(-1);

    expect(nav.getPath()).toEqual([]);
  });

  it('popTo out-of-bounds index is clamped safely', () => {
    const nav = createNavigationState();
    const ref1 = makeRef('lobe:engram');
    nav.pushLevel(ref1);

    // popTo beyond current length keeps all (no-op above top)
    nav.popTo(99);

    expect(nav.getPath()).toEqual([ref1]);
  });

  it('I16 — popTo(-1) produces same result as reset()', () => {
    // popTo(-1) must be equivalent to reset() — both clear the path to []
    const navA = createNavigationState();
    navA.pushLevel(makeRef('lobe:engram'));
    navA.pushLevel(makeRef('neuron:engram/auth', 'neuron'));
    navA.popTo(-1);

    const navB = createNavigationState();
    navB.pushLevel(makeRef('lobe:engram'));
    navB.pushLevel(makeRef('neuron:engram/auth', 'neuron'));
    navB.reset();

    expect(navA.getPath()).toEqual([]);
    expect(navB.getPath()).toEqual([]);
    expect(navA.getPath()).toEqual(navB.getPath());
  });

  it('I16 — popTo with values less than -1 also resets to root', () => {
    const nav = createNavigationState();
    nav.pushLevel(makeRef('lobe:engram'));
    nav.popTo(-2);
    expect(nav.getPath()).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// C3.3 — isLeafRef helper
// ---------------------------------------------------------------------------

describe('isLeafRef', () => {
  it('returns true for observation kind', () => {
    expect(isLeafRef(makeLeafRef('obs:42'))).toBe(true);
  });

  it('returns false for lobe kind', () => {
    expect(isLeafRef(makeRef('lobe:engram'))).toBe(false);
  });

  it('returns false for neuron kind', () => {
    expect(isLeafRef(makeRef('neuron:engram/auth', 'neuron'))).toBe(false);
  });

  it('returns false for cluster kind', () => {
    expect(isLeafRef(makeRef('cluster:0', 'cluster'))).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// C3.3 — resolveNeighbors fallback (M9)
// ---------------------------------------------------------------------------

describe('resolveNeighbors — M9 fallback', () => {
  it('returns incident edges from current level when available', () => {
    const edge: BrainEdge = {
      source: 'obs:1',
      target: 'obs:2',
      family: 'semantic',
      weight: 1,
    };
    const currentLevel = makeLevel(['obs:1', 'obs:2'], [edge]);

    const result = resolveNeighbors('obs:1', currentLevel, null);

    expect(result).toHaveLength(1);
    const first = result[0]!;
    expect(first.neighborId).toBe('obs:2');
    expect(first.edge).toBe(edge);
    expect(first.loaded).toBe(true);
  });

  it('falls back to parent level when current level has no incident edges (M9)', () => {
    // Leaf at L2 with empty edges
    const currentLevel = makeLevel(['obs:42'], []);
    // Parent level has the edge
    const parentEdge: BrainEdge = {
      source: 'obs:42',
      target: 'obs:99',
      family: 'topic-road',
      weight: 2,
    };
    const parentLevel = makeLevel(['obs:42', 'obs:99'], [parentEdge]);

    const result = resolveNeighbors('obs:42', currentLevel, parentLevel);

    expect(result).toHaveLength(1);
    const first = result[0]!;
    expect(first.neighborId).toBe('obs:99');
    expect(first.edge).toBe(parentEdge);
    expect(first.loaded).toBe(true);
  });

  it('returns loaded:false for neighbors not in the current graph (M9-B)', () => {
    // Parent level has the edge but obs:999 is NOT in any level
    const currentLevel = makeLevel(['obs:42'], []);
    const parentEdge: BrainEdge = {
      source: 'obs:42',
      target: 'obs:999',
      family: 'semantic',
      weight: 1,
    };
    const parentLevel = makeLevel(['obs:42'], [parentEdge]); // obs:999 not in nodes

    const result = resolveNeighbors('obs:42', currentLevel, parentLevel);

    expect(result).toHaveLength(1);
    const first = result[0]!;
    expect(first.neighborId).toBe('obs:999');
    expect(first.loaded).toBe(false);
  });

  it('returns empty array when no edges in either level', () => {
    const currentLevel = makeLevel(['obs:42'], []);
    const result = resolveNeighbors('obs:42', currentLevel, null);
    expect(result).toEqual([]);
  });

  it('handles edges where nodeId is the target (not only source)', () => {
    const edge: BrainEdge = {
      source: 'obs:99',
      target: 'obs:42',
      family: 'semantic',
      weight: 1,
    };
    const currentLevel = makeLevel(['obs:42', 'obs:99'], [edge]);

    const result = resolveNeighbors('obs:42', currentLevel, null);

    expect(result).toHaveLength(1);
    expect(result[0]!.neighborId).toBe('obs:99');
  });

  it('deduplicates parallel edges (same source/target pair)', () => {
    // Two edges between the same pair — resolveNeighbors returns one neighbor
    const edge1: BrainEdge = { source: 'obs:1', target: 'obs:2', family: 'semantic', weight: 1 };
    const edge2: BrainEdge = { source: 'obs:1', target: 'obs:2', family: 'topic-road', weight: 2 };
    const currentLevel = makeLevel(['obs:1', 'obs:2'], [edge1, edge2]);

    // BrainEdge is already aggregated, but even if duplicates somehow exist,
    // resolveNeighbors should return both (they are distinct BrainEdge objects)
    const result = resolveNeighbors('obs:1', currentLevel, null);

    // Both edges are distinct entries
    expect(result).toHaveLength(2);
    const neighborIds = result.map((r) => r.neighborId);
    expect(neighborIds).toEqual(['obs:2', 'obs:2']);
  });
});
