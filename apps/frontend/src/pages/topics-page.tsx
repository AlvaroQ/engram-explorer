import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { useMemo, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../lib/api.ts';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Input } from '../components/ui/input.tsx';
import { Badge } from '../components/ui/badge.tsx';

export function TopicsPage(): JSX.Element {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  const topics = useQuery({
    queryKey: ['topics'],
    queryFn: () => api.listTopics(),
    staleTime: 60_000,
  });

  const filtered = useMemo(() => {
    const items = topics.data?.items ?? [];
    if (filter.trim().length === 0) return items;
    const needle = filter.toLowerCase();
    return items.filter(
      (tt) =>
        tt.topic_key.toLowerCase().includes(needle) ||
        (tt.project ?? '').toLowerCase().includes(needle),
    );
  }, [topics.data, filter]);

  return (
    <div className="flex flex-col gap-5">
      <header>
        <h1 className="text-2xl font-semibold text-fg">{t('topics.title')}</h1>
        <p className="text-sm text-fg-muted">
          {t('topics.descriptionPart1')} <code className="font-mono text-xs">topic_key</code>
          {t('topics.descriptionPart2')}
        </p>
      </header>

      <Card>
        <CardBody>
          <Input
            placeholder={t('topics.filterPlaceholder')}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
        </CardBody>
      </Card>

      <Card>
        <CardHeader title={t('topics.listTitle')} description={t('topics.entries', { count: filtered.length })} />
        <CardBody className="p-0">
          {topics.isLoading ? (
            <Skeleton className="h-32" />
          ) : filtered.length === 0 ? (
            <p className="p-6 text-center text-sm text-fg-muted">{t('topics.empty')}</p>
          ) : (
            <ul className="divide-y divide-border">
              {filtered.map((tt) => (
                <li key={`${tt.topic_key}-${tt.project ?? ''}`}>
                  <Link
                    to="/observations"
                    search={() => ({
                      topic_key: tt.topic_key,
                      ...(tt.project ? { project: [tt.project] } : {}),
                    })}
                    className="flex items-center justify-between gap-3 px-4 py-2.5 hover:bg-surface-2"
                  >
                    <div className="min-w-0">
                      <p className="truncate font-mono text-sm text-fg">{tt.topic_key}</p>
                      <p className="text-xs text-fg-muted">
                        {tt.project ?? t('common.noProject')} · {t('topics.lastUpdated')} {tt.last_updated ?? '—'}
                      </p>
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <Badge tone="accent">{t('topics.obsCount', { count: tt.obs_count })}</Badge>
                      <Badge tone="neutral">{t('topics.revisions', { count: tt.revisions })}</Badge>
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </CardBody>
      </Card>
    </div>
  );
}
