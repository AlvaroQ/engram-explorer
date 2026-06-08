/**
 * project-edit-control.test.tsx
 *
 * Tests for the pure helper extracted from ProjectEditControl. Component
 * rendering tests are kept minimal because the component depends on TanStack
 * Query, i18next, and DOM events that need a heavy setup; the business-critical
 * logic (whether a typed target is worth saving) is extracted and tested here
 * as a pure function.
 */

import { describe, it, expect } from 'vitest';
import { isSubmittableTarget } from './project-edit-control.tsx';

// ---------------------------------------------------------------------------
// isSubmittableTarget — pure function
// ---------------------------------------------------------------------------
// A target is submittable when it is non-empty (after trim) AND different from
// the node's current project. A null current project is treated as ''.

describe('isSubmittableTarget', () => {
  it('returns true for a non-empty target different from the current project', () => {
    expect(isSubmittableTarget('beta', 'alpha')).toBe(true);
    expect(isSubmittableTarget('new-project', null)).toBe(true);
  });

  it('returns false when the target equals the current project', () => {
    expect(isSubmittableTarget('alpha', 'alpha')).toBe(false);
  });

  it('returns false for an empty or whitespace-only target', () => {
    expect(isSubmittableTarget('', 'alpha')).toBe(false);
    expect(isSubmittableTarget('   ', 'alpha')).toBe(false);
  });

  it('trims surrounding whitespace before comparing', () => {
    expect(isSubmittableTarget('  alpha  ', 'alpha')).toBe(false);
    expect(isSubmittableTarget('  beta  ', 'alpha')).toBe(true);
  });

  it('treats a null current project as the empty string', () => {
    expect(isSubmittableTarget('', null)).toBe(false);
    expect(isSubmittableTarget('x', null)).toBe(true);
  });
});
