/**
 * overview-collapse.ts — scroll math of the overview's collapsing hero (S3,
 * artboard 04-scroll-collapse). Pure: the runtime (islands/overview-collapse)
 * measures the page, calls these and writes the result as CSS custom
 * properties on .overview; styles.css turns them into transforms/opacities.
 *
 *   --ov-s  px the 500px hero has slid under the scroller top (0..hero-bar)
 *   --ov-p  hero collapse progress 0..1 (= s / (hero - bar)); with a 37px
 *           alert bar above it, scroll 37 → 465px maps to 0 → 1
 *   --ov-k  KPI strip progress 0..1, over the last `ramp` px before the cards
 *           pin under the bar
 *
 * The hero is sticky with top = bar - hero, so nothing reflows while scrolling.
 */

export interface CollapseGeometry {
  /** Height of the content above the hero (e.g. the sync alert bar). */
  heroTop: number;
  heroH: number;
  barH: number;
  /** KPI section top margin (negative: the cards overlap the hero). */
  kpiOverlap: number;
  ramp: number;
  /** prefers-reduced-motion: the hero scrolls away, only the strip pins. */
  reduced: boolean;
}

export interface CollapseState {
  s: number;
  p: number;
  k: number;
  /** Hero mostly collapsed: faded controls leave the tab order, graph pauses. */
  collapsed: boolean;
  /** Strip mostly formed: the strip links are exposed, the full cards hidden. */
  compact: boolean;
}

const clamp01 = (v: number): number => Math.min(Math.max(v, 0), 1);

export function collapseState(scrollTop: number, g: CollapseGeometry): CollapseState {
  const range = Math.max(g.heroH - g.barH, 1);
  const s = g.reduced ? 0 : Math.min(Math.max(scrollTop - g.heroTop, 0), range);
  const p = s / range;
  // scrollTop at which the KPI section reaches its sticky top
  const pin = g.heroTop + g.heroH + g.kpiOverlap - (g.reduced ? 0 : g.barH);
  const k = g.reduced
    ? scrollTop >= pin - 0.5
      ? 1
      : 0
    : clamp01((scrollTop - (pin - g.ramp)) / Math.max(g.ramp, 1));
  return { s, p, k, collapsed: p > 0.5, compact: k > 0.5 };
}

const px = (v: number): string => `${v.toFixed(2)}px`;

export function collapseVars(c: CollapseState): Record<string, string> {
  return { '--ov-s': px(c.s), '--ov-p': c.p.toFixed(4), '--ov-k': c.k.toFixed(4) };
}

export interface Rect {
  x: number;
  y: number;
  h: number;
}

/**
 * Docking offsets, measured at rest (progress 0) relative to the hero UI box:
 * the title moves to the bar's vertical centre (and scales 20px → 16px in
 * CSS), the search docks just right of the scaled title, and the graph
 * (the R3F scene centres it in the hero; scaled by 0.55) lifts into the bar.
 */
export function dockOffsets(m: {
  barH: number;
  title: Rect;
  titleW: number;
  form: Rect;
  heroH: number;
}): Record<string, string> {
  const mid = m.barH / 2;
  return {
    '--ov-h1-dy': px(mid - (m.title.y + m.title.h / 2)),
    '--ov-sx': px(m.title.x + m.titleW * 0.8 + 24 - m.form.x),
    '--ov-sy': px(mid - m.form.h / 2 - m.form.y),
    '--ov-cv-dy': px(mid - m.heroH * 0.5 * 0.55),
  };
}
