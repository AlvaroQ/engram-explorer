// Pure 3D force-directed layout used by force-layout.worker.ts.
// Kept free of worker globals so it can be unit-tested deterministically.
//
// Clustering strategy (per design):
//   cluster    = project index — STRONG attraction toward cluster centroid (CLUSTER_K)
//   subCluster = topicKey index — SOFT attraction toward sub-cluster centroid (SUBCLUSTER_K)
//   semantic edges treated as springs with constant SPRING coefficient.

export type LayoutNode = { id: number; cluster: number; subCluster: number };
export type LayoutEdge = { source: number; target: number };

const REPULSION = 800;
const SPRING = 0.02;
const REST_LEN = 12;
const DAMPING = 0.85;
// Hard caps prevent the simulation from exploding numerically (Infinity → NaN)
// for medium/large graphs. Without these, 500+ nodes with sparse edges
// accumulate huge repulsion forces (40M+ per node per iter) and overflow.
const MAX_PAIR_FORCE = 100;
const MAX_VEL = 5;

// Clustering force coefficients.
// CLUSTER_K (project) is strong — pulls same-project nodes into a visible galaxy.
// SUBCLUSTER_K (topicKey) is soft — creates loose sub-clusters within a galaxy.
const CLUSTER_K = 0.015;
const SUBCLUSTER_K = 0.005;

// Deterministic pseudo-random in [0, 1) — used to jitter the seed so no two
// nodes start at the exact same point (which makes their pairwise repulsion
// direction be (0,0,0) and traps them together forever).
function jitter(i: number, axis: number): number {
  const x = Math.sin(i * 12.9898 + axis * 78.233) * 43758.5453;
  return x - Math.floor(x) - 0.5;
}

// noUncheckedIndexedAccess-safe read from a Float32Array.
// All callers guarantee i is within [0, array.length).
function r(a: Float32Array, i: number): number {
  const v = a[i];
  return v === undefined ? 0 : v;
}

// Positions are normalized so the longest axis fills ±TARGET_HALF.
export const TARGET_HALF = 30;

// Scaling (S9). Pairwise repulsion is O(n²) per iteration; from
// BARNES_HUT_MIN_NODES on it is approximated with a Barnes–Hut octree
// (O(n log n), opening angle BH_THETA). The iteration budget shrinks for large
// graphs (see layoutIterations). The server caps /api/graph at 5000 nodes;
// MAX_LAYOUT_NODES is a hard safety cap on top of that: nodes past it keep
// their deterministic seed position and are not simulated.
export const MAX_LAYOUT_NODES = 5000;
const BARNES_HUT_MIN_NODES = 256;
const BH_THETA = 0.8;
const DEFAULT_ITERATIONS = 200;
const MIN_ITERATIONS = 80;
// Graphs up to this size get the full iteration budget.
const FULL_BUDGET_NODES = 1000;
// Octree depth limit: coincident points share a leaf instead of recursing forever.
const MAX_TREE_DEPTH = 24;

/**
 * Iteration budget for a graph of n nodes: the full 200 up to 1000 nodes,
 * then scaled by 1000/n with a floor of 80. An explicit lower request wins.
 */
export function layoutIterations(n: number, requested = DEFAULT_ITERATIONS): number {
  const budget =
    n <= FULL_BUDGET_NODES
      ? DEFAULT_ITERATIONS
      : Math.max(MIN_ITERATIONS, Math.round((DEFAULT_ITERATIONS * FULL_BUDGET_NODES) / n));
  return Math.min(requested, budget);
}

// Capped inverse-square repulsion magnitude for squared distance d2.
// Without the cap, very close pairs produce f = 800/0.01 = 80000 per pair and
// the integrator overflows to Infinity → NaN on medium/large graphs.
function repulse(d2: number): number {
  return Math.min(REPULSION / d2, MAX_PAIR_FORCE);
}

