/**
 * brain-tunnel-flow.test.ts — Unit tests for BrainTunnelFlow logic (C5).
 *
 * Tests cover the pure functions extracted from brain-tunnel-flow.ts:
 * - Ambient opacity constants (S5.3 / C5.4)
 * - computeTubeRadius: edge.weight → tube radius (C5.5)
 * - computeEdgeOpacity: focus/dim state → opacity (S5.4 / C5.1–C5.2)
 * - SYNAPSE_COLORS: relation → color constant (C5.3)
 * - computeEdgeBrightness: confidence → opacity multiplier (C5.6)
 *
 * R3F/Three.js visual rendering is NOT unit-tested (not feasible per strict TDD).
 */

import { describe, it, expect } from 'vitest';
import {
  // Ambient opacity constants (S5.3 / C5.4)
  SYNAPSE_AMBIENT_TOPIC_ROAD_MIN,
  SYNAPSE_AMBIENT_TOPIC_ROAD_MAX,
  SYNAPSE_AMBIENT_SEMANTIC_MIN,
  SYNAPSE_AMBIENT_SEMANTIC_MAX,
  // Tube radius constants (C5.5)
  TUBE_RADIUS_MIN,
  TUBE_RADIUS_SCALE,
  // Relation color config (C5.3)
  SYNAPSE_COLORS,
  // Pure functions
  computeTubeRadius,
  computeEdgeOpacity,
  computeEdgeBrightness,
} from './brain-tunnel-flow';

// ---------------------------------------------------------------------------
// S5.3 / C5.4 — Ambient opacity constants
// ---------------------------------------------------------------------------

describe('SYNAPSE_AMBIENT constants (S5.3)', () => {
  it('TOPIC_ROAD_MIN is 0.12', () => {
    expect(SYNAPSE_AMBIENT_TOPIC_ROAD_MIN).toBe(0.12);
  });

  it('TOPIC_ROAD_MAX is 0.18', () => {
    expect(SYNAPSE_AMBIENT_TOPIC_ROAD_MAX).toBe(0.18);
  });

  it('SEMANTIC_MIN is 0.3', () => {
    expect(SYNAPSE_AMBIENT_SEMANTIC_MIN).toBe(0.3);
  });

  it('SEMANTIC_MAX is 0.5', () => {
    expect(SYNAPSE_AMBIENT_SEMANTIC_MAX).toBe(0.5);
  });

  it('topic-road range is within [0.12, 0.18]', () => {
    expect(SYNAPSE_AMBIENT_TOPIC_ROAD_MIN).toBeGreaterThanOrEqual(0.12);
    expect(SYNAPSE_AMBIENT_TOPIC_ROAD_MAX).toBeLessThanOrEqual(0.18);
    expect(SYNAPSE_AMBIENT_TOPIC_ROAD_MIN).toBeLessThanOrEqual(SYNAPSE_AMBIENT_TOPIC_ROAD_MAX);
  });

  it('semantic range is within [0.3, 0.5]', () => {
    expect(SYNAPSE_AMBIENT_SEMANTIC_MIN).toBeGreaterThanOrEqual(0.3);
    expect(SYNAPSE_AMBIENT_SEMANTIC_MAX).toBeLessThanOrEqual(0.5);
    expect(SYNAPSE_AMBIENT_SEMANTIC_MIN).toBeLessThanOrEqual(SYNAPSE_AMBIENT_SEMANTIC_MAX);
  });
});

// ---------------------------------------------------------------------------
// C5.5 — Tube radius encoding
// ---------------------------------------------------------------------------

describe('computeTubeRadius (C5.5)', () => {
  it('returns TUBE_RADIUS_MIN for weight 0', () => {
    expect(computeTubeRadius(0)).toBe(TUBE_RADIUS_MIN);
  });

  it('increases with weight', () => {
    expect(computeTubeRadius(2)).toBeGreaterThan(computeTubeRadius(1));
    expect(computeTubeRadius(5)).toBeGreaterThan(computeTubeRadius(2));
  });

  it('formula is TUBE_RADIUS_MIN + weight * TUBE_RADIUS_SCALE', () => {
    for (const w of [0, 1, 3, 10]) {
      expect(computeTubeRadius(w)).toBeCloseTo(TUBE_RADIUS_MIN + w * TUBE_RADIUS_SCALE, 5);
    }
  });

  it('TUBE_RADIUS_MIN is a positive config constant (not hardcoded)', () => {
    expect(TUBE_RADIUS_MIN).toBeGreaterThan(0);
    expect(typeof TUBE_RADIUS_MIN).toBe('number');
  });

  it('TUBE_RADIUS_SCALE is a positive config constant (not hardcoded)', () => {
    expect(TUBE_RADIUS_SCALE).toBeGreaterThan(0);
    expect(typeof TUBE_RADIUS_SCALE).toBe('number');
  });
});

// ---------------------------------------------------------------------------
// C5.3 — Relation → color config
// ---------------------------------------------------------------------------

