/**
 * ProjectEditControl — editable project combobox for the NodeDetailPanel.
 *
 * Single action: reassign THIS observation to another project. Picking an
 * existing project from the dropdown (or typing a new name + Enter) saves
 * immediately — there are no explicit action buttons.
 *
 * Whole-project rename/merge is intentionally NOT exposed here: it's a
 * destructive, project-wide operation that doesn't belong on a single node's
 * detail (and must never auto-fire on a click). The backend endpoint still
 * exists for a future dedicated UI.
 *
 * Design constraints honored:
 *  - DOM overlay (NOT inside R3F Canvas) — safe for frameloop="demand".
 *  - Uses the cached ['projects'] TanStack Query key (no extra fetch).
 *  - On success invalidates ['graph'] AND ['projects'].
 *  - Strings via i18next under brain.editProject.*.
 *  - Manual composition, no UI library. Tailwind HSL tokens only.
 *  - Compact: lives in a 2-col slot beside the read-only weight chip.
 */

import { useMemo, useRef, useState, type JSX } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { Check, ChevronDown, Loader2, Plus } from 'lucide-react';
import { ApiRequestError, api, type GraphNode, type ProjectStats } from '../../lib/api.ts';

// ---------------------------------------------------------------------------
// Pure helper — exported so tests can import it directly.
// ---------------------------------------------------------------------------

/**
 * A typed target is submittable (worth saving) when it is non-empty after
 * trimming AND different from the node's current project. Treats a null current
 * project as the empty string.
 */
