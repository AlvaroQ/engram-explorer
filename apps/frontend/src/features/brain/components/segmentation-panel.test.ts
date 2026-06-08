/**
 * segmentation-panel.test.ts
 *
 * Strict TDD tests for the pure logic extracted from segmentation-panel.tsx.
 *
 * Unit-testable surface:
 *   - computeMatchedIds: given BrainNode[], ActiveFacets, and search query,
 *     returns the Set<string> of matching node ids (null = all match).
 *   - buildLegendEntries: given BrainNode[] and colorBy, returns label→color map.
 *   - isZeroMatch: predicate for the S9.3 "0 matches" state.
 *
 * These are exported from segmentation-panel-logic.ts (pure side-effect-free).
 *
 * TDD scenario S6.3 / S9.3:
 *   - Selecting a facet dims non-matching nodes (returns Set with only matching ids).
 *   - 0-match state returns empty Set (not null).
 *   - No filter (all empty) returns null (all pass).
 */

import { describe, it, expect } from 'vitest';
import type { ActiveFacets } from '../model/types.ts';
import type { BrainNode } from '../model/brain-model.ts';
import {
  computeMatchedIds,
  buildLegendEntries,
  isZeroMatch,
} from './segmentation-panel-logic.ts';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeBrainNode(
  id: string,
  opts: {
    project?: string;
    type?: string;
    scope?: string;
    label?: string;
    color?: string;
  } = {},
): BrainNode {
  return {
    id,
    kind: 'observation',
    label: opts.label ?? `label-${id}`,
    color: opts.color ?? '#aaa',
    weight: 1,
    childCount: 0,
    meta: {
      project: opts.project ?? 'default',
      type: opts.type ?? 'note',
      scope: opts.scope ?? 'project',
    },
  };
}

// ---------------------------------------------------------------------------
// computeMatchedIds
// ---------------------------------------------------------------------------

describe('computeMatchedIds', () => {
  const nodes = [
    makeBrainNode('obs:1', { project: 'engram', type: 'decision' }),
    makeBrainNode('obs:2', { project: 'engram', type: 'note' }),
    makeBrainNode('obs:3', { project: 'adivinaBandera', type: 'decision' }),
    makeBrainNode('obs:4', { project: 'adivinaBandera', type: 'note' }),
  ];

  it('returns null when no filters and no query (all nodes pass)', () => {
    const result = computeMatchedIds(nodes, {}, '');
    expect(result).toBeNull();
  });

  it('returns null when facets are all empty arrays', () => {
    const result = computeMatchedIds(nodes, { project: [], type: [] }, '');
    expect(result).toBeNull();
  });

  it('filters by single project facet (OR within group)', () => {
    const facets: ActiveFacets = { project: ['engram'] };
    const result = computeMatchedIds(nodes, facets, '');
    expect(result).not.toBeNull();
    expect(result!.has('obs:1')).toBe(true);
    expect(result!.has('obs:2')).toBe(true);
    expect(result!.has('obs:3')).toBe(false);
    expect(result!.has('obs:4')).toBe(false);
  });

  it('filters by two projects (OR within group)', () => {
    const facets: ActiveFacets = { project: ['engram', 'adivinaBandera'] };
    const result = computeMatchedIds(nodes, facets, '');
    // Both projects selected → all nodes match
    expect(result).not.toBeNull();
    expect(result!.size).toBe(4);
  });

  it('AND-across-groups: project AND type must both match', () => {
    const facets: ActiveFacets = { project: ['engram'], type: ['decision'] };
    const result = computeMatchedIds(nodes, facets, '');
    expect(result).not.toBeNull();
    expect(result!.has('obs:1')).toBe(true);   // engram + decision
    expect(result!.has('obs:2')).toBe(false);  // engram but NOT decision
    expect(result!.has('obs:3')).toBe(false);  // NOT engram
    expect(result!.has('obs:4')).toBe(false);  // NOT engram
  });

  it('filters by search query (case-insensitive label includes)', () => {
    const nodes2 = [
      makeBrainNode('obs:10', { label: 'auth model' }),
      makeBrainNode('obs:11', { label: 'jwt token' }),
      makeBrainNode('obs:12', { label: 'AUTH refresh' }),
    ];
    const result = computeMatchedIds(nodes2, {}, 'auth');
    expect(result).not.toBeNull();
    expect(result!.has('obs:10')).toBe(true);
    expect(result!.has('obs:11')).toBe(false);
    expect(result!.has('obs:12')).toBe(true);
  });

  it('query AND facet are AND-composed', () => {
    const facets: ActiveFacets = { project: ['engram'] };
    // obs:1 = engram + decision, label = "label-obs:1"
    // obs:2 = engram + note,     label = "label-obs:2"
    const result = computeMatchedIds(nodes, facets, 'label-obs:1');
    expect(result).not.toBeNull();
    expect(result!.has('obs:1')).toBe(true);
    expect(result!.has('obs:2')).toBe(false); // engram but label doesn't match
  });

  it('returns empty Set (not null) when 0 nodes match (S9.3)', () => {
    const facets: ActiveFacets = { project: ['nonexistent-project'] };
    const result = computeMatchedIds(nodes, facets, '');
    expect(result).not.toBeNull();
    expect(result!.size).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// isZeroMatch
// ---------------------------------------------------------------------------

describe('isZeroMatch', () => {
  it('returns true for empty Set (0 matches with active filter)', () => {
    expect(isZeroMatch(new Set())).toBe(true);
  });

  it('returns false for null (no filter active)', () => {
    expect(isZeroMatch(null)).toBe(false);
  });

  it('returns false for non-empty Set', () => {
    expect(isZeroMatch(new Set(['obs:1']))).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// buildLegendEntries
// ---------------------------------------------------------------------------

describe('buildLegendEntries', () => {
  const nodes = [
    makeBrainNode('obs:1', { color: '#ff0000', project: 'engram' }),
    makeBrainNode('obs:2', { color: '#00ff00', project: 'adivinaBandera' }),
    makeBrainNode('obs:3', { color: '#ff0000', project: 'engram' }),
  ];

  it('returns distinct color entries for nodes with unique colors', () => {
    const entries = buildLegendEntries(nodes, 'color');
    // 2 unique colors (not 3 nodes)
    const colors = entries.map((e) => e.color);
    expect(new Set(colors).size).toBe(2);
  });

  it('each entry has label and color', () => {
    const entries = buildLegendEntries(nodes, 'color');
    for (const e of entries) {
      expect(typeof e.label).toBe('string');
      expect(typeof e.color).toBe('string');
    }
  });

  it('returns empty array for empty node list', () => {
    const entries = buildLegendEntries([], 'color');
    expect(entries).toHaveLength(0);
  });
});
