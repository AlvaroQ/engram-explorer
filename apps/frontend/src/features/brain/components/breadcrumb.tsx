/**
 * breadcrumb.tsx — Navigation breadcrumb for the brain level stack.
 *
 * Renders Root + one item per entry in `path`.
 * Clicking any breadcrumb item pops the stack back to (but not including)
 * that item (S4.2 / S4-A scenario).
 *
 * Accessibility (S10.2):
 *   - aria-label on the nav element
 *   - aria-current="page" on the active (deepest) item
 *   - Each item has visible text + accessible label including node label
 */

import { type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronRight } from 'lucide-react';
import type { BrainNodeRef } from '../model/types';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface BreadcrumbProps {
  /** Current navigation path — empty means at root. */
  path: BrainNodeRef[];
  /**
   * Pop back to the given path index (inclusive).
   * Pass -1 to navigate to root.
   */
  onPopTo: (index: number) => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Brain navigation breadcrumb.
 *
 * S4-A scenario:
 *   Given path = [ref(lobe:engram), ref(neuron:engram/auth-model)]
 *   Then breadcrumb shows: Root > engram > auth-model
 *   Clicking 'engram' calls onPopTo(0) → path becomes [ref(lobe:engram)]
 */
export function BrainBreadcrumb({ path, onPopTo }: BreadcrumbProps): JSX.Element {
  const { t } = useTranslation();
  const items: Array<{ label: string; index: number; isCurrent: boolean }> = [
    // Root item — always present, clicking it goes to root (index -1)
    { label: t('brain.breadcrumb.root'), index: -1, isCurrent: path.length === 0 },
    // One item per path entry
    ...path.map((ref, i) => ({
      label: ref.label,
      index: i,
      isCurrent: i === path.length - 1,
    })),
  ];

  return (
    <nav aria-label={t('brain.breadcrumb.navAriaLabel')} className="flex items-center gap-0.5 text-xs">
      {items.map((item, i) => (
        <span key={`${item.index}-${item.label}`} className="flex items-center gap-0.5">
          {i > 0 && (
            <ChevronRight
              className="h-3 w-3 shrink-0 text-fg-muted"
              aria-hidden="true"
            />
          )}

          {item.isCurrent ? (
            // Active item — not clickable, aria-current="page"
            <span
              aria-current="page"
              className="font-medium text-fg"
            >
              {item.label}
            </span>
          ) : (
            // Ancestor item — clickable
            <button
              type="button"
              aria-label={t('brain.breadcrumb.navigateBackTo', { label: item.label })}
              onClick={() => onPopTo(item.index)}
              className="rounded px-0.5 text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
            >
              {item.label}
            </button>
          )}
        </span>
      ))}
    </nav>
  );
}
