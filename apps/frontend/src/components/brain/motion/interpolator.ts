/**
 * interpolator.ts — Easing registry, DURATIONS, useReducedMotion hook, and tween() API.
 *
 * Design contracts:
 * - All easing fns: f(0) === 0, f(1) === 1, monotonically non-decreasing.
 * - DURATIONS is the single source of truth for animation timings across the brain module.
 * - useReducedMotion() is the ONLY place that queries prefers-reduced-motion.
 * - tween<T>(): when reducedMotion is true, effective duration ≤ 50 ms (M5 contract).
 * - tween<T>() supports T = number | THREE.Vector3 | THREE.Color.
 * - TweenHandle.step(deltaMs): frame-driven advancement — called by motionStore.tick().
 *   No setInterval is used in production; the R3F useFrame is the sole clock.
 */

import * as THREE from 'three'

// ---------------------------------------------------------------------------
// Easing functions
// ---------------------------------------------------------------------------

/** Decelerate exponentially: fast start, slow finish. Use for camera fly-to. */
export function easeOutExpo(t: number): number {
  if (t === 0) return 0
  if (t === 1) return 1
  return 1 - Math.pow(2, -10 * t)
}

/** Symmetric cubic ease in-out: slow start, fast middle, slow finish. */
export function easeInOutCubic(t: number): number {
  return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2
}

/** Decelerate cubically: fast start, slow finish. Use for node entrance. */
export function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3)
}

/** No easing: constant speed. */
export function linear(t: number): number {
  return t
}

// ---------------------------------------------------------------------------
// Duration constants (ms) — no other file may define animation durations inline
// ---------------------------------------------------------------------------

export const DURATIONS = {
  cameraFlyTo: 700,
  nodeEnter: 400,
  dimBrighten: 300,
  /** Drill-in "node opens" burst: children expand from the parent's position. */
  nodeBurst: 600,
} as const

// ---------------------------------------------------------------------------
// useReducedMotion — single source of prefers-reduced-motion
// ---------------------------------------------------------------------------

/**
 * Returns true when the user has requested reduced motion.
 * This is a plain function (not a React hook) because it is also called from
 * the tween() helper outside of a React component context.
 * When used inside a React component it MUST be called at the top level
 * (same rules as a hook: read once per tree mount via matchMedia).
 *
 * Components MUST NOT query prefers-reduced-motion directly — use this instead.
 */
export function useReducedMotion(): boolean {
  if (typeof globalThis.window === 'undefined') return false
  return globalThis.window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

// ---------------------------------------------------------------------------
// TweenSpec and TweenHandle
// ---------------------------------------------------------------------------

export type Easing = (t: number) => number

export interface TweenSpec<T> {
  from: T
  to: T
  durationMs: number
  easing?: Easing
}

export interface TweenHandle {
  /** Cancel the tween without calling onComplete. */
  cancel(): void
  /**
   * Advance the tween by deltaMs milliseconds.
   * Called by motionStore.tick() on every R3F frame.
   * No-op if already done or cancelled.
   */
  step(deltaMs: number): void
  /** True once the tween has completed or been cancelled. */
  readonly done: boolean
}

// ---------------------------------------------------------------------------
// Interpolation helpers for supported types
// ---------------------------------------------------------------------------

function interpolateNumber(from: number, to: number, t: number): number {
  return from + (to - from) * t
}

function interpolateVector3(from: THREE.Vector3, to: THREE.Vector3, t: number): THREE.Vector3 {
  return new THREE.Vector3(
    interpolateNumber(from.x, to.x, t),
    interpolateNumber(from.y, to.y, t),
    interpolateNumber(from.z, to.z, t),
  )
}

function interpolateColor(from: THREE.Color, to: THREE.Color, t: number): THREE.Color {
  return new THREE.Color(
    interpolateNumber(from.r, to.r, t),
    interpolateNumber(from.g, to.g, t),
    interpolateNumber(from.b, to.b, t),
  )
}

function interpolate<T>(from: T, to: T, t: number): T {
  if (typeof from === 'number' && typeof to === 'number') {
    return interpolateNumber(from, to, t) as T
  }
  if (from instanceof THREE.Vector3 && to instanceof THREE.Vector3) {
    return interpolateVector3(from, to, t) as T
  }
  if (from instanceof THREE.Color && to instanceof THREE.Color) {
    return interpolateColor(from, to, t) as T
  }
  // Fallback: snap to target
  return t >= 1 ? to : from
}

// ---------------------------------------------------------------------------
// tween<T> — frame-driven tween
//
// The handle's step(deltaMs) method is the sole clock mechanism.
// motionStore.tick() calls step() on every R3F frame via TweenDriver's useFrame.
// No setInterval is used — the R3F frameloop is the production clock.
// ---------------------------------------------------------------------------

/**
 * Create a tween from `spec.from` to `spec.to`.
 *
 * @param spec          - Tween spec (from, to, durationMs, optional easing).
 * @param onUpdate      - Called on every step with the interpolated value.
 * @param onComplete    - Called once when the tween finishes naturally (not on cancel).
 * @param reducedMotion - When true, clamps effective duration to ≤ 50 ms (M5).
 * @returns TweenHandle with cancel(), step(deltaMs), and done property.
 */
export function tween<T>(
  spec: TweenSpec<T>,
  onUpdate: (value: T) => void,
  onComplete?: () => void,
  reducedMotion = false,
): TweenHandle {
  const effectiveDuration = reducedMotion ? Math.min(spec.durationMs, 50) : spec.durationMs
  const easingFn = spec.easing ?? linear

  let elapsed = 0
  let isDone = false

  const handle: TweenHandle = {
    step(deltaMs: number): void {
      if (isDone) return
      elapsed += deltaMs
      const rawT = Math.min(elapsed / effectiveDuration, 1)
      const easedT = easingFn(rawT)
      const value = interpolate(spec.from, spec.to, easedT)
      onUpdate(value)
      if (rawT >= 1) {
        isDone = true
        onComplete?.()
      }
    },

    cancel() {
      if (isDone) return
      isDone = true
    },

    get done() {
      return isDone
    },
  }

  return handle
}
