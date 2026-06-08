import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from '@tanstack/react-router';
import { ChevronLeft, CloudUpload, Loader2 } from 'lucide-react';
import { useEffect, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiRequestError, api, type SyncProjectDetailResponse } from '../lib/api.ts';
import { Badge } from '../components/ui/badge.tsx';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';

type SyncFlash =
  | { tone: 'ok' | 'fail'; message: string }
  | null;

export function SyncProjectPage(): JSX.Element {
  const { t } = useTranslation();
  const { project } = useParams({ from: '/sync/$project' });
  const detail = useQuery({
    queryKey: ['sync-project', project],
    queryFn: () => api.syncProjectDetail(project),
    refetchInterval: 15_000,
  });

  const enrolled = detail.data?.summary.enrolled ?? false;

  return (
    <div className="flex flex-col gap-5">
      <header className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Link
            to="/sync"
            className="inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-fg-muted hover:text-fg"
          >
            <ChevronLeft size={12} /> {t('common.back')}
          </Link>
          <h1 className="text-xl font-semibold text-fg">{project}</h1>
        </div>
        <ProjectSyncButton project={project} enrolled={enrolled} />
      </header>

      {detail.isLoading ? (
        <Skeleton className="h-32" />
      ) : detail.isError || !detail.data ? (
        <p className="text-sm text-fail">{t('syncHealth.syncProject.notFound')}</p>
      ) : (
        <DetailBody data={detail.data} />
      )}
    </div>
  );
}

