import { describe, expect, it } from 'vitest';
import {
  collapseState,
  collapseVars,
  dockOffsets,
  type CollapseGeometry,
} from './overview-collapse.ts';

// Artboard 04 geometry: a 37px issue bar above a 500px hero, 72px bar, KPI
// cards overlapping the hero by 64px, 120px strip ramp.
const g: CollapseGeometry = {
  heroTop: 37,
  heroH: 500,
  barH: 72,
  kpiOverlap: -64,
  ramp: 120,
  reduced: false,
};

describe('collapseState', () => {
  it('maps scroll 37 → 465px to hero progress 0 → 1 (S3)', () => {
    expect(collapseState(0, g).p).toBe(0);
    expect(collapseState(37, g).p).toBe(0);
    expect(collapseState(251, g).p).toBeCloseTo(0.5, 6);
    expect(collapseState(465, g)).toMatchObject({ p: 1, s: 428 });
    expect(collapseState(2000, g)).toMatchObject({ p: 1, s: 428 });
  });

  it('flags the hero collapsed past half way', () => {
    expect(collapseState(250, g).collapsed).toBe(false);
    expect(collapseState(252, g).collapsed).toBe(true);
  });

  it('ramps the KPI strip over the last 120px before the cards pin under the bar', () => {
    // pin = 37 + 500 - 64 - 72 = 401
    expect(collapseState(281, g).k).toBe(0);
    expect(collapseState(341, g).k).toBeCloseTo(0.5, 6);
    expect(collapseState(401, g).k).toBe(1);
    expect(collapseState(340, g).compact).toBe(false);
    expect(collapseState(342, g).compact).toBe(true);
  });

  it('under reduced motion the hero never collapses and the strip switches at once', () => {
    const r = { ...g, reduced: true };
    expect(collapseState(465, r)).toMatchObject({ s: 0, p: 0, collapsed: false });
    // the hero scrolls away; the strip pins at the top: 37 + 500 - 64 = 473
    expect(collapseState(472, r).k).toBe(0);
    expect(collapseState(473, r)).toMatchObject({ k: 1, compact: true });
  });
});

describe('collapseVars', () => {
  it('writes the custom properties the CSS reads', () => {
    expect(collapseVars(collapseState(251, g))).toEqual({
      '--ov-s': '214.00px',
      '--ov-p': '0.5000',
      '--ov-k': '0.0000',
    });
  });

  it('ends on canvas scale 0.55 and opacity 0.35 at full collapse', () => {
    const v = collapseVars(collapseState(465, g));
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
      heroH: 500,
    });
    expect(d['--ov-h1-dy']).toBe('-32.00px'); // 36 - (54 + 14)
    expect(d['--ov-sx']).toBe('-264.00px'); // 32 + 100 * 0.8 + 24 - 400
    expect(d['--ov-sy']).toBe('-10.00px'); // 36 - 22 - 24
    expect(d['--ov-cv-dy']).toBe('-101.50px'); // 36 - 500 * 0.5 * 0.55
  });
});
