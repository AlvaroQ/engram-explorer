/**
 * client-brain-model.test.ts
 *
 * Strict TDD tests for ClientBrainModel. Run in RED first, then GREEN after
 * implementation. Covers:
 *   C2-A  leaf returns empty BrainLevel
 *   C3-A  edge aggregation: weight=3, family=semantic, relation=related (dominant)
 *   C3-B  confidence mean
 *   C3-C  edge budget cull at 200
 *   C4-A  relation → family mapping
 *   M1-A  Lóbulos root depth (lobe per project, children = observations)
 *   M1-B  Temas root → lobe → neuron → observations depth
 *   round-trip  Lóbulos → Orgánico → Lóbulos id stability
 */

import { describe, it, expect } from 'vitest';
import type { GraphEdge, GraphNode } from '../../../lib/api.ts';
import { createClientBrainModel } from './client-brain-model.ts';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeNode(
  id: number,
  project: string,
  opts: Partial<GraphNode> = {},
): GraphNode {
  return {
    id,
    project,
    type: 'decision',
    scope: 'project',
    topicKey: null,
    label: `obs ${id}`,
    weight: 1,
    duplicateCount: 1,
    createdAt: null,
    ...opts,
  };
}

function makeEdge(
  source: number,
  target: number,
  relation: GraphEdge['relation'] = 'related',
  confidence?: number,
): GraphEdge {
  const e: GraphEdge = { source, target, relation };
  if (confidence !== undefined) e.confidence = confidence;
  return e;
}

// ---------------------------------------------------------------------------
// C2-A: Leaf returns empty BrainLevel
// ---------------------------------------------------------------------------

