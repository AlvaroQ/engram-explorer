// Floating project-title overlay.
//
// Renders a PROJECT name as an animated DOM label (drei <Html>) centered over
// the centroid of that project's node cluster — "the title floats over the
// cumulus that represents it". Driven by a plain `project` string so the same
// component serves both the SELECTED node's project (persistent) and the
// HOVERED node's project (transient) — see graph-scene LegacyScene.
//
// It is a pure overlay: pointer-events-none, so it never intercepts canvas
// raycasting. drei <Html> reprojects the world anchor to a 2D screen point each
// frame, so the label tracks the cluster as the camera orbits — without any 3D
// tilt/scale (flat, follows the 2D center). The entrance animation replays only
// when the PROJECT changes (keyed below).
import { useMemo } from 'react';
import { Html } from '@react-three/drei';
import type { GraphNode } from '../../lib/api.ts';
import { buildHexPalette, NULL_HEX } from './node-colors.ts';
import { useReducedMotion } from './motion/interpolator.ts';

// World-space margin above the cluster's TOP node. Small enough that the label
// rests just over the cumulus (not detached high above it), but clear of the
// node mass (not sitting on top of the nodes themselves).
const TITLE_TOP_MARGIN = 2;

interface ProjectTitleOverlayProps {
  nodes: GraphNode[];
  positions: Float32Array | null;
  /** Project whose cluster the title floats over. null/undefined = hidden. */
  project?: string | null | undefined;
  /** Quieter styling for the transient hover title vs the primary selected one. */
  subtle?: boolean | undefined;
}

export function ProjectTitleOverlay({
  nodes,
  positions,
  project,
  subtle = false,
}: ProjectTitleOverlayProps) {
  const reducedMotion = useReducedMotion();

  // Project color from the SAME palette the rail + instanced nodes use, so the
  // title's accent always agrees with how the cluster is colored.
  const color = useMemo(() => {
    if (project == null) return NULL_HEX;
    return buildHexPalette(nodes.map((n) => n.project)).get(project) ?? NULL_HEX;
  }, [nodes, project]);

  // Anchor = horizontal centroid of the project's nodes, lifted just above the
  // cluster's TOP node — the label rests over the cumulus without being detached.
  const anchor = useMemo<[number, number, number] | null>(() => {
    if (project == null || !positions) return null;
    let sumX = 0;
    let sumZ = 0;
    let maxY = -Infinity;
    let count = 0;
    for (let i = 0; i < nodes.length; i++) {
      if (nodes[i]?.project !== project) continue;
      sumX += positions[i * 3] ?? 0;
      const y = positions[i * 3 + 1] ?? 0;
      sumZ += positions[i * 3 + 2] ?? 0;
      if (y > maxY) maxY = y;
      count++;
    }
    if (count === 0) return null;
    return [sumX / count, maxY + TITLE_TOP_MARGIN, sumZ / count];
  }, [nodes, positions, project]);

  if (project == null || anchor == null) return null;

  return (
    // zIndexRange tops out at 19 so the floating title stays ABOVE the side rails
    // (z-10) but never renders OVER the node detail dialog (z-20) — otherwise it
    // covers the panel content, e.g. the project dropdown when reassigning.
    <Html position={anchor} center zIndexRange={[19, 0]} style={{ pointerEvents: 'none' }}>
      {/* key on project → the entrance animation replays only when the project
          changes, not when selecting/hovering another node within the same project. */}
      <div key={project} className={reducedMotion ? '' : 'animate-brain-title-float'}>
        <div
          className={[
            'relative overflow-hidden flex items-center whitespace-nowrap select-none',
            'rounded-full border shadow-xl backdrop-blur-md',
            subtle ? 'px-3.5 py-1 bg-surface/60' : 'px-4 py-1.5 bg-surface/70',
            reducedMotion ? '' : 'animate-brain-title-in',
          ].join(' ')}
          // Hover (non-selected) title reads darker than the active one, mirroring
          // how non-matched nodes are dimmed in the scene.
          style={{
            borderColor: color,
            opacity: subtle ? 0.9 : 1,
            filter: subtle ? 'brightness(0.55)' : undefined,
          }}
        >
          {/* Project-color tint behind the text — the shared "colored glass" dialog
              pattern (same treatment as the hover tooltip) so both clearly read as
              the project's color. */}
          <span
            aria-hidden
            className="pointer-events-none absolute inset-0 -z-10"
            style={{ backgroundColor: color, opacity: 0.18 }}
          />
          <span
            className={`${subtle ? 'text-sm' : 'text-base'} font-semibold tracking-tight`}
            style={{ color }}
          >
            {project}
          </span>
        </div>
      </div>
    </Html>
  );
}
