// WebGL capability probe.
//
// The 3D knowledge graph mounts an R3F <Canvas>, which constructs a
// THREE.WebGLRenderer. When Chrome's "Use graphics acceleration when available"
// is disabled, the browser may refuse to hand out a WebGL context at all
// (getContext returns null), and the renderer construction throws. That failure
// happens inside R3F's commit phase, so it either blanks the canvas or trips the
// generic ErrorBoundary fallback — neither tells the user the real cause.
//
// Probing up-front lets the page show an actionable message AND skip lazy-loading
// the heavy three.js chunk entirely when it could never render.

let cached: boolean | null = null;

/**
 * Returns true when the browser can create a WebGL context. The result is cached
 * because the answer can't change without a page reload (toggling Chrome's
 * hardware-acceleration setting requires a relaunch).
 */
export function isWebGLAvailable(): boolean {
  if (cached !== null) return cached;
  if (typeof document === 'undefined') return false; // SSR / non-DOM guard
  try {
    const canvas = document.createElement('canvas');
    const gl =
      canvas.getContext('webgl2') ??
      canvas.getContext('webgl') ??
      canvas.getContext('experimental-webgl');
    cached = gl != null;
  } catch {
    cached = false;
  }
  return cached;
}
