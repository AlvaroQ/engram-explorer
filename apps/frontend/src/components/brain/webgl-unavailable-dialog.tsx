/**
 * webgl-unavailable-dialog.tsx — modal shown when the browser can't create a
 * WebGL context (e.g. Chrome's "Use graphics acceleration when available" is
 * disabled). The 3D knowledge graph can't render, so we surface an actionable
 * dialog pointing the user at the relevant setting.
 *
 * Pure DOM overlay (rendered outside any R3F <Canvas>). Dismissible — closing it
 * leaves the inline placeholder + ⌘K spotlight + project rails usable.
 *
 * a11y: role="dialog" aria-modal, labelled/described by its title/body, Escape to
 * close, primary action auto-focused, backdrop click closes.
 */

import { useEffect, useRef, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { MonitorX, X } from 'lucide-react';

export interface WebGLUnavailableDialogProps {
  open: boolean;
  onClose: () => void;
}

export function WebGLUnavailableDialog({
  open,
  onClose,
}: WebGLUnavailableDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const reloadBtnRef = useRef<HTMLButtonElement | null>(null);

  // Escape closes; focus the primary action on open.
  useEffect(() => {
    if (!open) return;
    reloadBtnRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
      // Backdrop click closes; clicks inside the panel are stopped below.
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="webgl-dialog-title"
        aria-describedby="webgl-dialog-body"
        className="w-full max-w-md rounded-xl border border-border bg-surface p-5 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-surface-2 text-fg-muted">
            <MonitorX className="h-5 w-5" aria-hidden />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="webgl-dialog-title" className="text-base font-semibold text-fg">
              {t('brain.webglUnavailableTitle')}
            </h2>
            <p id="webgl-dialog-body" className="mt-1.5 text-sm leading-relaxed text-fg-muted">
              {t('brain.webglUnavailableHint')}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t('brain.webglDialogClose')}
            className="-mr-1 -mt-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-fg-muted hover:bg-surface-2 hover:text-fg focus:outline-none focus:ring-2 focus:ring-accent"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-border px-3 py-1.5 text-sm font-medium text-fg-muted hover:bg-surface-2 hover:text-fg focus:outline-none focus:ring-2 focus:ring-accent"
          >
            {t('brain.webglDialogDismiss')}
          </button>
          <button
            ref={reloadBtnRef}
            type="button"
            onClick={() => window.location.reload()}
            className="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-white hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-accent focus:ring-offset-2"
          >
            {t('brain.webglDialogReload')}
          </button>
        </div>
      </div>
    </div>
  );
}
