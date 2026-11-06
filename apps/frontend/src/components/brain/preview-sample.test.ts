import { describe, expect, it } from 'vitest';
import type { GraphEdge, GraphNode } from '../../lib/api.ts';
import { PREVIEW_LIMIT, samplePreviewGraph } from './preview-sample.ts';

function node(id: number, weight: number): GraphNode {
  return {
    id,
    weight,
    project: null,
    type: null,
    scope: null,
    topicKey: null,
    label: null,
    duplicateCount: 0,
    createdAt: null,
  };
}

describe('samplePreviewGraph', () => {
  it('keeps a small graph as is', () => {
    const nodes = [node(1, 1), node(2, 2)];
    const edges: GraphEdge[] = [{ source: 1, target: 2, relation: 'related' }];
    expect(samplePreviewGraph({ nodes, edges })).toEqual({ nodes, edges });
  });

  it('keeps the heaviest nodes and drops edges that touch a dropped node', () => {
    const nodes = [node(1, 5), node(2, 1), node(3, 9)];
    const edges: GraphEdge[] = [
      { source: 1, target: 3, relation: 'related' },
      { source: 1, target: 2, relation: 'related' },
    ];
    const got = samplePreviewGraph({ nodes, edges }, 2);
    expect(got.nodes.map((n) => n.id)).toEqual([3, 1]);
    expect(got.edges).toEqual([{ source: 1, target: 3, relation: 'related' }]);
  });

  it('caps at PREVIEW_LIMIT by default and handles no data', () => {
    const many = Array.from({ length: PREVIEW_LIMIT + 10 }, (_, i) => node(i, i));
    expect(samplePreviewGraph({ nodes: many, edges: [] }).nodes).toHaveLength(PREVIEW_LIMIT);
    expect(samplePreviewGraph(undefined)).toEqual({ nodes: [], edges: [] });
  });
});
