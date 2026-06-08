/**
 * brain-loading-overlay.tsx — Loading animation shown over the brain canvas.
 *
 * Covers the whole "tab clicked → nodes visible" gap: the graph query, the lazy
 * three.js chunk load, AND the force-layout worker computing node positions.
 * Pure DOM/CSS (no R3F) so it can render before the canvas mounts and stay crisp.
 *
 * Animation: expanding "synapse" rings + orbiting node dots around a pulsing core,
 * evoking a knowledge graph assembling itself. Respects prefers-reduced-motion via
 * Tailwind's motion-safe/motion-reduce variants (animations collapse to a static
 * cluster + label when the user opts out).
 */
import type { JSX } from 'react';
import { useTranslation } from 'react-i18next';

// Six orbiting "node" dots evenly spaced around the core (60° apart).
const ORBIT_DOTS = [0, 60, 120, 180, 240, 300];
// Three expanding rings, staggered so a new pulse leaves before the previous fades.
const RINGS = [0, 1, 2];
const ORBIT_RADIUS_PX = 44;

export function BrainLoadingOverlay(): JSX.Element {
  const { t } = useTranslation();

  return (
    <div
      className="absolute inset-0 z-20 flex items-center justify-center bg-black/80 backdrop-blur-sm"
      role="status"
      aria-live="polite"
      aria-label={t('brain.loading.label')}
    >
      {/* Label pinned to the top of the canvas area, in the accent color so it
          reads as one piece with the animation below. */}
      <p className="absolute top-6 left-1/2 -translate-x-1/2 text-sm font-medium text-accent">
        {t('brain.loading.label')}
      </p>

      <div className="relative h-28 w-28">
        {/* Expanding synapse rings */}
        {RINGS.map((i) => (
          <span
            key={`ring-${i}`}
            className="absolute inset-0 rounded-full border border-accent/40 motion-safe:animate-ping motion-reduce:opacity-30"
            style={{ animationDuration: '2.1s', animationDelay: `${i * 0.7}s` }}
          />
        ))}

        {/* Orbiting node dots — the spinning container carries them around the core */}
        <div
          className="absolute inset-0 motion-safe:animate-spin motion-reduce:animate-none"
          style={{ animationDuration: '3s' }}
        >
          {ORBIT_DOTS.map((deg) => (
            <span
              key={`dot-${deg}`}
              className="absolute left-1/2 top-1/2 h-2.5 w-2.5 rounded-full bg-accent/80 shadow-sm shadow-accent/60"
              style={{ transform: `translate(-50%, -50%) rotate(${deg}deg) translateY(-${ORBIT_RADIUS_PX}px)` }}
            />
          ))}
        </div>

        {/* Pulsing core */}
        <span className="absolute left-1/2 top-1/2 h-5 w-5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-accent shadow-lg shadow-accent/60 motion-safe:animate-pulse" />
      </div>
    </div>
  );
}
