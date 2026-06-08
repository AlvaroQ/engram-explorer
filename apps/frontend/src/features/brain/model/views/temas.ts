/**
 * temas.ts — Temas view grouping.
 *
 * Level 0 (root): one lobe per distinct project.
 * Level 1 (children of lobe): one neuron per distinct topicKey within the project.
 *   Observations without a topicKey are grouped under "neuron:{project}/__untagged__".
 * Level 2 (children of neuron): observation leaves in that topic.
 *
 * Default color-by: 'topic'.
 */

import type { GraphNode } from '../../../../lib/api.ts';
import type { BrainNode, BrainNodeRef } from '../types.ts';
import { buildHexPalette } from '../../../../components/brain/node-colors.ts';
import { obsNode } from './lobulos.ts';
import { buildLobesShared } from './shared.ts';

export const defaultColorBy = 'topic' as const;

/**
 * Build lobe nodes (same as Lóbulos root — one per project).
 * Delegates to buildLobesShared without dominantType (I18).
 */
export function buildLobes(nodes: GraphNode[]): BrainNode[] {
  return buildLobesShared(nodes, /* includeDominantType */ false);
}

/** Build neuron nodes for a given project (Level 1). */
export function buildNeurons(project: string, nodes: GraphNode[]): BrainNode[] {
  const members = nodes.filter((n) => (n.project ?? '∅') === project);

  const byTopic = new Map<string, GraphNode[]>();
  for (const n of members) {
    const key = n.topicKey ?? '__untagged__';
    const bucket = byTopic.get(key);
    if (bucket) bucket.push(n);
    else byTopic.set(key, [n]);
  }

  // Build a simple palette for topics within this project
  const topicKeys = Array.from(byTopic.keys());
  const n = topicKeys.length || 1;
  const topicPalette = new Map<string, string>(
    topicKeys.sort().map((k, i) => [k, `hsl(${(i * 360) / n}, 55%, 55%)`]),
  );

  return Array.from(byTopic.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([topicKey, obs]) => ({
      id: `neuron:${project}/${topicKey}`,
      kind: 'neuron' as const,
      label: topicKey === '__untagged__' ? `${project} (untagged)` : topicKey,
      color: topicPalette.get(topicKey) ?? '#4b5563',
      weight: obs.length,
      childCount: obs.length,
      meta: {
        project,
        topicKey: topicKey === '__untagged__' ? null : topicKey,
        observationCount: obs.length,
      },
      sourceIds: obs.map((m) => m.id),
    }));
}

/**
 * Build observation leaves for a given project + topicKey.
 * Parses the neuronId: "neuron:{project}/{topicKey}".
 */
export function buildNeuronChildren(neuronId: string, nodes: GraphNode[]): BrainNode[] {
  const rest = neuronId.slice('neuron:'.length); // "{project}/{topicKey}"
  const slashIdx = rest.indexOf('/');
  if (slashIdx === -1) return [];
  const project = rest.slice(0, slashIdx);
  const topicKey = rest.slice(slashIdx + 1);

  const palette = buildHexPalette(nodes.map((n) => n.project));

  return nodes
    .filter((n) => {
      const p = n.project ?? '∅';
      const t = n.topicKey ?? '__untagged__';
      return p === project && t === topicKey;
    })
    .map((n) => obsNode(n, palette));
}

/** Synthetic BrainNodeRef for a neuron. */
export function neuronRef(project: string, topicKey: string): BrainNodeRef {
  return {
    id: `neuron:${project}/${topicKey}`,
    kind: 'neuron',
    label: topicKey,
  };
}
