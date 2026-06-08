import { describe, it, expect } from 'vitest';
import {
  tokenize,
  termFrequencies,
  topKeywords,
  computeNodeKeywords,
  NON_TOPICAL_TYPES,
  STOPWORDS,
  MIN_TOKEN_LEN,
} from './tfidf.ts';
import type { GraphNode } from './api.ts';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeNode(
  id: number,
  opts: Partial<GraphNode> = {},
): GraphNode {
  return {
    id,
    project: 'test-project',
    type: 'decision',
    scope: 'project',
    topicKey: null,
    label: null,
    weight: 1,
    duplicateCount: 0,
    createdAt: null,
    ...opts,
  };
}

// ---------------------------------------------------------------------------
// tokenize
// ---------------------------------------------------------------------------

describe('tokenize', () => {
  it('returns empty array for null/undefined/empty', () => {
    expect(tokenize(null)).toEqual([]);
    expect(tokenize(undefined)).toEqual([]);
    expect(tokenize('')).toEqual([]);
  });

  it('splits on non-alphanumeric delimiters', () => {
    const result = tokenize('hello/world_foo-bar baz');
    expect(result).toContain('hello');
    expect(result).toContain('world');
    expect(result).toContain('foo');
    expect(result).toContain('bar');
    expect(result).toContain('baz');
  });

  it('drops tokens shorter than MIN_TOKEN_LEN', () => {
    // MIN_TOKEN_LEN = 3; 'ab' should be dropped, 'abc' kept
    const result = tokenize('ab abc xy xyz');
    expect(result).not.toContain('ab');
    expect(result).not.toContain('xy');
    expect(result).toContain('abc');
    expect(result).toContain('xyz');
  });

  it('drops English stopwords', () => {
    const result = tokenize('the quick fox and the lazy dog using fix');
    expect(result).not.toContain('the');
    expect(result).not.toContain('and');
    expect(result).not.toContain('using');
    expect(result).not.toContain('fix');
    expect(result).toContain('quick');
    expect(result).toContain('fox');
    expect(result).toContain('lazy');
    expect(result).toContain('dog');
  });

  it('drops Spanish stopwords', () => {
    const result = tokenize('los gatos con las cabras del rey que como');
    expect(result).not.toContain('los');
    expect(result).not.toContain('con');
    expect(result).not.toContain('las');
    expect(result).not.toContain('del');
    expect(result).not.toContain('que');
    expect(result).not.toContain('como');
    expect(result).toContain('gatos');
    expect(result).toContain('cabras');
    expect(result).toContain('rey');
  });

  it('preserves Spanish accented characters', () => {
    const result = tokenize('configuración índice árbol gestión');
    expect(result).toContain('configuración');
    expect(result).toContain('índice');
    expect(result).toContain('árbol');
    expect(result).toContain('gestión');
  });

  it('lowercases tokens', () => {
    const result = tokenize('GraphNode Architecture BACKEND');
    expect(result).toContain('graphnode');
    expect(result).toContain('architecture');
    expect(result).toContain('backend');
  });

  it('STOPWORDS set is consistent with MIN_TOKEN_LEN (all stopwords pass the length check)', () => {
    // Every stopword must be at least MIN_TOKEN_LEN chars; shorter ones would
    // be dropped by the length check before the stopword check matters.
    for (const sw of STOPWORDS) {
      expect(sw.length).toBeGreaterThanOrEqual(MIN_TOKEN_LEN);
    }
  });
});

// ---------------------------------------------------------------------------
// termFrequencies
// ---------------------------------------------------------------------------

describe('termFrequencies', () => {
  it('counts occurrences correctly', () => {
    const freq = termFrequencies('alpha beta alpha gamma alpha beta');
    expect(freq.get('alpha')).toBe(3);
    expect(freq.get('beta')).toBe(2);
    expect(freq.get('gamma')).toBe(1);
  });

  it('returns empty map for null/empty input', () => {
    expect(termFrequencies(null).size).toBe(0);
    expect(termFrequencies('').size).toBe(0);
  });

  it('respects stopword filtering (counts only valid tokens)', () => {
    const freq = termFrequencies('the quick fox and the fox');
    expect(freq.has('the')).toBe(false);
    expect(freq.has('and')).toBe(false);
    expect(freq.get('quick')).toBe(1);
    expect(freq.get('fox')).toBe(2);
  });
});

// ---------------------------------------------------------------------------
// topKeywords
// ---------------------------------------------------------------------------

