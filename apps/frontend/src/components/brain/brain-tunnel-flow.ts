/**
 * brain-tunnel-flow.ts — Pure config, constants, and functions for BrainTunnelFlow.
 *
 * This module is separated from the R3F component (brain-tunnel-flow.tsx) so that
 * the pure logic is unit-testable (no DOM, no Three.js, no R3F imports).
 *
 * Implements:
 * - Sub-slice 5a: TunnelFocusLayer opacity logic (S5.4 / C5.1–C5.2)
 * - Sub-slice 5b: Relation-color constants + ambient opacity constants (C5.3 / C5.4)
 * - Sub-slice 5c: Tube radius encoding from edge.weight (C5.5 / C5.6)
 *
 * graph3d-viz hard rules honored:
 * - No gl.lineWidth for thickness (C6) — TubeGeometry used in the .tsx consumer
 * - All opacity transitions must route through motion-store (S3.4 / C5)
 */

import type { EdgeFamily, GraphEdgeRelation } from './types.ts';

// ---------------------------------------------------------------------------
// Sub-slice 5b: Ambient opacity constants (S5.3 / C5.4)
// These are the ONLY place these values are defined. No other file may inline them.
// ---------------------------------------------------------------------------

/** Minimum ambient opacity for topic-road (dashed) edges. */
export const SYNAPSE_AMBIENT_TOPIC_ROAD_MIN = 0.12;

/** Maximum ambient opacity for topic-road (dashed) edges. */
export const SYNAPSE_AMBIENT_TOPIC_ROAD_MAX = 0.18;

/** Minimum ambient opacity for semantic (solid) edges. */
export const SYNAPSE_AMBIENT_SEMANTIC_MIN = 0.3;

/** Maximum ambient opacity for semantic (solid) edges. */
export const SYNAPSE_AMBIENT_SEMANTIC_MAX = 0.5;

/** Focus opacity for incident edges when a node is hovered/selected. */
export const SYNAPSE_FOCUS_SEMANTIC_OPACITY = 0.85;

/** Focus opacity for topic-road incident edges. */
export const SYNAPSE_FOCUS_TOPIC_ROAD_OPACITY = 0.55;

// ---------------------------------------------------------------------------
// Sub-slice 5b: Relation → color constants (C5.3)
// Extend the EDGE_COLORS from legacy tunnel-flow.tsx with BrainEdge relations.
// ---------------------------------------------------------------------------

/**
 * Per-relation color constants for BrainEdge synapse rendering.
 * color A = related (semantic, blue)
 * color B = compatible (semantic, emerald)
 * color C = scoped (topic-road, amber)
 * neutral = undefined / no-relation fallback
 */
export const SYNAPSE_COLORS: Record<GraphEdgeRelation | 'neutral', string> = {
  related: '#60a5fa', // blue-400  (color A)
  compatible: '#34d399', // emerald-400 (color B)
  scoped: '#fbbf24', // amber-400 (color C)
  neutral: '#6b80a0', // desaturated slate-blue (neutral fallback)
};

// ---------------------------------------------------------------------------
// Sub-slice 5c: Tube radius constants (C5.5)
// Thickness via TubeGeometry — NOT gl.lineWidth (C6 contract).
// ---------------------------------------------------------------------------

/** Minimum tube radius (config constant, not hardcoded). */
export const TUBE_RADIUS_MIN = 0.04;

/** Per-unit-weight radius increment (config constant). */
export const TUBE_RADIUS_SCALE = 0.015;

// ---------------------------------------------------------------------------
// Sub-slice 5c: computeTubeRadius (C5.5)
// ---------------------------------------------------------------------------

/**
 * Map BrainEdge.weight to a tube geometry radius.
 * Formula: TUBE_RADIUS_MIN + weight * TUBE_RADIUS_SCALE
 */
export function computeTubeRadius(weight: number): number {
  return TUBE_RADIUS_MIN + weight * TUBE_RADIUS_SCALE;
}

// ---------------------------------------------------------------------------
// Sub-slice 5a: computeEdgeOpacity (S5.4 / C5.1–C5.2)
// ---------------------------------------------------------------------------

/**
 * Compute the target opacity for an edge in a given focus state.
 *
 * @param family     - Edge rendering family ('semantic' | 'topic-road')
 * @param isIncident - True when both endpoints include the hovered/selected node
 * @param hoveredId  - Non-null when a node is hovered or selected (any non-null
 *                     triggers focus mode; the caller is responsible for the
 *                     isIncident predicate)
 *
 * Rules (S5.4):
 * - No focus (hoveredId = null): return midpoint of the ambient range
 * - Focus + incident: return FOCUS opacity (elevated)
 * - Focus + non-incident: return ≤ ambient_midpoint / 2 (dimmed per spec)
 */
export function computeEdgeOpacity(
  family: EdgeFamily,
  isIncident: boolean,
  hoveredId: string | null,
): number {
  const [ambientMin, ambientMax, focusOpacity] =
    family === 'semantic'
      ? [SYNAPSE_AMBIENT_SEMANTIC_MIN, SYNAPSE_AMBIENT_SEMANTIC_MAX, SYNAPSE_FOCUS_SEMANTIC_OPACITY]
      : [
          SYNAPSE_AMBIENT_TOPIC_ROAD_MIN,
          SYNAPSE_AMBIENT_TOPIC_ROAD_MAX,
          SYNAPSE_FOCUS_TOPIC_ROAD_OPACITY,
        ];

  const ambientMid = (ambientMin + ambientMax) / 2;

  if (hoveredId === null) {
    // Ambient state: return midpoint of range
    return ambientMid;
  }

  if (isIncident) {
    // Focus state: elevated opacity
    return focusOpacity;
  }

  // Dimmed state: ≤ ambient_mid / 2 (spec: "dim to ≤ ambient/2")
  return ambientMid / 2;
}

// ---------------------------------------------------------------------------
// Sub-slice 5c: computeEdgeBrightness (C5.6)
// ---------------------------------------------------------------------------

/**
 * Map BrainEdge.confidence to an opacity/emissive multiplier.
 * Defaults to 0.5 when confidence is undefined (spec: "confidence ?? 0.5").
 * Clamps to [0, 1].
 */
export function computeEdgeBrightness(confidence: number | undefined): number {
  const raw = confidence ?? 0.5;
  return Math.max(0, Math.min(1, raw));
}

// ---------------------------------------------------------------------------
// Helper: resolve edge color from BrainEdge.relation
// ---------------------------------------------------------------------------

/**
 * Map the dominant relation of a BrainEdge bundle to its display color.
 * Falls back to 'neutral' when relation is undefined.
 */
export function resolveEdgeColor(relation: GraphEdgeRelation | undefined): string {
  if (relation === undefined) return SYNAPSE_COLORS.neutral;
  return SYNAPSE_COLORS[relation];
}
