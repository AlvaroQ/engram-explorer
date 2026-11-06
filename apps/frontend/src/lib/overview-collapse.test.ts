import { describe, expect, it } from 'vitest';
import {
  collapseState,
  collapseVars,
  dockOffsets,
  type CollapseGeometry,
} from './overview-collapse.ts';

// Artboard 04 geometry: a 37px issue bar above a 420px hero, 72px bar, KPI
// cards overlapping the hero by 64px, 120px strip ramp.
const g: CollapseGeometry = {
  heroTop: 37,
  heroH: 420,
  barH: 72,
  kpiOverlap: -64,
  ramp: 120,
  reduced: false,
};

describe('collapseState', () => {
  it('maps scroll 37 → 385px to hero progress 0 → 1 (S3)', () => {
    expect(collapseState(0, g).p).toBe(0);
    expect(collapseState(37, g).p).toBe(0);
    expect(collapseState(211, g).p).toBeCloseTo(0.5, 6);
    expect(collapseState(385, g)).toMatchObject({ p: 1, s: 348 });
    expect(collapseState(2000, g)).toMatchObject({ p: 1, s: 348 });
  });

  it('flags the hero collapsed past half way', () => {
    expect(collapseState(210, g).collapsed).toBe(false);
    expect(collapseState(212, g).collapsed).toBe(true);
  });

  it('ramps the KPI strip over the last 120px before the cards pin under the bar', () => {
    // pin = 37 + 420 - 64 - 72 = 321
    expect(collapseState(201, g).k).toBe(0);
    expect(collapseState(261, g).k).toBeCloseTo(0.5, 6);
    expect(collapseState(321, g).k).toBe(1);
    expect(collapseState(260, g).compact).toBe(false);
    expect(collapseState(262, g).compact).toBe(true);
  });

  it('under reduced motion the hero never collapses and the strip switches at once', () => {
    const r = { ...g, reduced: true };
    expect(collapseState(385, r)).toMatchObject({ s: 0, p: 0, collapsed: false });
    // the hero scrolls away; the strip pins at the top: 37 + 420 - 64 = 393
    expect(collapseState(392, r).k).toBe(0);
    expect(collapseState(393, r)).toMatchObject({ k: 1, compact: true });
  });
});

describe('collapseVars', () => {
  it('writes the custom properties the CSS reads', () => {
    expect(collapseVars(collapseState(211, g))).toEqual({
      '--ov-s': '174.00px',
      '--ov-p': '0.5000',
      '--ov-k': '0.0833', // (211 - (321 - 120)) / 120
    });
  });

  it('ends on canvas scale 0.55 and opacity 0.35 at full collapse', () => {
    const v = collapseVars(collapseState(385, g));
    expect(v['--ov-p']).toBe('1.0000');
    // The CSS turns --ov-p into scale(1 - .45p) and opacity(1 - .65p).
    expect(1 - 0.45 * Number(v['--ov-p'])).toBeCloseTo(0.55, 6);
    expect(1 - 0.65 * Number(v['--ov-p'])).toBeCloseTo(0.35, 6);
  });
});

describe('dockOffsets', () => {
  it('centres the title on the bar and docks the search just right of the 16px title', () => {
    const d = dockOffsets({
      barH: 72,
      title: { x: 32, y: 54, h: 28 },
      titleW: 100,
      form: { x: 400, y: 24, h: 44 },
      heroH: 420,
    });
    expect(d['--ov-h1-dy']).toBe('-32.00px'); // 36 - (54 + 14)
    expect(d['--ov-sx']).toBe('-264.00px'); // 32 + 100 * 0.8 + 24 - 400
    expect(d['--ov-sy']).toBe('-10.00px'); // 36 - 22 - 24
    expect(d['--ov-cv-dy']).toBe('-79.50px'); // 36 - 420 * 0.5 * 0.55
  });
});
