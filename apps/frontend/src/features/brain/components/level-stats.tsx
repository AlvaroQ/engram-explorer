/**
 * level-stats.tsx — Per-level statistics strip.
 *
 * Displays LevelStats as:
 *   "{label} · {nodeCount} {noun} · {observationCount} obs · {crossSynapses} cross-synapses"
 *
 * The noun is level-appropriate: "lobes" at root, "neurons" at lobe level,
 * "clusters" at cluster level, "obs" at leaf level.
 * The label comes from the last path entry (or "Root" when at root level).
 * Matches spec S6.4 data fields exactly.
 */

import { type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import type { LevelStats, BrainNodeRef, NodeKind } from '../model/types';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface LevelStatsProps {
  stats: LevelStats;
  /** Current navigation path — last entry provides the level label and kind. */
  path: BrainNodeRef[];
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Returns the i18n key suffix for the plural noun at this level.
 * Prefers the real kind of the nodes in the current level (accurate across all
 * views — e.g. a Lóbulos lobe drills straight into observations); falls back to
 * the parent-kind heuristic when the level carries no node kind.
 */
function nodeNounKey(stats: LevelStats, path: BrainNodeRef[]): string {
  switch (stats.nodeKind) {
    case 'lobe': return 'lobes';
    case 'neuron': return 'neurons';
    case 'cluster': return 'clusters';
    case 'observation': return 'obs';
  }
  if (path.length === 0) return 'lobes';
  const parentKind: NodeKind | undefined = path[path.length - 1]?.kind;
  switch (parentKind) {
    case 'lobe': return 'neurons';
    case 'neuron': return 'obs';
    case 'cluster': return 'obs';
    default: return 'nodes';
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Compact stats strip rendered below the canvas controls.
 * Format: "{label} · {nodeCount} {noun} · {observationCount} obs · {crossSynapses} cross-synapses"
 */
export function LevelStatsStrip({ stats, path }: LevelStatsProps): JSX.Element {
  const { t } = useTranslation();
  const lastEntry = path[path.length - 1];
  const label = path.length > 0 && lastEntry ? lastEntry.label : t('brain.levelStats.root');
  const noun = t(`brain.levelStats.${nodeNounKey(stats, path)}`);

  const parts = [
    label,
    `${stats.nodeCount} ${noun}`,
    `${stats.observationCount} ${t('brain.levelStats.obs')}`,
    `${stats.crossSynapses} ${t('brain.levelStats.crossSynapses')}`,
  ];

  if (stats.dominantType) {
    parts.push(stats.dominantType);
  }

  return (
    <p
      className="text-[11px] text-fg-muted"
      aria-label={`Level stats: ${parts.join(', ')}`}
      aria-live="polite"
      aria-atomic="true"
    >
      {parts.join(' · ')}
    </p>
  );
}
