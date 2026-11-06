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

// Owner-shaped synthetic graph: 5000 nodes in ~50 projects with a skewed
// size distribution (39% / 10% / 6% / … down to a long tail of small
// projects), projects interleaved in API (recency) order, topic sub-clusters
// that change every few nodes, and 900 edges (80% inside a project).
function makeOwnerShapedGraph(): { nodes: LayoutNode[]; edges: LayoutEdge[] } {
  const n = 5000;
  const sizes = [1950, 520, 280, 230, 190, 160, 140, 120, 100, 90];
  let rest = n - sizes.reduce((a, b) => a + b, 0);
  for (let s = 80; rest > 0; s *= 0.93) {
    const take = Math.min(rest, Math.max(5, Math.round(s)));
    sizes.push(take);
    rest -= take;
  }
  let seed = 12345;
  const rand = (): number => {
    seed = (seed * 1103515245 + 12345) % 2147483648;
    return seed / 2147483648;
  };
  const remaining = sizes.slice();
  const topicOf = sizes.map(() => 0);
  let topic = 1;
  const nodes: LayoutNode[] = [];
  while (nodes.length < n) {
    let pick = rand() * remaining.reduce((a, b) => a + b, 0);
    let k = 0;
    while (pick >= (remaining[k] ?? 0)) pick -= remaining[k++] ?? 0;
    remaining[k] = (remaining[k] ?? 0) - 1;
    if (topicOf[k] === 0 || rand() < 0.15) topicOf[k] = topic++;
    nodes.push({ id: nodes.length, cluster: k + 1, subCluster: topicOf[k] ?? 0 });
  }
  const byCluster = new Map<number, number[]>();
  nodes.forEach((nd, i) => byCluster.set(nd.cluster, [...(byCluster.get(nd.cluster) ?? []), i]));
  const edges: LayoutEdge[] = [];
  while (edges.length < 900) {
    const a = Math.floor(rand() * n);
    const pool = rand() < 0.8 ? byCluster.get(nodes[a]?.cluster ?? 0) : undefined;
    const b = pool ? (pool[Math.floor(rand() * pool.length)] ?? 0) : Math.floor(rand() * n);
    if (a !== b) edges.push({ source: a, target: b });
  }
  return { nodes, edges };
}

// Mean distance between the centroids of the `top` largest projects divided by
// the mean distance of their nodes to their own centroid. Higher = more
// visible galaxies; a uniform cube scores well below 1.
function topProjectSeparation(pos: Float32Array, nodes: LayoutNode[], top: number): number {
  const groups = new Map<number, number[]>();
  nodes.forEach((nd, i) => groups.set(nd.cluster, [...(groups.get(nd.cluster) ?? []), i]));
  const largest = [...groups.values()].sort((a, b) => b.length - a.length).slice(0, top);
  const at = (i: number, axis: number): number => pos[i * 3 + axis] ?? 0;
  const cents = largest.map((ids) =>
    [0, 1, 2].map((axis) => ids.reduce((sum, i) => sum + at(i, axis), 0) / ids.length),
  );
  let intra = 0;
  let members = 0;
  largest.forEach((ids, k) => {
    const c = cents[k] ?? [0, 0, 0];
    for (const i of ids) {
      intra += Math.hypot(at(i, 0) - (c[0] ?? 0), at(i, 1) - (c[1] ?? 0), at(i, 2) - (c[2] ?? 0));
      members++;
    }
  });
  let inter = 0;
  let pairs = 0;
  for (let a = 0; a < cents.length; a++) {
    for (let b = a + 1; b < cents.length; b++) {
      const p = cents[a] ?? [0, 0, 0];
      const q = cents[b] ?? [0, 0, 0];
      inter += Math.hypot(
        (p[0] ?? 0) - (q[0] ?? 0),
        (p[1] ?? 0) - (q[1] ?? 0),
        (p[2] ?? 0) - (q[2] ?? 0),
      );
      pairs++;
    }
  }
  return inter / pairs / (intra / members);
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

  it('keeps project galaxies separated on an owner-shaped 5000-node graph', () => {
    // Regression: cutting the schedule to 80 iterations at 5000 nodes left
    // the graph as a uniform cube (separation 0.33 here). The pre-Barnes–Hut
    // exact O(n²) layout with 200 iterations scores 3.61 on this fixture;
    // the threshold is 85% of that.
    const { nodes, edges } = makeOwnerShapedGraph();
    const pos = computeLayout(nodes, edges);
    expectFiniteAndBounded(pos, nodes.length);
    expect(topProjectSeparation(pos, nodes, 10)).toBeGreaterThan(3.07);
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
    // machine; Barnes–Hut with the same 200 iterations takes ~2.3 s. The bound
    // leaves headroom for slower CI runners while still catching a return to
    // the exact O(n²) path.
    expect(ms).toBeLessThan(5000);
  });

  it('still returns a full, finite buffer above the simulation cap', () => {
    const n = MAX_LAYOUT_NODES + 50;
    const { nodes, edges } = makeGraph(n, 8);
    expectFiniteAndBounded(computeLayout(nodes, edges), n);
  });
});

describe('layoutIterations', () => {
  it('keeps the full 200 iterations at every graph size', () => {
    // Project clusters only separate late in the schedule: a 5000-node graph
    // cut to 80 iterations stayed a uniform cube (see the owner-shaped test).
    expect(layoutIterations(50)).toBe(200);
    expect(layoutIterations(1000)).toBe(200);
    expect(layoutIterations(MAX_LAYOUT_NODES)).toBe(200);
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
