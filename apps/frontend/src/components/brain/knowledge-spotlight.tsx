/**
 * knowledge-spotlight.tsx — Cmd/Ctrl+K command palette for the Brain page.
 *
 * Architecture:
 *   - Pure DOM overlay, mounted OUTSIDE the R3F <Canvas> as a fixed-position scrim.
 *   - Never imports useFrame/invalidate; never causes canvas remount.
 *   - z-40 (above hover tooltip z-30, above detail z-20).
 *   - Combobox pattern: focus stays on the input, aria-activedescendant tracks active row.
 *   - Focus trap: Tab/Shift+Tab cycles inside panel; restores focus to trigger on close.
 *   - prefers-reduced-motion: suppresses open/close animation.
 *
 * Empty state (simplified per user feedback — single rail only):
 *   - Shows ONLY "Recent sessions" (5 items, enrich=true).
 *   - Each session row renders: project · date / summary or recent_title / tag chips.
 *   - Typed state unchanged: "In the graph" + "In content" result sections.
 */

import {
  useEffect,
  useRef,
  useState,
  useCallback,
  useMemo,
  type JSX,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Search, Layers, X } from 'lucide-react';
import { api, type GraphNode, type ObservationSearchItem } from '../../lib/api.ts';
import { useSpotlightSearch } from '../../lib/use-spotlight-search.ts';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface KnowledgeSpotlightProps {
  open: boolean;
  onClose: () => void;
  nodes: GraphNode[];
  /** Precomputed lowercased search string per node id. Built in brain-page. */
  searchableIndex: Map<number, string>;
  /** Calls brain-page handleNodeClick (fly-to + opens NodeDetailPanel). */
  onSelectNode: (node: GraphNode) => void;
  /** Calls brain-page navigate (router push). */
  onNavigate: (path: string) => void;
}

// ---------------------------------------------------------------------------
// Snippet renderer — splits on <mark> to avoid dangerouslySetInnerHTML (ADR-5)
// ---------------------------------------------------------------------------

function renderSnippet(snippet: string): ReactNode[] {
  const parts = snippet.split(/(<mark>|<\/mark>)/);
  const nodes: ReactNode[] = [];
  let inside = false;
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    if (part === '<mark>') {
      inside = true;
    } else if (part === '</mark>') {
      inside = false;
    } else if (part) {
      if (inside) {
        nodes.push(
          <mark key={i} className="bg-accent/30 text-fg font-medium not-italic rounded-sm px-px">
            {part}
          </mark>,
        );
      } else {
        nodes.push(<span key={i}>{part}</span>);
      }
    }
  }
  return nodes;
}

// ---------------------------------------------------------------------------
// Section header
// ---------------------------------------------------------------------------

