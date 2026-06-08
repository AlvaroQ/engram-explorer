import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { Loader2, Plug, PlugZap, Search, X } from 'lucide-react';
import { useMemo, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { api, type CloudCapabilities, type ProjectStats, type SyncProjectRow } from '../lib/api.ts';
import { useEnrollProject, useUnenrollProject } from '../lib/cloud-mutations.ts';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Input } from '../components/ui/input.tsx';
import { Badge } from '../components/ui/badge.tsx';
import { ConfirmModal } from '../components/ui/confirm-modal.tsx';

/**
 * Refresh window after a fresh enroll. The backend's grace period is 5 min;
 * we poll every 5s during the first 2 min so the user sees the badge flip
 * from "syncing" → "healthy" as soon as the daemon's first ack lands.
 */
const POST_ENROLL_FAST_POLL_MS = 5_000;
const POST_ENROLL_FAST_POLL_DURATION_MS = 2 * 60_000;

function shouldFastPoll(syncRows: SyncProjectRow[] | undefined): boolean {
  if (!syncRows) return false;
  const nowMs = Date.now();
  return syncRows.some((p) => {
    if (!p.enrolled) return false;
    if (p.status !== 'idle' && p.status !== 'broken') return false;
    if ((p.last_acked_seq ?? 0) > 0) return false;
    if (!p.enrolled_at) return false;
    const enrolledMs = Date.parse(`${p.enrolled_at}Z`);
    if (Number.isNaN(enrolledMs)) return false;
    return nowMs - enrolledMs < POST_ENROLL_FAST_POLL_DURATION_MS;
  });
}

