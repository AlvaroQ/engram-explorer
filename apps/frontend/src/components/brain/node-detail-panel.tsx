import { lazy, Suspense, useEffect, useMemo, useRef, type JSX } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowUpRight, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  api,
  type GraphNode,
  type ObservationDetailResponse,
} from '../../lib/api.ts';
import { ProjectEditControl } from './project-edit-control.tsx';
import { TypeEditControl } from './type-edit-control.tsx';
import { InlineEditField } from './inline-edit-field.tsx';
import { termFrequencies, topKeywords } from '../../lib/tfidf.ts';
import { Skeleton } from '../ui/skeleton.tsx';
import type { BrainLevel, BrainNode } from './types.ts';
import { resolveNeighbors, type NeighborEntry } from './neighbors.ts';
import { SYNAPSE_COLORS } from './brain-tunnel-flow.ts';

// Markdown renderer (react-markdown + remark-gfm) is lazy-loaded so the parser
// stays out of the initial bundle — it only loads when a node is expanded.
const MarkdownContent = lazy(() => import('./markdown-content.tsx'));

// All nodes in the new contract are observations. Navigate to the observations page.
export function resolveNodeRoute(_node: GraphNode): string {
  return '/ui/observations';
}

// Field keys we know how to render, in display order. `type`, `scope` and
// `duplicates` are intentionally omitted: type is already shown as the badge,
// scope is always "project" here, and duplicate count is already folded into the
// node weight (bigger sphere) — so none of them add signal for the user.
// topicKey spans the full row (rendered first), then project + weight share the
// row below it.
const FIELD_ORDER = ['topicKey', 'project', 'weight'] as const;
type FieldKey = (typeof FIELD_ORDER)[number];

// Grid placement within the 3-column field list: topicKey takes the full row,
// project takes the remaining two columns of the second row, and weight gets the
// last narrow column, right-aligned.
const FIELD_SPAN: Record<FieldKey, string> = {
  topicKey: 'col-span-3',
  project: 'col-span-2',
  weight: 'ml-auto w-fit rounded-md border border-border px-2 py-1 text-center',
};

