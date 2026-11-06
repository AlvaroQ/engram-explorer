/**
 * use-tween.ts — Single master useFrame that drives all brain motion.
 *
 * Design contracts (C5, ADR-2, ADR-3):
 * - Exactly ONE useFrame in the entire R3F scene calls invalidate().
 * - invalidate() is called ONLY while motionStore.hasActive() is true, or
 *   while the caller allows ambient motion (`ambient` prop, see
 *   frame-demand.ts / use-ambient-motion.ts) — the canvas frameloop is always
 *   "demand", so this is what keeps the edge particles flowing (S9).
 * - No setState call anywhere in this file.
 * - Consumers register tweens via motionStore.register(handle); this hook
 *   takes care of ticking the store and invalidating the frame demand loop.
 * - document.hidden guard: when the tab is hidden, skip tick and invalidate
 *   so no work is done while the user cannot see the result.
 *
 * Usage: mount <TweenDriver /> once inside the R3F <Canvas>. Placement does
 * not matter — it registers a useFrame and returns null.
 */

import { useFrame, invalidate } from '@react-three/fiber';
import { motionStore } from './motion-store';
import { shouldInvalidateFrame } from './frame-demand.ts';

/**
 * R3F component that provides the single master animation driver.
 * Mount exactly once inside the brain <Canvas>.
 *
 * Returns null — purely a side-effect component.
 */
export function TweenDriver({ ambient = false }: { ambient?: boolean } = {}): null {
  useFrame((_state, delta) => {
    const hidden = typeof document !== 'undefined' && document.hidden;
    // Skip advancing or invalidating when the tab is hidden (CB3 guard).
    if (hidden) return;

    // delta is in seconds; motion-store.tick() expects milliseconds.
    motionStore.tick(delta * 1000);

    // Keep the demand frameloop running while tweens are active or ambient
    // motion is allowed; otherwise do nothing and the frameloop goes idle.
    if (shouldInvalidateFrame({ tweensActive: motionStore.hasActive(), ambient, hidden })) {
      invalidate();
    }
  });

  return null;
}
