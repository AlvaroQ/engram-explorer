/**
 * frame-demand.ts — When the brain canvas needs a new frame (S9).
 *
 * The canvas always runs R3F's `frameloop="demand"`: nothing renders unless
 * something calls invalidate(). TweenDriver (use-tween.ts) stays the single
 * invalidator and uses these pure rules to decide whether to keep the loop
 * alive: while a tween runs, or while the ambient edge-particle flow is
 * allowed. Ambient motion is off for reduced-motion users, hidden tabs and
 * canvases scrolled out of view, so an idle brain costs no GPU time.
 */

export interface AmbientInputs {
  /** Caller wants continuous ambient motion (GraphScene `animate`). */
  animate: boolean;
  /** prefers-reduced-motion: reduce */
  reducedMotion: boolean;
  /** document.hidden */
  hidden: boolean;
  /** Canvas intersects the viewport. */
  onScreen: boolean;
}

export function ambientMotionActive(i: AmbientInputs): boolean {
  return i.animate && !i.reducedMotion && !i.hidden && i.onScreen;
}

export interface FrameInputs {
  tweensActive: boolean;
  ambient: boolean;
  hidden: boolean;
}

export function shouldInvalidateFrame(i: FrameInputs): boolean {
  if (i.hidden) return false;
  return i.tweensActive || i.ambient;
}