function SectionHeader({ label }: { label: string }): JSX.Element {
  return (
    <div className="px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wide text-fg-muted select-none">
      {label}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Option row — using string | undefined (not just ?) for exactOptionalPropertyTypes
// ---------------------------------------------------------------------------

interface OptionRowProps {
  id: string;
  isActive: boolean;
  onClick: () => void;
  icon: ReactNode | undefined;
  label: string;
  sub: string | undefined;
  snippet: string | undefined;
  badge: string | undefined;
  /** Tag chips — topic_keys from enrich. */
  tags: string[] | undefined;
}

function OptionRow({
  id,
  isActive,
  onClick,
  icon,
  label,
  sub,
  snippet,
  badge,
  tags,
}: OptionRowProps): JSX.Element {
  const rowRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isActive && rowRef.current) {
      rowRef.current.scrollIntoView({ block: 'nearest' });
    }
  }, [isActive]);

  return (
    <div
      id={id}
      ref={rowRef}
      role="option"
      aria-selected={isActive}
      onClick={onClick}
      className={`flex cursor-pointer items-start gap-3 px-3 py-2 transition-colors ${
        isActive
          ? 'bg-accent/15 text-fg'
          : 'text-fg-muted hover:bg-surface-2 hover:text-fg'
      }`}
    >
      {icon && (
        <span className="mt-0.5 shrink-0 text-fg-muted">{icon}</span>
      )}
      <div className="min-w-0 flex-1">
        {/* Line 1: label + badge */}
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium text-fg">{label}</span>
          {badge !== undefined && (
            <span className="rounded border border-border px-1.5 py-0.5 text-[10px] text-fg-muted shrink-0">
              {badge}
            </span>
          )}
        </div>
        {/* Line 2: sub (date/summary) */}
        {sub !== undefined && (
          <div className="mt-0.5 truncate text-[11px] text-fg-muted line-clamp-1">{sub}</div>
        )}
        {/* Snippet (search results only) */}
        {snippet !== undefined && (
          <div className="mt-0.5 text-[11px] text-fg-muted leading-snug line-clamp-2 italic">
            {renderSnippet(snippet)}
          </div>
        )}
        {/* Line 3: tag chips */}
        {tags !== undefined && tags.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {tags.slice(0, 3).map((tag) => (
              <span
                key={tag}
                className="rounded border border-border px-1.5 py-0.5 text-[10px] text-fg-muted"
              >
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Focus trap helper
// ---------------------------------------------------------------------------

function getFocusable(container: HTMLElement): HTMLElement[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>(
      'input, button, [href], select, textarea, [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => !el.hasAttribute('disabled') && el.tabIndex !== -1);
}

// ---------------------------------------------------------------------------
// FlatRow — all optional string fields use string | undefined (exactOptionalPropertyTypes)
// ---------------------------------------------------------------------------

type RowAction =
  | { kind: 'select-node'; node: GraphNode }
  | { kind: 'navigate'; path: string }
  | { kind: 'select-content'; item: ObservationSearchItem };

interface FlatRow {
  id: string;
  action: RowAction;
  label: string;
  sub: string | undefined;
  snippet: string | undefined;
  badge: string | undefined;
  icon: ReactNode | undefined;
  tags: string[] | undefined;
}

// ---------------------------------------------------------------------------
// KnowledgeSpotlight — main export
// ---------------------------------------------------------------------------

export function KnowledgeSpotlight({
  open,
  onClose,
  nodes,
  searchableIndex,
  onSelectNode,
  onNavigate,
}: KnowledgeSpotlightProps): JSX.Element | null {
  const { t } = useTranslation();

  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);

  const panelRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);

  const { localResults, contentResults, isSearching } = useSpotlightSearch(
    query,
    nodes,
    searchableIndex,
    open,
  );

  // Build the set of node ids for in-graph resolution
  const nodeIdSet = useMemo(() => new Set(nodes.map((n) => n.id)), [nodes]);

  // Lazy recent sessions query — enriched, sorted by recent activity
  const railEnabled = open && query === '';

  const { data: sessionsData } = useQuery({
    queryKey: ['spotlight', 'sessions', 'enriched'],
    queryFn: () => api.listSessions({ limit: 5, sort: 'recent_activity', enrich: true }),
    enabled: railEnabled,
  });

  // ---------------------------------------------------------------------------
  // Flatten visible rows for keyboard nav
  // ---------------------------------------------------------------------------

  const flatRows = useMemo<FlatRow[]>(() => {
    const rows: FlatRow[] = [];
    const isTyping = query.trim() !== '';

    if (!isTyping) {
      // Single rail: Recent sessions (enriched)
      const recentSessions = sessionsData?.items?.slice(0, 5) ?? [];
      recentSessions.forEach((s) => {
        const dateSource = s.last_activity ?? s.started_at;
        const dateStr = dateSource
          ? new Date(dateSource).toLocaleDateString()
          : undefined;
        // Line 1: project · date
        const label = [s.project, dateStr].filter(Boolean).join(' · ') || s.id;
        // Line 2: brief summary = summary ?? recent_title
        const briefSummary = s.summary ?? s.recent_title ?? undefined;
        rows.push({
          id: `spotlight-opt-${String(rows.length)}`,
          action: { kind: 'navigate', path: `/sessions/${s.id}` },
          label,
          sub: briefSummary,
          snippet: undefined,
          badge: undefined,
          icon: <Layers className="h-3.5 w-3.5" />,
          tags: s.tags,
        });
      });
    } else {
      // In-graph results
      localResults.forEach((node) => {
        const sub = [node.project, node.type].filter(Boolean).join(' › ');
        rows.push({
          id: `spotlight-opt-${String(rows.length)}`,
          action: { kind: 'select-node', node },
          label: node.label ?? String(node.id),
          sub: sub || undefined,
          snippet: undefined,
          badge: undefined,
          icon: <Search className="h-3.5 w-3.5" />,
          tags: undefined,
        });
      });

      // In-content results
      contentResults.forEach((item) => {
        const inGraph = nodeIdSet.has(item.id);
        const sub = [item.project, item.type].filter(Boolean).join(' › ');
        rows.push({
          id: `spotlight-opt-${String(rows.length)}`,
          action: { kind: 'select-content', item },
          label: item.title ?? t('common.untitled'),
          sub: sub || undefined,
          snippet: item.snippet,
          badge: inGraph ? undefined : t('brain.spotlight.notInView'),
          icon: <Search className="h-3.5 w-3.5" />,
          tags: undefined,
        });
      });
    }

    return rows;
  }, [query, sessionsData, localResults, contentResults, nodeIdSet, t]);

  // Reset active index when results change
  useEffect(() => {
    setActiveIndex(0);
  }, [flatRows.length, query]);

  // ---------------------------------------------------------------------------
  // Activate a row
  // ---------------------------------------------------------------------------

  const activateRow = useCallback(
    (row: FlatRow) => {
      const { action } = row;
      if (action.kind === 'select-node') {
        onSelectNode(action.node);
        onClose();
        return;
      }
      if (action.kind === 'navigate') {
        onNavigate(action.path);
        onClose();
        return;
      }
      if (action.kind === 'select-content') {
        const inGraph = nodeIdSet.has(action.item.id);
        if (inGraph) {
          const node = nodes.find((n) => n.id === action.item.id);
          if (node) onSelectNode(node);
        } else {
          onNavigate(`/observations?id=${String(action.item.id)}`);
        }
        onClose();
        return;
      }
    },
    [onSelectNode, onNavigate, onClose, nodeIdSet, nodes],
  );

  // ---------------------------------------------------------------------------
  // Open/close effects
  // ---------------------------------------------------------------------------

  useEffect(() => {
    if (open) {
      previousFocusRef.current = document.activeElement as HTMLElement | null;
      window.setTimeout(() => inputRef.current?.focus(), 0);
      setQuery('');
      setActiveIndex(0);
    } else {
      if (previousFocusRef.current) {
        previousFocusRef.current.focus();
        previousFocusRef.current = null;
      }
    }
  }, [open]);

  // ---------------------------------------------------------------------------
  // Keyboard handler
  // ---------------------------------------------------------------------------

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLDivElement>) => {
      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault();
          setActiveIndex((i) => Math.min(i + 1, flatRows.length - 1));
          break;
        case 'ArrowUp':
          e.preventDefault();
          setActiveIndex((i) => Math.max(i - 1, 0));
          break;
        case 'Enter':
          e.preventDefault();
          if (flatRows[activeIndex] !== undefined) {
            activateRow(flatRows[activeIndex]!);
          }
          break;
        case 'Escape':
          e.preventDefault();
          onClose();
          break;
        case 'Tab': {
          if (!panelRef.current) break;
          const focusable = getFocusable(panelRef.current);
          if (focusable.length === 0) break;
          const first = focusable[0];
          const last = focusable[focusable.length - 1];
          if (first === undefined || last === undefined) break;
          if (e.shiftKey) {
            if (document.activeElement === first) {
              e.preventDefault();
              last.focus();
            }
          } else {
            if (document.activeElement === last) {
              e.preventDefault();
              first.focus();
            }
          }
          break;
        }
        default:
          break;
      }
    },
    [flatRows, activeIndex, activateRow, onClose],
  );

  // ---------------------------------------------------------------------------
  // Render helpers
  // ---------------------------------------------------------------------------

  const isTyping = query.trim() !== '';
  const resultCount = flatRows.length;
  const activeRowId = flatRows[activeIndex]?.id;
  const listboxId = 'spotlight-listbox';

  // Don't render anything when closed — keep all hooks above this guard
  if (!open) return null;

  const recentSessionsCount = sessionsData?.items?.slice(0, 5).length ?? 0;

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div
      className="fixed inset-0 z-40 flex items-start justify-center pt-[10vh] px-4 bg-black/55"
      onClick={onClose}
    >
      {/* Panel — stops click propagation so backdrop click is isolated */}
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={t('brain.spotlight.open')}
        className="w-full max-w-xl rounded-xl border border-border bg-surface shadow-2xl overflow-hidden flex flex-col"
        style={{ maxHeight: 'min(520px, 80vh)' }}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Search input row */}
        <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
          <Search className="h-4 w-4 shrink-0 text-fg-muted" aria-hidden />
          <input
            ref={inputRef}
            type="text"
            role="combobox"
            aria-expanded={true}
            aria-autocomplete="list"
            aria-controls={listboxId}
            aria-activedescendant={activeRowId}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('brain.spotlight.placeholder')}
            className="min-w-0 flex-1 bg-transparent text-sm text-fg placeholder:text-fg-muted focus:outline-none"
            autoComplete="off"
            spellCheck={false}
          />
          {query && (
            <button
              type="button"
              onClick={() => setQuery('')}
              aria-label={t('common.clear')}
              className="shrink-0 rounded p-0.5 text-fg-muted hover:text-fg focus:outline-none focus:ring-1 focus:ring-accent"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
          <button
            type="button"
            onClick={onClose}
            aria-label={t('brain.spotlight.close')}
            className="shrink-0 rounded p-0.5 text-fg-muted hover:text-fg focus:outline-none focus:ring-1 focus:ring-accent"
          >
            <kbd className="rounded border border-border px-1 py-0.5 text-[10px] font-mono text-fg-muted">
              ESC
            </kbd>
          </button>
        </div>

        {/* Results / rails */}
        <div
          id={listboxId}
          role="listbox"
          aria-label={t('brain.spotlight.open')}
          className="overflow-y-auto flex-1"
        >
          {!isTyping && (
            <>
              {/* Single rail: Recent sessions (enriched) */}
              {recentSessionsCount > 0 && (
                <div>
                  <SectionHeader label={t('brain.spotlight.sections.recentSessions')} />
                  {(sessionsData?.items?.slice(0, 5) ?? []).map((s, i) => {
                    const rowDef = flatRows[i];
                    return rowDef !== undefined ? (
                      <OptionRow
                        key={`sess-${s.id}`}
                        id={rowDef.id}
                        isActive={activeIndex === i}
                        onClick={() => activateRow(rowDef)}
                        label={rowDef.label}
                        sub={rowDef.sub}
                        snippet={rowDef.snippet}
                        badge={rowDef.badge}
                        icon={rowDef.icon}
                        tags={rowDef.tags}
                      />
                    ) : null;
                  })}
                </div>
              )}

              {/* Empty state when sessions rail is empty */}
              {recentSessionsCount === 0 && (
                <div className="px-3 py-8 text-center text-sm text-fg-muted">
                  {t('brain.spotlight.hint')}
                </div>
              )}
            </>
          )}

          {isTyping && (
            <>
              {/* In the graph */}
              <div>
                <SectionHeader label={t('brain.spotlight.sections.inGraph')} />
                {localResults.length === 0 ? (
                  <div className="px-3 py-2 text-sm text-fg-muted">{t('brain.spotlight.noMatches')}</div>
                ) : (
                  localResults.map((node, i) => {
                    const rowDef = flatRows[i];
                    return rowDef !== undefined ? (
                      <OptionRow
                        key={`local-${String(node.id)}`}
                        id={rowDef.id}
                        isActive={activeIndex === i}
                        onClick={() => activateRow(rowDef)}
                        label={rowDef.label}
                        sub={rowDef.sub}
                        snippet={rowDef.snippet}
                        badge={rowDef.badge}
                        icon={rowDef.icon}
                        tags={rowDef.tags}
                      />
                    ) : null;
                  })
                )}
              </div>

              {/* In content */}
              <div>
                <SectionHeader label={t('brain.spotlight.sections.inContent')} />
                {isSearching && (
                  <div className="px-3 py-2 text-sm text-fg-muted">{t('brain.spotlight.searching')}</div>
                )}
                {!isSearching && contentResults.length === 0 && (
                  <div className="px-3 py-2 text-sm text-fg-muted">{t('brain.spotlight.noMatches')}</div>
                )}
                {!isSearching &&
                  contentResults.map((item, i) => {
                    const globalIdx = localResults.length + i;
                    const rowDef = flatRows[globalIdx];
                    return rowDef !== undefined ? (
                      <OptionRow
                        key={`content-${String(item.id)}`}
                        id={rowDef.id}
                        isActive={activeIndex === globalIdx}
                        onClick={() => activateRow(rowDef)}
                        label={rowDef.label}
                        sub={rowDef.sub}
                        snippet={rowDef.snippet}
                        badge={rowDef.badge}
                        icon={rowDef.icon}
                        tags={rowDef.tags}
                      />
                    ) : null;
                  })}
              </div>
            </>
          )}
        </div>

        {/* Footer hint */}
        <div className="border-t border-border px-3 py-1.5 text-[10px] text-fg-muted flex items-center justify-between">
          <span>{t('brain.spotlight.hint')}</span>
          <span className="text-fg-muted/60">↑↓ ↵</span>
        </div>
      </div>

      {/* aria-live region — announces result count after search settles */}
      <div role="status" aria-live="polite" className="sr-only">
        {isTyping && !isSearching
          ? t('brain.spotlight.resultCount', { count: resultCount })
          : ''}
      </div>
    </div>
  );
}
