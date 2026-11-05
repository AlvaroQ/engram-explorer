import { describe, expect, it } from 'vitest';
import { HERO_PAUSE_EVENT, heroPauseEvent, heroPausedFromEvent } from './overview-hero.ts';

describe('hero pause event', () => {
  it('round-trips the paused state', () => {
    expect(heroPauseEvent(true).type).toBe(HERO_PAUSE_EVENT);
    expect(heroPausedFromEvent(heroPauseEvent(true))).toBe(true);
    expect(heroPausedFromEvent(heroPauseEvent(false))).toBe(false);
  });

  it('ignores events without a boolean detail', () => {
    expect(heroPausedFromEvent(new Event(HERO_PAUSE_EVENT))).toBeNull();
    expect(
      heroPausedFromEvent(new CustomEvent(HERO_PAUSE_EVENT, { detail: { paused: 'yes' } })),
    ).toBeNull();
  });
});
