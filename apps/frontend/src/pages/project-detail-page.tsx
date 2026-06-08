import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from '@tanstack/react-router';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { Activity, ChevronLeft, Database, FileText, Tags } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { api, type ProjectOverviewResponse, type SyncProjectRow } from '../lib/api.ts';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Badge } from '../components/ui/badge.tsx';
import { Button } from '../components/ui/button.tsx';

import type { JSX } from "react";

export function ProjectDetailPage(): JSX.Element {
  const { t } = useTranslation();
  const { project } = useParams({ from: '/projects/$project' });
  const overview = useQuery({
    queryKey: ['project-overview', project],
    queryFn: () => api.projectOverview(project),
    staleTime: 30_000,
  });
  const sync = useQuery({
    queryKey: ['sync-projects'],
    queryFn: api.syncProjects,
    staleTime: 30_000,
  });
  const syncRow: SyncProjectRow | undefined = sync.data?.projects.find(
    (p) => p.project === project,
  );

  return (
    <div className="flex flex-col gap-5">
      <header className="flex items-start justify-between gap-3">
        <div>
          <Link
            to="/projects"
            className="mb-2 inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-fg-muted hover:text-fg"
          >
            <ChevronLeft size={12} /> {t('projectDetail.allProjects')}
          </Link>
          <h1 className="text-2xl font-semibold text-fg">{project}</h1>
          <p className="text-sm text-fg-muted">{t('projectDetail.description')}</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Link
            to="/observations"
            search={() => ({ project: [project] })}
            className="inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-3 py-1.5 text-xs text-fg-muted hover:text-fg"
          >
            {t('projectDetail.allObservations')}
          </Link>
          <Link
            to="/sessions"
            className="inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-3 py-1.5 text-xs text-fg-muted hover:text-fg"
          >
            {t('projectDetail.allSessions')}
          </Link>
          {syncRow ? (
            <Link
              to="/sync/$project"
              params={{ project }}
              className="inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-3 py-1.5 text-xs text-fg-muted hover:text-fg"
            >
              {t('projectDetail.syncDetail')}
            </Link>
          ) : null}
        </div>
      </header>

      {overview.isLoading ? (
        <Skeleton className="h-64" />
      ) : overview.isError || !overview.data ? (
        <Card>
          <CardBody>
            <p className="text-sm text-fail">{t('projectDetail.notFound')}</p>
          </CardBody>
        </Card>
      ) : (
        <DetailBody data={overview.data} sync={syncRow} />
      )}
    </div>
  );
}

