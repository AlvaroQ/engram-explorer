import { describe, expect, it } from 'vitest';
import {
  barnesHutRepulsion,
  computeLayout,
  layoutIterations,
  MAX_LAYOUT_NODES,
  TARGET_HALF,
  type LayoutEdge,
  type LayoutNode,
} from './force-layout.ts';

// Synthetic graph: `clusters` projects, each with a few topic sub-clusters and a
// sparse ring of edges inside every project. Fully deterministic.
function makeGraph(n: number, clusters: number): { nodes: LayoutNode[]; edges: LayoutEdge[] } {
  const nodes: LayoutNode[] = [];
  const edges: LayoutEdge[] = [];
  for (let i = 0; i < n; i++) {
    nodes.push({ id: i, cluster: (i % clusters) + 1, subCluster: (i % (clusters * 3)) + 1 });
    if (i >= clusters) edges.push({ source: i, target: i - clusters });
  }
  return { nodes, edges };
}

function clusterSeparation(pos: Float32Array, nodes: LayoutNode[]): number {
  // Ratio of mean distance-to-own-centroid vs mean distance between centroids.
  const sums = new Map<number, [number, number, number, number]>();
  nodes.forEach((nd, i) => {
    const s = sums.get(nd.cluster) ?? [0, 0, 0, 0];
    s[0] += pos[i * 3] ?? 0;
    s[1] += pos[i * 3 + 1] ?? 0;
    s[2] += pos[i * 3 + 2] ?? 0;
    s[3] += 1;
    sums.set(nd.cluster, s);
  });
  const cent = new Map<number, [number, number, number]>();
  for (const [k, s] of sums) cent.set(k, [s[0] / s[3], s[1] / s[3], s[2] / s[3]]);
  let intra = 0;
  nodes.forEach((nd, i) => {
    const c = cent.get(nd.cluster) ?? [0, 0, 0];
    intra += Math.hypot(
      (pos[i * 3] ?? 0) - c[0],
      (pos[i * 3 + 1] ?? 0) - c[1],
      (pos[i * 3 + 2] ?? 0) - c[2],
    );
  });
  intra /= nodes.length;
  const cs = [...cent.values()];
  let inter = 0;
  let pairs = 0;
  for (let a = 0; a < cs.length; a++) {
    for (let b = a + 1; b < cs.length; b++) {
      const p = cs[a] ?? [0, 0, 0];
      const q = cs[b] ?? [0, 0, 0];
      inter += Math.hypot(p[0] - q[0], p[1] - q[1], p[2] - q[2]);
      pairs++;
    }
  }
  return intra / (inter / pairs);
}

function expectFiniteAndBounded(pos: Float32Array, n: number): void {
  expect(pos.length).toBe(n * 3);
  for (const v of pos) {
    expect(Number.isFinite(v)).toBe(true);
    expect(Math.abs(v)).toBeLessThanOrEqual(TARGET_HALF + 1e-3);
  }
}

describe('computeLayout', () => {
  it('is deterministic for the same input', () => {
    const { nodes, edges } = makeGraph(120, 4);
    const a = computeLayout(nodes, edges);
    const b = computeLayout(nodes, edges);
    expect(Array.from(a)).toEqual(Array.from(b));
  });

  it('returns finite positions normalized into the target cube', () => {
    const { nodes, edges } = makeGraph(200, 5);
    expectFiniteAndBounded(computeLayout(nodes, edges), 200);
  });

  it('keeps project clusters visibly separated (small graph, exact forces)', () => {
    const { nodes, edges } = makeGraph(150, 3);
    expect(clusterSeparation(computeLayout(nodes, edges), nodes)).toBeLessThan(0.75);
  });

  it('keeps project clusters visibly separated (large graph, approximated forces)', () => {
    const { nodes, edges } = makeGraph(1500, 6);
    const pos = computeLayout(nodes, edges);
    expectFiniteAndBounded(pos, 1500);
    expect(clusterSeparation(pos, nodes)).toBeLessThan(0.75);
  });

  it('handles empty and single-node graphs', () => {
    expect(computeLayout([], []).length).toBe(0);
    const one = computeLayout([{ id: 0, cluster: 1, subCluster: 1 }], []);
    expect(Array.from(one).every(Number.isFinite)).toBe(true);
  });

  it('lays out the server cap of 5000 nodes within a sane time budget', () => {
    const { nodes, edges } = makeGraph(5000, 8);
    const t0 = performance.now();
    const pos = computeLayout(nodes, edges);
    const ms = performance.now() - t0;
    expectFiniteAndBounded(pos, 5000);
    // The old exact O(n²) × 200 iterations took ~8.7 s here on a fast dev
    // machine; Barnes–Hut + the scaled budget take well under a second.
    expect(ms).toBeLessThan(3000);
  });

  it('still returns a full, finite buffer above the simulation cap', () => {
    const n = MAX_LAYOUT_NODES + 50;
    const { nodes, edges } = makeGraph(n, 8);
    expectFiniteAndBounded(computeLayout(nodes, edges), n);
  });
});

