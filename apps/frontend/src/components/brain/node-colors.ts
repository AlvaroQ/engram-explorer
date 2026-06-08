// Single source of truth for node colors, shared by the 3D instanced material
// (as THREE.Color), the detail-panel badge, and the legend (as CSS hex/hsl).
// C6: Also exports buildLegendMap for the segmentation panel legend strip.
import type { GraphNode } from '../../lib/api.ts'

export type ColorBy = 'project' | 'type'

// Muted gray used for null/unknown values in dynamic palettes.
export const NULL_HEX = '#4b5563' // gray-600

// Stable key used to group / filter nodes that have no value under the active
// colorBy (e.g. observations with no project). Kept distinct from any real value.
export const NULL_KEY = ' null'

// Build a hue-evenly-distributed palette (CSS hsl strings) for a set of values.
// Same ordering as the legend so swatches and nodes always agree.
export function buildHexPalette(values: readonly (string | null | undefined)[]): Map<string, string> {
  const sorted = [...new Set(values.filter((v): v is string => v != null))].sort()
  const palette = new Map<string, string>()
  const n = sorted.length || 1
  sorted.forEach((v, i) => {
    const hue = (i * 360) / n
    palette.set(v, `hsl(${hue}, 65%, 60%)`)
  })
  return palette
}

// The value a node is grouped by under a given colorBy mode (null when absent).
export function dynamicKey(node: GraphNode, colorBy: ColorBy): string | null {
  if (colorBy === 'project') return node.project ?? null
  if (colorBy === 'type') return node.type ?? null
  return null
}

// The legend/filter key for a node under the active colorBy. Always a string
// (NULL_KEY for missing values) so it can live in a Set of hidden keys.
export function groupKey(node: GraphNode, colorBy: ColorBy): string {
  return dynamicKey(node, colorBy) ?? NULL_KEY
}

// Resolve a single node's CSS color under the active colorBy, using the same
// palette the 3D scene draws. Lets overlays (detail-panel badge) match the node.
export function nodeColor(
  node: GraphNode,
  nodes: readonly GraphNode[],
  colorBy: ColorBy,
): string {
  const palette = buildHexPalette(nodes.map((n) => dynamicKey(n, colorBy)))
  const key = dynamicKey(node, colorBy)
  return (key != null ? palette.get(key) : undefined) ?? NULL_HEX
}

// ---------------------------------------------------------------------------
// Legend mapping (C6)
// ---------------------------------------------------------------------------

export interface LegendEntry {
  /** The human-readable label (project name or type name). */
  label: string;
  /** The CSS color for this legend entry. */
  color: string;
}

/**
 * Build a legend map from the active node set and colorBy.
 *
 * Returns an array of { label, color } sorted by label, suitable for
 * rendering in the segmentation panel's legend strip.
 */
export function buildLegendMap(
  nodes: readonly GraphNode[],
  colorBy: ColorBy,
): LegendEntry[] {
  if (nodes.length === 0) return [];

  const palette = buildHexPalette(nodes.map((n) => dynamicKey(n, colorBy)));
  const entries: LegendEntry[] = [];

  for (const [label, color] of palette.entries()) {
    entries.push({ label, color });
  }

  return entries.sort((a, b) => a.label.localeCompare(b.label));
}