// Exact pairwise repulsion over the first m nodes (used for small graphs).
function exactRepulsion(pos: Float32Array, vel: Float32Array, m: number): void {
  for (let i = 0; i < m; i++) {
    for (let j = i + 1; j < m; j++) {
      let dx = r(pos, i * 3) - r(pos, j * 3);
      let dy = r(pos, i * 3 + 1) - r(pos, j * 3 + 1);
      let dz = r(pos, i * 3 + 2) - r(pos, j * 3 + 2);
      const d2 = dx * dx + dy * dy + dz * dz + 0.01;
      const f = repulse(d2);
      const inv = 1 / Math.sqrt(d2);
      dx *= inv;
      dy *= inv;
      dz *= inv;
      vel[i * 3] = r(vel, i * 3) + dx * f;
      vel[i * 3 + 1] = r(vel, i * 3 + 1) + dy * f;
      vel[i * 3 + 2] = r(vel, i * 3 + 2) + dz * f;
      vel[j * 3] = r(vel, j * 3) - dx * f;
      vel[j * 3 + 1] = r(vel, j * 3 + 1) - dy * f;
      vel[j * 3 + 2] = r(vel, j * 3 + 2) - dz * f;
    }
  }
}

/**
 * Barnes–Hut octree in flat typed arrays, rebuilt every iteration.
 * Leaves hold a linked list of bodies (first/next) so coincident points at the
 * depth limit share a leaf. A far cell (size/distance < BH_THETA) that does
 * not contain the body acts as one body of `mass` bodies at its centre of
 * mass; its force is the capped pair force times mass, matching the exact
 * path for well-separated groups.
 */
class Octree {
  private cap = 0;
  private child = new Int32Array(0); // 8 per cell, -1 = none
  private first = new Int32Array(0); // leaf body list head, -1 = empty
  private isLeaf = new Uint8Array(0);
  private parent = new Int32Array(0);
  private depth = new Uint8Array(0);
  private ox = new Float64Array(0); // cell centre
  private oy = new Float64Array(0);
  private oz = new Float64Array(0);
  private half = new Float64Array(0);
  private mass = new Float64Array(0);
  private sx = new Float64Array(0); // position sums → centre of mass
  private sy = new Float64Array(0);
  private sz = new Float64Array(0);
  private stack = new Int32Array(0);
  private readonly next: Int32Array; // per body
  private count = 0;

  constructor(bodies: number) {
    this.next = new Int32Array(bodies);
    this.grow(Math.max(64, bodies * 4));
  }

  // (Re)allocate cell storage, keeping existing cells.
  private grow(cap: number): void {
    const copy = <T extends Int32Array | Uint8Array | Float64Array>(a: T, size: number): T => {
      const b = new (a.constructor as new (len: number) => T)(size);
      b.set(a);
      return b;
    };
    this.child = copy(this.child, cap * 8);
    this.first = copy(this.first, cap);
    this.isLeaf = copy(this.isLeaf, cap);
    this.parent = copy(this.parent, cap);
    this.depth = copy(this.depth, cap);
    this.ox = copy(this.ox, cap);
    this.oy = copy(this.oy, cap);
    this.oz = copy(this.oz, cap);
    this.half = copy(this.half, cap);
    this.mass = copy(this.mass, cap);
    this.sx = copy(this.sx, cap);
    this.sy = copy(this.sy, cap);
    this.sz = copy(this.sz, cap);
    this.stack = new Int32Array(cap * 8);
    this.cap = cap;
  }

  private newCell(x: number, y: number, z: number, h: number, parent: number, d: number): number {
    if (this.count >= this.cap) this.grow(this.cap * 2);
    const c = this.count++;
    this.child.fill(-1, c * 8, c * 8 + 8);
    this.first[c] = -1;
    this.isLeaf[c] = 1;
    this.parent[c] = parent;
    this.depth[c] = d;
    this.ox[c] = x;
    this.oy[c] = y;
    this.oz[c] = z;
    this.half[c] = h;
    this.mass[c] = 0;
    this.sx[c] = 0;
    this.sy[c] = 0;
    this.sz[c] = 0;
    return c;
  }