describe('topKeywords', () => {
  it('returns empty array for empty map', () => {
    expect(topKeywords(new Map(), 5)).toEqual([]);
  });

  it('returns empty array for k=0', () => {
    const vec = new Map([['alpha', 3], ['beta', 2]]);
    expect(topKeywords(vec, 0)).toEqual([]);
  });

  it('ranks tokens by weight descending', () => {
    const vec = new Map([
      ['alpha', 1],
      ['beta', 5],
      ['gamma', 3],
    ]);
    const result = topKeywords(vec, 3);
    expect(result[0]).toBe('beta');
    expect(result[1]).toBe('gamma');
    expect(result[2]).toBe('alpha');
  });

  it('respects k limit', () => {
    const vec = new Map([
      ['alpha', 5],
      ['beta', 4],
      ['gamma', 3],
      ['delta', 2],
    ]);
    expect(topKeywords(vec, 2)).toHaveLength(2);
    expect(topKeywords(vec, 2)[0]).toBe('alpha');
  });

  it('breaks weight ties alphabetically (determinism)', () => {
    const vec = new Map([
      ['zebra', 10],
      ['apple', 10],
      ['mango', 10],
    ]);
    const result = topKeywords(vec, 3);
    expect(result).toEqual(['apple', 'mango', 'zebra']);
  });

  it('respects minRatio floor — excludes tokens below the threshold', () => {
    const vec = new Map([
      ['dominant', 100],
      ['minor', 5],  // 5/100 = 0.05 < minRatio 0.15
      ['medium', 20], // 20/100 = 0.20 >= minRatio 0.15
    ]);
    const result = topKeywords(vec, 10, 0.15);
    expect(result).toContain('dominant');
    expect(result).toContain('medium');
    expect(result).not.toContain('minor');
  });

  it('excludes tokens below minRatio floor relative to max weight', () => {
    const vec = new Map([['only', 10], ['tiny', 1]]);
    const result = topKeywords(vec, 5, 0.5);
    expect(result).toContain('only');
    expect(result).not.toContain('tiny'); // 1/10 = 0.1 < minRatio 0.5
  });
});

// ---------------------------------------------------------------------------
// computeNodeKeywords
// ---------------------------------------------------------------------------

describe('computeNodeKeywords', () => {
  it('returns empty map for 0 nodes', () => {
    expect(computeNodeKeywords([])).toEqual(new Map());
  });

  it('skips projects with N < 2 (IDF undefined for singleton)', () => {
    const nodes = [makeNode(1, { topicKey: 'arch/singleton', label: 'Singleton node' })];
    const result = computeNodeKeywords(nodes);
    // The single node belongs to a project with N=1 — must be skipped
    expect(result.has(1)).toBe(false);
  });

  it('skips NON_TOPICAL_TYPES nodes', () => {
    const type = [...NON_TOPICAL_TYPES][0]!; // 'session_summary'
    const nodes = [
      makeNode(1, { type, topicKey: 'session/one', label: 'Session one' }),
      makeNode(2, { type, topicKey: 'session/two', label: 'Session two' }),
      makeNode(3, { type, topicKey: 'session/three', label: 'Session three' }),
    ];
    // All nodes are non-topical → none should appear in result
    const result = computeNodeKeywords(nodes);
    expect(result.has(1)).toBe(false);
    expect(result.has(2)).toBe(false);
    expect(result.has(3)).toBe(false);
  });

  it('returns non-empty keywords for a normal multi-node project', () => {
    const nodes = [
      makeNode(10, {
        project: 'my-project',
        topicKey: 'architecture/auth-model',
        label: 'Auth model design',
      }),
      makeNode(11, {
        project: 'my-project',
        topicKey: 'architecture/database-schema',
        label: 'Database schema design',
      }),
      makeNode(12, {
        project: 'my-project',
        topicKey: 'bugfix/auth-token-refresh',
        label: 'Fix auth token refresh',
      }),
    ];
    const result = computeNodeKeywords(nodes, 5);
    // All nodes are in the same project with N=3 ≥ 2 → should get keywords
    expect(result.has(10)).toBe(true);
    expect(result.has(11)).toBe(true);
    expect(result.has(12)).toBe(true);
    // Each node should have at least 1 keyword
    expect((result.get(10) ?? []).length).toBeGreaterThanOrEqual(1);
    expect((result.get(11) ?? []).length).toBeGreaterThanOrEqual(1);
    expect((result.get(12) ?? []).length).toBeGreaterThanOrEqual(1);
    // Distinctive tokens should appear: 'auth' is shared (IDF < max) but
    // 'database' and 'schema' are distinctive for node 11
    const kw11 = result.get(11) ?? [];
    const kw10 = result.get(10) ?? [];
    // Node 10 (auth-model) should have 'model' or 'auth'
    // Node 11 (database-schema) should have 'database' or 'schema'
    const hasDistinctive10 = kw10.some((k) => ['model', 'auth'].includes(k));
    const hasDistinctive11 = kw11.some((k) => ['database', 'schema'].includes(k));
    expect(hasDistinctive10).toBe(true);
    expect(hasDistinctive11).toBe(true);
  });

  it('keeps nodes from different projects separate (intra-project IDF)', () => {
    // Two projects; node ids must not interfere with each other
    const nodes = [
      makeNode(20, { project: 'proj-a', topicKey: 'feature/alpha', label: 'Alpha feature' }),
      makeNode(21, { project: 'proj-a', topicKey: 'feature/beta', label: 'Beta feature' }),
      makeNode(30, { project: 'proj-b', topicKey: 'release/v1', label: 'Version one release' }),
      makeNode(31, { project: 'proj-b', topicKey: 'release/v2', label: 'Version two release' }),
    ];
    const result = computeNodeKeywords(nodes, 5);
    // Both projects have N=2 ≥ 2 → all nodes get keywords
    expect(result.has(20)).toBe(true);
    expect(result.has(21)).toBe(true);
    expect(result.has(30)).toBe(true);
    expect(result.has(31)).toBe(true);
  });

  it('keys results by numeric GraphNode.id', () => {
    const nodes = [
      makeNode(99, { project: 'p', topicKey: 'foo/bar', label: 'Foo bar baz' }),
      makeNode(100, { project: 'p', topicKey: 'foo/qux', label: 'Foo qux quux' }),
    ];
    const result = computeNodeKeywords(nodes);
    // Keys are numbers
    for (const key of result.keys()) {
      expect(typeof key).toBe('number');
    }
  });
});
