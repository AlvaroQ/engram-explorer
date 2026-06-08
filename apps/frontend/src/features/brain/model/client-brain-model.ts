/**
 * client-brain-model.ts — Client-side BrainModel implementation.
 *
 * Implements the BrainModel interface by aggregating the flat /graph response
 * into a hierarchical brain structure, keyed by ViewMode.
 *
 * Contracts enforced:
 *   C2  Leaf contract: children(leafId) → empty BrainLevel, never throws.
 *   C3  Edge aggregation: weight=count, confidence=avg, relation=dominant, cull >200.
 *   C4  Relation → family: related/compatible→semantic; scoped/synthetic→topic-road.
 *   C8  availableFacets() hides tool/session when backing field is absent.
 *   M1  Per-view hierarchy matrix.
 *   M2  Orgánico: fixed-seed clustering; id stable across view switches.
 *   S1.5 Memoization: same (view, filters, path-key) → same object reference.
 */

import type { GraphEdge, GraphNode } from '../../../lib/api.ts';
import { buildHexPalette } from '../../../components/brain/node-colors.ts';
import type {
  ActiveFacets,
  BrainEdge,
  BrainLevel,
  BrainModel,
  BrainNodeRef,
  EdgeFamily,
  FacetGroupId,
  GraphEdgeRelation,
  ViewMode,
} from './types.ts';
import { buildLobes as lobulosLobes, buildLobeChildren } from './views/lobulos.ts';
import { buildLobes as temasLobes, buildNeurons, buildNeuronChildren } from './views/temas.ts';
import { buildClusters, buildClusterChildren } from './views/organico.ts';
import { availableFacets as computeAvailableFacets } from './facets.ts';

// ---------------------------------------------------------------------------
// Relation → family mapping (ADR-6, C4)
// Confirmed against backend schema: enum is exactly ('related','scoped','compatible')
// ---------------------------------------------------------------------------

function relationToFamily(relation: GraphEdgeRelation): EdgeFamily {
  switch (relation) {
    case 'related': return 'semantic';
    case 'compatible': return 'semantic';
    case 'scoped': return 'topic-road';
    default:
      // TypeScript exhaustive check — any future relation value surfaces here at compile time
      return ((_r: never) => 'semantic' as EdgeFamily)(relation);
  }
}

// ---------------------------------------------------------------------------
// Edge aggregation helpers (C3)
// ---------------------------------------------------------------------------

interface RawEdgeSource {
  relation: GraphEdgeRelation;
  confidence?: number;
}

interface RawEdgeBundle {
  sources: RawEdgeSource[];
}

/**
 * Aggregate raw GraphEdge rows into BrainEdge objects.
 *
 * Grouping key: (sourceAggId, targetAggId, family).
 *   weight     = count of distinct underlying GraphEdge rows.
 *   confidence = arithmetic mean of underlying confidences (undefined rows → 0.5
 *                for averaging purposes; if ALL undefined → confidence is undefined).
 *   relation   = dominant (most frequent; ties → string sort ascending).
 *
 * Edge budget: if >200 results, cull weakest by (confidence ?? 0.5) × weight.
 */
