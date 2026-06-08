import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { AlertTriangle, ChevronRight, Plug, PlugZap } from 'lucide-react';
import { useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { api, type SyncIssue, type SyncProjectRow } from '../lib/api.ts';
import { useEnrollProject, useUnenrollProject } from '../lib/cloud-mutations.ts';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Badge } from '../components/ui/badge.tsx';
import { Button } from '../components/ui/button.tsx';
import { ConfirmModal } from '../components/ui/confirm-modal.tsx';

export function SyncHealthPage(): JSX.Element {
  const { t } = useTranslation();
  const projects = useQuery({
    queryKey: ['sync-projects'],
    queryFn: api.syncProjects,
    refetchInterval: 15_000,
    staleTime: 5_000,
  });
  const issues = useQuery({
    queryKey: ['sync-issues'],
    queryFn: api.syncIssues,
    refetchInterval: 15_000,
    staleTime: 5_000,
  });
  const capabilities = useQuery({
    queryKey: ['cloud-capabilities'],
    queryFn: api.cloudCapabilities,
    staleTime: 5 * 60_000,
  });
  const caps = capabilities.data ?? { enroll: true, unenroll: true, raw: '' };

  return (
    <div className="flex flex-col gap-5">
      <header>
        <h1 className="text-2xl font-semibold text-fg">{t('syncHealth.title')}</h1>
        <p className="text-sm text-fg-muted">{t('syncHealth.description')}</p>
      </header>

      {projects.data?.global_target ? (
        <GlobalTargetCard target={projects.data.global_target} />
      ) : null}

      <Card>
        <CardHeader title={t('syncHealth.projects.title')} description={t('syncHealth.projects.description')} />
        <CardBody className="p-0">
          {projects.isLoading ? (
            <Skeleton className="h-32" />
          ) : projects.isError || !projects.data ? (
            <p className="p-6 text-center text-sm text-fail">{t('syncHealth.projects.loadError')}</p>
          ) : (
            <ProjectsTable rows={projects.data.projects} caps={caps} />
          )}
        </CardBody>
      </Card>

      <IssuesBanner issues={issues.data?.issues ?? []} loading={issues.isLoading} />
    </div>
  );
}

