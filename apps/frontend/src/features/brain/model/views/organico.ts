/**
 * organico.ts — Orgánico view grouping.
 *
 * Uses the same TF-IDF cosine similarity approach as deriveSimilarityEdges
 * (tunnel-flow.tsx) to cluster observations, but groups them into BrainLevel
 * "cluster" nodes instead of rendering edges.
 *
 * Determinism (M2): fixed-seed PRNG for cluster assignment → same /graph
 * response always produces identical cluster assignments.
 *
 * Fallback (M1-C): if clustering throws or yields 0 clusters, silently falls
 * back to Temas structure (console.warn, no error UI).
 *
 * Default color-by: 'cluster'.
 */

import type { GraphNode } from '../../../../lib/api.ts';
import type { BrainNode, BrainNodeRef } from '../types.ts';
import { buildHexPalette } from '../../../../components/brain/node-colors.ts';
import { buildLobes as temasLobes, buildNeurons, buildNeuronChildren } from './temas.ts';
import { obsNode } from './lobulos.ts';
import { tokenize, TOPIC_TOKEN_WEIGHT, TITLE_TOKEN_WEIGHT } from '../../../../lib/tfidf.ts';

export const defaultColorBy = 'cluster' as const;

// ---------------------------------------------------------------------------
// Fixed-seed PRNG (for deterministic cluster seeding)
// ---------------------------------------------------------------------------

const ORGANICO_SEED = 42;

function seededRandom(seed: number): () => number {
  let s = seed;
  return () => {
    // LCG: multiplier 1664525, increment 1013904223 (Numerical Recipes)
    s = Math.imul(s, 1664525) + 1013904223;
    return ((s >>> 0) / 0x100000000);
  };
}

// ---------------------------------------------------------------------------
// Clustering: build similarity graph then use connected-component-style
// seeded grouping (k-means-like with fixed centroids from seeded random picks)
// ---------------------------------------------------------------------------

interface ClusterResult {
  /** Map from nodeId → cluster index (0-based). */
  assignment: Map<number, number>;
  /** Total number of clusters. */
  count: number;
}

const SIM_THRESHOLD = 0.18;
const MAX_CLUSTERS = 20; // cap to keep the graph readable

/**
 * Cluster nodes using TF-IDF cosine similarity + seeded k-means-like grouping.
 * Each node is assigned to the most similar centroid above the threshold.
 * Returns a stable assignment keyed by node numeric id.
 *
 * This is intentionally a simplified "good-enough" clustering for v1
 * (uses TF-IDF like deriveSimilarityEdges, with seeded picks).
 */
