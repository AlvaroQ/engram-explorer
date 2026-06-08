import { useEffect, type ReactNode, type JSX } from 'react';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '../../lib/cn.ts';
import { Button } from './button.tsx';

interface DrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  widthClass?: string;
}

export function Drawer({
  open,
  onOpenChange,
  title,
  description,
  children,
  widthClass = 'w-full sm:w-[640px]',
}: DrawerProps): JSX.Element | null {
  const { t } = useTranslation();
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onOpenChange(false);
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onOpenChange]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex" role="dialog" aria-modal="true">
      <div
        className="flex-1 bg-black/40 backdrop-blur-sm"
        onClick={() => onOpenChange(false)}
        aria-hidden
      />
      <aside
        className={cn(
          'flex h-full flex-col border-l border-border bg-surface shadow-2xl shadow-black/40',
          widthClass,
        )}
      >
        <header className="flex items-start justify-between gap-3 border-b border-border px-5 py-4">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-fg">{title}</h2>
            {description ? (
              <p className="mt-1 truncate text-xs text-fg-muted">{description}</p>
            ) : null}
          </div>
          <Button variant="ghost" size="sm" aria-label={t('drawer.closeAria')} onClick={() => onOpenChange(false)}>
            <X size={16} />
          </Button>
        </header>
        <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
      </aside>
    </div>
  );
}
