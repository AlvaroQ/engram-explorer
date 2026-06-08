import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CloudUpload, Loader2 } from 'lucide-react';
import { useEffect, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ApiRequestError,
  api,
  type CloudSyncAllResponse,
  type HealthResponse,
} from '../../lib/api.ts';
import { cn } from '../../lib/cn.ts';

type Tone = 'neutral' | 'ok' | 'warn' | 'fail';

type SyncFlash = { tone: 'ok' | 'warn' | 'fail'; message: string } | null;

const DOT_COLOR: Record<Tone, string> = {
  neutral: 'bg-fg-muted',
  ok: 'bg-ok',
  warn: 'bg-warn',
  fail: 'bg-fail',
};

const FLASH_COLOR: Record<NonNullable<SyncFlash>['tone'], string> = {
  ok: 'text-ok',
  warn: 'text-warn',
  fail: 'text-fail',
};

export function SidebarFooter({ collapsed }: { collapsed: boolean }): JSX.Element {
  return (
    <div className={cn('mt-auto flex flex-col gap-2 border-t border-border pt-3', collapsed && 'items-center')}>
      <StatusIndicator collapsed={collapsed} />
      <SyncButton collapsed={collapsed} />
    </div>
  );
}

function deriveStatus(
  data: HealthResponse | undefined,
  loading: boolean,
): { tone: Tone; labelKey: string } {
  if (loading) return { tone: 'neutral', labelKey: 'sidebar.status.checking' };
  if (!data) return { tone: 'fail', labelKey: 'sidebar.status.backendUnreachable' };
  if (!data.db.ok) return { tone: 'fail', labelKey: 'sidebar.status.dbError' };
  if (!data.daemon.ok) return { tone: 'warn', labelKey: 'sidebar.status.daemonOffline' };
  return { tone: 'ok', labelKey: 'sidebar.status.allOk' };
}

function StatusIndicator({ collapsed }: { collapsed: boolean }): JSX.Element {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ['health'],
    queryFn: api.health,
    refetchInterval: 10_000,
    staleTime: 5_000,
  });
  const { tone, labelKey } = deriveStatus(data, isLoading);
  const label = t(labelKey);

  if (collapsed) {
    return (
      <div className="flex justify-center py-1.5" role="status" aria-label={label} title={label}>
        <span className={cn('size-2.5 rounded-full', DOT_COLOR[tone])} />
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2 px-2 py-1 text-xs text-fg-muted" role="status">
      <span className={cn('size-2 shrink-0 rounded-full', DOT_COLOR[tone])} />
      <span className="truncate">{label}</span>
    </div>
  );
}

function SyncButton({ collapsed }: { collapsed: boolean }): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [flash, setFlash] = useState<SyncFlash>(null);

  const mutation = useMutation<CloudSyncAllResponse>({
    mutationFn: () => api.cloudSyncAll(),
    onSuccess: (res) => {
      if (res.total === 0) {
        setFlash({ tone: 'warn', message: t('sidebar.sync.noEnrolled') });
      } else if (res.failed === 0) {
        setFlash({
          tone: 'ok',
          message: t('sidebar.sync.ok', { ok: String(res.ok), total: String(res.total) }),
        });
      } else {
        setFlash({
          tone: 'fail',
          message: t('sidebar.sync.partial', {
            ok: String(res.ok),
            total: String(res.total),
            failed: String(res.failed),
          }),
        });
      }
      void queryClient.invalidateQueries({ queryKey: ['health'] });
      void queryClient.invalidateQueries({ queryKey: ['overview'] });
      void queryClient.invalidateQueries({ queryKey: ['sync-projects'] });
      void queryClient.invalidateQueries({ queryKey: ['sync-project'] });
      void queryClient.invalidateQueries({ queryKey: ['sync-issues'] });
    },
    onError: (err) => {
      const message = err instanceof ApiRequestError ? err.api.message : (err as Error).message;
      setFlash({ tone: 'fail', message: t('sidebar.sync.failed', { message }) });
    },
  });

  useEffect(() => {
    if (!flash) return;
    const timer = setTimeout(() => setFlash(null), 6_000);
    return () => clearTimeout(timer);
  }, [flash]);

  const isPending = mutation.isPending;
  const icon = isPending ? <Loader2 size={16} className="animate-spin" /> : <CloudUpload size={16} />;

  if (collapsed) {
    return (
      <button
        type="button"
        className="flex size-9 items-center justify-center rounded-md border border-border bg-surface-2 text-fg-muted transition-colors hover:text-fg disabled:cursor-not-allowed disabled:opacity-60"
        aria-label={t('sidebar.sync.ariaLabel')}
        title={t('sidebar.sync.tooltip')}
        onClick={() => mutation.mutate()}
        disabled={isPending}
      >
        {icon}
      </button>
    );
  }

  return (
    <div className="flex flex-col gap-1">
      <button
        type="button"
        className="inline-flex w-full items-center justify-center gap-2 rounded-md border border-border bg-surface-2 px-3 py-2 text-xs font-medium text-fg-muted transition-colors hover:text-fg disabled:cursor-not-allowed disabled:opacity-60"
        aria-label={t('sidebar.sync.ariaLabel')}
        title={t('sidebar.sync.tooltip')}
        onClick={() => mutation.mutate()}
        disabled={isPending}
      >
        {icon}
        <span>{isPending ? t('sidebar.sync.syncing') : t('sidebar.sync.cloud')}</span>
      </button>
      {flash ? (
        <p
          className={cn('px-1 text-[11px] leading-tight', FLASH_COLOR[flash.tone])}
          aria-live="polite"
        >
          {flash.message}
        </p>
      ) : null}
    </div>
  );
}