function ProjectSyncButton({
  project,
  enrolled,
}: {
  project: string;
  enrolled: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [flash, setFlash] = useState<SyncFlash>(null);

  const mutation = useMutation({
    mutationFn: () => api.cloudSyncProject(project),
    onSuccess: () => {
      setFlash({ tone: 'ok', message: t('syncHealth.syncProject.syncOk') });
      void queryClient.invalidateQueries({ queryKey: ['sync-project', project] });
      void queryClient.invalidateQueries({ queryKey: ['sync-projects'] });
      void queryClient.invalidateQueries({ queryKey: ['sync-issues'] });
      void queryClient.invalidateQueries({ queryKey: ['overview'] });
    },
    onError: (err) => {
      const message = err instanceof ApiRequestError ? err.api.message : (err as Error).message;
      setFlash({ tone: 'fail', message: t('syncHealth.syncProject.syncFailed', { message }) });
    },
  });

  useEffect(() => {
    if (!flash) return;
    const timer = setTimeout(() => setFlash(null), 6_000);
    return () => clearTimeout(timer);
  }, [flash]);

  const isPending = mutation.isPending;
  const disabled = isPending || !enrolled;
  const title = enrolled
    ? t('syncHealth.syncProject.tooltipEnabled')
    : t('syncHealth.syncProject.tooltipDisabled');

  return (
    <div className="flex items-center gap-2">
      {flash ? (
        <Badge tone={flash.tone} aria-live="polite">
          {flash.message}
        </Badge>
      ) : null}
      <button
        type="button"
        className="inline-flex items-center gap-2 rounded-md border border-border bg-surface-2 px-2.5 py-1.5 text-xs font-medium text-fg-muted hover:text-fg disabled:cursor-not-allowed disabled:opacity-60"
        aria-label={t('syncHealth.syncProject.ariaLabel', { project })}
        title={title}
        onClick={() => mutation.mutate()}
        disabled={disabled}
      >
        {isPending ? <Loader2 size={14} className="animate-spin" /> : <CloudUpload size={14} />}
        <span>{isPending ? t('syncHealth.syncProject.syncing') : t('syncHealth.syncProject.syncCloud')}</span>
      </button>
    </div>
  );
}

function DetailBody({ data }: { data: SyncProjectDetailResponse }): JSX.Element {
  const { t } = useTranslation();
  const { summary, pending, upgrade_state } = data;
  return (
    <>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader title={t('syncHealth.syncProject.status')} />
          <CardBody className="space-y-2 text-xs">
            <Row label={t('syncHealth.syncProject.status')} value={summary.status} />
            <Row label={t('syncHealth.syncProject.lifecycle')} value={summary.lifecycle} />
            <Row label={t('syncHealth.syncProject.enrolled')} value={summary.enrolled ? t('common.yes') : t('common.no')} />
            <Row label={t('syncHealth.syncProject.enrolledAt')} value={summary.enrolled_at} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title={t('syncHealth.syncProject.sequence')} />
          <CardBody className="space-y-2 text-xs">
            <Row label={t('syncHealth.syncProject.lastEnqueued')} value={String(summary.last_enqueued_seq ?? 0)} />
            <Row label={t('syncHealth.syncProject.lastAcked')} value={String(summary.last_acked_seq ?? 0)} />
            <Row label={t('syncHealth.syncProject.lastPulled')} value={String(summary.last_pulled_seq ?? 0)} />
            <Row label={t('syncHealth.syncProject.consecutiveFailures')} value={String(summary.consecutive_failures ?? 0)} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title={t('syncHealth.syncProject.counts')} />
          <CardBody className="space-y-2 text-xs">
            <Row label={t('syncHealth.syncProject.observations')} value={String(summary.obs_count)} />
            <Row label={t('syncHealth.syncProject.sessions')} value={String(summary.sessions_count)} />
            <Row label={t('syncHealth.syncProject.prompts')} value={String(summary.prompts_count)} />
            <Row label={t('syncHealth.syncProject.pendingMutations')} value={String(summary.pending_mutations)} />
          </CardBody>
        </Card>
      </div>

      {summary.last_error || summary.reason_message ? (
        <Card className="border-fail/40">
          <CardHeader title={t('syncHealth.syncProject.lastError')} />
          <CardBody className="space-y-1 text-sm text-fg">
            {summary.reason_message ? (
              <p className="text-fail">{summary.reason_message}</p>
            ) : null}
            {summary.last_error ? (
              <pre className="whitespace-pre-wrap text-xs text-fg-muted">{summary.last_error}</pre>
            ) : null}
            {summary.reason_code ? (
              <p className="text-xs text-fg-muted">{t('syncHealth.syncProject.errorCode', { code: summary.reason_code })}</p>
            ) : null}
          </CardBody>
        </Card>
      ) : null}

      <Card>
        <CardHeader
          title={t('syncHealth.syncProject.pendingTitle')}
          description={t('syncHealth.syncProject.pendingDesc', { count: pending.length })}
        />
        <CardBody className="p-0">
          {pending.length === 0 ? (
            <p className="p-4 text-sm text-fg-muted">{t('syncHealth.syncProject.noPending')}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead className="bg-surface-2 text-left text-fg-muted">
                  <tr>
                    <th className="px-3 py-2">{t('syncHealth.syncProject.table.seq')}</th>
                    <th className="px-3 py-2">{t('syncHealth.syncProject.table.entity')}</th>
                    <th className="px-3 py-2">{t('syncHealth.syncProject.table.op')}</th>
                    <th className="px-3 py-2">{t('syncHealth.syncProject.table.key')}</th>
                    <th className="px-3 py-2">{t('syncHealth.syncProject.table.occurredAt')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {pending.map((m) => (
                    <tr key={m.seq}>
                      <td className="px-3 py-1.5 font-mono">{m.seq}</td>
                      <td className="px-3 py-1.5">
                        <Badge tone="neutral">{m.entity}</Badge>
                      </td>
                      <td className="px-3 py-1.5">{m.op}</td>
                      <td className="px-3 py-1.5 truncate font-mono text-fg-muted">
                        {m.entity_key ?? '—'}
                      </td>
                      <td className="px-3 py-1.5 font-mono text-fg-muted">{m.occurred_at ?? '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardBody>
      </Card>

      {upgrade_state ? (
        <Card>
          <CardHeader title={t('syncHealth.syncProject.cloudUpgrade')} />
          <CardBody className="space-y-2 text-xs">
            <Row label={t('syncHealth.syncProject.stage')} value={upgrade_state.stage} />
            <Row label={t('syncHealth.syncProject.repairClass')} value={upgrade_state.repair_class} />
            <Row label={t('syncHealth.syncProject.updated')} value={upgrade_state.updated_at} />
            {upgrade_state.findings ? (
              <pre className="overflow-auto rounded-md border border-border bg-surface-2 p-3">
                {JSON.stringify(upgrade_state.findings, null, 2)}
              </pre>
            ) : null}
          </CardBody>
        </Card>
      ) : null}
    </>
  );
}

function Row({ label, value }: { label: string; value: string | null }): JSX.Element {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-fg-muted">{label}</span>
      <span className="truncate text-fg">{value ?? '—'}</span>
    </div>
  );
}