function IssuesBanner({
  issues,
  loading,
}: {
  issues: SyncIssue[];
  loading: boolean;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (loading) return <Skeleton className="h-16" />;
  if (issues.length === 0) {
    return (
      <Card>
        <CardBody className="flex items-center gap-2">
          <span className="text-sm text-ok">{t('syncHealth.issues.none')}</span>
        </CardBody>
      </Card>
    );
  }
  const high = issues.filter((i) => i.severity === 'HIGH');
  return (
    <Card className={high.length > 0 ? 'border-fail/40' : 'border-warn/40'}>
      <CardHeader
        title={
          <span className="inline-flex items-center gap-2">
            <AlertTriangle size={14} className="text-warn" /> {t('syncHealth.issues.title', { count: issues.length })}
          </span>
        }
        description={
          high.length > 0
            ? t('syncHealth.issues.highSeverity', { count: high.length })
            : t('syncHealth.issues.nothingCritical')
        }
      />
      <CardBody className="space-y-2">
        {issues.slice(0, 6).map((i) => (
          <div
            key={`${i.code}-${i.project ?? 'global'}`}
            className="flex items-start gap-3 rounded-md border border-border bg-surface-2 p-3"
          >
            <Badge tone={severityTone(i.severity)}>{i.severity}</Badge>
            <div className="min-w-0">
              <p className="text-sm text-fg">{i.message}</p>
              {i.hint ? <p className="mt-1 text-xs text-fg-muted">{i.hint}</p> : null}
            </div>
          </div>
        ))}
      </CardBody>
    </Card>
  );
}

function severityTone(s: SyncIssue['severity']): 'fail' | 'warn' | 'accent' | 'neutral' {
  switch (s) {
    case 'HIGH':
      return 'fail';
    case 'MEDIUM':
      return 'warn';
    case 'LOW':
      return 'accent';
    default:
      return 'neutral';
  }
}

function statusTone(status: SyncProjectRow['status']): 'ok' | 'warn' | 'fail' | 'neutral' {
  switch (status) {
    case 'healthy':
      return 'ok';
    case 'degraded':
      return 'warn';
    case 'broken':
      return 'fail';
    default:
      return 'neutral';
  }
}

function GlobalTargetCard({
  target,
}: {
  target: NonNullable<Awaited<ReturnType<typeof api.syncProjects>>['global_target']>;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader title={t('syncHealth.globalTarget.title')} description={t('syncHealth.globalTarget.description')} />
      <CardBody className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label={t('syncHealth.globalTarget.lifecycle')} value={target.lifecycle ?? '—'} />
        <Stat label={t('syncHealth.globalTarget.lastEnqueued')} value={String(target.last_enqueued_seq ?? 0)} />
        <Stat label={t('syncHealth.globalTarget.lastAcked')} value={String(target.last_acked_seq ?? 0)} />
        <Stat label={t('syncHealth.globalTarget.pending')} value={String(target.pending_mutations)} />
      </CardBody>
    </Card>
  );
}

function Stat({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <div>
      <p className="text-xs uppercase tracking-wide text-fg-muted">{label}</p>
      <p className="text-base font-semibold text-fg">{value}</p>
    </div>
  );
}

function ProjectsTable({
  rows,
  caps,
}: {
  rows: SyncProjectRow[];
  caps: { enroll: boolean; unenroll: boolean };
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead className="bg-surface-2 text-left text-xs uppercase tracking-wide text-fg-muted">
          <tr>
            <th className="px-3 py-2">{t('syncHealth.projects.table.project')}</th>
            <th className="px-3 py-2">{t('syncHealth.projects.table.obs')}</th>
            <th className="px-3 py-2">{t('syncHealth.projects.table.lastActivity')}</th>
            <th className="px-3 py-2">{t('syncHealth.projects.table.enrolled')}</th>
            <th className="px-3 py-2">{t('syncHealth.projects.table.status')}</th>
            <th className="px-3 py-2">{t('syncHealth.projects.table.pending')}</th>
            <th className="px-3 py-2">{t('syncHealth.projects.table.cloudSeq')}</th>
            <th className="px-3 py-2 text-right">{t('syncHealth.projects.table.actions')}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {rows.map((p) => (
            <ProjectRow key={p.project || '__empty__'} row={p} caps={caps} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ProjectRow({
  row,
  caps,
}: {
  row: SyncProjectRow;
  caps: { enroll: boolean; unenroll: boolean };
}): JSX.Element {
  const { t } = useTranslation();
  const [confirm, setConfirm] = useState<'enroll' | 'unenroll' | null>(null);

  const enroll = useEnrollProject(row.project);
  const unenroll = useUnenrollProject(row.project);

  const projectLabel = row.project === '' ? t('common.empty') : row.project;
  const detailHref = row.project === '' ? null : `/sync/${encodeURIComponent(row.project)}`;
  const isPending = enroll.isPending || unenroll.isPending;

  return (
    <tr className="hover:bg-surface-2">
      <td className="px-3 py-2">
        <div className="flex items-center gap-2">
          <span className={row.project === '' ? 'font-mono text-xs text-fail' : 'text-fg'}>
            {projectLabel}
          </span>
        </div>
      </td>
      <td className="px-3 py-2 text-fg-muted">{row.obs_count}</td>
      <td className="px-3 py-2 font-mono text-xs text-fg-muted">{row.last_activity ?? '—'}</td>
      <td className="px-3 py-2">
        {row.enrolled ? (
          <Badge tone="ok">{t('syncHealth.projects.table.enrolledBadge')}</Badge>
        ) : (
          <Badge tone="neutral">{t('syncHealth.projects.table.notEnrolled')}</Badge>
        )}
      </td>
      <td className="px-3 py-2">
        <Badge tone={statusTone(row.status)}>{row.status}</Badge>
      </td>
      <td className="px-3 py-2 text-fg-muted">{row.pending_mutations}</td>
      <td className="px-3 py-2 font-mono text-xs text-fg-muted">
        {row.last_acked_seq ?? '—'} / {row.last_enqueued_seq ?? '—'}
      </td>
      <td className="px-3 py-2 text-right">
        <div className="flex items-center justify-end gap-2">
          {row.project !== '' &&
            (row.enrolled ? (
              <Button
                variant="danger"
                size="sm"
                disabled={isPending || !caps.unenroll}
                title={caps.unenroll ? undefined : t('syncHealth.projects.actions.unenrollUnsupported')}
                onClick={() => setConfirm('unenroll')}
              >
                <PlugZap size={12} /> {t('syncHealth.projects.actions.unenroll')}
              </Button>
            ) : (
              <Button
                variant="primary"
                size="sm"
                disabled={isPending || !caps.enroll}
                title={caps.enroll ? undefined : t('syncHealth.projects.actions.enrollUnsupported')}
                onClick={() => setConfirm('enroll')}
              >
                <Plug size={12} /> {t('syncHealth.projects.actions.enroll')}
              </Button>
            ))}
          {detailHref ? (
            <Link
              to="/sync/$project"
              params={{ project: row.project }}
              className="inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-fg-muted hover:text-fg"
            >
              {t('syncHealth.projects.table.detail')} <ChevronRight size={12} />
            </Link>
          ) : null}
        </div>
        {confirm ? (
          <ConfirmModal
            title={
              confirm === 'enroll'
                ? t('syncHealth.projects.confirmEnroll', { project: row.project })
                : t('syncHealth.projects.confirmUnenroll', { project: row.project })
            }
            description={
              confirm === 'enroll'
                ? t('syncHealth.projects.confirmEnrollDesc', { project: row.project })
                : t('syncHealth.projects.confirmUnenrollDesc', { project: row.project })
            }
            confirmLabel={
              confirm === 'enroll'
                ? t('syncHealth.projects.actions.enroll')
                : t('syncHealth.projects.actions.unenroll')
            }
            tone={confirm === 'enroll' ? 'primary' : 'danger'}
            onConfirm={() => {
              setConfirm(null);
              if (confirm === 'enroll') enroll.mutate();
              else unenroll.mutate();
            }}
            onCancel={() => setConfirm(null)}
          />
        ) : null}
      </td>
    </tr>
  );
}
