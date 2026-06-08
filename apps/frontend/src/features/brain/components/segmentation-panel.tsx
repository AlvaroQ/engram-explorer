/**
 * segmentation-panel.tsx — Collapsible left segmentation panel.
 *
 * Implements D8/D9 (segmentation panel B): search box + stackable facet groups
 * (AND-across-groups / OR-within-group) + legend strip.
 *
 * Contracts:
 *   S6.1  Search: label.includes(query), AND-composed with facets.
 *   S6.1  Facet groups: project, type, scope, tool, date — multi-select checkboxes.
 *   S6.2  Facet availability (C8): only renders groups returned by availableFacets.
 *   S6.3  Non-match dimming: calls onMatchedIdsChange with the computed matched Set.
 *   S9.3  Zero-match message + Clear filters action.
 *   S10.2 Focusable controls: native <input type="checkbox">, focusable buttons.
 *
 * ProjectFacetGroup (C6.3): replaces the old ProjectRail for project isolation.
 * The old left/right project rails are no longer rendered from brain-page.tsx.
 */

import { useState, useMemo, useCallback, useEffect, type JSX, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronDown, ChevronRight, Search, X } from 'lucide-react';
import type { BrainModel, BrainLevel, ActiveFacets, FacetGroupId, ViewMode } from '../model/types.ts';
import { buildFacetIndex } from '../model/facets.ts';
import { computeMatchedIds, isZeroMatch, buildLegendEntries } from './segmentation-panel-logic.ts';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SegmentationPanelProps {
  /** The active brain model — used to get availableFacets(). */
  model: BrainModel;
  /** The current level's nodes for facet index + dimming. */
  level: BrainLevel;
  /** Called whenever the matched id set changes (for canvas dimming). */
  onMatchedIdsChange: (ids: Set<string> | null) => void;
  /** Active color-by for legend rendering. */
  colorBy: 'project' | 'type';
  /** Active view mode — controls legend label. */
  viewMode: ViewMode;
}

// ---------------------------------------------------------------------------
// ProjectFacetGroup (C6.3) — replaces ProjectRail
// ---------------------------------------------------------------------------

/**
 * A facet group specifically for projects.
 * Replaces the old left/right ProjectRail components (C6.3).
 * Integrated into SegmentationPanel as the 'project' facet group.
 */
