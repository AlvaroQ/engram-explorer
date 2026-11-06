import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { CANVAS_RESIZE } from './canvas-resize.ts';

describe('graph canvas resize options', () => {
  it('ignore scroll and CSS transforms when sizing the drawing buffer', () => {
    // scroll: true re-measures on every scroll; without offsetSize the size
    // comes from getBoundingClientRect, which includes the overview hero's
    // collapse scale, so the canvas would be scaled down twice and pinned to the left.
    expect(CANVAS_RESIZE).toEqual({ scroll: false, offsetSize: true });
  });

  it('are the options GraphScene passes to the R3F Canvas', () => {
    const src = readFileSync(
      new URL('../components/brain/graph-scene.tsx', import.meta.url),
      'utf8',
    );
    expect(src).toContain('resize={CANVAS_RESIZE}');
  });
});
