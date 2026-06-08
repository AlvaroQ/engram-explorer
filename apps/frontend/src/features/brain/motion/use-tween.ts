/**
 * use-tween.ts — Single master useFrame that drives all brain motion.
 *
 * Design contracts (C5, ADR-2, ADR-3):
 * - Exactly ONE useFrame in the entire R3F scene calls invalidate().
 * - invalidate() is called ONLY while motionStore.hasActive() is true.
 * - No setState call anywhere in this file.
 * - Consumers register tweens via motionStore.register(handle); this hook
 *   takes care of ticking the store and invalidating the frame demand loop.
 * - document.hidden guard: when the tab is hidden, skip tick and invalidate
 *   so no work is done while the user cannot see the result.
 *
 * Usage: mount <TweenDriver /> once inside the R3F <Canvas>. Placement does
 * not matter — it registers a useFrame and returns null.
 */

import { useFrame, invalidate } from '@react-three/fiber'
import { motionStore } from './motion-store'

/**
 * R3F component that provides the single master animation driver.
 * Mount exactly once inside the brain <Canvas>.
 *
 * Returns null — purely a side-effect component.
 */
export function TweenDriver(): null {
  useFrame((_state, delta) => {
    // Skip advancing or invalidating when the tab is hidden (CB3 guard).
    if (typeof document !== 'undefined' && document.hidden) return

    // delta is in seconds; motion-store.tick() expects milliseconds.
    motionStore.tick(delta * 1000)

    if (motionStore.hasActive()) {
      // Keep the demand frameloop running while tweens are active.
      invalidate()
    }
    // When hasActive() returns false we do nothing — frameloop goes idle.
  })

  return null
}
