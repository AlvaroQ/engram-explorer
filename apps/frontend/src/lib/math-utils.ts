/**
 * math-utils.ts — Shared math helpers used across the brain graph renderers.
 */

/**
 * Deterministic [0,1) pseudo-random based on a single numeric seed.
 * Uses a sin-based hash so rebuilds don't reshuffle particle phases/speeds.
 */
export function hash01(n: number): number {
  const x = Math.sin(n * 127.1) * 43758.5453;
  return x - Math.floor(x);
}
