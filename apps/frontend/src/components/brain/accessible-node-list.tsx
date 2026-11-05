// Keyboard + screen-reader fallback for the WebGL canvas, which is otherwise
// opaque to assistive tech. The list is visually hidden (sr-only) for pointer
// users who explore the 3D graph, but is fully exposed to screen readers and
// reveals itself as a real panel the moment it receives keyboard focus
// (focus-within:not-sr-only). It mirrors the active filters/search via matchedIds.
//
// I8 (S8): Roving tabIndex — only the focused item has tabIndex=0; all others
// are tabIndex=-1. ArrowUp/Down/Home/End move focus within the list.
import { useRef, type JSX, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { GraphNode } from '../../lib/api.ts';

// Cap the DOM size — screen-reader users refine with the search box instead of
// tabbing through hundreds of nodes.
const MAX_LIST_ITEMS = 150;

interface Props {
  nodes: GraphNode[];
  /** Ids passing the active filters/search; null = no filter. */
  matchedIds: Set<number> | null;
  onSelect: (node: GraphNode) => void;
}

// ---------------------------------------------------------------------------
// Shared roving-list keyboard handler
// ---------------------------------------------------------------------------

function makeRovingKeyHandler(
  refs: React.MutableRefObject<(HTMLButtonElement | null)[]>,
  count: number,
  focusedIndex: React.MutableRefObject<number>,
) {
  return function handleKey(e: KeyboardEvent<HTMLButtonElement>, index: number): void {
    let next: number | null = null;

    if (e.key === 'ArrowDown') {
      next = Math.min(index + 1, count - 1);
    } else if (e.key === 'ArrowUp') {
      next = Math.max(index - 1, 0);
    } else if (e.key === 'Home') {
      next = 0;
    } else if (e.key === 'End') {
      next = count - 1;
    }

    if (next !== null) {
      e.preventDefault();
      focusedIndex.current = next;
      refs.current[next]?.focus();
    }
  };
}

// ---------------------------------------------------------------------------
// Flat GraphNode list — roving tabIndex (I8)
// ---------------------------------------------------------------------------

export function AccessibleNodeList({ nodes, matchedIds, onSelect }: Props): JSX.Element {
  const { t } = useTranslation();
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const focusedIndex = useRef(0);

  const visible = matchedIds ? nodes.filter((n) => matchedIds.has(n.id)) : nodes;
  const capped = visible.slice(0, MAX_LIST_ITEMS);
  const overflow = visible.length - capped.length;

  const handleKey = makeRovingKeyHandler(buttonRefs, capped.length, focusedIndex);

  return (
    <nav
      aria-label={t('brain.a11y.listLabel')}
      className="sr-only z-30 focus-within:not-sr-only focus-within:absolute focus-within:left-3 focus-within:top-3 focus-within:flex focus-within:max-h-[calc(100%-1.5rem)] focus-within:w-72 focus-within:flex-col focus-within:overflow-auto focus-within:rounded-lg focus-within:border focus-within:border-border focus-within:bg-surface focus-within:p-2 focus-within:shadow-xl"
    >
      <p className="px-1 pb-1 text-[11px] text-fg-muted">
        {t('brain.a11y.listHint', { count: visible.length })}
      </p>
      <ul className="flex flex-col">
        {capped.map((node, index) => (
          <li key={node.id}>
            <button
              ref={(el) => {
                buttonRefs.current[index] = el;
              }}
              type="button"
              tabIndex={index === focusedIndex.current ? 0 : -1}
              onFocus={() => {
                focusedIndex.current = index;
              }}
              onKeyDown={(e) => handleKey(e, index)}
              onClick={() => onSelect(node)}
              className="w-full rounded px-2 py-1 text-left text-xs text-fg hover:bg-surface-2 focus:bg-surface-2 focus:outline-none"
            >
              <span className="font-medium">{node.label ?? String(node.id)}</span>
              <span className="text-fg-muted">
                {' — '}
                {node.type ?? 'observation'}
                {node.project ? `, ${node.project}` : ''}
              </span>
            </button>
          </li>
        ))}
        {overflow > 0 ? (
          <li className="px-2 py-1 text-[11px] text-fg-muted">
            {t('brain.a11y.listMore', { count: overflow })}
          </li>
        ) : null}
      </ul>
    </nav>
  );
}