export function ProjectsPage(): JSX.Element {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');

  const projects = useQuery({
    queryKey: ['projects'],
    queryFn: api.listProjects,
    staleTime: 60_000,
  });
  const sync = useQuery({
    queryKey: ['sync-projects'],
    queryFn: api.syncProjects,
    staleTime: 30_000,
    refetchInterval: (query) =>
      shouldFastPoll(query.state.data?.projects) ? POST_ENROLL_FAST_POLL_MS : false,
  });
  const capsQuery = useQuery({
    queryKey: ['cloud-capabilities'],
    queryFn: api.cloudCapabilities,
    staleTime: 5 * 60_000,
  });
  const caps: CloudCapabilities = capsQuery.data ?? { enroll: false, unenroll: false, raw: '' };

  const syncByName = useMemo(() => {
    const map = new Map<string, SyncProjectRow>();
    for (const p of sync.data?.projects ?? []) map.set(p.project, p);
    return map;
  }, [sync.data]);

  const filtered = useMemo(() => {
    const items = projects.data?.items ?? [];
    if (filter.trim().length === 0) return items;
    const needle = filter.toLowerCase();
    return items.filter((p) => p.project.toLowerCase().includes(needle));
  }, [projects.data, filter]);

  return (
    <div className="flex flex-col gap-5">
      <header>
        <h1 className="text-2xl font-semibold text-fg">{t('projects.title')}</h1>
        <p className="text-sm text-fg-muted">{t('projects.description')}</p>
      </header>

      <Card>
        <CardHeader
          title={t('projects.all')}
          description={t('projects.count', { count: filtered.length })}
          actions={
            <div className="relative w-64">
              <Search
                size={14}
                className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-fg-muted"
              />
              <Input
                className={filter ? 'pl-9 pr-9' : 'pl-9'}
                placeholder={t('projects.filterPlaceholder')}
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
              />
              {filter ? (
                <button
                  type="button"
                  onClick={() => setFilter('')}
                  aria-label={t('common.clear')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-fg-muted hover:bg-surface-2 hover:text-fg focus:outline-none focus:ring-1 focus:ring-accent/40"
                >
                  <X size={14} />
                </button>
              ) : null}
            </div>
          }
        />
        <CardBody className="p-0">
          {projects.isLoading ? (
            <Skeleton className="h-32" />
          ) : filtered.length === 0 ? (
            <p className="p-6 text-center text-sm text-fg-muted">{t('projects.noMatch')}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-surface-2 text-left text-xs uppercase tracking-wide text-fg-muted">
                  <tr>
                    <th className="px-3 py-2">{t('projects.table.project')}</th>
                    <th className="px-3 py-2">{t('projects.table.observations')}</th>
                    <th className="px-3 py-2">{t('projects.table.sessions')}</th>
                    <th className="px-3 py-2">{t('projects.table.prompts')}</th>
                    <th className="px-3 py-2">{t('projects.table.lastActivity')}</th>
                    <th className="px-3 py-2">{t('projects.table.syncLag')}</th>
                    <th className="px-3 py-2">{t('projects.table.sync')}</th>
                    <th className="px-3 py-2 text-right">{t('syncHealth.projects.table.actions')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {filtered.map((p) => (
                    <ProjectRow
                      key={p.project || '__empty__'}
                      stats={p}
                      sync={syncByName.get(p.project)}
                      caps={caps}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardBody>
      </Card>
    </div>
  );
}

function ProjectRow({
  stats,
  sync,
  caps,
}: {
  stats: ProjectStats;
  sync: SyncProjectRow | undefined;
  caps: CloudCapabilities;
}): JSX.Element {
  const { t } = useTranslation();
  const [confirm, setConfirm] = useState<'enroll' | 'unenroll' | null>(null);
  const isEmpty = stats.project === '';
  const label = isEmpty ? t('common.emptyNameBug') : stats.project;
  const enrolled = Boolean(sync?.enrolled);

  const enroll = useEnrollProject(stats.project);
  const unenroll = useUnenrollProject(stats.project);

  const isPending = enroll.isPending || unenroll.isPending;
  const actionDisabled = isEmpty || isPending || (enrolled ? !caps.unenroll : !caps.enroll);
  const actionTitle = isEmpty
    ? undefined
    : enrolled
      ? caps.unenroll
        ? t('syncHealth.projects.actions.unenroll')
        : t('syncHealth.projects.actions.unenrollUnsupported')
      : caps.enroll
        ? t('syncHealth.projects.actions.enroll')
        : t('syncHealth.projects.actions.enrollUnsupported');
  const actionAriaLabel = enrolled
    ? t('syncHealth.projects.actions.unenroll')
    : t('syncHealth.projects.actions.enroll');

  return (
    <tr className="hover:bg-surface-2">
      <td className="px-3 py-2">
        {isEmpty ? (
          <Link
            to="/projects/_orphans"
            className="font-mono text-xs text-fail hover:underline"
          >
            {label}
          </Link>
        ) : (
          <Link
            to="/projects/$project"
            params={{ project: stats.project }}
            className="font-medium text-fg hover:text-accent"
          >
            {label}
          </Link>
        )}
      </td>
      <td className="px-3 py-2 text-fg-muted">{stats.obs_count}</td>
      <td className="px-3 py-2 text-fg-muted">{stats.sessions_count}</td>
      <td className="px-3 py-2 text-fg-muted">{stats.prompts_count}</td>
      <td className="px-3 py-2 font-mono text-xs text-fg-muted">{stats.last_activity ?? '—'}</td>
      <td className="px-3 py-2">
        <SyncLagCell
          enrolled={enrolled}
          pending={stats.pending_mutations}
          syncUpdatedAt={stats.sync_updated_at}
        />
      </td>
      <td className="px-3 py-2">
        <SyncStatusCell sync={sync} />
      </td>
      <td className="px-3 py-2 text-right">
        {!isEmpty && (
          <button
            type="button"
            onClick={() => setConfirm(enrolled ? 'unenroll' : 'enroll')}
            disabled={actionDisabled}
            title={actionTitle}
            aria-label={actionAriaLabel}
            className={
              enrolled
                ? 'inline-flex items-center justify-center rounded-md p-1.5 text-fail hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-40'
                : 'inline-flex items-center justify-center rounded-md p-1.5 text-accent hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-40'
            }
          >
            {enrolled ? <PlugZap size={16} /> : <Plug size={16} />}
          </button>
        )}
        {confirm ? (
          <ConfirmModal
            title={
              confirm === 'enroll'
                ? t('syncHealth.projects.confirmEnroll', { project: stats.project })
                : t('syncHealth.projects.confirmUnenroll', { project: stats.project })
            }
            description={
              confirm === 'enroll'
                ? t('syncHealth.projects.confirmEnrollDesc', { project: stats.project })
                : t('syncHealth.projects.confirmUnenrollDesc', { project: stats.project })
            }
            confirmLabel={
              confirm === 'enroll'
                ? t('syncHealth.projects.actions.enroll')
                : t('syncHealth.projects.actions.unenroll')
            }
            tone={confirm === 'enroll' ? 'primary' : 'danger'}
            onConfirm={() => {
              const action = confirm;
              setConfirm(null);
              if (action === 'enroll') enroll.mutate();
              else unenroll.mutate();
            }}
            onCancel={() => setConfirm(null)}
          />
        ) : null}
      </td>
    </tr>
  );
}

function SyncLagCell({
  enrolled,
  pending,
  syncUpdatedAt,
}: {
  enrolled: boolean;
  pending: number;
  syncUpdatedAt: string | null;
}): JSX.Element {
  const { t } = useTranslation();

  if (!enrolled || syncUpdatedAt === null) {
    return <span className="text-xs text-fg-muted">{t('projects.syncLag.noSync')}</span>;
  }

  const elapsed = elapsedSince(syncUpdatedAt);
  const ago =
    elapsed === null
      ? null
      : elapsed.unit === 'd'
        ? t('projects.syncLag.agoDays', { value: elapsed.value })
        : elapsed.unit === 'h'
          ? t('projects.syncLag.agoHours', { value: elapsed.value })
          : elapsed.unit === 'm'
            ? t('projects.syncLag.agoMinutes', { value: elapsed.value })
            : t('projects.syncLag.agoSeconds', { value: elapsed.value });
  const lastSyncTitle = t('projects.syncLag.lastSyncTitle', { time: syncUpdatedAt });

  if (pending === 0) {
    return (
      <span
        className="inline-flex items-center gap-1.5 text-xs text-ok"
        title={lastSyncTitle}
      >
        <span className="h-1.5 w-1.5 rounded-full bg-ok" aria-hidden />
        {t('projects.syncLag.fresh')}
      </span>
    );
  }

  return (
    <span
      className="inline-flex items-center gap-2 text-xs"
      title={t('projects.syncLag.pendingTitle', { count: pending })}
    >
      <Badge tone="fail">{t('projects.syncLag.pendingShort', { count: pending })}</Badge>
      {ago ? <span className="text-fg-muted">{ago}</span> : null}
    </span>
  );
}

function SyncStatusCell({ sync }: { sync: SyncProjectRow | undefined }): JSX.Element {
  const { t } = useTranslation();

  if (!sync || !sync.enrolled) {
    return <Badge tone="neutral">{t('projects.syncBadge.notEnrolled')}</Badge>;
  }

  const enq = sync.last_enqueued_seq ?? 0;
  const ack = sync.last_acked_seq ?? 0;

  // Recently enrolled, daemon hasn't sent its first ack yet. The backend
  // returns status='idle' during the grace period — we show a distinctive
  // "syncing" pill with a spinner so the user knows it's not broken, just
  // working.
  if (sync.status === 'idle' && sync.lifecycle === 'pending' && ack === 0 && enq > 0) {
    return (
      <span
        className="inline-flex items-center gap-1.5 rounded-full border border-accent/40 bg-accent/10 px-2 py-0.5 text-xs text-accent"
        title={t('projects.syncBadge.syncingTitle', { enq })}
      >
        <Loader2 size={12} className="animate-spin" aria-hidden />
        {t('projects.syncBadge.syncing')}
      </span>
    );
  }

  if (sync.status === 'broken') {
    return <Badge tone="fail">{t('projects.syncBadge.broken')}</Badge>;
  }

  if (sync.status === 'degraded') {
    return (
      <Badge
        tone="warn"
        title={
          sync.last_error || sync.reason_message
            ? t('projects.syncBadge.degradedTitle', {
                error: sync.last_error ?? sync.reason_message ?? '',
              })
            : undefined
        }
      >
        {t('projects.syncBadge.degraded')}
      </Badge>
    );
  }

  if (sync.status === 'healthy') {
    // When we have telemetry, show ack/enq progress on hover. enq===ack
    // means everything is caught up; otherwise the user can see exactly
    // how far along the daemon is.
    const percent = enq > 0 ? Math.floor((ack / enq) * 100) : 100;
    return (
      <Badge
        tone="ok"
        title={
          enq > 0
            ? t('projects.syncBadge.progressTitle', { acked: ack, enq, percent })
            : undefined
        }
      >
        {t('projects.syncBadge.healthy')}
      </Badge>
    );
  }

  // Fallback for any other lifecycle we don't model explicitly.
  return <Badge tone="warn">{sync.status}</Badge>;
}

function elapsedSince(iso: string): { value: number; unit: 's' | 'm' | 'h' | 'd' } | null {
  const ms = Date.now() - Date.parse(iso);
  if (Number.isNaN(ms) || ms < 0) return null;
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return { value: sec, unit: 's' };
  const min = Math.floor(sec / 60);
  if (min < 60) return { value: min, unit: 'm' };
  const hr = Math.floor(min / 60);
  if (hr < 24) return { value: hr, unit: 'h' };
  return { value: Math.floor(hr / 24), unit: 'd' };
}