export function isSubmittableTarget(target: string, currentProject: string | null): boolean {
  const value = target.trim();
  return value.length > 0 && value !== (currentProject ?? '');
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export interface ProjectEditControlProps {
  /** The graph node (observation) whose project is being edited. */
  node: GraphNode;
  /** The node's current project name (null for orphan nodes). */
  currentProject: string | null;
}

export function ProjectEditControl({ node, currentProject }: ProjectEditControlProps): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [draft, setDraft] = useState(currentProject ?? '');
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Two-phase click: the first click acts like a select (opens the dropdown, no
  // text caret); a second click while already focused switches to text editing.
  const [editing, setEditing] = useState(false);
  const focusedRef = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // Cached projects list (staleTime 60s — do NOT refetch on every render).
  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: api.listProjects,
    staleTime: 60_000,
  });

  const projectNames = useMemo((): string[] => {
    const items = projectsQuery.data?.items ?? [];
    return items
      .filter((p: ProjectStats) => p.project && p.project.trim() !== '')
      .map((p) => p.project)
      .sort((a, b) => a.localeCompare(b));
  }, [projectsQuery.data]);

  // Open on an empty input OR on the current selection (draft exactly matches an
  // existing project) → show the FULL list so the user can pick any project. Only
  // filter once they type a partial query that isn't itself an exact project name.
  const filteredOptions = useMemo((): string[] => {
    const needle = draft.trim().toLowerCase();
    const isExactExisting =
      needle.length > 0 && projectNames.some((p) => p.toLowerCase() === needle);
    if (needle.length === 0 || isExactExisting) return projectNames;
    return projectNames.filter((p) => p.toLowerCase().includes(needle));
  }, [projectNames, draft]);

  const targetExists = projectNames.includes(draft.trim());
  // Offer "New project: …" only when the draft is a new, non-empty name.
  const showCreateOption = draft.trim().length > 0 && !targetExists;

  // The single mutation: move THIS observation to the chosen project.
  const moveMutation = useMutation({
    mutationFn: (target: string) => api.assignProject('observation', node.id, target),
    onSuccess: (_res, target) => {
      void queryClient.invalidateQueries({ queryKey: ['graph'] });
      void queryClient.invalidateQueries({ queryKey: ['projects'] });
      setError(null);
      setDropdownOpen(false);
      setEditing(false);
      setDraft(target);
    },
    onError: (err) => {
      setError(resolveErrorMessage(err, t));
    },
  });

  // Autosave: move the node to `target` (existing or new name) when it's valid.
  function submit(target: string): void {
    const value = target.trim();
    if (!isSubmittableTarget(value, currentProject) || moveMutation.isPending) return;
    setError(null);
    moveMutation.mutate(value);
  }

  function handleSelectOption(name: string): void {
    setDraft(name);
    setEditing(false);
    submit(name);
  }

  return (
    <div className="col-span-2 flex min-w-0 flex-col gap-0.5">
      <dt className="text-[10px] uppercase tracking-wide text-fg-muted">
        {t('brain.editProject.label')}
      </dt>

      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={draft}
          readOnly={!editing}
          onMouseDown={() => {
            // First click (not yet focused) just opens the dropdown in select mode.
            // A second click while already focused switches to text-edit mode.
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
            // Delay close so a dropdown click registers before focus leaves.
            setTimeout(() => setDropdownOpen(false), 150);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              submit(draft);
              setDropdownOpen(false);
            }
          }}
          placeholder={t('brain.editProject.placeholder')}
          disabled={moveMutation.isPending}
          aria-label={t('brain.editProject.label')}
          className={`w-full rounded-md border border-border bg-surface-2 py-1 pl-2 pr-7 text-xs text-fg outline-none transition-[border-color,box-shadow] duration-150 placeholder:text-fg-muted/60 hover:border-fg-muted/40 focus:border-accent focus:ring-2 focus:ring-accent/25 disabled:cursor-not-allowed disabled:opacity-60 ${
            editing || dropdownOpen ? 'cursor-text' : 'cursor-pointer'
          } ${editing ? '' : 'select-none caret-transparent'}`}
        />

        {/* Right adornment: spinner while saving, otherwise the dropdown chevron. */}
        {moveMutation.isPending ? (
          <Loader2
            className="pointer-events-none absolute right-2 top-1/2 size-3.5 -translate-y-1/2 animate-spin text-accent"
            aria-hidden
          />
        ) : (
          <ChevronDown
            className={`pointer-events-none absolute right-2 top-1/2 size-3.5 -translate-y-1/2 text-fg-muted transition-transform duration-150 ${
              dropdownOpen ? 'rotate-180' : ''
            }`}
            aria-hidden
          />
        )}

        {dropdownOpen && (filteredOptions.length > 0 || showCreateOption) && (
          <ul
            role="listbox"
            className="absolute left-0 right-0 top-full z-30 mt-1 max-h-44 overflow-auto rounded-md border border-border bg-surface py-1 shadow-lg ring-1 ring-black/5"
          >
            {filteredOptions.map((name) => {
              const selected = (currentProject ?? '') === name;
              return (
                <li key={name} role="option" aria-selected={selected}>
                  <button
                    type="button"
                    className={`flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-surface-2 ${
                      selected ? 'font-medium text-accent' : 'text-fg'
                    }`}
                    onMouseDown={(e) => {
                      // mousedown fires before blur so the selection registers first.
                      e.preventDefault();
                      handleSelectOption(name);
                    }}
                  >
                    <Check
                      className={`size-3.5 shrink-0 ${selected ? 'text-accent' : 'text-transparent'}`}
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
                  <Plus className="size-3.5 shrink-0" aria-hidden />
                  <span className="min-w-0 truncate">
                    {t('brain.editProject.createOption', { name: draft.trim() })}
                  </span>
                </button>
              </li>
            )}
          </ul>
        )}
      </div>

      {error !== null && (
        <p role="alert" className="mt-1 text-[11px] leading-snug text-fail">
          {error}
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Error message extraction — standalone (uses no hooks).
// ---------------------------------------------------------------------------

function resolveErrorMessage(err: unknown, t: TFunction): string {
  if (err instanceof ApiRequestError) {
    return t('brain.editProject.error.generic', { message: err.api.message });
  }
  const msg = err instanceof Error ? err.message : String(err);
  return t('brain.editProject.error.generic', { message: msg });
}
