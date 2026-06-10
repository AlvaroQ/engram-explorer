// Off-main-thread 3D force-directed layout.
// Receives { nodes: {id: number; cluster: number; subCluster: number}[], edges: {source: number; target: number}[], iterations? }
// Returns transferable Float32Array of positions (x,y,z interleaved).
// The caller maps numeric IDs → indices and derives cluster/subCluster from project/topicKey before posting.
//
// Clustering strategy (per design):
//   cluster    = project index — STRONG attraction toward cluster centroid (CLUSTER_K)
//   subCluster = topicKey index — SOFT attraction toward sub-cluster centroid (SUBCLUSTER_K)
//   semantic edges treated as springs with constant SPRING coefficient.

type InNode = { id: number; cluster: number; subCluster: number };
type InEdge = { source: number; target: number };
type WorkerMsg = { nodes: InNode[]; edges: InEdge[]; iterations?: number };

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

self.onmessage = (e: MessageEvent<WorkerMsg>) => {
  const { nodes, edges, iterations = 200 } = e.data;
  const n = nodes.length;
  const pos = new Float32Array(n * 3);
  const vel = new Float32Array(n * 3);
  const t0 = performance.now();
  if (import.meta.env.DEV) {
    console.log(`[force-layout] starting: n=${n} edges=${edges.length} iterations=${iterations}`);
  }

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

  for (let step = 0; step < iterations; step++) {
    // Repulsion — O(n²); acceptable for hundreds of nodes.
    // Above ~5k nodes swap for a Barnes-Hut octree.
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        let dx = r(pos, i * 3) - r(pos, j * 3);
        let dy = r(pos, i * 3 + 1) - r(pos, j * 3 + 1);
        let dz = r(pos, i * 3 + 2) - r(pos, j * 3 + 2);
        const d2 = dx * dx + dy * dy + dz * dz + 0.01;
        // Cap pairwise repulsion. Without the cap, very close pairs produce
        // f = 800/0.01 = 80000 N per pair; with 500 nodes that's 40M N per
        // node per iter and the integrator overflows to Infinity → NaN.
        const f = Math.min(REPULSION / d2, MAX_PAIR_FORCE);
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

    // Clustering forces — compute centroids per cluster and subCluster first,
    // then apply attraction. Centroids change each step so nodes converge
    // gradually (this is a Gaussian attraction, not a hard constraint).

    // Build cluster centroids (project-level, STRONG).
    const clusterCx = new Map<number, number>();
    const clusterCy = new Map<number, number>();
    const clusterCz = new Map<number, number>();
    const clusterCount = new Map<number, number>();
    for (let i = 0; i < n; i++) {
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
    for (let i = 0; i < n; i++) {
      const s = nodes[i]?.subCluster ?? 0;
      subCx.set(s, (subCx.get(s) ?? 0) + r(pos, i * 3));
      subCy.set(s, (subCy.get(s) ?? 0) + r(pos, i * 3 + 1));
      subCz.set(s, (subCz.get(s) ?? 0) + r(pos, i * 3 + 2));
      subCount.set(s, (subCount.get(s) ?? 0) + 1);
    }

    // Apply clustering forces.
    for (let i = 0; i < n; i++) {
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
    for (let i = 0; i < n * 3; i++) {
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
  const TARGET_HALF = 30;
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
    if (import.meta.env.DEV) {
      console.log(
        `[force-layout] bbox before normalize: x=[${minX.toFixed(1)},${maxX.toFixed(1)}] y=[${minY.toFixed(1)},${maxY.toFixed(1)}] z=[${minZ.toFixed(1)},${maxZ.toFixed(1)}] extent=${extent.toFixed(1)}`,
      );
    }
    if (Number.isFinite(extent) && extent > 0.0001) {
      const scale = (2 * TARGET_HALF) / extent;
      for (let i = 0; i < n; i++) {
        pos[i * 3] = (r(pos, i * 3) - cx) * scale;
        pos[i * 3 + 1] = (r(pos, i * 3 + 1) - cy) * scale;
        pos[i * 3 + 2] = (r(pos, i * 3 + 2) - cz) * scale;
      }
      if (import.meta.env.DEV) {
        console.log(
          `[force-layout] normalized with scale=${scale.toFixed(3)}, target half=${TARGET_HALF}`,
        );
      }
    } else {
      console.warn('[force-layout] degenerate extent, skipping normalization');
    }
  }

  if (import.meta.env.DEV) {
    console.log(`[force-layout] done in ${(performance.now() - t0).toFixed(0)}ms`);
  }

  // Transfer the buffer zero-copy back to the main thread.
  (self as unknown as Worker).postMessage({ positions: pos, count: n }, [pos.buffer]);
};
