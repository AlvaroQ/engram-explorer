/**
 * query-config.ts — Named staleTime constants for TanStack Query.
 *
 * Centralizes the magic numbers that were scattered across ~18 call sites.
 * Import the constant that matches the data's expected change frequency instead
 * of inlining a numeric literal.
 *
 * Tiers (aligned with existing values in the codebase):
 *   STALE_FAST       — 5 s    live/polling data (sync health, sidebar stats)
 *   STALE_DEFAULT    — 30 s   matches the QueryClient global default in App.tsx
 *   STALE_SLOW       — 60 s   semi-stable data (observation detail, type lists)
 *   STALE_VERY_SLOW  — 5 min  stable/reference data (project lists, sessions)
 */

export const STALE_FAST = 5_000;
export const STALE_DEFAULT = 30_000;
export const STALE_SLOW = 60_000;
export const STALE_VERY_SLOW = 5 * 60_000;
