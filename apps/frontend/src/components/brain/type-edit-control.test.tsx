/**
 * type-edit-control.test.tsx
 *
 * Tests for the pure helper exported from TypeEditControl. Component
 * rendering tests are kept minimal — the business-critical logic (whether a
 * typed type value is submittable) is tested here as a pure function.
 */

import { describe, it, expect } from 'vitest';
import { isSubmittableType } from './type-edit-control.tsx';

// ---------------------------------------------------------------------------
// isSubmittableType — pure function
// ---------------------------------------------------------------------------
// A type value is submittable when it is non-empty (after trim) AND different
// from the node's current type. A null current type is treated as ''.

describe('isSubmittableType', () => {
  it('returns true for a non-empty type different from the current type', () => {
    expect(isSubmittableType('decision', 'bugfix')).toBe(true);
    expect(isSubmittableType('retrospective', null)).toBe(true);
  });

  it('returns false when the type equals the current type', () => {
    expect(isSubmittableType('bugfix', 'bugfix')).toBe(false);
  });

  it('returns false for an empty or whitespace-only type', () => {
    expect(isSubmittableType('', 'bugfix')).toBe(false);
    expect(isSubmittableType('   ', 'bugfix')).toBe(false);
  });

  it('trims surrounding whitespace before comparing', () => {
    expect(isSubmittableType('  bugfix  ', 'bugfix')).toBe(false);
    expect(isSubmittableType('  decision  ', 'bugfix')).toBe(true);
  });

  it('treats a null current type as the empty string', () => {
    expect(isSubmittableType('', null)).toBe(false);
    expect(isSubmittableType('pattern', null)).toBe(true);
  });
});
