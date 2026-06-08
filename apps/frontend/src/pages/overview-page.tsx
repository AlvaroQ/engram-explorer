import { Fragment, useMemo, useState, type JSX } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
} from 'recharts';
import { Activity, ChevronDown, ChevronRight, FileText, Folders, Database } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  api,
  type OverviewResponse,
  type SyncIssue,
  type SyncIssuesResponse,
} from '../lib/api.ts';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Badge } from '../components/ui/badge.tsx';
import { cn } from '../lib/cn.ts';
import { ActivityByProjectChart } from '../components/charts/activity-by-project-chart.tsx';
import { BrainPreviewCard } from '../components/brain/brain-preview-card.tsx';

export function OverviewPage(): JSX.Element {
  const { t } = useTranslation();
  const overview = useQuery({ queryKey: ['overview'], queryFn: api.overview });
  const issues = useQuery({ queryKey: ['sync-issues'], queryFn: api.syncIssues });

  return (
    <div className="flex flex-col gap-6">
      <KpiRow data={overview.data} loading={overview.isLoading} />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-4">
        <Card className="flex flex-col lg:col-span-2">
          <CardHeader title={t('overview.activity.title')} description={t('overview.activity.description')} />
          <CardBody className="flex min-h-[260px] flex-1 flex-col">
            <ActivityByProjectChart />
          </CardBody>
        </Card>

        <Card className="flex flex-col">
          <CardHeader title={t('overview.byType.title')} description={t('overview.byType.description')} />
          <CardBody className="flex min-h-0 flex-1 flex-col">
            {overview.isLoading ? (
              <Skeleton className="h-full" />
            ) : overview.data ? (
              <TypeBreakdown data={overview.data.by_type} />
            ) : null}
          </CardBody>
        </Card>

        <SyncSummaryCard data={overview.data} loading={overview.isLoading} />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <BrainPreviewCard
          title={t('overview.activityDetail.byProject.title')}
          colorBy="project"
        />
        <BrainPreviewCard
          title={t('overview.activityDetail.byType.title')}
          colorBy="type"
        />
      </div>

      <Card>
        <CardHeader title={t('overview.recent.title')} description={t('overview.recent.description')} />
        <CardBody className="p-0">
          {overview.isLoading ? (
            <div className="px-5 py-4">
              <Skeleton className="h-32" />
            </div>
          ) : overview.data ? (
            <RecentTable items={overview.data.recent_observations} />
          ) : null}
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title={t('overview.issues.title')}
          description={t('overview.issues.description')}
          actions={
            issues.data ? (
              <span className="text-xs text-fg-muted">
                {t('overview.issues.generatedAt', {
                  time: formatDateTime(issues.data.generated_at),
                })}
              </span>
            ) : null
          }
        />
        <CardBody className="p-0">
          <IssuesTable data={issues.data} loading={issues.isLoading} />
        </CardBody>
      </Card>
    </div>
  );
}

function KpiRow({
  data,
  loading,
}: {
  data: OverviewResponse | undefined;
  loading: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const items: Array<{
    label: string;
    sublabel: string;
    value: string | number;
    icon: JSX.Element;
    to: '/observations' | '/sessions' | '/projects' | '/prompts';
  }> = [
    {
      label: t('overview.kpi.projects'),
      sublabel: t('overview.kpi.drillProject'),
      value: data?.kpis.projects ?? '—',
      icon: <Folders size={16} />,
      to: '/projects',
    },
    {
      label: t('overview.kpi.sessions'),
      sublabel: t('overview.kpi.seeTimeline'),
      value: data?.kpis.sessions ?? '—',
      icon: <Activity size={16} />,
      to: '/sessions',
    },
    {
      label: t('overview.kpi.observations'),
      sublabel: t('overview.kpi.browseAll'),
      value: data?.kpis.observations ?? '—',
      icon: <Database size={16} />,
      to: '/observations',
    },
    {
      label: t('overview.kpi.prompts'),
      sublabel: t('overview.kpi.searchPrompts'),
      value: data?.kpis.prompts ?? '—',
      icon: <FileText size={16} />,
      to: '/prompts',
    },
  ];
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
      {items.map((kpi) => (
        <Link
          key={kpi.label}
          to={kpi.to}
          className="group block rounded-lg border border-border bg-surface transition-colors hover:border-accent/40 hover:bg-surface-2"
        >
          <div className="flex items-start justify-between gap-3 px-5 py-4">
            <div>
              <p className="text-xs uppercase tracking-wide text-fg-muted">{kpi.label}</p>
              {loading ? (
                <Skeleton className="mt-2 h-7 w-16" />
              ) : (
                <p className="mt-1 text-2xl font-semibold text-fg">{kpi.value}</p>
              )}
              <p className="mt-1 inline-flex items-center gap-1 text-xs text-fg-muted group-hover:text-accent">
                {kpi.sublabel}
                <ChevronRight size={12} />
              </p>
            </div>
            <span className="rounded-md bg-surface-2 p-2 text-accent">{kpi.icon}</span>
          </div>
        </Link>
      ))}
    </div>
  );
}

