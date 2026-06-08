import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { useMemo, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Button } from '../components/ui/button.tsx';
import { Badge } from '../components/ui/badge.tsx';
import { MultiSelectChips } from '../components/observations/multi-select-chips.tsx';
import { Select } from '../components/ui/select.tsx';
import { api, type SessionListItem, type SessionListParams } from '../lib/api.ts';

export function SessionsPage(): JSX.Element {
  const { t } = useTranslation();
  const [projectFilter, setProjectFilter] = useState<string[]>([]);
  const [summaryFilter, setSummaryFilter] = useState<'all' | 'with' | 'without'>('all');

  const projects = useQuery({
    queryKey: ['projects'],
    queryFn: api.listProjects,
    staleTime: 5 * 60_000,
  });
  const projectOptions = (projects.data?.items ?? []).map((p) => p.project);

  const params = useMemo<SessionListParams>(() => {
    const out: SessionListParams = { limit: 50 };
    if (projectFilter.length > 0) out.project = projectFilter;
    if (summaryFilter === 'with') out.has_summary = true;
    if (summaryFilter === 'without') out.has_summary = false;
    return out;
  }, [projectFilter, summaryFilter]);

  const sessions = useInfiniteQuery({
    queryKey: ['sessions', params],
    queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
      api.listSessions({ ...params, ...(pageParam ? { cursor: pageParam } : {}) }),
    initialPageParam: undefined,
    getNextPageParam: (last) => last.nextCursor ?? undefined,
    staleTime: 30_000,
  });

  const items = useMemo(
    () => sessions.data?.pages.flatMap((p) => p.items) ?? [],
    [sessions.data],
  );

  return (
    <div className="flex flex-col gap-5">
      <header>
        <h1 className="text-2xl font-semibold text-fg">{t('sessions.title')}</h1>
        <p className="text-sm text-fg-muted">{t('sessions.description')}</p>
      </header>

      <Card>
        <CardHeader
          title={t('sessions.listTitle')}
          description={
            sessions.isLoading
              ? t('common.loading')
              : t('sessions.listLoaded', { count: items.length })
          }
          actionsClassName="flex-1 min-w-0"
          actions={
            <div className="flex flex-wrap items-center gap-2 sm:flex-nowrap">
              <div className="min-w-[12rem] flex-1">
                <MultiSelectChips
                  compact
                  label={t('sessions.filterProject')}
                  placeholder={t('sessions.filterProject')}
                  values={projectFilter}
                  options={projectOptions}
                  onChange={setProjectFilter}
                />
              </div>
              <Select
                aria-label={t('sessions.filterSummary')}
                className="w-full sm:w-40"
                value={summaryFilter}
                onChange={(e) =>
                  setSummaryFilter(e.target.value as 'all' | 'with' | 'without')
                }
              >
                <option value="all">{t('sessions.summaryAll')}</option>
                <option value="with">{t('sessions.summaryWith')}</option>
                <option value="without">{t('sessions.summaryWithout')}</option>
              </Select>
            </div>
          }
        />
        <CardBody className="p-0">
          {sessions.isLoading ? (
            <div className="space-y-2 p-4">
              <Skeleton className="h-12" />
              <Skeleton className="h-12" />
              <Skeleton className="h-12" />
            </div>
          ) : items.length === 0 ? (
            <p className="p-6 text-center text-sm text-fg-muted">{t('sessions.noMatch')}</p>
          ) : (
            <ul className="divide-y divide-border">
              {items.map((s) => (
                <SessionListRow key={s.id} item={s} />
              ))}
            </ul>
          )}
          {sessions.hasNextPage ? (
            <div className="p-3">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => void sessions.fetchNextPage()}
                disabled={sessions.isFetchingNextPage}
              >
                {sessions.isFetchingNextPage ? t('common.loading') : t('common.loadMore')}
              </Button>
            </div>
          ) : null}
        </CardBody>
      </Card>
    </div>
  );
}

function SessionListRow({ item }: { item: SessionListItem }): JSX.Element {
  const { t } = useTranslation();
  const duration = computeDuration(item.started_at, item.ended_at);
  return (
    <li>
      <Link
        to="/sessions/$id"
        params={{ id: item.id }}
        className="flex items-center justify-between gap-3 px-4 py-3 hover:bg-surface-2"
      >
        <div className="min-w-0">
          <p className="truncate font-mono text-xs text-fg">{item.id}</p>
          <p className="text-xs text-fg-muted">
            {item.started_at ?? '—'}
            {item.project ? ` · ${item.project}` : ''}
            {duration ? ` · ${duration}` : ''}
          </p>
          {item.summary ? (
            <p className="mt-1 line-clamp-2 text-xs text-fg-muted">{item.summary}</p>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Badge tone="neutral">{t('sessions.obsCount', { count: item.obs_count })}</Badge>
          <Badge tone={item.prompts_count > 0 ? 'accent' : 'neutral'}>
            {t('sessions.promptsCount', { count: item.prompts_count })}
          </Badge>
        </div>
      </Link>
    </li>
  );
}

function computeDuration(start: string | null, end: string | null): string | null {
  if (!start || !end) return null;
  const ms = Date.parse(end) - Date.parse(start);
  if (!Number.isFinite(ms) || ms < 0) return null;
  const minutes = Math.round(ms / 60_000);
  if (minutes < 60) return `${String(minutes)}min`;
  const hours = Math.floor(minutes / 60);
  const remMin = minutes % 60;
  return remMin === 0 ? `${String(hours)}h` : `${String(hours)}h ${String(remMin)}m`;
}
