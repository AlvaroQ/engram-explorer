/**
 * types.ts — Shared types for the BrainModel abstraction layer.
 *
 * Zero runtime code — types only. All types are consumed by brain-model.ts,
 * client-brain-model.ts, facets.ts, views/*, and the navigation/rendering layers.
 */

// ---------------------------------------------------------------------------
// Core enumerations
// ---------------------------------------------------------------------------

export type ViewMode = 'lobulos' | 'temas' | 'organico';

/** Structural role of a node in the brain hierarchy. */
export type NodeKind = 'lobe' | 'neuron' | 'cluster' | 'observation';

/** Edge rendering family — determines tube style and ambient opacity range. */
export type EdgeFamily = 'semantic' | 'topic-road';

/** Canonical relation values as produced by the backend (exhaustive). */
export type GraphEdgeRelation = 'related' | 'scoped' | 'compatible';

// ---------------------------------------------------------------------------
// Node types
// ---------------------------------------------------------------------------

/** Lightweight reference used in breadcrumb/navigation path stacks. */
export interface BrainNodeRef {
  id: string;
  kind: NodeKind;
  label: string;
}

/**
 * A node in a BrainLevel — may be a lobe, neuron, cluster, or leaf observation.
 *
 * Leaf contract (C2): childCount === 0 ⟺ kind === 'observation'. Selecting a
 * leaf opens the detail panel without pushing a canvas level or triggering a
 * camera fly-to.
 */
export interface BrainNode {
  /** Synthetic id for aggregates; "obs:{numericId}" for leaves. */
  id: string;
  kind: NodeKind;
  label: string;
  /** CSS color resolved per active color-by setting. */
  color: string;
  /** Aggregate: member count. Leaf: raw observation weight. */
  weight: number;
  /** 0 ⟹ leaf. */
  childCount: number;
  meta: Record<string, unknown>;
  /** Underlying observation ids (for stats and detail queries). */
  sourceIds?: number[];
}

// ---------------------------------------------------------------------------
// Edge types
// ---------------------------------------------------------------------------

/**
 * An aggregated edge in a BrainLevel.
 *
 * Edge aggregation math (C3): all underlying GraphEdge rows sharing
 * (source-aggregate, target-aggregate, family) are merged into one BrainEdge.
 */
export interface BrainEdge {
  source: string;
  target: string;
  family: EdgeFamily;
  /** Dominant relation in the bundle (most frequent; drives color). */
  relation?: GraphEdgeRelation;
  /** Count of distinct underlying GraphEdge rows → tube radius. */
  weight: number;
  /** Arithmetic mean of underlying confidences → brightness. */
  confidence?: number;
}

// ---------------------------------------------------------------------------
// Level types
// ---------------------------------------------------------------------------

/** Per-level summary statistics shown in the stats strip. */
export interface LevelStats {
  nodeCount: number;
  observationCount: number;
  crossSynapses: number;
  dominantType?: string;
  /** Kind of the nodes at this level — drives the stats noun. */
  nodeKind?: NodeKind;
}

/**
 * The complete data for one rendered level in the brain hierarchy.
 *
 * The renderer posts nodes + edges to the layout worker and renders from the
 * result. parentPath tracks the navigation context for breadcrumb rendering.
 */
export interface BrainLevel {
  nodes: BrainNode[];
  edges: BrainEdge[];
  parentPath: BrainNodeRef[];
  stats: LevelStats;
}

// ---------------------------------------------------------------------------
// Facets
// ---------------------------------------------------------------------------

/**
 * Active facet selections. Each entry is a group → set of selected values.
 * AND-across-groups / OR-within-group composition (see facets.ts).
 */
export interface ActiveFacets {
  project?: string[];
  type?: string[];
  scope?: string[];
  tool?: string[];
  date?: string[];
}

/**
 * Identifiers for the available facet groups.
 *
 * Facet availability (C8): BrainModel.availableFacets() only includes groups
 * whose backing field is present and non-empty in at least one GraphNode.
 */
export type FacetGroupId = 'project' | 'type' | 'scope' | 'tool' | 'date';

// ---------------------------------------------------------------------------
// BrainModel interface
// ---------------------------------------------------------------------------

/**
 * The swappable model interface consumed by all UI components.
 *
 * v1: implemented by client-brain-model.ts (client-side aggregation).
 * v2: implementable against backend hierarchy endpoints with NO UI changes.
 *
 * Memoization (S1.5): root() and children() must be memoized by
 * (view, JSON.stringify(filters), path-key) — same args ⟹ same object ref.
 */
export interface BrainModel {
  view: ViewMode;
  /**
   * Returns the root level for the current view + filters.
   * Never throws; returns a valid BrainLevel with empty arrays when no data.
   *
   * v1: filters reserved for future backend narrowing; client-side dimming
   * handled via computeMatchedIds. The parameter is accepted but not applied.
   */
  root(filters: ActiveFacets): BrainLevel;
  /**
   * Returns the children level for nodeId.
   * Leaf contract (C2): when nodeId is a leaf, returns an empty BrainLevel.
   * Never throws, never returns undefined.
   *
   * v1: filters reserved for future backend narrowing; client-side dimming
   * handled via computeMatchedIds. The parameter is accepted but not applied.
   */
  children(nodeId: string, filters: ActiveFacets): BrainLevel;
  /** Full-text search across the model's observation labels. */
  search(query: string, filters: ActiveFacets): BrainNodeRef[];
  /**
   * Returns the facet group ids whose backing field is present in the loaded
   * data. Tool/Session groups are hidden until the backend exposes those fields.
   */
  availableFacets(): FacetGroupId[];
}