const PIE_COLORS = [
  'hsl(224 90% 70%)',
  'hsl(142 76% 60%)',
  'hsl(38 92% 60%)',
  'hsl(0 84% 67%)',
  'hsl(280 80% 70%)',
  'hsl(190 80% 60%)',
  'hsl(330 70% 65%)',
  'hsl(60 80% 60%)',
];

function TypeBreakdown({ data }: { data: OverviewResponse['by_type'] }): JSX.Element {
  const { t } = useTranslation();
  const sorted = [...data].sort((a, b) => b.count - a.count);
  const MAX_VISIBLE = 6;
  const visible = sorted.slice(0, MAX_VISIBLE);
  const rest = sorted.slice(MAX_VISIBLE);
  const slices: Array<{ type: string; count: number; isOthers?: boolean }> =
    rest.length > 0
      ? [...visible, { type: t('overview.byType.others'), count: rest.reduce((s, d) => s + d.count, 0), isOthers: true }]
      : visible;
  const total = slices.reduce((sum, d) => sum + d.count, 0);

  // Column-first ordering for legend (1,3,5 left col / 2,4,6 right col)
  const half = Math.ceil(slices.length / 2);
  const legend: Array<{ item: (typeof slices)[number]; colorIdx: number }> = [];
  for (let i = 0; i < half; i++) {
    const left = slices[i];
    if (left) legend.push({ item: left, colorIdx: i });
    const right = slices[i + half];
    if (right) legend.push({ item: right, colorIdx: i + half });
  }

  if (slices.length === 0) {
    return <p className="text-sm text-fg-muted">{t('overview.byType.empty')}</p>;
  }

  return (
    <div className="flex h-full flex-col gap-3">
      <div className="min-h-0 flex-1">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={slices}
              dataKey="count"
              nameKey="type"
              innerRadius="55%"
              outerRadius="90%"
              paddingAngle={2}
              stroke="hsl(var(--surface))"
              strokeWidth={2}
            >
              {slices.map((_, i) => (
                <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
              ))}
            </Pie>
            <Tooltip
              formatter={(value: number, name: string) => {
                const pct = total > 0 ? ((value / total) * 100).toFixed(1) : '0';
                return [`${value} (${pct}%)`, name];
              }}
              contentStyle={{
                backgroundColor: 'hsl(var(--surface-2))',
                border: '1px solid hsl(var(--border))',
                borderRadius: 8,
                color: 'hsl(var(--fg))',
                fontSize: 12,
              }}
              itemStyle={{ color: 'hsl(var(--fg))' }}
              labelStyle={{ color: 'hsl(var(--fg))' }}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      <ul className="mt-auto grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
        {legend.map(({ item, colorIdx }) => (
          <li key={item.type} className="flex items-center gap-2" title={item.type}>
            <span
              className="h-2 w-2 shrink-0 rounded-sm"
              style={{ backgroundColor: PIE_COLORS[colorIdx % PIE_COLORS.length] }}
            />
            <span
              className={cn(
                'truncate',
                item.isOthers ? 'italic text-fg-muted' : 'text-fg',
              )}
            >
              {item.type}
            </span>
            <span className="ml-auto font-mono tabular-nums text-fg-muted">{item.count}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function SyncSummaryCard({
  data,
  loading,
}: {
  data: OverviewResponse | undefined;
  loading: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <Card className="flex flex-col">
      <div className="flex items-start justify-between gap-3 border-b border-border px-5 py-4">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-fg">{t('syncSummary.title')}</h2>
          <p className="mt-1 max-w-[28ch] text-xs text-fg-muted">
            {t('syncSummary.description')}
          </p>
        </div>
        <Link
          to="/sync"
          className="inline-flex items-center gap-0.5 text-xs text-fg-muted transition-colors hover:text-accent"
        >
          {t('syncSummary.open')}
          <ChevronRight size={12} />
        </Link>
      </div>
      <ul className="divide-y divide-border/50 px-5 pt-2">
        <SyncRow
          label={t('syncSummary.enrolled')}
          value={data?.sync_summary.enrolled_count}
          loading={loading}
        />
        <SyncRow
          label={t('syncSummary.healthy')}
          value={data?.sync_summary.healthy_count}
          tone="ok"
          loading={loading}
        />
        <SyncRow
          label={t('syncSummary.broken')}
          value={data?.sync_summary.broken_count}
          tone={data && data.sync_summary.broken_count > 0 ? 'fail' : 'neutral'}
          loading={loading}
        />
        <SyncRow
          label={t('syncSummary.pendingMutations')}
          value={data?.sync_summary.pending_mutations_total}
          tone={
            data && data.sync_summary.pending_mutations_total > 100 ? 'warn' : 'neutral'
          }
          loading={loading}
        />
      </ul>
    </Card>
  );
}

const SYNC_TONE: Record<'ok' | 'fail' | 'warn' | 'neutral', string> = {
  ok: 'text-ok',
  fail: 'text-fail',
  warn: 'text-warn',
  neutral: 'text-fg',
};

function SyncRow({
  label,
  value,
  tone,
  loading,
}: {
  label: string;
  value: number | undefined;
  tone?: 'ok' | 'fail' | 'warn' | 'neutral';
  loading: boolean;
}): JSX.Element {
  return (
    <li className="flex h-10 items-center justify-between">
      <span className="text-sm text-fg-muted">{label}</span>
      {loading ? (
        <Skeleton className="h-4 w-10" />
      ) : (
        <span className={cn('font-mono text-sm font-semibold tabular-nums', SYNC_TONE[tone ?? 'neutral'])}>
          {value ?? '—'}
        </span>
      )}
    </li>
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

function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

const SEVERITY_RANK: Record<SyncIssue['severity'], number> = {
  HIGH: 3,
  MEDIUM: 2,
  LOW: 1,
  INFO: 0,
};

const GLOBAL_KEY = '__global__';

interface IssueGroup {
  key: string;
  label: string;
  isGlobal: boolean;
  issues: SyncIssue[];
  maxSeverity: SyncIssue['severity'];
  counts: { high: number; medium: number; low: number; info: number };
}

function groupIssues(issues: SyncIssue[], globalLabel: string): IssueGroup[] {
  const buckets = new Map<string, IssueGroup>();
  for (const issue of issues) {
    const key = issue.project && issue.project !== '' ? issue.project : GLOBAL_KEY;
    const existing = buckets.get(key);
    if (existing) {
      existing.issues.push(issue);
      if (SEVERITY_RANK[issue.severity] > SEVERITY_RANK[existing.maxSeverity]) {
        existing.maxSeverity = issue.severity;
      }
    } else {
      buckets.set(key, {
        key,
        label: key === GLOBAL_KEY ? globalLabel : (issue.project ?? globalLabel),
        isGlobal: key === GLOBAL_KEY,
        issues: [issue],
        maxSeverity: issue.severity,
        counts: { high: 0, medium: 0, low: 0, info: 0 },
      });
    }
  }
  for (const group of buckets.values()) {
    for (const issue of group.issues) {
      if (issue.severity === 'HIGH') group.counts.high += 1;
      else if (issue.severity === 'MEDIUM') group.counts.medium += 1;
      else if (issue.severity === 'LOW') group.counts.low += 1;
      else group.counts.info += 1;
    }
  }
  return [...buckets.values()].sort((a, b) => {
    if (a.isGlobal !== b.isGlobal) return a.isGlobal ? -1 : 1;
    const sevDiff = SEVERITY_RANK[b.maxSeverity] - SEVERITY_RANK[a.maxSeverity];
    if (sevDiff !== 0) return sevDiff;
    if (b.issues.length !== a.issues.length) return b.issues.length - a.issues.length;
    return a.label.localeCompare(b.label);
  });
}

function IssuesTable({
  data,
  loading,
}: {
  data: SyncIssuesResponse | undefined;
  loading: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  const groups = useMemo(
    () => (data ? groupIssues(data.issues, t('overview.issues.global')) : []),
    [data, t],
  );

  if (loading) {
    return (
      <div className="px-5 py-4">
        <Skeleton className="h-32" />
      </div>
    );
  }
  if (!data || data.issues.length === 0) {
    return <p className="px-5 py-4 text-sm text-ok">{t('overview.issues.none')}</p>;
  }

  const lastUpdated = formatDateTime(data.generated_at);

  function toggle(key: string): void {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead className="bg-surface-2 text-xs uppercase tracking-wide text-fg-muted">
          <tr>
            <th className="px-5 py-3 text-left font-medium">{t('overview.issues.col.lastUpdate')}</th>
            <th className="px-3 py-3 text-left font-medium">{t('overview.issues.col.project')}</th>
            <th className="px-3 py-3 text-left font-medium">{t('overview.issues.col.severity')}</th>
            <th className="px-5 py-3 text-left font-medium">{t('overview.issues.col.count')}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {groups.map((group) => {
            const isOpen = expanded.has(group.key);
            const total = group.issues.length;
            const countLabel =
              total === 1
                ? t('overview.issues.group.countOne', { count: total })
                : t('overview.issues.group.count', { count: total });
            const breakdown = t('overview.issues.group.severityBreakdown', {
              high: group.counts.high,
              medium: group.counts.medium,
              low: group.counts.low,
            });
            const ariaLabel = isOpen
              ? t('overview.issues.group.collapseAria', { project: group.label })
              : t('overview.issues.group.expandAria', { project: group.label });

            return (
              <Fragment key={group.key}>
                <tr
                  className={cn(
                    'cursor-pointer align-top transition-colors hover:bg-surface-2/50',
                    isOpen ? 'bg-surface-2/40' : null,
                  )}
                  onClick={() => toggle(group.key)}
                  aria-expanded={isOpen}
                  aria-label={ariaLabel}
                >
                  <td className="whitespace-nowrap px-5 py-3 text-xs text-fg-muted">
                    <span className="inline-flex items-center gap-2">
                      <span className="text-fg-muted" aria-hidden>
                        {isOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                      </span>
                      {lastUpdated}
                    </span>
                  </td>
                  <td className="px-3 py-3">
                    {group.isGlobal ? (
                      <span className="italic text-fg-muted">{group.label}</span>
                    ) : (
                      <span className="font-medium text-fg">{group.label}</span>
                    )}
                  </td>
                  <td className="px-3 py-3">
                    <Badge tone={severityTone(group.maxSeverity)}>{group.maxSeverity}</Badge>
                  </td>
                  <td className="px-5 py-3 text-fg">
                    <span className="text-fg">{countLabel}</span>
                    <span className="ml-2 font-mono text-xs text-fg-muted">{breakdown}</span>
                  </td>
                </tr>
                {isOpen ? (
                  <tr>
                    <td colSpan={4} className="bg-surface-2/30 px-5 py-3">
                      <ul className="flex flex-col divide-y divide-border/60">
                        {group.issues.map((issue, idx) => (
                          <li
                            key={`${issue.code}-${idx}`}
                            className="flex flex-col gap-1 py-2 first:pt-0 last:pb-0 sm:flex-row sm:items-start sm:gap-3"
                          >
                            <div className="flex shrink-0 items-center gap-2">
                              <Badge tone={severityTone(issue.severity)}>{issue.severity}</Badge>
                              <code className="rounded bg-surface px-1.5 py-0.5 font-mono text-xs text-fg">
                                {issue.code}
                              </code>
                            </div>
                            <div className="min-w-0 flex-1 text-fg">
                              <p className="leading-snug">{issue.message}</p>
                              {issue.hint ? (
                                <p className="mt-1 text-xs text-fg-muted leading-snug">
                                  {issue.hint}
                                </p>
                              ) : null}
                            </div>
                          </li>
                        ))}
                      </ul>
                    </td>
                  </tr>
                ) : null}
              </Fragment>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function RecentTable({
  items,
}: {
  items: OverviewResponse['recent_observations'];
}): JSX.Element {
  const { t } = useTranslation();
  if (items.length === 0) {
    return <p className="px-5 py-4 text-sm text-fg-muted">{t('overview.recent.empty')}</p>;
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead className="bg-surface-2 text-xs uppercase tracking-wide text-fg-muted">
          <tr>
            <th className="px-5 py-3 text-left font-medium">{t('overview.recent.col.created')}</th>
            <th className="px-3 py-3 text-left font-medium">{t('overview.recent.col.type')}</th>
            <th className="px-3 py-3 text-left font-medium">{t('overview.recent.col.title')}</th>
            <th className="px-5 py-3 text-left font-medium">{t('overview.recent.col.project')}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {items.map((o) => (
            <tr key={o.id} className="align-top hover:bg-surface-2/50">
              <td className="whitespace-nowrap px-5 py-3 text-xs text-fg-muted">
                {formatDateTime(o.created_at)}
              </td>
              <td className="px-3 py-3">
                <Badge tone="neutral">{o.type}</Badge>
              </td>
              <td className="px-3 py-3 text-fg">
                <p className="truncate leading-snug">{o.title ?? t('common.untitled')}</p>
              </td>
              <td className="px-5 py-3 text-fg">
                {o.project ? (
                  <span className="font-medium">{o.project}</span>
                ) : (
                  <span className="italic text-fg-muted">—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