  private octant(c: number, pos: Float32Array, b: number): number {
    return (
      (r(pos, b * 3) >= (this.ox[c] ?? 0) ? 1 : 0) |
      (r(pos, b * 3 + 1) >= (this.oy[c] ?? 0) ? 2 : 0) |
      (r(pos, b * 3 + 2) >= (this.oz[c] ?? 0) ? 4 : 0)
    );
  }

  private childFor(c: number, o: number): number {
    const existing = this.child[c * 8 + o] ?? -1;
    if (existing >= 0) return existing;
    const h = (this.half[c] ?? 0) / 2;
    const k = this.newCell(
      (this.ox[c] ?? 0) + (o & 1 ? h : -h),
      (this.oy[c] ?? 0) + (o & 2 ? h : -h),
      (this.oz[c] ?? 0) + (o & 4 ? h : -h),
      h,
      c,
      (this.depth[c] ?? 0) + 1,
    );
    this.child[c * 8 + o] = k;
    return k;
  }

  build(pos: Float32Array, m: number): void {
    this.count = 0;
    let minX = Infinity,
      minY = Infinity,
      minZ = Infinity,
      maxX = -Infinity,
      maxY = -Infinity,
      maxZ = -Infinity;
    for (let i = 0; i < m; i++) {
      const x = r(pos, i * 3),
        y = r(pos, i * 3 + 1),
        z = r(pos, i * 3 + 2);
      if (x < minX) minX = x;
      if (x > maxX) maxX = x;
      if (y < minY) minY = y;
      if (y > maxY) maxY = y;
      if (z < minZ) minZ = z;
      if (z > maxZ) maxZ = z;
    }
    const h = Math.max(maxX - minX, maxY - minY, maxZ - minZ, 1e-3) / 2 + 1e-3;
    this.newCell((minX + maxX) / 2, (minY + maxY) / 2, (minZ + maxZ) / 2, h, -1, 0);
    for (let b = 0; b < m; b++) this.insert(pos, b);
    // Children are always created after their parent, so a reverse sweep
    // accumulates mass and position sums bottom-up (leaves already hold theirs).
    for (let c = this.count - 1; c > 0; c--) {
      const p = this.parent[c] ?? 0;
      this.mass[p] = (this.mass[p] ?? 0) + (this.mass[c] ?? 0);
      this.sx[p] = (this.sx[p] ?? 0) + (this.sx[c] ?? 0);
      this.sy[p] = (this.sy[p] ?? 0) + (this.sy[c] ?? 0);
      this.sz[p] = (this.sz[p] ?? 0) + (this.sz[c] ?? 0);
    }
  }

  private addToLeaf(c: number, pos: Float32Array, b: number): void {
    this.next[b] = this.first[c] ?? -1;
    this.first[c] = b;
    this.mass[c] = (this.mass[c] ?? 0) + 1;
    this.sx[c] = (this.sx[c] ?? 0) + r(pos, b * 3);
    this.sy[c] = (this.sy[c] ?? 0) + r(pos, b * 3 + 1);
    this.sz[c] = (this.sz[c] ?? 0) + r(pos, b * 3 + 2);
  }

  private insert(pos: Float32Array, b: number): void {
    let c = 0;
    for (;;) {
      if (!this.isLeaf[c]) {
        c = this.childFor(c, this.octant(c, pos, b));
        continue;
      }
      const head = this.first[c] ?? -1;
      if (head < 0 || (this.depth[c] ?? 0) >= MAX_TREE_DEPTH) {
        this.addToLeaf(c, pos, b);
        return;
      }
      // Split an occupied leaf: push its bodies down, then retry this body.
      this.isLeaf[c] = 0;
      this.first[c] = -1;
      this.mass[c] = 0;
      this.sx[c] = 0;
      this.sy[c] = 0;
      this.sz[c] = 0;
      for (let j = head; j >= 0; ) {
        const nxt = this.next[j] ?? -1;
        this.addToLeaf(this.childFor(c, this.octant(c, pos, j)), pos, j);
        j = nxt;
      }
    }
  }

