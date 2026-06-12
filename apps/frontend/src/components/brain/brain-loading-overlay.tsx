/**
 * brain-loading-overlay.tsx — Loading animation shown over the brain canvas.
 *
 * Covers the whole "tab clicked → nodes visible" gap: the graph query, the lazy
 * three.js chunk load, AND the force-layout worker computing node positions.
 * Pure DOM/CSS (no R3F) so it can render before the canvas mounts and stay crisp.
 *
 * Animation: expanding "synapse" rings radiating from the Engram elephant logo,
 * which gently pulses at the center — evoking memories firing out of the brain.
 * Respects prefers-reduced-motion via Tailwind's motion-safe/motion-reduce
 * variants (animations collapse to a static logo + label when the user opts out).
 */
import type { JSX } from 'react';
import { useTranslation } from 'react-i18next';

// Three expanding rings, staggered so a new pulse leaves before the previous fades.
const RINGS = [0, 1, 2];

// label overrides the default "loading the knowledge graph" caption and logoSrc
// overrides the centre logo, so the same animation can front other slow loads
// (e.g. the Claude Code usage scan on the CC overview, which uses the Claude
// logo) with a context-appropriate message.
export function BrainLoadingOverlay({
  label,
  logoSrc = '/static/engram.png',
}: { label?: string; logoSrc?: string } = {}): JSX.Element {
  const { t } = useTranslation();
  const caption = label ?? t('brain.loading.label');

  return (
    <div
      className="absolute inset-0 z-20 flex items-center justify-center bg-black/80 backdrop-blur-sm"
      role="status"
      aria-live="polite"
      aria-label={caption}
    >
      {/* Label pinned to the top of the canvas area, in the accent color so it
          reads as one piece with the animation below. */}
      <p className="absolute top-6 left-1/2 -translate-x-1/2 text-sm font-medium text-accent">
        {caption}
      </p>

      <div className="relative flex h-28 w-28 items-center justify-center">
        {/* Expanding synapse rings radiating out from the logo */}
        {RINGS.map((i) => (
          <span
            key={`ring-${i}`}
            className="absolute inset-0 rounded-full border border-accent/40 motion-safe:animate-ping motion-reduce:opacity-30"
            style={{ animationDuration: '2.1s', animationDelay: `${i * 0.7}s` }}
          />
        ))}

        {/* Engram elephant logo at the center, gently pulsing */}
        <img
          src={logoSrc}
          alt=""
          aria-hidden="true"
          className="relative h-14 w-14 object-contain drop-shadow-[0_0_10px_hsl(var(--accent)/0.6)] motion-safe:animate-pulse"
        />
      </div>
    </div>
  );
}
