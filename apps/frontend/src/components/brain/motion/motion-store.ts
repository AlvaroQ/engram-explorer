/**
 * motion-store.ts — Plain mutable singleton tween registry.
 *
 * Design contracts (ADR-3, C5):
 * - NOT a Zustand store. A module-level mutable object to avoid React re-renders per frame.
 * - register(handle): add a tween handle to the active set.
 * - tick(deltaMs): call step(deltaMs) on each active handle, THEN sweep done handles.
 * - hasActive(): true while at least one tween is running.
 * - cancelAll(): cancel all active tweens (e.g. on level change or unmount).
 *
 * The ONLY place that calls invalidate() is use-tween.ts (TweenDriver), which drives
 * this store from a single useFrame callback (C5 contract).
 */

import type { TweenHandle } from './interpolator';

interface MotionStore {
  register(handle: TweenHandle): void;
  tick(deltaMs: number): void;
  hasActive(): boolean;
  cancelAll(): void;
}

function createMotionStore(): MotionStore {
  // Plain mutable set — never a React state setter target.
  const handles = new Set<TweenHandle>();

  return {
    register(handle: TweenHandle): void {
      if (!handle.done) {
        handles.add(handle);
      }
    },

    tick(deltaMs: number): void {
      // Step all active handles first, then sweep any that are now done.
      for (const handle of handles) {
        if (!handle.done) {
          handle.step(deltaMs);
        }
      }
      // Sweep completed handles after stepping (including those that just finished).
      for (const handle of handles) {
        if (handle.done) {
          handles.delete(handle);
        }
      }
    },

    hasActive(): boolean {
      // Sweep any handles that finished between ticks, then report.
      for (const handle of handles) {
        if (handle.done) {
          handles.delete(handle);
        }
      }
      return handles.size > 0;
    },

    cancelAll(): void {
      for (const handle of handles) {
        handle.cancel();
      }
      handles.clear();
    },
  };
}

/** Module-level singleton — the ONE motion manager for the entire brain scene. */
export const motionStore: MotionStore = createMotionStore();