  /** Accumulate the approximate repulsion on body i into vel. */
  applyRepulsion(pos: Float32Array, vel: Float32Array, i: number): void {
    const xi = r(pos, i * 3),
      yi = r(pos, i * 3 + 1),
      zi = r(pos, i * 3 + 2);
    let fx = 0,
      fy = 0,
      fz = 0;
    let sp = 0;
    this.stack[sp++] = 0;
    const theta2 = BH_THETA * BH_THETA;
    while (sp > 0) {
      const c = this.stack[--sp] ?? 0;
      const mass = this.mass[c] ?? 0;
      if (mass === 0) continue;
      if (this.isLeaf[c]) {
        for (let j = this.first[c] ?? -1; j >= 0; j = this.next[j] ?? -1) {
          if (j === i) continue;
          const dx = xi - r(pos, j * 3),
            dy = yi - r(pos, j * 3 + 1),
            dz = zi - r(pos, j * 3 + 2);
          const d2 = dx * dx + dy * dy + dz * dz + 0.01;
          const k = repulse(d2) / Math.sqrt(d2);
          fx += dx * k;
          fy += dy * k;
          fz += dz * k;
        }
        continue;
      }
      const dx = xi - (this.sx[c] ?? 0) / mass,
        dy = yi - (this.sy[c] ?? 0) / mass,
        dz = zi - (this.sz[c] ?? 0) / mass;
      const d2 = dx * dx + dy * dy + dz * dz + 0.01;
      const h = this.half[c] ?? 0;
      const size = 2 * h;
      // A cell holding body i is always opened: its centre of mass includes
      // i itself, so approximating it would push i with its own mass.
      const holdsI =
        Math.abs(xi - (this.ox[c] ?? 0)) <= h &&
        Math.abs(yi - (this.oy[c] ?? 0)) <= h &&
        Math.abs(zi - (this.oz[c] ?? 0)) <= h;
      if (!holdsI && (size * size) / d2 < theta2) {
        const k = (repulse(d2) * mass) / Math.sqrt(d2);
        fx += dx * k;
        fy += dy * k;
        fz += dz * k;
        continue;
      }
      for (let o = 0; o < 8; o++) {
        const k = this.child[c * 8 + o] ?? -1;
        if (k >= 0) this.stack[sp++] = k;
      }
    }
    vel[i * 3] = r(vel, i * 3) + fx;
    vel[i * 3 + 1] = r(vel, i * 3 + 1) + fy;
    vel[i * 3 + 2] = r(vel, i * 3 + 2) + fz;
  }
}

/**
 * One Barnes–Hut repulsion pass over the first m nodes, as the velocity
 * increment per node (exposed for the accuracy test against the exact sum).
 */
export function barnesHutRepulsion(pos: Float32Array, m: number): Float32Array {
  const vel = new Float32Array(m * 3);
  const tree = new Octree(m);
  tree.build(pos, m);
  for (let i = 0; i < m; i++) tree.applyRepulsion(pos, vel, i);
  return vel;
}

