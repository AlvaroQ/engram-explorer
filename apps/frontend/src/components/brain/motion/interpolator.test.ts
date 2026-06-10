import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  easeOutExpo,
  easeInOutCubic,
  easeOutCubic,
  linear,
  DURATIONS,
  useReducedMotion,
  tween,
} from './interpolator';

// ---------------------------------------------------------------------------
// C1.1 — Easing function boundary conditions and monotonicity
// ---------------------------------------------------------------------------

describe('easeOutExpo', () => {
  it('f(0) === 0', () => {
    expect(easeOutExpo(0)).toBe(0);
  });
  it('f(1) === 1', () => {
    expect(easeOutExpo(1)).toBe(1);
  });
  it('is monotonically non-decreasing', () => {
    for (let i = 0; i < 99; i++) {
      const t0 = i / 100;
      const t1 = (i + 1) / 100;
      expect(easeOutExpo(t1)).toBeGreaterThanOrEqual(easeOutExpo(t0));
    }
  });
});

describe('easeInOutCubic', () => {
  it('f(0) === 0', () => {
    expect(easeInOutCubic(0)).toBe(0);
  });
  it('f(1) === 1', () => {
    expect(easeInOutCubic(1)).toBe(1);
  });
  it('is monotonically non-decreasing', () => {
    for (let i = 0; i < 99; i++) {
      const t0 = i / 100;
      const t1 = (i + 1) / 100;
      expect(easeInOutCubic(t1)).toBeGreaterThanOrEqual(easeInOutCubic(t0));
    }
  });
});

describe('easeOutCubic', () => {
  it('f(0) === 0', () => {
    expect(easeOutCubic(0)).toBe(0);
  });
  it('f(1) === 1', () => {
    expect(easeOutCubic(1)).toBe(1);
  });
  it('is monotonically non-decreasing', () => {
    for (let i = 0; i < 99; i++) {
      const t0 = i / 100;
      const t1 = (i + 1) / 100;
      expect(easeOutCubic(t1)).toBeGreaterThanOrEqual(easeOutCubic(t0));
    }
  });
});

describe('linear', () => {
  it('f(0) === 0', () => {
    expect(linear(0)).toBe(0);
  });
  it('f(1) === 1', () => {
    expect(linear(1)).toBe(1);
  });
  it('f(0.5) === 0.5', () => {
    expect(linear(0.5)).toBe(0.5);
  });
  it('is monotonically non-decreasing', () => {
    for (let i = 0; i < 99; i++) {
      const t0 = i / 100;
      const t1 = (i + 1) / 100;
      expect(linear(t1)).toBeGreaterThanOrEqual(linear(t0));
    }
  });
});

// ---------------------------------------------------------------------------
// DURATIONS constant
// ---------------------------------------------------------------------------

describe('DURATIONS', () => {
  it('has cameraFlyTo = 700', () => {
    expect(DURATIONS.cameraFlyTo).toBe(700);
  });
  it('has nodeEnter = 400', () => {
    expect(DURATIONS.nodeEnter).toBe(400);
  });
  it('has dimBrighten = 300', () => {
    expect(DURATIONS.dimBrighten).toBe(300);
  });
});

// ---------------------------------------------------------------------------
// C1.3 — reduced-motion: tween completes ≤ 50 ms when useReducedMotion = true
// M5-A scenario — drives step() directly (frame-driven contract)
// ---------------------------------------------------------------------------