describe('layoutIterations', () => {
  it('keeps the full 200 iterations for typical graphs', () => {
    expect(layoutIterations(50)).toBe(200);
    expect(layoutIterations(1000)).toBe(200);
  });

  it('scales the budget down for large graphs but never below the floor', () => {
    expect(layoutIterations(5000)).toBeLessThan(200);
    expect(layoutIterations(5000)).toBeGreaterThanOrEqual(80);
    expect(layoutIterations(2000)).toBeGreaterThan(layoutIterations(5000));
  });

  it('honours an explicit lower request', () => {
    expect(layoutIterations(100, 30)).toBe(30);
  });
});

// Three deterministic blobs of 100 nodes (a 3-project graph after a few
// iterations): dense clusters far from each other, the case Barnes–Hut
// approximates most.
function threeClusterPositions(perCluster: number): Float32Array {
  const centres = [
    [-25, 0, 0],
    [20, 18, -6],
    [8, -22, 14],
  ] as const;
  const pos = new Float32Array(centres.length * perCluster * 3);
  let seed = 7;
  const rand = (): number => {
    seed = (seed * 1103515245 + 12345) % 2147483648;
    return seed / 2147483648 - 0.5;
  };
  centres.forEach((c, k) => {
    for (let j = 0; j < perCluster; j++) {
      const i = (k * perCluster + j) * 3;
      pos[i] = c[0] + rand() * 12;
      pos[i + 1] = c[1] + rand() * 12;
      pos[i + 2] = c[2] + rand() * 12;
    }
  });
  return pos;
}

// Exact O(n²) reference of one repulsion pass, written independently from the
// implementation: capped inverse-square force min(800/d², 100) along the unit
// vector, with the same 0.01 softening.
function exactRepulsionReference(pos: Float32Array, n: number): Float64Array {
  const out = new Float64Array(n * 3);
  for (let i = 0; i < n; i++) {
    for (let j = 0; j < n; j++) {
      if (i === j) continue;
      const dx = (pos[i * 3] ?? 0) - (pos[j * 3] ?? 0);
      const dy = (pos[i * 3 + 1] ?? 0) - (pos[j * 3 + 1] ?? 0);
      const dz = (pos[i * 3 + 2] ?? 0) - (pos[j * 3 + 2] ?? 0);
      const d2 = dx * dx + dy * dy + dz * dz + 0.01;
      const k = Math.min(800 / d2, 100) / Math.sqrt(d2);
      out[i * 3] = (out[i * 3] ?? 0) + dx * k;
      out[i * 3 + 1] = (out[i * 3 + 1] ?? 0) + dy * k;
      out[i * 3 + 2] = (out[i * 3 + 2] ?? 0) + dz * k;
    }
  }
  return out;
}

describe('barnesHutRepulsion', () => {
  it('matches the exact O(n²) repulsion on a 300-node 3-cluster graph (median error < 5%)', () => {
    const n = 300;
    const pos = threeClusterPositions(n / 3);
    const approx = barnesHutRepulsion(pos, n);
    const exact = exactRepulsionReference(pos, n);
    const errors: number[] = [];
    for (let i = 0; i < n; i++) {
      const ex = exact[i * 3] ?? 0,
        ey = exact[i * 3 + 1] ?? 0,
        ez = exact[i * 3 + 2] ?? 0;
      const ax = approx[i * 3] ?? 0,
        ay = approx[i * 3 + 1] ?? 0,
        az = approx[i * 3 + 2] ?? 0;
      errors.push(Math.hypot(ax - ex, ay - ey, az - ez) / Math.hypot(ex, ey, ez));
    }
    errors.sort((a, b) => a - b);
    const median = errors[Math.floor(n / 2)] ?? Infinity;
    expect(median).toBeLessThan(0.05);
    // No node may be pushed by its own mass (a cell holding the node itself
    // must be opened, never approximated): its error stays bounded too.
    expect(errors[n - 1] ?? Infinity).toBeLessThan(0.5);
  });

  it('opens a cell that contains the node itself instead of approximating it', () => {
    // The root cell holds a lone node in one corner and a tight group of
    // three in the opposite corner: seen from the lone node the cell passes
    // the opening test, but its centre of mass includes the node itself.
    const pos = new Float32Array([0, 0, 0, 10, 10, 10, 10, 10, 10.001, 10, 10.001, 10]);
    const approx = barnesHutRepulsion(pos, 4);
    const exact = exactRepulsionReference(pos, 4);
    const err = Math.hypot(
      (approx[0] ?? 0) - (exact[0] ?? 0),
      (approx[1] ?? 0) - (exact[1] ?? 0),
      (approx[2] ?? 0) - (exact[2] ?? 0),
    );
    expect(err / Math.hypot(exact[0] ?? 0, exact[1] ?? 0, exact[2] ?? 0)).toBeLessThan(0.01);
  });
});
