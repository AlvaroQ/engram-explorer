import type { CanvasProps } from '@react-three/fiber';

/**
 * How the R3F Canvas measures its container (react-use-measure options).
 *
 * The overview hero scales and lifts the graph with CSS transforms while the
 * page scrolls. R3F's defaults (scroll: true, getBoundingClientRect) would
 * re-measure on every scroll and read the already-transformed box, so the
 * drawing buffer shrank under the CSS scale (scaled down twice), its inline
 * width pinned it to the left edge, and each resize re-rendered the scene.
 * offsetWidth/offsetHeight ignore transforms, and the size only changes on a
 * real layout resize (ResizeObserver).
 */
export const CANVAS_RESIZE = {
  scroll: false,
  offsetSize: true,
} as const satisfies NonNullable<CanvasProps['resize']>;