function DetailBody({
  data,
  sync,
}: {
  data: ProjectOverviewResponse;
  sync: SyncProjectRow | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <KpiRow data={data} sync={sync} />

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader
            title={t('projectDetail.activity.title')}
            description={t('projectDetail.activity.description')}
          />
          <CardBody>
            <ActivityChart data={data.activity_30d} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title={t('projectDetail.byType.title')} description={t('projectDetail.byType.description')} />
          <CardBody>
            <TypeBreakdown data={data.by_type} />
          </CardBody>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader title={t('projectDetail.tools.title')} description={t('projectDetail.tools.description')} />
          <CardBody>
            {data.by_tool.length === 0 ? (
              <p className="text-sm text-fg-muted">{t('projectDetail.tools.empty')}</p>
            ) : (
              <ul className="space-y-1.5">
                {data.by_tool.map((tt) => (
                  <li key={tt.tool_name} className="flex items-center justify-between gap-2">
                    <span className="truncate font-mono text-xs text-fg">{tt.tool_name}</span>
                    <Badge tone="neutral">{tt.count}</Badge>
                  </li>
                ))}
              </ul>
            )}
          </CardBody>
        </Card>
        <Card>
          <CardHeader title={t('projectDetail.topics.title')} description={t('projectDetail.topics.description')} />
          <CardBody>
            {data.top_topics.length === 0 ? (
              <p className="text-sm text-fg-muted">{t('projectDetail.topics.empty')}</p>
            ) : (
              <ul className="space-y-1.5">
                {data.top_topics.map((tt) => (
                  <li
                    key={tt.topic_key}
                    className="flex items-center justify-between gap-2"
                  >
                    <Link
                      to="/observations"
                      search={() => ({ project: [data.project], topic_key: tt.topic_key })}
                      className="truncate font-mono text-xs text-accent hover:underline"
                    >
                      {tt.topic_key}
                    </Link>
                    <Badge tone="neutral">{tt.obs_count}</Badge>
                  </li>
                ))}
              </ul>
            )}
          </CardBody>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader
            title={t('projectDetail.recentSessions.title')}
            description={t('projectDetail.recentSessions.description', {
              shown: data.recent_sessions.length,
              total: data.kpis.sessions,
            })}
          />
          <CardBody className="p-0">
            {data.recent_sessions.length === 0 ? (
              <p className="p-4 text-sm text-fg-muted">{t('projectDetail.recentSessions.empty')}</p>
            ) : (
              <ul className="divide-y divide-border">
                {data.recent_sessions.map((s) => (
                  <li key={s.id}>
                    <Link
                      to="/sessions/$id"
                      params={{ id: s.id }}
                      className="block px-4 py-2.5 hover:bg-surface-2"
                    >
                      <p className="truncate font-mono text-xs text-fg">{s.id}</p>
                      <p className="text-xs text-fg-muted">
                        {s.started_at ?? '—'} · {t('projectDetail.recentSessions.obsCount', { count: s.obs_count })}
                      </p>
                      {s.summary ? (
                        <p className="mt-1 line-clamp-2 text-xs text-fg-muted">{s.summary}</p>
                      ) : null}
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </CardBody>
        </Card>

        <Card>
          <CardHeader
            title={t('projectDetail.recentObservations.title')}
            description={t('projectDetail.recentObservations.description', { count: data.recent_observations.length })}
          />
          <CardBody className="p-0">
            {data.recent_observations.length === 0 ? (
              <p className="p-4 text-sm text-fg-muted">{t('projectDetail.recentObservations.empty')}</p>
            ) : (
              <ul className="divide-y divide-border">
                {data.recent_observations.map((o) => (
                  <li
                    key={o.id}
                    className="flex items-center justify-between gap-2 px-4 py-2"
                  >
                    <div className="min-w-0">
                      <p className="truncate text-sm text-fg">{o.title ?? t('common.untitled')}</p>
                      <p className="font-mono text-xs text-fg-muted">
                        {o.created_at ?? '—'}
                        {o.tool_name ? ` · ${o.tool_name}` : ''}
                      </p>
                    </div>
                    <Badge tone="accent">{o.type}</Badge>
                  </li>
                ))}
              </ul>
            )}
            {data.recent_observations.length > 0 ? (
              <div className="border-t border-border p-3">
                <Link
                  to="/observations"
                  search={() => ({ project: [data.project] })}
                >
                  <Button variant="secondary" size="sm">
                    {t('projectDetail.recentObservations.openInObservations')}
                  </Button>
                </Link>
              </div>
            ) : null}
          </CardBody>
        </Card>
      </div>

      {data.recent_prompts.length > 0 ? (
        <Card>
          <CardHeader title={t('projectDetail.recentPrompts.title')} />
          <CardBody className="space-y-2">
            {data.recent_prompts.map((p) => (
              <article
                key={p.id}
                className="rounded-md border border-border bg-surface-2 p-3"
              >
                <p className="font-mono text-xs text-fg-muted">{p.created_at ?? '—'}</p>
                <p className="mt-1 line-clamp-3 whitespace-pre-wrap text-sm text-fg">
                  {p.content ?? t('common.empty')}
                </p>
              </article>
            ))}
          </CardBody>
        </Card>
      ) : null}
    </>
  );
}

function KpiRow({
  data,
  sync,
}: {
  data: ProjectOverviewResponse;
  sync: SyncProjectRow | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const items: Array<{ label: string; value: string | number; icon: JSX.Element }> = [
    { label: t('projectDetail.kpi.observations'), value: data.kpis.observations, icon: <Database size={16} /> },
    { label: t('projectDetail.kpi.sessions'), value: data.kpis.sessions, icon: <Activity size={16} /> },
    { label: t('projectDetail.kpi.topics'), value: data.kpis.topics, icon: <Tags size={16} /> },
    { label: t('projectDetail.kpi.prompts'), value: data.kpis.prompts, icon: <FileText size={16} /> },
  ];
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 lg:grid-cols-5">
      {items.map((kpi) => (
        <Card key={kpi.label}>
          <CardBody className="flex items-center justify-between gap-3">
            <div>
              <p className="text-xs uppercase tracking-wide text-fg-muted">{kpi.label}</p>
              <p className="mt-1 text-2xl font-semibold text-fg">{kpi.value}</p>
            </div>
            <span className="rounded-md bg-surface-2 p-2 text-accent">{kpi.icon}</span>
          </CardBody>
        </Card>
      ))}
      <Card>
        <CardBody>
          <p className="text-xs uppercase tracking-wide text-fg-muted">{t('projectDetail.kpi.sync')}</p>
          <div className="mt-1">
            {!sync ? (
              <Badge tone="neutral">{t('projectDetail.kpi.unknown')}</Badge>
            ) : !sync.enrolled ? (
              <Badge tone="neutral">{t('projectDetail.kpi.notEnrolled')}</Badge>
            ) : sync.status === 'broken' ? (
              <Badge tone="fail">{t('projectDetail.kpi.brokenPending', { count: sync.pending_mutations })}</Badge>
            ) : sync.status === 'healthy' ? (
              <Badge tone="ok">{t('projectDetail.kpi.healthy')}</Badge>
            ) : (
              <Badge tone="warn">{sync.status}</Badge>
            )}
          </div>
          {sync ? (
            <p className="mt-2 font-mono text-xs text-fg-muted">
              {t('projectDetail.kpi.ackedEnq', { acked: sync.last_acked_seq ?? 0, enq: sync.last_enqueued_seq ?? 0 })}
            </p>
          ) : null}
        </CardBody>
      </Card>
    </div>
  );
}

function ActivityChart({
  data,
}: {
  data: ProjectOverviewResponse['activity_30d'];
}): JSX.Element {
  const { t } = useTranslation();
  if (data.length === 0) {
    return <p className="text-sm text-fg-muted">{t('projectDetail.activity.empty')}</p>;
  }
  return (
    <div style={{ width: '100%', height: 260 }}>
      <ResponsiveContainer>
        <BarChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
          <XAxis
            dataKey="day"
            stroke="hsl(var(--fg-muted))"
            tickFormatter={(v: string) => v.slice(5)}
            fontSize={11}
            interval="preserveStartEnd"
          />
          <YAxis stroke="hsl(var(--fg-muted))" fontSize={11} allowDecimals={false} width={32} />
          <Tooltip
            contentStyle={{
              backgroundColor: 'hsl(var(--surface-2))',
              border: '1px solid hsl(var(--border))',
              borderRadius: 8,
              color: 'hsl(var(--fg))',
              fontSize: 12,
            }}
            cursor={{ fill: 'hsl(var(--surface-2))', opacity: 0.4 }}
          />
          <Bar dataKey="count" fill="hsl(var(--accent))" radius={[4, 4, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
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

function TypeBreakdown({
  data,
}: {
  data: ProjectOverviewResponse['by_type'];
}): JSX.Element {
  const { t } = useTranslation();
  if (data.length === 0) {
    return <p className="text-sm text-fg-muted">{t('projectDetail.byType.empty')}</p>;
  }
  const top = data.slice(0, 8);
  return (
    <div style={{ width: '100%', height: 260 }}>
      <ResponsiveContainer>
        <PieChart>
          <Pie
            data={top}
            dataKey="count"
            nameKey="type"
            innerRadius={50}
            outerRadius={90}
            paddingAngle={2}
          >
            {top.map((_, i) => (
              <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
            ))}
          </Pie>
          <Tooltip
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
  );
}
