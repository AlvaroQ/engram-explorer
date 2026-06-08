/**
 * TypeEditControl — editable type combobox for the NodeDetailPanel type badge.
 *
 * Mirrors the structure of ProjectEditControl: two-phase click, autosave on
 * select/Enter, spinner during save, error via i18next. The list comes from
 * GET /api/observations/types (cached 60 s) merged with WELL_KNOWN_TYPES seeds.
 *
 * Design constraints:
 *  - DOM overlay, NOT inside R3F Canvas — safe for frameloop="demand".
 *  - Cached ['observationTypes'] query — no eager refetch.
 *  - On success invalidates ['graph'], ['observation', id], ['observationTypes'].
 *  - Strings via i18next under brain.editType.*.
 *  - Manual composition, no UI library. Tailwind HSL tokens only.
 */

import { useMemo, useRef, useState, type JSX } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { Check, ChevronDown, Loader2, Plus } from 'lucide-react';
import { ApiRequestError, api, type GraphNode } from '../../lib/api.ts';

// ---------------------------------------------------------------------------
// Well-known type seeds — shown even on a fresh DB with no observations.
// ---------------------------------------------------------------------------

const WELL_KNOWN_TYPES: string[] = [
  'architecture',
  'bugfix',
  'config',
  'decision',
  'discovery',
  'learning',
  'manual',
  'pattern',
  'preference',
];

// ---------------------------------------------------------------------------
// Pure helper — exported so tests can import it directly.
// ---------------------------------------------------------------------------

/**
 * A typed type value is submittable when it is non-empty after trimming AND
 * different from the node's current type. Treats a null current type as ''.
 */