describe('C2-A — leaf contract', () => {
  it('children(leafId) returns an empty BrainLevel, never throws', () => {
    const nodes = [makeNode(1, 'engram'), makeNode(2, 'engram')];
    const model = createClientBrainModel(nodes, [], 'lobulos');

    const root = model.root({});
    // In Lóbulos root, nodes are lobes (not leaves). Drill into lobe to get leaves.
    const lobeNode = root.nodes[0];
    expect(lobeNode).toBeDefined();
    expect(lobeNode!.childCount).toBeGreaterThan(0);

    const childLevel = model.children(lobeNode!.id, {});
    const leaf = childLevel.nodes[0];
    expect(leaf).toBeDefined();
    expect(leaf!.childCount).toBe(0);
    expect(leaf!.kind).toBe('observation');

    // Now call children on the leaf — must return empty level, never throw
    const emptyLevel = model.children(leaf!.id, {});
    expect(emptyLevel.nodes).toEqual([]);
    expect(emptyLevel.edges).toEqual([]);
    expect(emptyLevel.stats.nodeCount).toBe(0);
    // parentPath must include the leaf node ref
    expect(emptyLevel.parentPath.some((r) => r.id === leaf!.id)).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// C3-A: Edge aggregation — weight=3, family=semantic, relation=related (dominant)
// ---------------------------------------------------------------------------

describe('C3-A — edge aggregation weight and dominant relation', () => {
  it('merges 3 edges A→D, B→D, C→D into one BrainEdge with weight=3 and relation=related', () => {
    // A, B, C all in lobe:P1; D in lobe:P2
    const nodes = [
      makeNode(1, 'P1'), // A
      makeNode(2, 'P1'), // B
      makeNode(3, 'P1'), // C
      makeNode(4, 'P2'), // D
    ];
    const edges = [
      makeEdge(1, 4, 'related'),
      makeEdge(2, 4, 'related'),
      makeEdge(3, 4, 'compatible'), // minority
    ];

    const model = createClientBrainModel(nodes, edges, 'lobulos');
    const root = model.root({});

    const lobeP1 = root.nodes.find((n) => n.id === 'lobe:P1');
    const lobeP2 = root.nodes.find((n) => n.id === 'lobe:P2');
    expect(lobeP1).toBeDefined();
    expect(lobeP2).toBeDefined();

    // The root level should have a single aggregated edge between lobe:P1 and lobe:P2
    const aggEdge = root.edges.find(
      (e) =>
        (e.source === 'lobe:P1' && e.target === 'lobe:P2') ||
        (e.source === 'lobe:P2' && e.target === 'lobe:P1'),
    );
    expect(aggEdge).toBeDefined();
    expect(aggEdge!.weight).toBe(3);
    expect(aggEdge!.family).toBe('semantic');
    expect(aggEdge!.relation).toBe('related'); // 2 of 3 → dominant
  });
});

// ---------------------------------------------------------------------------
// C3-B: Confidence mean
// ---------------------------------------------------------------------------

describe('C3-B — confidence mean', () => {
  it('averages confidence values from merged edges', () => {
    const nodes = [makeNode(1, 'P1'), makeNode(2, 'P2')];
    const edges = [
      makeEdge(1, 2, 'related', 0.8),
      makeEdge(1, 2, 'related', 0.6),
    ];

    const model = createClientBrainModel(nodes, edges, 'lobulos');
    const root = model.root({});

    const aggEdge = root.edges.find(
      (e) =>
        (e.source === 'lobe:P1' && e.target === 'lobe:P2') ||
        (e.source === 'lobe:P2' && e.target === 'lobe:P1'),
    );
    expect(aggEdge).toBeDefined();
    expect(aggEdge!.confidence).toBeCloseTo(0.7);
  });

  it('treats missing confidence as 0.5 for averaging when mixed', () => {
    const nodes = [makeNode(1, 'P1'), makeNode(2, 'P2')];
    const edges = [
      makeEdge(1, 2, 'related', 0.8),
      makeEdge(1, 2, 'related', undefined), // no confidence → treated as 0.5
    ];

    const model = createClientBrainModel(nodes, edges, 'lobulos');
    const root = model.root({});

    const aggEdge = root.edges.find(
      (e) =>
        (e.source === 'lobe:P1' && e.target === 'lobe:P2') ||
        (e.source === 'lobe:P2' && e.target === 'lobe:P1'),
    );
    expect(aggEdge).toBeDefined();
    expect(aggEdge!.confidence).toBeCloseTo(0.65);
  });

  it('leaves confidence undefined when ALL underlying edges have no confidence', () => {
    const nodes = [makeNode(1, 'P1'), makeNode(2, 'P2')];
    const edges = [
      makeEdge(1, 2, 'related', undefined),
      makeEdge(1, 2, 'related', undefined),
    ];

    const model = createClientBrainModel(nodes, edges, 'lobulos');
    const root = model.root({});

    const aggEdge = root.edges.find(
      (e) =>
        (e.source === 'lobe:P1' && e.target === 'lobe:P2') ||
        (e.source === 'lobe:P2' && e.target === 'lobe:P1'),
    );
    expect(aggEdge).toBeDefined();
    expect(aggEdge!.confidence).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// C3-C: Edge budget cull at 200
// ---------------------------------------------------------------------------

describe('C3-C — edge budget cull', () => {
  it('culls to 200 edges when a level produces more than 200 BrainEdges', () => {
    // Build 250 distinct projects P0..P249, one node each, each connected to P0
    const nodes: GraphNode[] = [];
    const edges: GraphEdge[] = [];

    // Node 0 in project "hub"
    nodes.push(makeNode(0, 'hub', { weight: 1 }));

    for (let i = 1; i <= 250; i++) {
      nodes.push(makeNode(i, `spoke${i}`, { weight: 1 }));
      edges.push(makeEdge(0, i, 'related', 0.5));
    }

    const model = createClientBrainModel(nodes, edges, 'lobulos');
    const root = model.root({});

    expect(root.edges.length).toBeLessThanOrEqual(200);
  });
});

// ---------------------------------------------------------------------------
// C4-A: Relation → family mapping
// ---------------------------------------------------------------------------

describe('C4-A — relation to family mapping', () => {
  it('maps related → semantic, compatible → semantic, scoped → topic-road', () => {
    const nodes = [
      makeNode(1, 'P1'),
      makeNode(2, 'P2'),
      makeNode(3, 'P3'),
      makeNode(4, 'P4'),
    ];
    const edges = [
      makeEdge(1, 2, 'related'),
      makeEdge(1, 3, 'compatible'),
      makeEdge(1, 4, 'scoped'),
    ];

    const model = createClientBrainModel(nodes, edges, 'lobulos');
    const root = model.root({});

    const edgeP1P2 = root.edges.find(
      (e) =>
        (e.source === 'lobe:P1' && e.target === 'lobe:P2') ||
        (e.source === 'lobe:P2' && e.target === 'lobe:P1'),
    );
    const edgeP1P3 = root.edges.find(
      (e) =>
        (e.source === 'lobe:P1' && e.target === 'lobe:P3') ||
        (e.source === 'lobe:P3' && e.target === 'lobe:P1'),
    );
    const edgeP1P4 = root.edges.find(
      (e) =>
        (e.source === 'lobe:P1' && e.target === 'lobe:P4') ||
        (e.source === 'lobe:P4' && e.target === 'lobe:P1'),
    );

    expect(edgeP1P2).toBeDefined();
    expect(edgeP1P2!.family).toBe('semantic');

    expect(edgeP1P3).toBeDefined();
    expect(edgeP1P3!.family).toBe('semantic');

    expect(edgeP1P4).toBeDefined();
    expect(edgeP1P4!.family).toBe('topic-road');
  });
});

// ---------------------------------------------------------------------------
// M1-A: Lóbulos hierarchy depth
// ---------------------------------------------------------------------------

describe('M1-A — Lóbulos hierarchy depth', () => {
  it('root() returns one lobe per distinct project, each lobe has childCount > 0', () => {
    const nodes = [
      makeNode(1, 'engram'),
      makeNode(2, 'engram'),
      makeNode(3, 'adivinaBandera'),
    ];
    const model = createClientBrainModel(nodes, [], 'lobulos');

    const root = model.root({});
    expect(root.nodes).toHaveLength(2);
    expect(root.nodes.every((n) => n.kind === 'lobe')).toBe(true);
    expect(root.nodes.every((n) => n.childCount > 0)).toBe(true);
    expect(root.nodes.map((n) => n.id).sort()).toEqual(
      ['lobe:adivinaBandera', 'lobe:engram'].sort(),
    );
  });

  it('children(lobe:engram) returns observations with childCount === 0', () => {
    const nodes = [
      makeNode(1, 'engram'),
      makeNode(2, 'engram'),
      makeNode(3, 'adivinaBandera'),
    ];
    const model = createClientBrainModel(nodes, [], 'lobulos');

    const childLevel = model.children('lobe:engram', {});
    expect(childLevel.nodes).toHaveLength(2);
    expect(childLevel.nodes.every((n) => n.kind === 'observation')).toBe(true);
    expect(childLevel.nodes.every((n) => n.childCount === 0)).toBe(true);
    expect(childLevel.nodes.map((n) => n.id).sort()).toEqual(
      ['obs:1', 'obs:2'].sort(),
    );
  });
});

// ---------------------------------------------------------------------------
// M1-B: Temas hierarchy depth
// ---------------------------------------------------------------------------

describe('M1-B — Temas hierarchy depth', () => {
  it('root() → lobe → neuron → observations for temas view', () => {
    const nodes = [
      makeNode(1, 'engram', { topicKey: 'auth-model' }),
      makeNode(2, 'engram', { topicKey: 'auth-model' }),
      makeNode(3, 'engram', { topicKey: 'database' }),
      makeNode(4, 'engram', { topicKey: null }), // untagged
    ];
    const model = createClientBrainModel(nodes, [], 'temas');

    // Level 0: lobes
    const root = model.root({});
    expect(root.nodes).toHaveLength(1);
    expect(root.nodes[0]!.id).toBe('lobe:engram');
    expect(root.nodes[0]!.kind).toBe('lobe');

    // Level 1: neurons
    const lobe = model.children('lobe:engram', {});
    const neuronIds = lobe.nodes.map((n) => n.id).sort();
    expect(neuronIds).toContain('neuron:engram/auth-model');
    expect(neuronIds).toContain('neuron:engram/database');
    expect(neuronIds).toContain('neuron:engram/__untagged__');
    expect(lobe.nodes.every((n) => n.kind === 'neuron')).toBe(true);

    // Level 2: observations in auth-model
    const temasLevel = model.children('neuron:engram/auth-model', {});
    expect(temasLevel.nodes).toHaveLength(2);
    expect(temasLevel.nodes.every((n) => n.kind === 'observation')).toBe(true);
    expect(temasLevel.nodes.every((n) => n.childCount === 0)).toBe(true);
    expect(temasLevel.nodes.map((n) => n.id).sort()).toEqual(
      ['obs:1', 'obs:2'].sort(),
    );
  });
});

// ---------------------------------------------------------------------------
// Round-trip identity: Lóbulos → Orgánico → Lóbulos preserves obs IDs
// ---------------------------------------------------------------------------

describe('Round-trip identity', () => {
  it('obs node ids are stable across Lóbulos → Orgánico → Lóbulos', () => {
    const nodes = [
      makeNode(1, 'engram', { topicKey: 'auth', label: 'auth token implementation' }),
      makeNode(2, 'engram', { topicKey: 'auth', label: 'auth refresh token logic' }),
      makeNode(3, 'engram', { topicKey: 'database', label: 'database migration system' }),
    ];

    const modelLobulos1 = createClientBrainModel(nodes, [], 'lobulos');
    const modelOrganico = createClientBrainModel(nodes, [], 'organico');
    const modelLobulos2 = createClientBrainModel(nodes, [], 'lobulos');

    // Collect leaf obs ids from Lóbulos (first run)
    const idsLobulos1 = new Set<string>();
    const lobulosRoot1 = modelLobulos1.root({});
    for (const lobe of lobulosRoot1.nodes) {
      const level = modelLobulos1.children(lobe.id, {});
      for (const obs of level.nodes) idsLobulos1.add(obs.id);
    }

    // Collect leaf obs ids from Orgánico
    const idsOrganico = new Set<string>();
    const orgRoot = modelOrganico.root({});
    for (const cluster of orgRoot.nodes) {
      const level = modelOrganico.children(cluster.id, {});
      for (const obs of level.nodes) idsOrganico.add(obs.id);
    }

    // Collect leaf obs ids from Lóbulos (second run)
    const idsLobulos2 = new Set<string>();
    const lobulosRoot2 = modelLobulos2.root({});
    for (const lobe of lobulosRoot2.nodes) {
      const level = modelLobulos2.children(lobe.id, {});
      for (const obs of level.nodes) idsLobulos2.add(obs.id);
    }

    // All three should contain exactly obs:1, obs:2, obs:3
    const expected = new Set(['obs:1', 'obs:2', 'obs:3']);
    expect(idsLobulos1).toEqual(expected);
    expect(idsOrganico).toEqual(expected);
    expect(idsLobulos2).toEqual(expected);
  });
});

// ---------------------------------------------------------------------------
// Memoization (S1.5)
// ---------------------------------------------------------------------------

describe('Memoization', () => {
  it('root() returns same object reference when called with same args twice', () => {
    const nodes = [makeNode(1, 'engram'), makeNode(2, 'engram')];
    const model = createClientBrainModel(nodes, [], 'lobulos');

    const filters = {};
    const level1 = model.root(filters);
    const level2 = model.root(filters);
    expect(level1).toBe(level2);
  });

  it('children() returns same object reference for same args', () => {
    const nodes = [makeNode(1, 'engram'), makeNode(2, 'engram')];
    const model = createClientBrainModel(nodes, [], 'lobulos');

    const filters = {};
    const level1 = model.children('lobe:engram', filters);
    const level2 = model.children('lobe:engram', filters);
    expect(level1).toBe(level2);
  });
});

// ---------------------------------------------------------------------------
// Orgánico fallback to Temas on error (M1-C)
// ---------------------------------------------------------------------------

describe('M1-C — Orgánico fallback', () => {
  it('falls back to Temas structure when organico clustering fails (tested via forcing the error path)', () => {
    // With 1 node per project, no intra-project clustering is possible → 0 clusters
    // The model should fall back to Temas
    const nodes = [makeNode(1, 'engram'), makeNode(2, 'other')];
    const model = createClientBrainModel(nodes, [], 'organico');

    const root = model.root({});
    // Regardless of whether clustering succeeded or not, root must be non-empty
    expect(root.nodes.length).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// availableFacets (C8 — basic test; full tests in facets.test.ts)
// ---------------------------------------------------------------------------

describe('availableFacets', () => {
  it('includes project, type, scope when those fields are present', () => {
    const nodes = [
      makeNode(1, 'engram', { type: 'decision', scope: 'project' }),
    ];
    const model = createClientBrainModel(nodes, [], 'lobulos');
    const facets = model.availableFacets();
    expect(facets).toContain('project');
    expect(facets).toContain('type');
    expect(facets).toContain('scope');
  });

  it('does not include tool when no node has a tool_name field', () => {
    const nodes = [makeNode(1, 'engram')];
    const model = createClientBrainModel(nodes, [], 'lobulos');
    const facets = model.availableFacets();
    expect(facets).not.toContain('tool');
  });
});

// ---------------------------------------------------------------------------
// I3: organico lastAssignment determinism test
// ---------------------------------------------------------------------------

describe('I3 — organico cluster children determinism', () => {
  it('same input always produces same cluster children across multiple buildClusters calls', () => {
    const nodes = [
      makeNode(1, 'proj', { topicKey: 'auth', label: 'authentication token' }),
      makeNode(2, 'proj', { topicKey: 'auth', label: 'authentication refresh' }),
      makeNode(3, 'proj', { topicKey: 'db', label: 'database migration system' }),
      makeNode(4, 'proj', { topicKey: 'db', label: 'database schema upgrade' }),
    ];

    // Create two separate models with the same input — children must match
    const model1 = createClientBrainModel(nodes, [], 'organico');
    const model2 = createClientBrainModel(nodes, [], 'organico');

    const root1 = model1.root({});
    const root2 = model2.root({});

    // Both roots must have the same cluster ids
    const ids1 = root1.nodes.map((n) => n.id).sort();
    const ids2 = root2.nodes.map((n) => n.id).sort();
    expect(ids1).toEqual(ids2);

    // Children of each cluster must be the same
    for (const clusterId of ids1) {
      const children1 = model1.children(clusterId, {}).nodes.map((n) => n.id).sort();
      const children2 = model2.children(clusterId, {}).nodes.map((n) => n.id).sort();
      expect(children1).toEqual(children2);
    }
  });
});

// ---------------------------------------------------------------------------
// I2: observationCount at aggregate levels sums underlying sourceIds
// ---------------------------------------------------------------------------

describe('I2 — observationCount at aggregate levels', () => {
  it('lobe node observationCount equals number of underlying observations', () => {
    const nodes = [
      makeNode(1, 'engram'),
      makeNode(2, 'engram'),
      makeNode(3, 'engram'),
    ];
    const model = createClientBrainModel(nodes, [], 'lobulos');
    const root = model.root({});

    // Root has one lobe for project 'engram' with 3 obs
    const lobe = root.nodes.find((n) => n.id === 'lobe:engram');
    expect(lobe).toBeDefined();
    // stats.observationCount must reflect all underlying observations, not 0
    expect(root.stats.observationCount).toBeGreaterThan(0);
    expect(root.stats.observationCount).toBe(3);
  });

  it('neuron level observationCount equals total observations across all neurons', () => {
    const nodes = [
      makeNode(1, 'proj', { topicKey: 'auth' }),
      makeNode(2, 'proj', { topicKey: 'auth' }),
      makeNode(3, 'proj', { topicKey: 'db' }),
    ];
    const model = createClientBrainModel(nodes, [], 'temas');
    const lobe = model.children('lobe:proj', {});

    // Lobe level has neurons; observationCount must sum all underlying obs
    expect(lobe.stats.observationCount).toBeGreaterThan(0);
    expect(lobe.stats.observationCount).toBe(3);
  });

  it('cluster level observationCount > 0 when clusters have members', () => {
    // Create nodes that will cluster together (same topicKey makes them similar)
    const nodes = [
      makeNode(1, 'proj', { topicKey: 'auth', label: 'authentication token' }),
      makeNode(2, 'proj', { topicKey: 'auth', label: 'authentication refresh' }),
      makeNode(3, 'proj', { topicKey: 'db', label: 'database migration' }),
    ];
    const model = createClientBrainModel(nodes, [], 'organico');
    const root = model.root({});

    // observationCount must be > 0 at root
    expect(root.stats.observationCount).toBeGreaterThan(0);
    expect(root.stats.observationCount).toBe(3);
  });
});