// Turn "2026-03-04 21:27:23" into a locale-friendly string. Falls back to the
// raw value if it doesn't parse.
function formatCreated(raw: string): string {
  const parsed = new Date(raw.includes('T') ? raw : raw.replace(' ', 'T'));
  if (Number.isNaN(parsed.getTime())) return raw;
  return parsed.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

// A stacked label/value cell — small muted label over the value.
function FieldCell({
  label,
  value,
  mono = false,
  className,
}: {
  label: string;
  value: string;
  mono?: boolean;
  className?: string;
}): JSX.Element {
  return (
    <div className={`flex min-w-0 flex-col gap-0.5 ${className ?? ''}`}>
      <dt className="text-[10px] uppercase tracking-wide text-fg-muted">{label}</dt>
      <dd className={`break-words text-fg ${mono ? 'font-mono' : 'font-medium'}`}>{value}</dd>
    </div>
  );
}

export interface NodeDetailPanelProps {
  node: GraphNode;
  onClose: () => void;
  onNavigate: (path: string) => void;
  /** When true, renders a narrower panel suitable for small preview cards. */
  compact?: boolean;
  /**
   * Rendering variant:
   * - 'overlay' (default): absolute floating panel, anchored top-right inside the
   *   nearest positioned ancestor. Used by preview cards and legacy callers.
   * - 'panel': block element that fills its parent column (no absolute positioning,
   *   no overflow-hidden; the parent column handles scroll).
   */
  variant?: 'overlay' | 'panel';
  /**
   * CSS color of the selected node under the active colorBy. Used to tint the
   * type badge so it matches the node the user clicked. Falls back to a neutral
   * blue badge when absent (e.g. preview cards without a resolved palette).
   */
  color?: string | undefined;
  /**
   * Navigate to another observation by its `topic_key` when a `[[wikilink]]`
   * chip in the expanded content is clicked. Omitted on compact preview cards,
   * where wikilinks render as static pills.
   */
  onWikilink?: ((topicKey: string) => void) | undefined;
  /**
   * The BrainNode representation of the selected node (if in brain model mode).
   * Used for neighbor resolution (M9).
   */
  brainNode?: BrainNode | undefined;
  /**
   * The current brain level (for neighbor resolution).
   * Required for the Neighbors section (S7.1 / M9).
   */
  currentLevel?: BrainLevel | undefined;
  /**
   * The parent brain level (for M9 leaf fallback).
   * When selected is a leaf and current level has no incident edges,
   * neighbors are resolved from parentLevel.
   */
  parentLevel?: BrainLevel | null | undefined;
  /**
   * Called when the user clicks a neighbor entry to select/enter that node.
   * Receives the neighbor's BrainNode id.
   */
  onNeighborSelect?: ((nodeId: string) => void) | undefined;
}

export function NodeDetailPanel({
  node,
  onClose,
  onNavigate,
  compact = false,
  variant = 'overlay',
  color,
  onWikilink,
  brainNode,
  currentLevel,
  parentLevel,
  onNeighborSelect,
}: NodeDetailPanelProps): JSX.Element {
  const { t } = useTranslation();
  const panelRef = useRef<HTMLDivElement>(null);
  const route = resolveNodeRoute(node);
  // All nodes are observations — show a neutral type badge using the node's type field.
  const badgeLabel = node.type ?? 'observation';
  const title = node.label ?? String(node.id);

  // Close on click outside the panel and on Escape.
  // - pointerup (not pointerdown) so a drag-release on the canvas doesn't dismiss.
  // - Clicks that originated inside the canvas wrapper are ignored to prevent
  //   node-click events from closing the panel immediately after opening it.
  useEffect(() => {
    function handlePointerUp(e: PointerEvent): void {
      const target = e.target as Element | null;
      // Ignore if the pointer-up happened inside a brain canvas wrapper.
      if (target?.closest?.('[data-brain-canvas]')) return;
      if (panelRef.current && !panelRef.current.contains(target)) {
        onClose();
      }
    }
    function handleKeyDown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    }
    document.addEventListener('pointerup', handlePointerUp);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('pointerup', handlePointerUp);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [onClose]);

  // The expanded detail is always shown, so fetch the full record up front.
  // Disabled for synthetic nodes (id <= 0) that are not in the API data set.
  const obsQuery = useQuery({
    queryKey: ['observation', node.id],
    queryFn: () => api.getObservation(node.id),
    staleTime: 60_000,
    enabled: node.id > 0,
  });

  // Build the base definition list.
  const rawValues: Record<FieldKey, string | null> = {
    project: node.project ?? null,
    topicKey: node.topicKey ?? null,
    weight: String(node.weight),
  };
  const createdLabel = node.createdAt != null ? formatCreated(node.createdAt) : null;
  const fields = FIELD_ORDER.filter((key) => rawValues[key] !== null).map((key) => ({
    key,
    label: t(`brain.fields.${key}`),
    value: rawValues[key] as string,
  }));

  const widthClass = compact ? 'w-72' : variant === 'panel' ? 'w-full' : 'w-[28rem]';

  const bodyPaddingClass = compact ? 'p-3' : 'p-4';

  const titleHint = t('brain.openHint');

  // Positional classes depend on variant:
  // - 'overlay': absolute floating panel anchored top-right, clipped to parent
  // - 'panel': normal block, fills parent column; parent handles scroll
  const containerPositionClass =
    variant === 'panel'
      ? 'flex flex-col rounded-lg border border-border bg-surface shadow-xl transition-[width] duration-200 ease-out pointer-events-auto max-h-[calc(100vh-128px)] overflow-y-auto'
      : `absolute right-3 top-3 z-20 flex max-h-[calc(100%-1.5rem)] flex-col overflow-hidden rounded-lg border border-border bg-surface shadow-xl transition-[width] duration-200 ease-out pointer-events-auto`;

  return (
    <div
      ref={panelRef}
      role="dialog"
      aria-label={title}
      className={`${containerPositionClass} ${widthClass}`}
    >
      <div className={variant === 'panel' ? bodyPaddingClass : `overflow-y-auto ${bodyPaddingClass}`}>
        {/* Type badge on the left (now an editable combobox), creation date on the right. */}
        <div className="flex items-center justify-between gap-2">
          {node.id > 0 ? (
            <TypeEditControl node={node} currentType={node.type} color={color} />
          ) : (
            <span
              className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${
                color != null ? '' : 'bg-blue-500/20 text-blue-300'
              }`}
              style={
                color != null
                  ? { color, backgroundColor: `color-mix(in srgb, ${color} 18%, transparent)` }
                  : undefined
              }
            >
              {badgeLabel}
            </span>
          )}
          <div className="flex shrink-0 items-center gap-2">
            {createdLabel !== null ? (
              <span className="text-[11px] text-fg-muted">{createdLabel}</span>
            ) : null}
            {/* Close just this detail dialog. Separate from the page's exit-focus /
                "restore view" button — this leaves the camera focus untouched. */}
            <button
              type="button"
              onClick={onClose}
              aria-label={t('brain.closeDetail')}
              title={t('brain.closeDetail')}
              className="flex size-5 items-center justify-center rounded text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg focus:outline-none focus:ring-2 focus:ring-accent"
            >
              <X className="size-3.5" aria-hidden />
            </button>
          </div>
        </div>

        {/* Title — inline editable; ArrowUpRight opens the observation page. */}
        <div className="mt-2 flex w-full items-start gap-1.5">
          {node.id > 0 ? (
            <TitleEditField node={node} />
          ) : (
            <span className="min-w-0 flex-1 text-sm font-semibold leading-snug text-fg">
              {title}
            </span>
          )}
          <button
            type="button"
            onClick={() => onNavigate(route)}
            title={titleHint}
            className="group mt-0.5 shrink-0"
          >
            <ArrowUpRight
              className="size-3.5 text-fg-muted transition-colors group-hover:text-accent"
              aria-hidden
            />
          </button>
        </div>

        {/* Base definition list — 3 columns. The project field is replaced by the
            editable ProjectEditControl (a compact combobox in the 2-col slot); the
            weight chip sits beside it in the remaining column, and topicKey uses
            an InlineEditField on its own row. */}
        <dl className="mt-3 grid grid-cols-3 gap-x-4 gap-y-2.5 border-t border-border pt-3 text-xs">
          {fields.map((f) =>
            f.key === 'project' ? (
              <ProjectEditControl key="project" node={node} currentProject={node.project} />
            ) : f.key === 'topicKey' && node.id > 0 ? (
              <div key="topicKey" className={`flex min-w-0 flex-col gap-0.5 ${FIELD_SPAN['topicKey']}`}>
                <dt className="text-[10px] uppercase tracking-wide text-fg-muted">{f.label}</dt>
                <dd>
                  <TopicKeyEditField node={node} value={node.topicKey} />
                </dd>
              </div>
            ) : (
              <FieldCell
                key={f.key}
                label={f.label}
                value={f.value}
                mono={f.key === 'topicKey'}
                className={FIELD_SPAN[f.key]}
              />
            ),
          )}
        </dl>

        {/* Full observation detail — always shown. */}
        <section className="mt-3">
          {node.id <= 0 ? (
            <p className="text-xs text-fg-muted">{t('brain.contentNotAvailable')}</p>
          ) : obsQuery.isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-3 w-1/3" />
              <Skeleton className="h-20" />
            </div>
          ) : obsQuery.isError || obsQuery.data == null ? (
            <p className="text-xs text-fail">{t('brain.detailError')}</p>
          ) : (
            <ObservationExpanded
              data={obsQuery.data as ObservationDetailResponse}
              nodeId={node.id}
              t={t}
              onWikilink={onWikilink}
            />
          )}
        </section>

        {/* Neighbors / Synapses section (S7.1 / M9) — only in brain model mode */}
        {brainNode && currentLevel ? (
          <NeighborsSection
            brainNode={brainNode}
            currentLevel={currentLevel}
            parentLevel={parentLevel ?? null}
            onNeighborSelect={onNeighborSelect}
          />
        ) : null}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// NeighborsSection — S7.1 / M9
// ---------------------------------------------------------------------------

/** Color badge for an edge relation. */
function RelationBadge({ relation }: { relation?: string | undefined }): JSX.Element {
  const color =
    relation === 'related'
      ? SYNAPSE_COLORS.related
      : relation === 'compatible'
      ? SYNAPSE_COLORS.compatible
      : relation === 'scoped'
      ? SYNAPSE_COLORS.scoped
      : SYNAPSE_COLORS.neutral;

  return (
    <span
      className="shrink-0 rounded px-1 py-0.5 text-[9px] font-medium uppercase"
      style={{
        color,
        backgroundColor: `color-mix(in srgb, ${color} 18%, transparent)`,
      }}
    >
      {relation ?? 'edge'}
    </span>
  );
}

/**
 * Neighbors / Synapses section in the detail panel.
 *
 * Resolves incident edges using M9 (leaf fallback to parent level).
 * Each neighbor entry shows the label, relation color badge, and is
 * clickable to select/enter the target node (S7.1).
 *
 * M9-B: neighbors whose node is not in the loaded graph render as
 * disabled items with "[not loaded]" label.
 */
function NeighborsSection({
  brainNode,
  currentLevel,
  parentLevel,
  onNeighborSelect,
}: {
  brainNode: BrainNode;
  currentLevel: BrainLevel;
  parentLevel: BrainLevel | null;
  onNeighborSelect?: ((nodeId: string) => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const neighbors: NeighborEntry[] = resolveNeighbors(
    brainNode.id,
    currentLevel,
    parentLevel,
  );

  // Build a map of id → label from both levels for display
  const idToLabel = new Map<string, string>();
  for (const n of currentLevel.nodes) idToLabel.set(n.id, n.label);
  if (parentLevel) {
    for (const n of parentLevel.nodes) {
      if (!idToLabel.has(n.id)) idToLabel.set(n.id, n.label);
    }
  }

  if (neighbors.length === 0) return null;

  return (
    <section className="mt-3 border-t border-border pt-3">
      <p className="mb-1.5 text-[10px] font-medium uppercase tracking-wide text-fg-muted">
        {t('brain.neighbors')}
      </p>
      <ul className="flex flex-col gap-1">
        {neighbors.map(({ neighborId, edge, loaded }) => {
          const label = idToLabel.get(neighborId) ?? neighborId;
          return (
            <li key={neighborId}>
              {loaded && onNeighborSelect ? (
                <button
                  type="button"
                  onClick={() => onNeighborSelect(neighborId)}
                  className="group flex w-full items-center gap-1.5 rounded px-1 py-0.5 text-left hover:bg-surface-2"
                >
                  <RelationBadge relation={edge.relation ?? undefined} />
                  <span className="min-w-0 flex-1 truncate text-xs text-fg transition-colors group-hover:text-accent">
                    {label}
                  </span>
                </button>
              ) : (
                <div
                  className="flex items-center gap-1.5 rounded px-1 py-0.5 opacity-50"
                  aria-disabled="true"
                >
                  <RelationBadge relation={edge.relation ?? undefined} />
                  <span className="min-w-0 flex-1 truncate text-xs text-fg-muted">
                    {loaded ? label : '[not loaded]'}
                  </span>
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function ObservationExpanded({
  data,
  nodeId,
  t,
  onWikilink,
}: {
  data: ObservationDetailResponse;
  nodeId: number;
  t: (key: string) => string;
  onWikilink?: ((topicKey: string) => void) | undefined;
}): JSX.Element {
  const o = data.observation;
  const queryClient = useQueryClient();

  // Compute top-5 keywords from the content; consistent with tfidf.ts tokenization.
  const contentKeywords = useMemo(
    () => (o.content ? topKeywords(termFrequencies(o.content), 5) : []),
    [o.content],
  );

  const contentMutation = useMutation({
    mutationFn: (content: string) => api.updateObservation(nodeId, { content }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['graph'] });
      void queryClient.invalidateQueries({ queryKey: ['observation', nodeId] });
    },
  });

  return (
    <div className="space-y-3 text-xs">
      {/* Keyword chips — shown between the metadata list and the markdown body.
          Rendered only when at least one keyword is available. */}
      {contentKeywords.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {contentKeywords.map((kw) => (
            <span key={kw} className="rounded bg-surface-2 px-1.5 py-0.5 text-[10px] text-fg-muted">
              {kw}
            </span>
          ))}
        </div>
      )}

      {/* topic_key intentionally omitted here — already shown in the collapsed
          field list above, so repeating it in the expanded view is redundant. */}
      <div>
        <p className="mb-1 text-[10px] font-medium uppercase tracking-wide text-fg-muted">
          {t('brain.fields.content')}
        </p>
        <InlineEditField
          value={o.content}
          label={t('brain.fields.content')}
          multiline
          onSave={async (next) => {
            await contentMutation.mutateAsync(next);
          }}
          renderRead={(v) => (
            <Suspense fallback={<Skeleton className="h-16" />}>
              <MarkdownContent content={v} onWikilink={onWikilink} />
            </Suspense>
          )}
        />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// TitleEditField — wraps InlineEditField for the panel heading (title field).
// ---------------------------------------------------------------------------

function TitleEditField({ node }: { node: GraphNode }): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const titleMutation = useMutation({
    mutationFn: (title: string) =>
      api.updateObservation(node.id, { title: title === '' ? null : title }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['graph'] });
      void queryClient.invalidateQueries({ queryKey: ['observation', node.id] });
    },
  });

  const displayValue = node.label ?? String(node.id);

  return (
    <div className="min-w-0 flex-1">
      <InlineEditField
        value={displayValue}
        label={t('brain.fields.title')}
        multiline={false}
        onSave={async (next) => {
          await titleMutation.mutateAsync(next);
        }}
        renderRead={(v) => (
          <span className="min-w-0 text-sm font-semibold leading-snug text-fg">{v}</span>
        )}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// TopicKeyEditField — wraps InlineEditField for the topic_key field.
// ---------------------------------------------------------------------------

function TopicKeyEditField({ node, value }: { node: GraphNode; value: string | null }): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const topicKeyMutation = useMutation({
    mutationFn: (topic_key: string) =>
      api.updateObservation(node.id, { topic_key: topic_key === '' ? null : topic_key }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['graph'] });
      void queryClient.invalidateQueries({ queryKey: ['observation', node.id] });
    },
  });

  return (
    <InlineEditField
      value={value}
      label={t('brain.fields.topicKey')}
      multiline={false}
      onSave={async (next) => {
        await topicKeyMutation.mutateAsync(next);
      }}
      renderRead={(v) => <span className="break-words font-mono text-fg">{v}</span>}
    />
  );
}
