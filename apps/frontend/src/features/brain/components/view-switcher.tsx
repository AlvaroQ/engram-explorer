/**
 * view-switcher.tsx — Horizontal tab bar for switching between brain view modes.
 *
 * Positioned right of the Brain title (D2).
 * Three tabs: Lóbulos / Temas / Orgánico.
 * Uses role="tablist" + role="tab" with aria-selected (S10.2).
 *
 * Switching tabs re-derives the BrainModel for the new view mode and resets
 * the navigation stack to root (view change always starts fresh).
 */

import { useRef, type JSX, type KeyboardEvent } from 'react';
import type { ViewMode } from '../model/types';

// ---------------------------------------------------------------------------
// View tab definitions
// ---------------------------------------------------------------------------

interface ViewTab {
  mode: ViewMode;
  /** Display label — intentionally in Spanish per the design spec (D1). */
  label: string;
  /** Tooltip description of this view mode (title attribute). */
  description: string;
}

const VIEW_TABS: ViewTab[] = [
  {
    mode: 'lobulos',
    label: 'Lóbulos',
    description: 'Group observations by project lobe',
  },
  {
    mode: 'temas',
    label: 'Temas',
    description: 'Group observations by topic neuron',
  },
  {
    mode: 'organico',
    label: 'Orgánico',
    description: 'Group observations by similarity cluster',
  },
];

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ViewSwitcherProps {
  /** Currently active view mode. */
  activeView: ViewMode;
  /** Called when the user selects a different tab. */
  onViewChange: (mode: ViewMode) => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Horizontal tab bar for brain view mode selection.
 *
 * Accessibility (WAI-ARIA tablist pattern):
 *   - role="tablist" on the container
 *   - role="tab" + aria-selected on each button
 *   - Roving tabIndex: only the active tab is in the tab order (tabIndex=0);
 *     others are tabIndex=-1. Arrow keys move focus within the tablist.
 *   - title attribute provides the tooltip/description without overriding
 *     the visible label that a screen reader already announces.
 */
export function ViewSwitcher({ activeView, onViewChange }: ViewSwitcherProps): JSX.Element {
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([]);

  function handleKeyDown(e: KeyboardEvent<HTMLButtonElement>, currentIndex: number): void {
    let nextIndex: number | null = null;

    if (e.key === 'ArrowRight') {
      nextIndex = (currentIndex + 1) % VIEW_TABS.length;
    } else if (e.key === 'ArrowLeft') {
      nextIndex = (currentIndex - 1 + VIEW_TABS.length) % VIEW_TABS.length;
    } else if (e.key === 'Home') {
      nextIndex = 0;
    } else if (e.key === 'End') {
      nextIndex = VIEW_TABS.length - 1;
    }

    if (nextIndex !== null) {
      e.preventDefault();
      const tab = VIEW_TABS[nextIndex];
      if (tab) {
        onViewChange(tab.mode);
        buttonRefs.current[nextIndex]?.focus();
      }
    }
  }

  return (
    <div
      role="tablist"
      aria-label="Brain view mode"
      className="flex items-center gap-0.5 rounded-lg border border-border bg-surface px-1.5 py-1 shadow-sm"
    >
      {VIEW_TABS.map((tab, index) => {
        const isActive = tab.mode === activeView;

        return (
          <button
            key={tab.mode}
            ref={(el) => { buttonRefs.current[index] = el; }}
            type="button"
            role="tab"
            aria-selected={isActive}
            tabIndex={isActive ? 0 : -1}
            title={tab.description}
            onClick={() => {
              if (!isActive) onViewChange(tab.mode);
            }}
            onKeyDown={(e) => handleKeyDown(e, index)}
            className={[
              'rounded px-2.5 py-0.5 text-xs font-medium transition-colors',
              isActive
                ? 'bg-accent text-white shadow-sm'
                : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
            ].join(' ')}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}