export function ProjectFacetGroup({
  values,
  selected,
  onToggle,
}: {
  values: string[];
  selected: string[];
  onToggle: (value: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <FacetGroup
      groupId="project"
      label={t('brain.facetLabels.project')}
      values={values}
      selected={selected}
      onToggle={onToggle}
    />
  );
}

// ---------------------------------------------------------------------------
// FacetGroup
// ---------------------------------------------------------------------------

function FacetGroup({
  groupId,
  label,
  values,
  selected,
  onToggle,
}: {
  groupId: FacetGroupId;
  label: string;
  values: string[];
  selected: string[];
  onToggle: (value: string) => void;
}): JSX.Element {
  const [expanded, setExpanded] = useState(true);

  return (
    <div className="border-b border-border last:border-b-0">
      {/* Group header */}
      <button
        type="button"
        className="flex w-full items-center justify-between px-3 py-2 text-xs font-medium text-fg hover:bg-surface-2"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
        aria-controls={`facet-group-${groupId}`}
      >
        <span>{label}</span>
        {expanded ? (
          <ChevronDown className="h-3 w-3 text-fg-muted" aria-hidden />
        ) : (
          <ChevronRight className="h-3 w-3 text-fg-muted" aria-hidden />
        )}
      </button>

      {/* Values list — always in the DOM so aria-controls id is always valid */}
      <ul
        id={`facet-group-${groupId}`}
        hidden={!expanded}
        className="px-3 pb-2 pt-0.5 flex flex-col gap-1"
      >
        {values.map((val) => {
          const checked = selected.includes(val);
          const checkboxId = `facet-${groupId}-${val}`;
          return (
            <li key={val} className="flex items-center gap-2">
              <input
                id={checkboxId}
                type="checkbox"
                checked={checked}
                onChange={() => onToggle(val)}
                className="h-3 w-3 rounded border-border accent-accent"
                aria-label={`${label}: ${val}`}
              />
              <label
                htmlFor={checkboxId}
                className="flex-1 cursor-pointer truncate text-[11px] text-fg"
              >
                {val}
              </label>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Legend strip
// ---------------------------------------------------------------------------

function LegendStrip({
  level,
  colorBy,
  viewMode,
}: {
  level: BrainLevel;
  colorBy: 'project' | 'type';
  viewMode: ViewMode;
}): JSX.Element | null {
  const { t } = useTranslation();

  const rawEntries = useMemo(
    () => buildLegendEntries(level.nodes, 'color'),
    [level.nodes],
  );

  if (rawEntries.length === 0) return null;

  const isOrganico = viewMode === 'organico';

  // In organico mode the colors map to clusters, not projects/types.
  // Re-label each entry as "Cluster N" (1-based).
  const entries = isOrganico
    ? rawEntries.map((e, i) => ({ ...e, label: t('brain.legend.clusterN', { n: i + 1 }) }))
    : rawEntries;

  const heading = isOrganico
    ? t('brain.legend.colorByCluster')
    : t('brain.legend.colorByLabel', { grouping: t(`brain.colorBy.${colorBy}`) });

  return (
    <div className="px-3 py-2 border-t border-border">
      <p className="mb-1.5 text-[10px] uppercase tracking-wide text-fg-muted">
        {heading}
      </p>
      <ul className="flex flex-col gap-1">
        {entries.map((e) => (
          <li key={e.color} className="flex items-center gap-2">
            <span
              className="h-2 w-2 shrink-0 rounded-full"
              style={{ backgroundColor: e.color }}
              aria-hidden
            />
            <span className="truncate text-[11px] text-fg">{e.label}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

// ---------------------------------------------------------------------------
// SegmentationPanel (main)
// ---------------------------------------------------------------------------

export function SegmentationPanel({
  model,
  level,
  onMatchedIdsChange,
  colorBy,
  viewMode,
}: SegmentationPanelProps): JSX.Element {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const [activeFacets, setActiveFacets] = useState<ActiveFacets>({});
  const [collapsed, setCollapsed] = useState(false);

  // Which facet groups to show (C8 — driven by model.availableFacets())
  const availableGroups = useMemo(() => model.availableFacets(), [model]);

  // Facet value index for the current level
  const facetIndex = useMemo(() => buildFacetIndex(level.nodes), [level.nodes]);

  // Compute matched ids whenever inputs change
  const matchedIds = useMemo(() => {
    return computeMatchedIds(level.nodes, activeFacets, query);
  }, [level.nodes, activeFacets, query]);

  // Notify parent of changes — useEffect (not useMemo) is the correct place for side effects
  useEffect(() => {
    onMatchedIdsChange(matchedIds);
  }, [matchedIds, onMatchedIdsChange]);

  const handleQueryChange = useCallback((e: ChangeEvent<HTMLInputElement>) => {
    setQuery(e.target.value);
  }, []);

  const handleToggleFacetValue = useCallback((groupId: FacetGroupId, value: string) => {
    setActiveFacets((prev) => {
      const current = prev[groupId] ?? [];
      const next = current.includes(value)
        ? current.filter((v) => v !== value)
        : [...current, value];
      return { ...prev, [groupId]: next };
    });
  }, []);

  const handleClearFilters = useCallback(() => {
    setQuery('');
    setActiveFacets({});
  }, []);

  const zeroMatch = isZeroMatch(matchedIds);
  const hasActiveFilter =
    query.length > 0 ||
    Object.values(activeFacets).some((vals) => (vals?.length ?? 0) > 0);

  if (collapsed) {
    return (
      <div className="pointer-events-auto flex flex-col items-center rounded-lg border border-border bg-surface shadow-lg p-1.5">
        <button
          type="button"
          aria-label={t('brain.segments.expandPanel')}
          onClick={() => setCollapsed(false)}
          className="rounded p-1 text-fg-muted hover:bg-surface-2 hover:text-fg"
        >
          <ChevronRight className="h-4 w-4" aria-hidden />
        </button>
      </div>
    );
  }

  return (
    <div className="pointer-events-auto flex w-52 flex-col overflow-hidden rounded-lg border border-border bg-surface shadow-lg">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <span className="text-xs font-semibold text-fg">{t('brain.segments.title')}</span>
        <button
          type="button"
          aria-label={t('brain.segments.collapsePanel')}
          onClick={() => setCollapsed(true)}
          className="rounded p-0.5 text-fg-muted hover:bg-surface-2 hover:text-fg"
        >
          <ChevronDown className="h-3.5 w-3.5 rotate-90" aria-hidden />
        </button>
      </div>

      {/* Search box */}
      <div className="relative border-b border-border px-3 py-2">
        <Search className="pointer-events-none absolute left-5 top-1/2 h-3 w-3 -translate-y-1/2 text-fg-muted" aria-hidden />
        <input
          type="search"
          placeholder={t('brain.segments.searchPlaceholder')}
          value={query}
          onChange={handleQueryChange}
          className="w-full rounded border border-border bg-transparent py-1 pl-6 pr-2 text-xs text-fg placeholder:text-fg-muted focus:border-accent focus:outline-none"
          aria-label={t('brain.segments.searchPlaceholder')}
        />
      </div>

      {/* Zero-match message (S9.3) */}
      {zeroMatch && (
        <div className="border-b border-border px-3 py-2">
          <p className="text-[11px] text-fg-muted">{t('brain.segments.zeroMatches')}</p>
          <button
            type="button"
            onClick={handleClearFilters}
            className="mt-1 text-[11px] text-accent underline hover:no-underline"
          >
            {t('brain.segments.clearFilters')}
          </button>
        </div>
      )}

      {/* Facet groups — stackable, one per availableFacets() */}
      <div className="flex-1 overflow-y-auto">
        {availableGroups.map((groupId) => {
          const values = [...(facetIndex.get(groupId) ?? [])].sort();
          if (values.length === 0) return null;
          const selected = activeFacets[groupId] ?? [];
          return (
            <FacetGroup
              key={groupId}
              groupId={groupId}
              label={t(`brain.facetLabels.${groupId}`)}
              values={values}
              selected={selected}
              onToggle={(val) => handleToggleFacetValue(groupId, val)}
            />
          );
        })}

        {/* Show hint when no facet groups available */}
        {availableGroups.length === 0 && (
          <p className="px-3 py-4 text-[11px] text-fg-muted text-center">
            {t('brain.segments.noFacets')}
          </p>
        )}
      </div>

      {/* Clear all filters button (when filter is active but not zero-match) */}
      {hasActiveFilter && !zeroMatch && (
        <div className="border-t border-border px-3 py-2">
          <button
            type="button"
            onClick={handleClearFilters}
            className="flex items-center gap-1 text-[11px] text-fg-muted hover:text-fg"
          >
            <X className="h-3 w-3" aria-hidden />
            {t('brain.segments.clearFilters')}
          </button>
        </div>
      )}

      {/* Legend strip */}
      <LegendStrip level={level} colorBy={colorBy} viewMode={viewMode} />
    </div>
  );
}