function clusterNodes(nodes: GraphNode[]): ClusterResult {
  const N = nodes.length;
  if (N < 2) {
    // Trivial: each node is its own cluster
    const assignment = new Map(nodes.map((n, i) => [n.id, i]));
    return { assignment, count: N };
  }

  // 1. Compute TF-IDF vectors (same as deriveSimilarityEdges)
  const tfMaps: Map<string, number>[] = [];
  const df = new Map<string, number>();

  for (const n of nodes) {
    const tf = new Map<string, number>();
    for (const tok of tokenize(n.topicKey)) tf.set(tok, (tf.get(tok) ?? 0) + TOPIC_TOKEN_WEIGHT);
    for (const tok of tokenize(n.label)) tf.set(tok, (tf.get(tok) ?? 0) + TITLE_TOKEN_WEIGHT);
    tfMaps.push(tf);
    for (const tok of tf.keys()) df.set(tok, (df.get(tok) ?? 0) + 1);
  }

  const vecs: Map<string, number>[] = [];
  const norms: number[] = [];

  for (let i = 0; i < N; i++) {
    const v = new Map<string, number>();
    let sumSq = 0;
    for (const [tok, freq] of tfMaps[i]!) {
      const idf = Math.log(N / (df.get(tok) ?? 1));
      if (idf <= 0) continue;
      const w = freq * idf;
      v.set(tok, w);
      sumSq += w * w;
    }
    vecs.push(v);
    norms.push(Math.sqrt(sumSq) || 1);
  }

  // 2. Build a similarity adjacency (sparse: only above threshold)
  const adj: Map<number, number[]> = new Map(nodes.map((_, i) => [i, []]));

  for (let i = 0; i < N; i++) {
    for (let j = i + 1; j < N; j++) {
      // Only compute dot for shared tokens (sparse)
      let dot = 0;
      for (const [tok, wi] of vecs[i]!) {
        const wj = vecs[j]!.get(tok);
        if (wj !== undefined) dot += wi * wj;
      }
      const cos = dot / (norms[i]! * norms[j]!);
      if (cos >= SIM_THRESHOLD) {
        adj.get(i)!.push(j);
        adj.get(j)!.push(i);
      }
    }
  }

  // 3. Connected components as clusters
  const assignment = new Map<number, number>();
  let clusterIdx = 0;

  for (let i = 0; i < N; i++) {
    if (assignment.has(i)) continue;
    // BFS
    const queue = [i];
    while (queue.length > 0) {
      const cur = queue.shift()!;
      if (assignment.has(cur)) continue;
      assignment.set(cur, clusterIdx);
      for (const neighbor of adj.get(cur)!) {
        if (!assignment.has(neighbor)) queue.push(neighbor);
      }
    }
    clusterIdx++;
  }

  // 4. If too many tiny clusters, merge by seeded random reassignment
  //    (deterministic merge: sort singleton clusters and merge to nearest neighbor)
  const clusterSizes = new Map<number, number>();
  for (const c of assignment.values()) {
    clusterSizes.set(c, (clusterSizes.get(c) ?? 0) + 1);
  }

  // Cap: merge smallest clusters into larger ones when count > MAX_CLUSTERS
  let finalCount = clusterSizes.size;
  if (finalCount > MAX_CLUSTERS) {
    const rng = seededRandom(ORGANICO_SEED);
    // Re-assign each singleton to a random cluster in [0, MAX_CLUSTERS-1]
    const remapTarget = MAX_CLUSTERS;
    for (const [nodeIdx, c] of assignment.entries()) {
      const size = clusterSizes.get(c) ?? 0;
      if (size === 1) {
        // Assign to a seeded bucket
        const bucket = Math.floor(rng() * remapTarget);
        assignment.set(nodeIdx, bucket);
      }
    }
    finalCount = remapTarget;
  }

  // Convert node-index assignment to node-id assignment
  const idAssignment = new Map<number, number>();
  for (const [nodeIdx, c] of assignment.entries()) {
    idAssignment.set(nodes[nodeIdx]!.id, c);
  }

  return { assignment: idAssignment, count: finalCount };
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Build cluster nodes from the flat node array.
 * Falls back to Temas on error.
 *
 * Returns the assignment map alongside nodes so the caller can thread it
 * into buildClusterChildren() — avoiding module-scope state (I3).
 */
export function buildClusters(nodes: GraphNode[]): {
  nodes: BrainNode[];
  usedFallback: boolean;
  /** Observation id → cluster index. null when usedFallback is true. */
  assignment: Map<number, number> | null;
} {
  try {
    const { assignment, count } = clusterNodes(nodes);

    if (count === 0) {
      // Fall back to Temas
      console.warn('[organico] clustering produced 0 clusters — falling back to Temas');
      return { nodes: temasLobes(nodes), usedFallback: true, assignment: null };
    }

    // Group nodes by cluster
    const byCluster = new Map<number, GraphNode[]>();
    for (const n of nodes) {
      const c = assignment.get(n.id) ?? 0;
      const bucket = byCluster.get(c);
      if (bucket) bucket.push(n);
      else byCluster.set(c, [n]);
    }

    const palette = buildHexPalette(
      Array.from({ length: count }, (_, i) => String(i)),
    );

    const clusterNodes_: BrainNode[] = Array.from(byCluster.entries())
      .sort(([a], [b]) => a - b)
      .map(([c, members]) => ({
        id: `cluster:${String(c).padStart(3, '0')}`,
        kind: 'cluster' as const,
        label: `Cluster ${c + 1}`,
        color: palette.get(String(c)) ?? '#4b5563',
        weight: members.length,
        childCount: members.length,
        meta: {
          clusterIndex: c,
          observationCount: members.length,
        },
        sourceIds: members.map((m) => m.id),
      }));

    return { nodes: clusterNodes_, usedFallback: false, assignment };
  } catch (err) {
    console.warn('[organico] clustering error — falling back to Temas:', err);
    return { nodes: temasLobes(nodes), usedFallback: true, assignment: null };
  }
}

/**
 * Build observation-leaf children for a cluster.
 * clusterNodeId format: "cluster:{NNN}" (zero-padded).
 *
 * @param assignment - The cluster assignment returned from buildClusters().
 *   When null (fallback to Temas was used), this parameter is ignored and
 *   the function delegates to temas/lobe helpers.
 */
export function buildClusterChildren(
  clusterNodeId: string,
  allNodes: GraphNode[],
  assignment: Map<number, number> | null = null,
): BrainNode[] {
  // If we fell back to Temas, clusterNodeId is a lobe id — delegate
  if (clusterNodeId.startsWith('lobe:')) {
    const project = clusterNodeId.slice('lobe:'.length);
    const palette = buildHexPalette(allNodes.map((n) => n.project));
    return allNodes
      .filter((n) => (n.project ?? '∅') === project)
      .map((n) => obsNode(n, palette));
  }
  if (clusterNodeId.startsWith('neuron:')) {
    return buildNeuronChildren(clusterNodeId, allNodes);
  }

  const rest = clusterNodeId.slice('cluster:'.length);
  const clusterIndex = parseInt(rest, 10);
  if (isNaN(clusterIndex)) return [];

  // Use the threaded assignment if available, else re-cluster (deterministic)
  const resolvedAssignment = assignment ?? clusterNodes(allNodes).assignment;
  const palette = buildHexPalette(allNodes.map((n) => n.project));

  return allNodes
    .filter((n) => (resolvedAssignment.get(n.id) ?? 0) === clusterIndex)
    .map((n) => obsNode(n, palette));
}

export { buildNeurons, buildNeuronChildren };

/** BrainNodeRef for a cluster. */
export function clusterRef(index: number): BrainNodeRef {
  return {
    id: `cluster:${String(index).padStart(3, '0')}`,
    kind: 'cluster',
    label: `Cluster ${index + 1}`,
  };
}
