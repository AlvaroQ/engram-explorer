/**
 * facets.test.ts
 *
 * Strict TDD tests for facets.ts. Run in RED first, then GREEN after
 * implementation. Covers:
 *   AND-across-groups / OR-within-group predicate composition
 *   C8-A: tool absent → hidden from availableFacets
 *   C8-B: tool present → visible in availableFacets
 *   search query filter (label includes, AND-composed with facets)
 */

import { describe, it, expect } from 'vitest';
import type { GraphNode } from '../../../lib/api.ts';
import {
  buildFacetIndex,
  applyFacets,
  availableFacets,
} from './facets.ts';
import type { BrainNode } from './brain-model.ts';
import type { ActiveFacets } from './types.ts';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeGraphNode(
  id: number,
  project: string,
  opts: Partial<GraphNode & { tool_name?: string | null }> = {},
): GraphNode & { tool_name?: string | null } {
  return {
    id,
    project,
    type: 'decision',
    scope: 'project',
    topicKey: null,
    label: `label-${id}`,
    weight: 1,
    duplicateCount: 1,
    createdAt: null,
    ...opts,
  };
}

function makeBrainNode(id: string, project: string, type = 'decision', scope = 'project'): BrainNode {
  return {
    id,
    kind: 'observation',
    label: `label-${id}`,
    color: '#aaa',
    weight: 1,
    childCount: 0,
    meta: { project, type, scope },
    sourceIds: [],
  };
}

// ---------------------------------------------------------------------------
// AND-across-groups / OR-within-group
// ---------------------------------------------------------------------------