function aggregateEdges(
  graphEdges: GraphEdge[],
  /** Map from numeric observation id → aggregate node id (e.g. "lobe:engram"). */
  idToAggregate: Map<number, string>,
  /** Additional synthetic edges (topic-road family). */
  syntheticEdges: Array<{ source: number; target: number }> = [],
): BrainEdge[] {
  const bundles = new Map<string, RawEdgeBundle>();

  const addEdge = (
    srcAgg: string,
    tgtAgg: string,
    family: EdgeFamily,
    relation: GraphEdgeRelation | undefined,
    confidence: number | undefined,
  ) => {
    if (srcAgg === tgtAgg) return; // skip self-loops at aggregate level
    // Normalize direction (canonical: smaller string first for dedup)
    const [a, b] = srcAgg < tgtAgg ? [srcAgg, tgtAgg] : [tgtAgg, srcAgg];
    const key = `${a}\x00${b}\x00${family}`;
    const src: RawEdgeSource = { relation: relation ?? 'related' };
    if (confidence !== undefined) src.confidence = confidence;
    const existing = bundles.get(key);
    if (existing) {
      existing.sources.push(src);
    } else {
      bundles.set(key, { sources: [src] });
    }
  };

  // Process explicit GraphEdge rows
  for (const e of graphEdges) {
    const srcAgg = idToAggregate.get(e.source);
    const tgtAgg = idToAggregate.get(e.target);
    if (srcAgg === undefined || tgtAgg === undefined) continue;
    const family = relationToFamily(e.relation);
    addEdge(srcAgg, tgtAgg, family, e.relation, e.confidence);
  }

  // Process synthetic topic-road edges
  for (const e of syntheticEdges) {
    const srcAgg = idToAggregate.get(e.source);
    const tgtAgg = idToAggregate.get(e.target);
    if (srcAgg === undefined || tgtAgg === undefined) continue;
    addEdge(srcAgg, tgtAgg, 'topic-road', undefined, undefined);
  }

  // Build BrainEdge[] from bundles
  const result: BrainEdge[] = [];
  for (const [key, bundle] of bundles) {
    const [srcAgg, tgtAgg, family] = key.split('\x00') as [string, string, EdgeFamily];
    const weight = bundle.sources.length;

    // confidence: mean of defined values; undefined row → 0.5 for averaging
    // if ALL undefined → confidence is undefined
    const allUndefined = bundle.sources.every((s) => s.confidence === undefined);
    const confidence = allUndefined
      ? undefined
      : bundle.sources.reduce((sum, s) => sum + (s.confidence ?? 0.5), 0) / weight;

    // dominant relation: most frequent; ties → string sort ascending
    const relCount = new Map<string, number>();
    for (const { relation } of bundle.sources) {
      relCount.set(relation, (relCount.get(relation) ?? 0) + 1);
    }
    let dominantRel: GraphEdgeRelation = 'related';
    let dominantCount = 0;
    for (const [rel, count] of [...relCount.entries()].sort(([a], [b]) => a.localeCompare(b))) {
      if (count > dominantCount) {
        dominantRel = rel as GraphEdgeRelation;
        dominantCount = count;
      }
    }

    const edge: BrainEdge = {
      source: srcAgg,
      target: tgtAgg,
      family,
      relation: dominantRel,
      weight,
    };
    if (confidence !== undefined) edge.confidence = confidence;
    result.push(edge);
  }

  // Edge budget cull: keep top 200 by (confidence ?? 0.5) × weight (C3)
  if (result.length > 200) {
    result.sort((a, b) => {
      const scoreB = (b.confidence ?? 0.5) * b.weight;
      const scoreA = (a.confidence ?? 0.5) * a.weight;
      if (scoreB !== scoreA) return scoreB - scoreA;
      // Deterministic tiebreak: source asc, then target asc
      const cmp = a.source.localeCompare(b.source);
      if (cmp !== 0) return cmp;
      return a.target.localeCompare(b.target);
    });
    result.splice(200);
  }

  return result;
}

// ---------------------------------------------------------------------------
// BrainLevel factory
// ---------------------------------------------------------------------------

function makeEmptyLevel(parentPath: BrainNodeRef[]): BrainLevel {
  return {
    nodes: [],
    edges: [],
    parentPath,
    stats: { nodeCount: 0, observationCount: 0, crossSynapses: 0 },
  };
}

function computeStats(level: Omit<BrainLevel, 'stats'>): BrainLevel {
  const nodeCount = level.nodes.length;
  // Sum real underlying observations at aggregate levels (I2):
  //   - leaf observation: sourceIds has exactly 1 element → count as 1
  //   - aggregate node: sum sourceIds.length; if no sourceIds and childCount>0
  //     use childCount as lower-bound estimate; otherwise count as 1
  const observationCount = level.nodes.reduce((sum, n) => {
    if (n.sourceIds !== undefined && n.sourceIds.length > 0) {
      return sum + n.sourceIds.length;
    }
    if (n.childCount > 0) {
      return sum + n.childCount;
    }
    return sum + 1;
  }, 0);
  // Cross-synapses: edges that cross different aggregate nodes
  const crossSynapses = level.edges.filter(
    (e) => e.source !== e.target,
  ).length;

  // dominant type: most common meta.type among nodes
  const typeCounts = new Map<string, number>();
  for (const n of level.nodes) {
    const t = n.meta['type'];
    if (typeof t === 'string') typeCounts.set(t, (typeCounts.get(t) ?? 0) + 1);
  }
  let dominantType: string | undefined;
  let maxCount = 0;
  for (const [t, c] of typeCounts) {
    if (c > maxCount) { dominantType = t; maxCount = c; }
  }

  // Kind of the nodes at this level — drives the stats noun (lobes/neurons/clusters/obs)
  const nodeKind = level.nodes[0]?.kind;
  const stats = { nodeCount, observationCount, crossSynapses } as { nodeCount: number; observationCount: number; crossSynapses: number; dominantType?: string; nodeKind?: NonNullable<typeof nodeKind> };
  if (dominantType !== undefined) stats.dominantType = dominantType;
  if (nodeKind !== undefined) stats.nodeKind = nodeKind;
  return { ...level, stats };
}