export function computeLayout(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  requestedIterations = DEFAULT_ITERATIONS,
): Float32Array {
  const n = nodes.length;
  const pos = new Float32Array(n * 3);
  const vel = new Float32Array(n * 3);

  // Deterministic spherical seed + small jitter on every axis. Jitter is
  // critical: without it, every node where (i % 50 === 0) starts at exactly
  // (0, 0, i%25 - 12), the pairwise repulsion direction collapses to zero
  // and those nodes stay glued together throughout the whole simulation.
  for (let i = 0; i < n; i++) {
    const t = i * 2.399963; // golden angle increment
    pos[i * 3] = Math.cos(t) * (i % 50) + jitter(i, 0) * 2;
    pos[i * 3 + 1] = Math.sin(t) * (i % 50) + jitter(i, 1) * 2;
    pos[i * 3 + 2] = (i % 25) - 12 + jitter(i, 2) * 2;
  }

  // Only the first m nodes are simulated; the rest keep their seed position.
  const m = Math.min(n, MAX_LAYOUT_NODES);
  const iterations = layoutIterations(m, requestedIterations);
  const tree = m >= BARNES_HUT_MIN_NODES ? new Octree(m) : null;

  for (let step = 0; step < iterations; step++) {
    if (tree) {
      tree.build(pos, m);
      for (let i = 0; i < m; i++) tree.applyRepulsion(pos, vel, i);
    } else {
      exactRepulsion(pos, vel, m);
    }

    // Clustering forces — compute centroids per cluster and subCluster first,
    // then apply attraction. Centroids change each step so nodes converge
    // gradually (this is a Gaussian attraction, not a hard constraint).

    // Build cluster centroids (project-level, STRONG).
    const clusterCx = new Map<number, number>();
    const clusterCy = new Map<number, number>();
    const clusterCz = new Map<number, number>();
    const clusterCount = new Map<number, number>();
    for (let i = 0; i < m; i++) {
      const c = nodes[i]?.cluster ?? 0;
      clusterCx.set(c, (clusterCx.get(c) ?? 0) + r(pos, i * 3));
      clusterCy.set(c, (clusterCy.get(c) ?? 0) + r(pos, i * 3 + 1));
      clusterCz.set(c, (clusterCz.get(c) ?? 0) + r(pos, i * 3 + 2));
      clusterCount.set(c, (clusterCount.get(c) ?? 0) + 1);
    }

    // Build subCluster centroids (topicKey-level, SOFT).
    const subCx = new Map<number, number>();
    const subCy = new Map<number, number>();
    const subCz = new Map<number, number>();
    const subCount = new Map<number, number>();
    for (let i = 0; i < m; i++) {
      const s = nodes[i]?.subCluster ?? 0;
      subCx.set(s, (subCx.get(s) ?? 0) + r(pos, i * 3));
      subCy.set(s, (subCy.get(s) ?? 0) + r(pos, i * 3 + 1));
      subCz.set(s, (subCz.get(s) ?? 0) + r(pos, i * 3 + 2));
      subCount.set(s, (subCount.get(s) ?? 0) + 1);
    }

    // Apply clustering forces.
    for (let i = 0; i < m; i++) {
      const node = nodes[i];
      if (!node) continue;
      const c = node.cluster;
      const s = node.subCluster;
      const cnt = clusterCount.get(c) ?? 1;
      const scnt = subCount.get(s) ?? 1;

      // Cluster centroid (mean includes self — acceptable bias at n>1).
      const ccx = (clusterCx.get(c) ?? 0) / cnt;
      const ccy = (clusterCy.get(c) ?? 0) / cnt;
      const ccz = (clusterCz.get(c) ?? 0) / cnt;

      // Sub-cluster centroid.
      const scx = (subCx.get(s) ?? 0) / scnt;
      const scy = (subCy.get(s) ?? 0) / scnt;
      const scz = (subCz.get(s) ?? 0) / scnt;

      // Strong pull toward project centroid.
      vel[i * 3] = r(vel, i * 3) + (ccx - r(pos, i * 3)) * CLUSTER_K;
      vel[i * 3 + 1] = r(vel, i * 3 + 1) + (ccy - r(pos, i * 3 + 1)) * CLUSTER_K;
      vel[i * 3 + 2] = r(vel, i * 3 + 2) + (ccz - r(pos, i * 3 + 2)) * CLUSTER_K;

      // Soft pull toward topicKey centroid.
      vel[i * 3] = r(vel, i * 3) + (scx - r(pos, i * 3)) * SUBCLUSTER_K;
      vel[i * 3 + 1] = r(vel, i * 3 + 1) + (scy - r(pos, i * 3 + 1)) * SUBCLUSTER_K;
      vel[i * 3 + 2] = r(vel, i * 3 + 2) + (scz - r(pos, i * 3 + 2)) * SUBCLUSTER_K;
    }

    // Spring attraction along semantic edges.
    for (const ed of edges) {
      if (ed.source >= m || ed.target >= m || ed.source < 0 || ed.target < 0) continue;
      const a = ed.source * 3;
      const b = ed.target * 3;
      const dx = r(pos, b) - r(pos, a);
      const dy = r(pos, b + 1) - r(pos, a + 1);
      const dz = r(pos, b + 2) - r(pos, a + 2);
      const dist = Math.sqrt(dx * dx + dy * dy + dz * dz) + 0.01;
      const f = ((dist - REST_LEN) * SPRING) / dist;
      vel[a] = r(vel, a) + dx * f;
      vel[a + 1] = r(vel, a + 1) + dy * f;
      vel[a + 2] = r(vel, a + 2) + dz * f;
      vel[b] = r(vel, b) - dx * f;
      vel[b + 1] = r(vel, b + 1) - dy * f;
      vel[b + 2] = r(vel, b + 2) - dz * f;
    }

    // Integrate: damping, hard velocity cap, then position update.
    // The cap is the second safety net (the pair-force cap above is the
    // first): it ensures even pathological accumulated forces can't move a
    // node more than MAX_VEL units per step. Net effect: positions stay
    // bounded, simulation is numerically stable for any reasonable n.
    for (let i = 0; i < m * 3; i++) {
      let v = r(vel, i) * DAMPING;
      if (v > MAX_VEL) v = MAX_VEL;
      else if (v < -MAX_VEL) v = -MAX_VEL;
      vel[i] = v;
      pos[i] = r(pos, i) + v;
    }
  }

  // Normalize positions so the bounding box always fits within a fixed cube.
  // Without this, large graphs spread far past the camera frustum (camera at
  // z=80 sees ~±37) and look empty. Center on the centroid of the cloud and
  // rescale uniformly so the longest axis fills ±TARGET_HALF.
  if (n > 0) {
    let minX = Infinity,
      minY = Infinity,
      minZ = Infinity;
    let maxX = -Infinity,
      maxY = -Infinity,
      maxZ = -Infinity;
    let hasNonFinite = false;
    for (let i = 0; i < n; i++) {
      const x = r(pos, i * 3);
      const y = r(pos, i * 3 + 1);
      const z = r(pos, i * 3 + 2);
      // Guard: any NaN/Infinity poisons min/max and propagates to every node.
      if (!Number.isFinite(x) || !Number.isFinite(y) || !Number.isFinite(z)) {
        pos[i * 3] = 0;
        pos[i * 3 + 1] = 0;
        pos[i * 3 + 2] = 0;
        hasNonFinite = true;
        continue;
      }
      if (x < minX) minX = x;
      if (x > maxX) maxX = x;
      if (y < minY) minY = y;
      if (y > maxY) maxY = y;
      if (z < minZ) minZ = z;
      if (z > maxZ) maxZ = z;
    }
    if (hasNonFinite) {
      console.warn(
        '[force-layout] reset non-finite positions to origin (see force calc / NaN propagation)',
      );
    }
    const cx = (minX + maxX) / 2;
    const cy = (minY + maxY) / 2;
    const cz = (minZ + maxZ) / 2;
    const extent = Math.max(maxX - minX, maxY - minY, maxZ - minZ);
    if (Number.isFinite(extent) && extent > 0.0001) {
      const scale = (2 * TARGET_HALF) / extent;
      for (let i = 0; i < n; i++) {
        pos[i * 3] = (r(pos, i * 3) - cx) * scale;
        pos[i * 3 + 1] = (r(pos, i * 3 + 1) - cy) * scale;
        pos[i * 3 + 2] = (r(pos, i * 3 + 2) - cz) * scale;
      }
    } else {
      console.warn('[force-layout] degenerate extent, skipping normalization');
    }
  }

  return pos;
}
