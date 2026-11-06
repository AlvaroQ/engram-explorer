import { describe, expect, it } from 'vitest';
import { ambientMotionActive, shouldInvalidateFrame } from './frame-demand.ts';

const base = { animate: true, reducedMotion: false, hidden: false, onScreen: true };

describe('ambientMotionActive', () => {
  it('runs the ambient particle flow when animating, visible and on screen', () => {
    expect(ambientMotionActive(base)).toBe(true);
  });

  it('stays idle when the caller disables animation', () => {
    expect(ambientMotionActive({ ...base, animate: false })).toBe(false);
  });

  it('respects prefers-reduced-motion', () => {
    expect(ambientMotionActive({ ...base, reducedMotion: true })).toBe(false);
  });

  it('pauses while the tab is hidden or the canvas is scrolled off screen', () => {
    expect(ambientMotionActive({ ...base, hidden: true })).toBe(false);
    expect(ambientMotionActive({ ...base, onScreen: false })).toBe(false);
  });
});

describe('shouldInvalidateFrame', () => {
  it('requests another frame while a tween runs or ambient motion is on', () => {
    expect(shouldInvalidateFrame({ tweensActive: true, ambient: false, hidden: false })).toBe(true);
    expect(shouldInvalidateFrame({ tweensActive: false, ambient: true, hidden: false })).toBe(true);
  });

  it('lets the demand frameloop go idle otherwise', () => {
    expect(shouldInvalidateFrame({ tweensActive: false, ambient: false, hidden: false })).toBe(
      false,
    );
  });

  it('never requests frames for a hidden tab', () => {
    expect(shouldInvalidateFrame({ tweensActive: true, ambient: true, hidden: true })).toBe(false);
  });
});