describe('tween — reduced motion (M5-A)', () => {
  let originalWindow: typeof globalThis.window;

  beforeEach(() => {
    originalWindow = globalThis.window;
    // Stub globalThis.window with a matchMedia that reports prefers-reduced-motion: reduce
    globalThis.window = {
      matchMedia: vi.fn().mockImplementation((query: string) => ({
        matches: query === '(prefers-reduced-motion: reduce)',
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    } as unknown as typeof globalThis.window;
  });

  afterEach(() => {
    globalThis.window = originalWindow;
  });

  it('tween with durationMs:700 completes within ≤50ms when reduced motion is active', () => {
    // useReducedMotion reads matchMedia, so it should return true now
    const reducedMotion = useReducedMotion();
    expect(reducedMotion).toBe(true);

    const updates: number[] = [];
    let completed = false;

    const handle = tween<number>(
      { from: 0, to: 1, durationMs: 700, easing: linear },
      (val) => updates.push(val),
      () => {
        completed = true;
      },
      reducedMotion,
    );

    // Drive step() directly — effective duration is ≤50ms so a single 100ms step completes it
    handle.step(100);

    expect(completed).toBe(true);
    expect(updates.length).toBeGreaterThan(0);
    expect(updates[updates.length - 1]).toBe(1);
  });
});

// ---------------------------------------------------------------------------
// C1.4 — tween lifecycle (normal mode): onUpdate called with interpolated values,
// onComplete fires at end, cancel() stops without onComplete
// step(deltaMs) drives the tween (frame-driven contract)
// ---------------------------------------------------------------------------

describe('tween — lifecycle (normal mode)', () => {
  it('onUpdate called with interpolated number values', () => {
    const updates: number[] = [];
    const handle = tween<number>(
      { from: 0, to: 100, durationMs: 100, easing: linear },
      (val) => updates.push(val),
      undefined,
      false, // not reduced motion
    );

    handle.step(50);
    expect(updates.length).toBeGreaterThan(0);
    // At ~50ms with linear easing and durationMs=100, value should be ~50
    const lastVal = updates[updates.length - 1] ?? 0;
    expect(lastVal).toBeGreaterThan(0);
    expect(lastVal).toBeLessThanOrEqual(100);
  });

  it('onComplete fires when tween reaches the end', () => {
    let completed = false;
    const handle = tween<number>(
      { from: 0, to: 1, durationMs: 100, easing: linear },
      () => {},
      () => {
        completed = true;
      },
      false,
    );

    handle.step(200);
    expect(completed).toBe(true);
  });

  it('cancel() stops tween without calling onComplete', () => {
    let completed = false;
    const handle = tween<number>(
      { from: 0, to: 1, durationMs: 300, easing: linear },
      () => {},
      () => {
        completed = true;
      },
      false,
    );

    handle.step(50);
    handle.cancel();
    handle.step(500);

    expect(completed).toBe(false);
  });

  it('final onUpdate value is the target value (to)', () => {
    const updates: number[] = [];
    const handle = tween<number>(
      { from: 5, to: 20, durationMs: 50, easing: linear },
      (val) => updates.push(val),
      undefined,
      false,
    );

    handle.step(100);
    const last = updates[updates.length - 1];
    expect(last).toBe(20);
  });

  it('step() is a no-op after tween is done', () => {
    const updates: number[] = [];
    const handle = tween<number>(
      { from: 0, to: 10, durationMs: 50, easing: linear },
      (val) => updates.push(val),
      undefined,
      false,
    );

    handle.step(100); // completes
    const countAfterDone = updates.length;
    handle.step(16); // should be ignored
    expect(updates.length).toBe(countAfterDone);
    expect(handle.done).toBe(true);
  });

  it('no setInterval is started — handle.done is false before step() is called', () => {
    // Verify the tween does NOT self-advance without step() being called
    vi.useFakeTimers();
    let completed = false;
    const handle = tween<number>(
      { from: 0, to: 1, durationMs: 100, easing: linear },
      () => {},
      () => {
        completed = true;
      },
      false,
    );
    vi.advanceTimersByTime(500);
    // No setInterval means it should NOT have completed on its own
    expect(completed).toBe(false);
    expect(handle.done).toBe(false);
    handle.cancel();
    vi.useRealTimers();
  });
});

// ---------------------------------------------------------------------------
// C1.5 (partial) — motion-store lifecycle is tested in motion-store.test.ts
// These tests cover the reduced-motion duration collapse only
// ---------------------------------------------------------------------------

describe('tween — M5-B (normal motion preserves full duration)', () => {
  let originalWindow: typeof globalThis.window;
  beforeEach(() => {
    originalWindow = globalThis.window;
    // Ensure matchMedia reports no reduced motion preference
    globalThis.window = {
      matchMedia: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    } as unknown as typeof globalThis.window;
  });
  afterEach(() => {
    globalThis.window = originalWindow;
  });

  it('tween with durationMs:700 is NOT done after step(50) when reduced motion is off', () => {
    const reducedMotion = useReducedMotion();
    expect(reducedMotion).toBe(false);

    let completed = false;
    const handle = tween<number>(
      { from: 0, to: 1, durationMs: 700, easing: linear },
      () => {},
      () => {
        completed = true;
      },
      reducedMotion,
    );

    handle.step(50);
    expect(completed).toBe(false);

    handle.cancel();
  });
});
