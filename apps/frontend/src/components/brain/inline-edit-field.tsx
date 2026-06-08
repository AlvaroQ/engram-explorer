/**
 * InlineEditField — reusable pencil-toggle inline edit for single-line and
 * multiline fields in the NodeDetailPanel.
 *
 * Read mode: shows the value (or a custom renderRead renderer for markdown)
 * plus a small Pencil icon button that enters edit mode.
 *
 * Edit mode: shows an <input> (single-line) or <textarea> (multiline) pre-filled
 * with the current value, plus Save (Check) and Cancel (X) buttons.
 *
 * Keyboard:
 *  - Single-line: Enter = save, Escape = cancel.
 *  - Multiline: Ctrl+Enter = save, Escape = cancel.
 *
 * Accessibility:
 *  - Pencil button has aria-label from brain.editField.edit (interpolated label).
 *  - Save / Cancel buttons are aria-labeled.
 *  - Focus moves into the field automatically on entering edit mode.
 *
 * Design constraints:
 *  - Pessimistic: disabled while pending, error surface via i18next.
 *  - No optimistic update: save calls onSave, on success parent invalidates cache.
 *  - Manual composition, no UI library. Tailwind HSL tokens only.
 */

import { useEffect, useRef, useState, type JSX, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { Check, Pencil, X } from 'lucide-react';
import { ApiRequestError } from '../../lib/api.ts';

// ---------------------------------------------------------------------------
// Pure helper — exported so tests can import it directly.
// ---------------------------------------------------------------------------

/**
 * Returns true when `next` (trimmed) differs from `original` (null treated as '').
 * Used to prevent saving a value that hasn't actually changed.
 */
export function hasChanged(next: string, original: string | null): boolean {
  return next.trim() !== (original ?? '').trim();
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export interface InlineEditFieldProps {
  /** Current persisted value (null rendered as empty). */
  value: string | null;
  /** Human-readable field label — used for aria-labels. */
  label: string;
  /** When true, renders a <textarea> instead of an <input>. Default false. */
  multiline?: boolean;
  /**
   * Called with the new value when the user confirms. May return a Promise.
   * Throw to signal failure; the component surfaces the error.
   */
  onSave: (next: string) => Promise<void>;
  /**
   * Optional custom read-mode renderer (e.g. MarkdownContent for the content
   * field). When omitted, the raw string value is rendered as plain text.
   */
  renderRead?: ((value: string) => ReactNode) | undefined;
  /** Optional additional className on the wrapper div. */
  className?: string;
}

export function InlineEditField({
  value,
  label,
  multiline = false,
  onSave,
  renderRead,
  className,
}: InlineEditFieldProps): JSX.Element {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value ?? '');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | HTMLTextAreaElement | null>(null);

  // Reset draft to the latest persisted value when value changes externally
  // (e.g. after a successful save + query invalidation re-renders the panel).
  useEffect(() => {
    if (!editing) {
      setDraft(value ?? '');
    }
  }, [value, editing]);

  // Move focus into the field as soon as edit mode activates.
  useEffect(() => {
    if (editing) {
      inputRef.current?.focus();
    }
  }, [editing]);

  function enterEdit(): void {
    setDraft(value ?? '');
    setError(null);
    setEditing(true);
  }

  function cancelEdit(): void {
    setDraft(value ?? '');
    setError(null);
    setEditing(false);
  }

  async function confirmSave(): Promise<void> {
    if (pending) return;
    // We allow saving even when value hasn't changed (idempotent) but the
    // hasChanged guard lets callers skip the network call if needed.
    setPending(true);
    setError(null);
    try {
      await onSave(draft);
      setEditing(false);
    } catch (err) {
      setError(resolveErrorMessage(err, label, t));
    } finally {
      setPending(false);
    }
  }

  const sharedInputClass =
    'w-full rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-fg outline-none transition-[border-color,box-shadow] duration-150 placeholder:text-fg-muted/60 focus:border-accent focus:ring-2 focus:ring-accent/25 disabled:cursor-not-allowed disabled:opacity-60';

  if (editing) {
    return (
      <div className={`flex flex-col gap-1 ${className ?? ''}`}>
        {multiline ? (
          <textarea
            ref={inputRef as React.Ref<HTMLTextAreaElement>}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.preventDefault();
                cancelEdit();
              }
              if (e.key === 'Enter' && e.ctrlKey) {
                e.preventDefault();
                void confirmSave();
              }
            }}
            disabled={pending}
            rows={6}
            aria-label={label}
            className={`${sharedInputClass} resize-y`}
          />
        ) : (
          <input
            ref={inputRef as React.Ref<HTMLInputElement>}
            type="text"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.preventDefault();
                cancelEdit();
              }
              if (e.key === 'Enter') {
                e.preventDefault();
                void confirmSave();
              }
            }}
            disabled={pending}
            aria-label={label}
            className={sharedInputClass}
          />
        )}

        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => void confirmSave()}
            disabled={pending}
            aria-label={t('brain.editField.save')}
            className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium text-accent transition-colors hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Check className="size-3 shrink-0" aria-hidden />
            {t('brain.editField.save')}
          </button>
          <button
            type="button"
            onClick={cancelEdit}
            disabled={pending}
            aria-label={t('brain.editField.cancel')}
            className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-fg-muted transition-colors hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <X className="size-3 shrink-0" aria-hidden />
            {t('brain.editField.cancel')}
          </button>
        </div>

        {error !== null && (
          <p role="alert" className="text-[11px] leading-snug text-fail">
            {error}
          </p>
        )}
      </div>
    );
  }

  // Read mode.
  const hasValue = value !== null && value !== '';

  return (
    <div className={`group flex min-w-0 items-start gap-1 ${className ?? ''}`}>
      <div className="min-w-0 flex-1">
        {hasValue ? (
          renderRead ? (
            renderRead(value)
          ) : (
            <span className="break-words text-xs text-fg">{value}</span>
          )
        ) : (
          <span className="text-xs text-fg-muted">{t('common.empty')}</span>
        )}
      </div>
      <button
        type="button"
        onClick={enterEdit}
        aria-label={t('brain.editField.edit', { label })}
        className="mt-0.5 shrink-0 rounded p-0.5 text-fg-muted opacity-0 transition-opacity group-hover:opacity-100 hover:bg-surface-2 focus:opacity-100"
      >
        <Pencil className="size-3" aria-hidden />
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Error message extraction.
// ---------------------------------------------------------------------------

function resolveErrorMessage(err: unknown, label: string, t: TFunction): string {
  if (err instanceof ApiRequestError) {
    return t('brain.editField.error.generic', { label, message: err.api.message });
  }
  const msg = err instanceof Error ? err.message : String(err);
  return t('brain.editField.error.generic', { label, message: msg });
}
