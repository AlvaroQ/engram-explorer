import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from '@tanstack/react-router';
import { ChevronLeft } from 'lucide-react';
import { useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '../components/ui/badge.tsx';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { api, type SessionDetailResponse, type SessionTimelineEvent } from '../lib/api.ts';

export function SessionDetailPage(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams({ from: '/sessions/$id' });
  const detail = useQuery({
    queryKey: ['session', id],
    queryFn: () => api.getSession(id),
    staleTime: 60_000,
  });

  return (
    <div className="flex flex-col gap-5">
      <header className="flex items-center gap-2">
        <Link
          to="/sessions"
          className="inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-fg-muted hover:text-fg"
        >
          <ChevronLeft size={12} /> {t('common.back')}
        </Link>
        <h1 className="truncate text-xl font-semibold text-fg">{id}</h1>
      </header>

      {detail.isLoading ? (
        <Skeleton className="h-32" />
      ) : detail.isError || !detail.data ? (
        <p className="text-sm text-fail">{t('sessions.notFound')}</p>
      ) : (
        <DetailBody data={detail.data} />
      )}
    </div>
  );
}

function DetailBody({ data }: { data: SessionDetailResponse }): JSX.Element {
  const { t } = useTranslation();
  const { session, stats, events } = data;
  const [active, setActive] = useState<SessionTimelineEvent | null>(null);

  return (
    <>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader title={t('sessions.session')} />
          <CardBody className="space-y-2 text-xs">
            <Row label={t('sessions.fields.project')} value={session.project} />
            <Row label={t('sessions.fields.directory')} value={session.directory} />
            <Row label={t('sessions.fields.started')} value={session.started_at} />
            <Row label={t('sessions.fields.ended')} value={session.ended_at} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title={t('sessions.counts')} />
          <CardBody className="space-y-2 text-xs">
            <Row label={t('sessions.fields.observations')} value={String(stats.obs_total)} />
            <Row label={t('sessions.fields.prompts')} value={String(stats.prompts_total)} />
            {stats.by_type.length > 0 ? (
              <div className="flex flex-wrap gap-1 pt-2">
                {stats.by_type.map((tt) => (
                  <Badge key={tt.type} tone="neutral">
                    {tt.type} · {tt.count}
                  </Badge>
                ))}
              </div>
            ) : null}
          </CardBody>
        </Card>
        <Card>
          <CardHeader title={t('sessions.summaryCard')} />
          <CardBody>
            {session.summary ? (
              <pre className="whitespace-pre-wrap text-xs text-fg">{session.summary}</pre>
            ) : (
              <p className="text-xs text-fg-muted">{t('sessions.summaryEmpty')}</p>
            )}
          </CardBody>
        </Card>
      </div>

      <Card>
        <CardHeader
          title={t('sessions.timeline')}
          description={t('sessions.timelineDesc', { count: events.length })}
        />
        <CardBody>
          <Timeline events={events} active={active} onSelect={setActive} />
          {active ? <EventDetail event={active} /> : null}
        </CardBody>
      </Card>
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

const TYPE_COLORS: Record<string, string> = {
  prompt: 'hsl(var(--accent))',
  architecture: 'hsl(280 80% 70%)',
  bugfix: 'hsl(0 84% 67%)',
  decision: 'hsl(38 92% 60%)',
  discovery: 'hsl(190 80% 60%)',
  session_summary: 'hsl(220 15% 65%)',
  config: 'hsl(60 80% 60%)',
  pattern: 'hsl(330 70% 65%)',
};

function colorFor(event: SessionTimelineEvent): string {
  if (event.kind === 'prompt') return TYPE_COLORS.prompt!;
  return TYPE_COLORS[event.type ?? ''] ?? 'hsl(var(--fg-muted))';
}

interface TimelineProps {
  events: SessionTimelineEvent[];
  active: SessionTimelineEvent | null;
  onSelect: (event: SessionTimelineEvent) => void;
}

function Timeline({ events, active, onSelect }: TimelineProps): JSX.Element {
  const { t } = useTranslation();
  if (events.length === 0) {
    return <p className="text-sm text-fg-muted">{t('sessions.noEvents')}</p>;
  }
  const times = events
    .map((e) => (e.at ? Date.parse(e.at) : NaN))
    .filter((n) => Number.isFinite(n));
  if (times.length === 0) {
    return (
      <p className="text-sm text-fg-muted">{t('sessions.noTimestamps')}</p>
    );
  }
  const min = Math.min(...times);
  const max = Math.max(...times);
  const span = max - min || 1;

  return (
    <div className="space-y-3">
      <div className="relative h-12 rounded-md border border-border bg-surface-2" role="img" aria-label={t('sessions.timeline')}>
        <div className="absolute left-0 right-0 top-1/2 h-px bg-border" />
        {events.map((e) => {
          if (!e.at) return null;
          const tm = Date.parse(e.at);
          if (!Number.isFinite(tm)) return null;
          const pct = ((tm - min) / span) * 100;
          const isActive = active?.id === e.id && active.kind === e.kind;
          return (
            <button
              key={`${e.kind}-${String(e.id)}`}
              type="button"
              className="absolute top-1/2 h-3 w-3 -translate-x-1/2 -translate-y-1/2 rounded-full border border-bg shadow-sm transition-transform hover:scale-125"
              style={{
                left: `${String(pct)}%`,
                backgroundColor: colorFor(e),
                outline: isActive ? '2px solid hsl(var(--fg))' : 'none',
              }}
              aria-label={`${e.kind} ${e.title ?? e.content ?? t('common.untitled')}`}
              onClick={() => onSelect(e)}
            />
          );
        })}
      </div>
      <div className="flex flex-wrap gap-2 text-xs">
        {Object.entries(TYPE_COLORS)
          .filter(([key]) => key === 'prompt' || events.some((e) => e.type === key))
          .map(([key, color]) => (
            <span key={key} className="inline-flex items-center gap-1 text-fg-muted">
              <span
                className="inline-block h-2 w-2 rounded-full"
                style={{ backgroundColor: color }}
              />
              {key}
            </span>
          ))}
      </div>
    </div>
  );
}

function EventDetail({ event }: { event: SessionTimelineEvent }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="mt-4 rounded-md border border-border bg-surface-2 p-3">
      <div className="mb-2 flex items-center gap-2">
        <Badge tone={event.kind === 'prompt' ? 'accent' : 'neutral'}>{event.kind}</Badge>
        {event.type ? <Badge tone="neutral">{event.type}</Badge> : null}
        {event.tool_name ? <Badge tone="neutral">{event.tool_name}</Badge> : null}
        <span className="ml-auto font-mono text-xs text-fg-muted">{event.at ?? '—'}</span>
      </div>
      {event.title ? <p className="text-sm font-medium text-fg">{event.title}</p> : null}
      {event.content ? (
        <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap text-xs text-fg">
          {event.content}
        </pre>
      ) : null}
      {event.kind === 'observation' ? (
        <Link
          to="/observations"
          search={() => ({})}
          className="mt-2 inline-block text-xs text-accent hover:underline"
        >
          {t('sessions.openInObservations')}
        </Link>
      ) : null}
    </div>
  );
}