describe('applyFacets — AND across groups / OR within group', () => {
  const nodes: BrainNode[] = [
    { ...makeBrainNode('obs:1', 'engram'), meta: { project: 'engram', type: 'decision', scope: 'project' } },
    { ...makeBrainNode('obs:2', 'engram'), meta: { project: 'engram', type: 'bugfix', scope: 'project' } },
    { ...makeBrainNode('obs:3', 'other'), meta: { project: 'other', type: 'decision', scope: 'personal' } },
    { ...makeBrainNode('obs:4', 'other'), meta: { project: 'other', type: 'bugfix', scope: 'personal' } },
  ];

  it('empty facets → all nodes pass', () => {
    const result = applyFacets(nodes, {});
    expect(result).toHaveLength(4);
  });

  it('single group filter: project=engram → only engram nodes', () => {
    const active: ActiveFacets = { project: ['engram'] };
    const result = applyFacets(nodes, active);
    expect(result.map((n) => n.id).sort()).toEqual(['obs:1', 'obs:2'].sort());
  });

  it('OR within group: project=[engram,other] → all nodes', () => {
    const active: ActiveFacets = { project: ['engram', 'other'] };
    const result = applyFacets(nodes, active);
    expect(result).toHaveLength(4);
  });

  it('AND across groups: project=engram AND type=decision → only obs:1', () => {
    const active: ActiveFacets = { project: ['engram'], type: ['decision'] };
    const result = applyFacets(nodes, active);
    expect(result.map((n) => n.id)).toEqual(['obs:1']);
  });

  it('AND across groups: project=other AND type=bugfix → only obs:4', () => {
    const active: ActiveFacets = { project: ['other'], type: ['bugfix'] };
    const result = applyFacets(nodes, active);
    expect(result.map((n) => n.id)).toEqual(['obs:4']);
  });

  it('no match: project=engram AND type=bugfix AND scope=personal → empty', () => {
    const active: ActiveFacets = { project: ['engram'], type: ['bugfix'], scope: ['personal'] };
    const result = applyFacets(nodes, active);
    expect(result).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Search query (label includes) AND-composed with facets
// ---------------------------------------------------------------------------

describe('applyFacets — search query', () => {
  const nodes: BrainNode[] = [
    { ...makeBrainNode('obs:1', 'engram'), label: 'auth token implementation', meta: { project: 'engram', type: 'decision', scope: 'project' } },
    { ...makeBrainNode('obs:2', 'engram'), label: 'database migration system', meta: { project: 'engram', type: 'decision', scope: 'project' } },
    { ...makeBrainNode('obs:3', 'other'), label: 'auth refresh logic', meta: { project: 'other', type: 'bugfix', scope: 'personal' } },
  ];

  it('query=auth → matches obs:1 and obs:3', () => {
    const result = applyFacets(nodes, {}, 'auth');
    expect(result.map((n) => n.id).sort()).toEqual(['obs:1', 'obs:3'].sort());
  });

  it('query=database → matches only obs:2', () => {
    const result = applyFacets(nodes, {}, 'database');
    expect(result.map((n) => n.id)).toEqual(['obs:2']);
  });

  it('query=auth AND project=engram → only obs:1', () => {
    const result = applyFacets(nodes, { project: ['engram'] }, 'auth');
    expect(result.map((n) => n.id)).toEqual(['obs:1']);
  });

  it('empty query → all nodes pass the search filter', () => {
    const result = applyFacets(nodes, {}, '');
    expect(result).toHaveLength(3);
  });
});

// ---------------------------------------------------------------------------
// buildFacetIndex
// ---------------------------------------------------------------------------

describe('buildFacetIndex', () => {
  it('returns a map of group → Set of values', () => {
    const nodes: BrainNode[] = [
      { ...makeBrainNode('obs:1', 'engram'), meta: { project: 'engram', type: 'decision', scope: 'project' } },
      { ...makeBrainNode('obs:2', 'other'), meta: { project: 'other', type: 'bugfix', scope: 'personal' } },
    ];

    const index = buildFacetIndex(nodes);
    expect(index.get('project')).toEqual(new Set(['engram', 'other']));
    expect(index.get('type')).toEqual(new Set(['decision', 'bugfix']));
    expect(index.get('scope')).toEqual(new Set(['project', 'personal']));
  });
});

// ---------------------------------------------------------------------------
// C8-A: Tool absent → hidden
// ---------------------------------------------------------------------------

describe('C8-A — tool absent → not in availableFacets', () => {
  it('does not return "tool" when no node has a non-null tool_name', () => {
    const nodes = [
      makeGraphNode(1, 'engram'),
      makeGraphNode(2, 'engram'),
    ];
    const result = availableFacets(nodes);
    expect(result).not.toContain('tool');
  });
});

// ---------------------------------------------------------------------------
// C8-B: Tool present → visible
// ---------------------------------------------------------------------------

describe('C8-B — tool present → in availableFacets', () => {
  it('returns "tool" when at least one node has a non-null tool_name', () => {
    const nodes = [
      makeGraphNode(1, 'engram', { tool_name: 'code-editor' }),
      makeGraphNode(2, 'engram'),
    ];
    const result = availableFacets(nodes);
    expect(result).toContain('tool');
  });
});

// ---------------------------------------------------------------------------
// availableFacets baseline
// ---------------------------------------------------------------------------

describe('availableFacets — baseline groups', () => {
  it('includes project, type, scope when those are non-null in any node', () => {
    const nodes = [
      makeGraphNode(1, 'engram', { type: 'decision', scope: 'project' }),
    ];
    const result = availableFacets(nodes);
    expect(result).toContain('project');
    expect(result).toContain('type');
    expect(result).toContain('scope');
  });

  it('does not include date when no node has createdAt', () => {
    const nodes = [
      makeGraphNode(1, 'engram', { createdAt: null }),
    ];
    const result = availableFacets(nodes);
    expect(result).not.toContain('date');
  });

  it('includes date when at least one node has createdAt', () => {
    const nodes = [
      makeGraphNode(1, 'engram', { createdAt: '2026-01-01' }),
    ];
    const result = availableFacets(nodes);
    expect(result).toContain('date');
  });
});