export function isSubmittableType(next: string, currentType: string | null): boolean {
  const value = next.trim();
  return value.length > 0 && value !== (currentType ?? '');
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export interface TypeEditControlProps {
  /** The graph node (observation) whose type is being edited. */
  node: GraphNode;
  /** The node's current type (null treated as empty). */
  currentType: string | null;
  /**
   * CSS color tint from the active colorBy palette.
   * Used to apply the same chip styling as the read-only badge.
   */
  color?: string | undefined;
}

export function TypeEditControl({ node, currentType, color }: TypeEditControlProps): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [draft, setDraft] = useState(currentType ?? '');
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Two-phase click: first click opens dropdown in select mode;
  // second click while focused switches to text-edit mode.
  const [editing, setEditing] = useState(false);
  const focusedRef = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // Cached types list (staleTime 60s).
  const typesQuery = useQuery({
    queryKey: ['observationTypes'],
    queryFn: api.listObservationTypes,
    staleTime: 60_000,
  });

  // Merge DB types with well-known seeds, deduplicate, sort.
  const typeNames = useMemo((): string[] => {
    const dbItems = typesQuery.data?.items ?? [];
    const merged = new Set<string>([...WELL_KNOWN_TYPES, ...dbItems]);
    return [...merged].sort((a, b) => a.localeCompare(b));
  }, [typesQuery.data]);

  // Show full list when input is empty or exactly matches an existing type;
  // filter when user types a partial query that isn't an exact match.
  const filteredOptions = useMemo((): string[] => {
    const needle = draft.trim().toLowerCase();
    const isExactExisting = needle.length > 0 && typeNames.some((t) => t.toLowerCase() === needle);
    if (needle.length === 0 || isExactExisting) return typeNames;
    return typeNames.filter((t) => t.toLowerCase().includes(needle));
  }, [typeNames, draft]);

  const targetExists = typeNames.includes(draft.trim());
  // Offer "New type: …" only for new non-empty values.
  const showCreateOption = draft.trim().length > 0 && !targetExists;

  const updateMutation = useMutation({
    mutationFn: (type: string) => api.updateObservation(node.id, { type }),
    onSuccess: (_res, type) => {
      void queryClient.invalidateQueries({ queryKey: ['graph'] });
      void queryClient.invalidateQueries({ queryKey: ['observation', node.id] });
      void queryClient.invalidateQueries({ queryKey: ['observationTypes'] });
      setError(null);
      setDropdownOpen(false);
      setEditing(false);
      setDraft(type);
    },
    onError: (err) => {
      setError(resolveErrorMessage(err, t));
    },
  });

  function submit(target: string): void {
    const value = target.trim();
    if (!isSubmittableType(value, currentType) || updateMutation.isPending) return;
    setError(null);
    updateMutation.mutate(value);
  }

  function handleSelectOption(name: string): void {
    setDraft(name);
    setEditing(false);
    submit(name);
  }

  // Chip styling mirrors the read-mode badge in node-detail-panel.
  const chipStyle =
    color != null
      ? { color, backgroundColor: `color-mix(in srgb, ${color} 18%, transparent)` }
      : undefined;
  const chipColorClass = color != null ? '' : 'bg-blue-500/20 text-blue-300';

  return (
    <div className="flex flex-col gap-0.5">
      {/* The combobox replaces the read-only badge. Keep badge-like sizing. */}
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={draft}
          readOnly={!editing}
          onMouseDown={() => {
            if (focusedRef.current && !editing) setEditing(true);
          }}
          onChange={(e) => {
            setDraft(e.target.value);
            setDropdownOpen(true);
            setError(null);
          }}
          onFocus={() => {
            focusedRef.current = true;
            setDropdownOpen(true);
          }}
          onBlur={() => {
            focusedRef.current = false;
            setEditing(false);
            setTimeout(() => setDropdownOpen(false), 150);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              submit(draft);
              setDropdownOpen(false);
            }
            if (e.key === 'Escape') {
              setDropdownOpen(false);
            }
          }}
          placeholder={t('brain.editType.placeholder')}
          disabled={updateMutation.isPending}
          aria-label={t('brain.editType.label')}
          style={!editing && !dropdownOpen ? chipStyle : undefined}
          className={`rounded px-2 py-0.5 text-xs font-medium outline-none transition-[border-color,box-shadow] duration-150 disabled:cursor-not-allowed disabled:opacity-60 ${
            !editing && !dropdownOpen
              ? `${chipColorClass} cursor-pointer select-none caret-transparent pr-6`
              : 'cursor-text border border-border bg-surface-2 pr-6 text-fg ring-2 ring-accent/25 focus:border-accent'
          }`}
        />

        {updateMutation.isPending ? (
          <Loader2
            className="pointer-events-none absolute right-1.5 top-1/2 size-3 -translate-y-1/2 animate-spin text-accent"
            aria-hidden
          />
        ) : (
          <ChevronDown
            className={`pointer-events-none absolute right-1.5 top-1/2 size-3 -translate-y-1/2 text-fg-muted transition-transform duration-150 ${
              dropdownOpen ? 'rotate-180' : ''
            }`}
            aria-hidden
          />
        )}

        {dropdownOpen && (filteredOptions.length > 0 || showCreateOption) && (
          <ul
            role="listbox"
            className="absolute left-0 top-full z-30 mt-1 max-h-44 min-w-[10rem] overflow-auto rounded-md border border-border bg-surface py-1 shadow-lg ring-1 ring-black/5"
          >
            {filteredOptions.map((name) => {
              const selected = (currentType ?? '') === name;
              return (
                <li key={name} role="option" aria-selected={selected}>
                  <button
                    type="button"
                    className={`flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-surface-2 ${
                      selected ? 'font-medium text-accent' : 'text-fg'
                    }`}
                    onMouseDown={(e) => {
                      e.preventDefault();
                      handleSelectOption(name);
                    }}
                  >
                    <Check
                      className={`size-3 shrink-0 ${selected ? 'text-accent' : 'text-transparent'}`}
                      aria-hidden
                    />
                    <span className="min-w-0 truncate">{name}</span>
                  </button>
                </li>
              );
            })}
            {showCreateOption && (
              <li role="option" aria-selected={false}>
                <button
                  type="button"
                  className="flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-xs text-accent transition-colors hover:bg-surface-2"
                  onMouseDown={(e) => {
                    e.preventDefault();
                    handleSelectOption(draft.trim());
                  }}
                >
                  <Plus className="size-3 shrink-0" aria-hidden />
                  <span className="min-w-0 truncate">
                    {t('brain.editType.createOption', { name: draft.trim() })}
                  </span>
                </button>
              </li>
            )}
          </ul>
        )}
      </div>

      {error !== null && (
        <p role="alert" className="mt-0.5 text-[11px] leading-snug text-fail">
          {error}
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Error message extraction.
// ---------------------------------------------------------------------------

function resolveErrorMessage(err: unknown, t: TFunction): string {
  if (err instanceof ApiRequestError) {
    return t('brain.editType.error.generic', { message: err.api.message });
  }
  const msg = err instanceof Error ? err.message : String(err);
  return t('brain.editType.error.generic', { message: msg });
}
