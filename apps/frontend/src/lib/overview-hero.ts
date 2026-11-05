// Contract between the overview collapse runtime and the hero graph island:
// the runtime dispatches HERO_PAUSE_EVENT on document when the hero collapses
// into the bar (or expands back) so the graph stops its ambient motion.

export const HERO_PAUSE_EVENT = 'overview:hero-pause';

export function heroPauseEvent(paused: boolean): CustomEvent<{ paused: boolean }> {
  return new CustomEvent(HERO_PAUSE_EVENT, { detail: { paused } });
}

/** The paused state carried by the event, or null for a malformed event. */
export function heroPausedFromEvent(e: Event): boolean | null {
  const detail = (e as CustomEvent<unknown>).detail as { paused?: unknown } | null | undefined;
  return detail && typeof detail.paused === 'boolean' ? detail.paused : null;
}
