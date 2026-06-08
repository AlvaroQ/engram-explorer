// Keyboard + screen-reader fallback for the WebGL canvas, which is otherwise
// opaque to assistive tech. The list is visually hidden (sr-only) for pointer
// users who explore the 3D graph, but is fully exposed to screen readers and
// reveals itself as a real panel the moment it receives keyboard focus
// (focus-within:not-sr-only). It mirrors the active filters/search via matchedIds.
//
// C6.5 / S10.1: Level-aware extension.
// When brainLevel is provided, the list shows the CURRENT level's BrainNodes
// (not the flat graph). The aria-label includes the breadcrumb path context.
// When brainLevel is absent, falls back to the legacy flat GraphNode list.
//
// I8 (S8): Roving tabIndex — only the focused item has tabIndex=0; all others
// are tabIndex=-1. ArrowUp/Down/Home/End move focus within the list.
import { useRef, type JSX, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { GraphNode } from '../../lib/api.ts'
import type { BrainLevel, BrainNode, BrainNodeRef } from './types.ts'

// Cap the DOM size — screen-reader users refine with the search box instead of
// tabbing through hundreds of nodes.
const MAX_LIST_ITEMS = 150

// ---------------------------------------------------------------------------
// Legacy props (flat GraphNode list — used when brainLevel is not provided)
// ---------------------------------------------------------------------------

interface LegacyProps {
  nodes: GraphNode[]
  /** Ids passing the active filters/search; null = no filter. */
  matchedIds: Set<number> | null
  onSelect: (node: GraphNode) => void
  brainLevel?: undefined
  brainPath?: undefined
  brainMatchedIds?: undefined
  onSelectBrain?: undefined
}

// ---------------------------------------------------------------------------
// Brain model props (level-aware — S10.1)
// ---------------------------------------------------------------------------

interface BrainLevelProps {
  brainLevel: BrainLevel
  /** Current navigation path — included in the aria-label for context. */
  brainPath: BrainNodeRef[]
  /** Ids of brain nodes passing the active filters; null = no filter. */
  brainMatchedIds: Set<string> | null
  /** Called when a brain node is selected via keyboard/SR. */
  onSelectBrain: (node: BrainNode) => void
  nodes?: undefined
  matchedIds?: undefined
  onSelect?: undefined
}

type Props = LegacyProps | BrainLevelProps

// ---------------------------------------------------------------------------
// Shared roving-list keyboard handler
// ---------------------------------------------------------------------------

function makeRovingKeyHandler(
  refs: React.MutableRefObject<(HTMLButtonElement | null)[]>,
  count: number,
  focusedIndex: React.MutableRefObject<number>,
) {
  return function handleKey(e: KeyboardEvent<HTMLButtonElement>, index: number): void {
    let next: number | null = null

    if (e.key === 'ArrowDown') {
      next = Math.min(index + 1, count - 1)
    } else if (e.key === 'ArrowUp') {
      next = Math.max(index - 1, 0)
    } else if (e.key === 'Home') {
      next = 0
    } else if (e.key === 'End') {
      next = count - 1
    }

    if (next !== null) {
      e.preventDefault()
      focusedIndex.current = next
      refs.current[next]?.focus()
    }
  }
}

// ---------------------------------------------------------------------------
// Brain-level variant (S10.1) — roving tabIndex (I8)
// ---------------------------------------------------------------------------

function BrainLevelList({
  brainLevel,
  brainPath,
  brainMatchedIds,
  onSelectBrain,
}: BrainLevelProps): JSX.Element {
  const { t } = useTranslation()
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([])
  const focusedIndex = useRef(0)

  const visibleNodes = brainMatchedIds
    ? brainLevel.nodes.filter((n) => brainMatchedIds.has(n.id))
    : brainLevel.nodes

  const capped = visibleNodes.slice(0, MAX_LIST_ITEMS)
  const overflow = visibleNodes.length - capped.length

  const handleKey = makeRovingKeyHandler(buttonRefs, capped.length, focusedIndex)

  const pathContext =
    brainPath.length > 0
      ? `: ${brainPath.map((r) => r.label).join(' › ')}`
      : ''
  const ariaLabel = `Brain nodes${pathContext} (${visibleNodes.length} visible)`

  return (
    <nav
      aria-label={ariaLabel}
      className="sr-only z-30 focus-within:not-sr-only focus-within:absolute focus-within:left-3 focus-within:top-3 focus-within:flex focus-within:max-h-[calc(100%-1.5rem)] focus-within:w-72 focus-within:flex-col focus-within:overflow-auto focus-within:rounded-lg focus-within:border focus-within:border-border focus-within:bg-surface focus-within:p-2 focus-within:shadow-xl"
    >
      <p className="px-1 pb-1 text-[11px] text-fg-muted">
        {t('brain.a11y.listHint', { count: visibleNodes.length })}
      </p>
      <ul className="flex flex-col">
        {capped.map((node, index) => (
          <li key={node.id}>
            <button
              ref={(el) => { buttonRefs.current[index] = el }}
              type="button"
              tabIndex={index === focusedIndex.current ? 0 : -1}
              onFocus={() => { focusedIndex.current = index }}
              onKeyDown={(e) => handleKey(e, index)}
              onClick={() => onSelectBrain(node)}
              className="w-full rounded px-2 py-1 text-left text-xs text-fg hover:bg-surface-2 focus:bg-surface-2 focus:outline-none"
            >
              <span className="font-medium">{node.label}</span>
              <span className="text-fg-muted">
                {' — '}
                {node.kind}
                {typeof node.meta['project'] === 'string' && node.meta['project']
                  ? `, ${node.meta['project']}`
                  : ''}
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
  )
}

// ---------------------------------------------------------------------------
// Legacy variant (flat GraphNode list) — roving tabIndex (I8)
// ---------------------------------------------------------------------------

function LegacyList({ nodes, matchedIds, onSelect }: LegacyProps): JSX.Element {
  const { t } = useTranslation()
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([])
  const focusedIndex = useRef(0)

  const visible = matchedIds ? nodes.filter((n) => matchedIds.has(n.id)) : nodes
  const capped = visible.slice(0, MAX_LIST_ITEMS)
  const overflow = visible.length - capped.length

  const handleKey = makeRovingKeyHandler(buttonRefs, capped.length, focusedIndex)

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
              ref={(el) => { buttonRefs.current[index] = el }}
              type="button"
              tabIndex={index === focusedIndex.current ? 0 : -1}
              onFocus={() => { focusedIndex.current = index }}
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
  )
}

// ---------------------------------------------------------------------------
// Public component — dispatches to the appropriate variant
// ---------------------------------------------------------------------------

export function AccessibleNodeList(props: Props): JSX.Element {
  if (props.brainLevel !== undefined) {
    return <BrainLevelList {...props} />
  }
  return <LegacyList {...props} />
}