describe('SYNAPSE_COLORS (C5.3)', () => {
  it('has a color for "related"', () => {
    expect(typeof SYNAPSE_COLORS.related).toBe('string');
    expect(SYNAPSE_COLORS.related).toMatch(/^#/);
  });

  it('has a color for "compatible"', () => {
    expect(typeof SYNAPSE_COLORS.compatible).toBe('string');
    expect(SYNAPSE_COLORS.compatible).toMatch(/^#/);
  });

  it('has a color for "scoped"', () => {
    expect(typeof SYNAPSE_COLORS.scoped).toBe('string');
    expect(SYNAPSE_COLORS.scoped).toMatch(/^#/);
  });

  it('has a neutral fallback color', () => {
    expect(typeof SYNAPSE_COLORS.neutral).toBe('string');
    expect(SYNAPSE_COLORS.neutral).toMatch(/^#/);
  });

  it('"related" and "compatible" are different colors from "scoped"', () => {
    // related/compatible are semantic (same family), scoped is topic-road
    expect(SYNAPSE_COLORS.scoped).not.toBe(SYNAPSE_COLORS.related);
  });
});

// ---------------------------------------------------------------------------
// C5.1–C5.2 — computeEdgeOpacity: focus/dim/ambient state
// (S5.4 — TunnelFocusLayer scenario S5-A)
// ---------------------------------------------------------------------------

describe('computeEdgeOpacity (S5.4 / C5.1–C5.2)', () => {
  const SEMANTIC_AMBIENT = (SYNAPSE_AMBIENT_SEMANTIC_MIN + SYNAPSE_AMBIENT_SEMANTIC_MAX) / 2;
  const TOPIC_AMBIENT = (SYNAPSE_AMBIENT_TOPIC_ROAD_MIN + SYNAPSE_AMBIENT_TOPIC_ROAD_MAX) / 2;

  describe('no focus active (hoveredId = null)', () => {
    it('semantic edge returns ambient value in [SEMANTIC_MIN, SEMANTIC_MAX]', () => {
      const opacity = computeEdgeOpacity('semantic', false, null);
      expect(opacity).toBeGreaterThanOrEqual(SYNAPSE_AMBIENT_SEMANTIC_MIN);
      expect(opacity).toBeLessThanOrEqual(SYNAPSE_AMBIENT_SEMANTIC_MAX);
    });

    it('topic-road edge returns ambient value in [TOPIC_MIN, TOPIC_MAX]', () => {
      const opacity = computeEdgeOpacity('topic-road', false, null);
      expect(opacity).toBeGreaterThanOrEqual(SYNAPSE_AMBIENT_TOPIC_ROAD_MIN);
      expect(opacity).toBeLessThanOrEqual(SYNAPSE_AMBIENT_TOPIC_ROAD_MAX);
    });
  });

  describe('focus active, incident edge (isIncident = true)', () => {
    it('semantic incident edge opacity > ambient', () => {
      const focused = computeEdgeOpacity('semantic', true, 'node-A');
      expect(focused).toBeGreaterThan(SEMANTIC_AMBIENT);
    });

    it('topic-road incident edge opacity > ambient', () => {
      const focused = computeEdgeOpacity('topic-road', true, 'node-A');
      expect(focused).toBeGreaterThan(TOPIC_AMBIENT);
    });
  });

  describe('focus active, non-incident edge (isIncident = false)', () => {
    it('non-incident edge opacity ≤ ambient / 2 (S5.4 spec)', () => {
      // S5.4: "Non-incident edges MUST dim to ≤ ambient/2"
      const dimmed = computeEdgeOpacity('semantic', false, 'node-A');
      expect(dimmed).toBeLessThanOrEqual(SEMANTIC_AMBIENT / 2);
    });

    it('topic-road non-incident edge opacity ≤ ambient / 2', () => {
      const dimmed = computeEdgeOpacity('topic-road', false, 'node-A');
      expect(dimmed).toBeLessThanOrEqual(TOPIC_AMBIENT / 2);
    });
  });

  describe('focus removal', () => {
    it('removing focus (hoveredId → null) returns to ambient range', () => {
      const ambient = computeEdgeOpacity('semantic', false, null);
      expect(ambient).toBeGreaterThanOrEqual(SYNAPSE_AMBIENT_SEMANTIC_MIN);
      expect(ambient).toBeLessThanOrEqual(SYNAPSE_AMBIENT_SEMANTIC_MAX);
    });
  });
});

// ---------------------------------------------------------------------------
// C5.6 — computeEdgeBrightness: confidence → opacity multiplier
// ---------------------------------------------------------------------------

describe('computeEdgeBrightness (C5.6)', () => {
  it('returns 0.5 when confidence is undefined', () => {
    expect(computeEdgeBrightness(undefined)).toBe(0.5);
  });

  it('returns confidence value when defined', () => {
    expect(computeEdgeBrightness(0.8)).toBe(0.8);
    expect(computeEdgeBrightness(0.2)).toBe(0.2);
    expect(computeEdgeBrightness(1.0)).toBe(1.0);
    expect(computeEdgeBrightness(0.0)).toBe(0.0);
  });

  it('clamps values to [0, 1]', () => {
    expect(computeEdgeBrightness(-0.1)).toBeGreaterThanOrEqual(0);
    expect(computeEdgeBrightness(1.5)).toBeLessThanOrEqual(1);
  });
});