// ---------------------------------------------------------------------------
// ClientBrainModel
// ---------------------------------------------------------------------------

export function createClientBrainModel(
  rawNodes: GraphNode[],
  rawEdges: GraphEdge[],
  view: ViewMode,
): BrainModel {
  // Memoization cache: key → BrainLevel
  const memo = new Map<string, BrainLevel>();

  function memoKey(method: string, nodeId: string, filters: ActiveFacets): string {
    return `${method}\x00${view}\x00${nodeId}\x00${JSON.stringify(filters)}`;
  }

  // Palette for observations (project-based by default)
  const obsPalette = buildHexPalette(rawNodes.map((n) => n.project));

  // ---------------------------------------------------------------------------
  // Organico cluster state — scoped to this model instance (I3)
  // Computed once on first root() call; threaded into buildClusterChildren()
  // so no module-scope mutable state is needed.
  // ---------------------------------------------------------------------------

  let organicoAssignment: Map<number, number> | null = null;

  // ---------------------------------------------------------------------------
  // Root-level builders (per view)
  // ---------------------------------------------------------------------------

  function buildRoot(_reservedFilters: ActiveFacets): BrainLevel {
    const parentPath: BrainNodeRef[] = [];

    if (view === 'lobulos') {
      const nodes = lobulosLobes(rawNodes);
      // Aggregate-to-raw id mapping: lobe:P → all observation ids in P
      const idToAgg = new Map<number, string>();
      for (const n of rawNodes) {
        idToAgg.set(n.id, `lobe:${n.project ?? '∅'}`);
      }
      const edges = aggregateEdges(rawEdges, idToAgg);
      return computeStats({ nodes, edges, parentPath });
    }

    if (view === 'temas') {
      const nodes = temasLobes(rawNodes);
      const idToAgg = new Map<number, string>();
      for (const n of rawNodes) {
        idToAgg.set(n.id, `lobe:${n.project ?? '∅'}`);
      }
      const edges = aggregateEdges(rawEdges, idToAgg);
      return computeStats({ nodes, edges, parentPath });
    }

    // organico
    const { nodes, usedFallback, assignment } = buildClusters(rawNodes);
    // Store assignment in closure so buildChildren can use it (I3)
    organicoAssignment = assignment;

    let idToAgg: Map<number, string>;

    if (usedFallback) {
      // Fell back to Temas lobes
      idToAgg = new Map<number, string>();
      for (const n of rawNodes) {
        idToAgg.set(n.id, `lobe:${n.project ?? '∅'}`);
      }
    } else {
      // Use cluster assignment
      idToAgg = new Map<number, string>();
      for (const clusterNode of nodes) {
        for (const srcId of clusterNode.sourceIds ?? []) {
          idToAgg.set(srcId, clusterNode.id);
        }
      }
    }
    const edges = aggregateEdges(rawEdges, idToAgg);
    return computeStats({ nodes, edges, parentPath });
  }

  // ---------------------------------------------------------------------------
  // Children builders (per view, per node kind)
  // ---------------------------------------------------------------------------

  function buildChildren(nodeId: string, _reservedFilters: ActiveFacets): BrainLevel {
    // Leaf: "obs:{n}" → empty level (C2)
    if (nodeId.startsWith('obs:')) {
      const obsIdStr = nodeId.slice('obs:'.length);
      const obsId = parseInt(obsIdStr, 10);
      const rawNode = rawNodes.find((n) => n.id === obsId);
      const leafRef: BrainNodeRef = rawNode
        ? { id: nodeId, kind: 'observation', label: rawNode.label ?? nodeId }
        : { id: nodeId, kind: 'observation', label: nodeId };
      return makeEmptyLevel([leafRef]);
    }

    if (view === 'lobulos') {
      // nodeId = "lobe:{project}"
      const project = nodeId.slice('lobe:'.length);
      const nodes = buildLobeChildren(project, rawNodes, obsPalette);
      // At leaf level (observations), no aggregate edges — each obs is its own id
      const idToAgg = new Map<number, string>();
      for (const n of nodes) {
        const srcId = n.sourceIds?.[0];
        if (srcId !== undefined) idToAgg.set(srcId, `obs:${srcId}`);
      }
      const edges = aggregateEdges(rawEdges, idToAgg);
      const lobeRef: BrainNodeRef = { id: nodeId, kind: 'lobe', label: project };
      return computeStats({ nodes, edges, parentPath: [lobeRef] });
    }

    if (view === 'temas') {
      if (nodeId.startsWith('lobe:')) {
        const project = nodeId.slice('lobe:'.length);
        const nodes = buildNeurons(project, rawNodes);
        const idToAgg = new Map<number, string>();
        for (const neuron of nodes) {
          for (const srcId of neuron.sourceIds ?? []) {
            idToAgg.set(srcId, neuron.id);
          }
        }
        const edges = aggregateEdges(rawEdges, idToAgg);
        const lobeRef: BrainNodeRef = { id: nodeId, kind: 'lobe', label: project };
        return computeStats({ nodes, edges, parentPath: [lobeRef] });
      }

      if (nodeId.startsWith('neuron:')) {
        const nodes = buildNeuronChildren(nodeId, rawNodes);
        const idToAgg = new Map<number, string>();
        for (const n of nodes) {
          const srcId = n.sourceIds?.[0];
          if (srcId !== undefined) idToAgg.set(srcId, `obs:${srcId}`);
        }
        const edges = aggregateEdges(rawEdges, idToAgg);

        // Parse neuron id for breadcrumb
        const rest = nodeId.slice('neuron:'.length);
        const slashIdx = rest.indexOf('/');
        const project = slashIdx >= 0 ? rest.slice(0, slashIdx) : rest;
        const topicKey = slashIdx >= 0 ? rest.slice(slashIdx + 1) : '';
        const lobeRef: BrainNodeRef = { id: `lobe:${project}`, kind: 'lobe', label: project };
        const neuronRef: BrainNodeRef = { id: nodeId, kind: 'neuron', label: topicKey };
        return computeStats({ nodes, edges, parentPath: [lobeRef, neuronRef] });
      }

      return makeEmptyLevel([]);
    }

    // organico — thread the assignment from buildClusters() to avoid re-clustering (I3)
    if (nodeId.startsWith('cluster:') || nodeId.startsWith('lobe:') || nodeId.startsWith('neuron:')) {
      const nodes = buildClusterChildren(nodeId, rawNodes, organicoAssignment);
      const idToAgg = new Map<number, string>();
      for (const n of nodes) {
        const srcId = n.sourceIds?.[0];
        if (srcId !== undefined) idToAgg.set(srcId, `obs:${srcId}`);
      }
      const edges = aggregateEdges(rawEdges, idToAgg);
      const parentRef: BrainNodeRef = { id: nodeId, kind: 'cluster', label: nodeId };
      return computeStats({ nodes, edges, parentPath: [parentRef] });
    }

    return makeEmptyLevel([]);
  }

  // ---------------------------------------------------------------------------
  // BrainModel implementation
  // ---------------------------------------------------------------------------

  return {
    view,

    root(filters: ActiveFacets): BrainLevel {
      const key = memoKey('root', '', filters);
      if (memo.has(key)) return memo.get(key)!;
      const level = buildRoot(filters);
      memo.set(key, level);
      return level;
    },

    children(nodeId: string, filters: ActiveFacets): BrainLevel {
      const key = memoKey('children', nodeId, filters);
      if (memo.has(key)) return memo.get(key)!;
      const level = buildChildren(nodeId, filters);
      memo.set(key, level);
      return level;
    },

    search(query: string, _filters: ActiveFacets): BrainNodeRef[] {
      const q = query.toLowerCase();
      if (!q) return [];
      const refs: BrainNodeRef[] = [];
      for (const n of rawNodes) {
        if ((n.label ?? '').toLowerCase().includes(q)) {
          refs.push({ id: `obs:${n.id}`, kind: 'observation', label: n.label ?? `obs:${n.id}` });
        }
      }
      return refs;
    },

    availableFacets(): FacetGroupId[] {
      return computeAvailableFacets(rawNodes as Parameters<typeof computeAvailableFacets>[0]);
    },
  };
}
