/**
 * inline-edit-field.test.tsx
 *
 * Tests for the pure helper exported from InlineEditField. Component
 * rendering tests are kept minimal — the business-critical logic (whether the
 * value has changed and is worth saving) is tested here as a pure function.
 */

import { describe, it, expect } from 'vitest';
import { hasChanged } from './inline-edit-field.tsx';

// ---------------------------------------------------------------------------
// hasChanged — pure function
// ---------------------------------------------------------------------------
// Returns true when `next` (trimmed) differs from `original` (null treated
// as ''). Prevents saving values that haven't actually changed.

describe('hasChanged', () => {
  it('returns true when next differs from original', () => {
    expect(hasChanged('new value', 'old value')).toBe(true);
    expect(hasChanged('something', null)).toBe(true);
  });

  it('returns false when next equals original after trimming', () => {
    expect(hasChanged('same', 'same')).toBe(false);
    expect(hasChanged('  same  ', 'same')).toBe(false);
  });

  it('treats null original as empty string', () => {
    expect(hasChanged('', null)).toBe(false);
    expect(hasChanged('   ', null)).toBe(false);
    expect(hasChanged('x', null)).toBe(true);
  });

  it('is trim-aware: whitespace-only next equals null original', () => {
    expect(hasChanged('   ', null)).toBe(false);
  });

  it('treats empty string original as equivalent to null', () => {
    expect(hasChanged('', '')).toBe(false);
    expect(hasChanged('value', '')).toBe(true);
  });
});
