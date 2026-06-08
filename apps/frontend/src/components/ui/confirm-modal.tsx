import { useEffect, type ReactNode, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from './button.tsx';

interface ConfirmModalProps {
  title: ReactNode;
  description: ReactNode;
  confirmLabel: string;
  tone?: 'primary' | 'danger';
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmModal({
  title,
  description,
  confirmLabel,
  tone = 'primary',
  onConfirm,
  onCancel,
}: ConfirmModalProps): JSX.Element {
  const { t } = useTranslation();
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onCancel();
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onCancel]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
    >
      <div className="w-full max-w-md rounded-lg border border-border bg-surface p-5 shadow-2xl">
        <h2 className="text-base font-semibold text-fg">{title}</h2>
        <p className="mt-2 text-sm text-fg-muted">{description}</p>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="secondary" size="md" onClick={onCancel}>
            {t('modal.cancel')}
          </Button>
          <Button variant={tone === 'danger' ? 'danger' : 'primary'} size="md" onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
