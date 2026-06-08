import { useQuery } from '@tanstack/react-query';
import { Search } from 'lucide-react';
import { useState, useEffect, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { api, type PromptRow } from '../lib/api.ts';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Input } from '../components/ui/input.tsx';
import { Badge } from '../components/ui/badge.tsx';

export function PromptsPage(): JSX.Element {
  const { t } = useTranslation();
  const [draft, setDraft] = useState('');
  const [q, setQ] = useState('');
  useEffect(() => {
    const tm = window.setTimeout(() => setQ(draft.trim()), 250);
    return () => window.clearTimeout(tm);
  }, [draft]);

  const list = useQuery({
    queryKey: ['prompts'],
    queryFn: () => api.listPrompts(),
    enabled: q.length === 0,
    staleTime: 30_000,
  });
  const search = useQuery({
    queryKey: ['prompts-search', q],
    queryFn: () => api.searchPrompts(q),
    enabled: q.length > 0,
    staleTime: 30_000,
  });

  const items: Array<PromptRow & { snippet?: string }> =
    q.length > 0 ? (search.data?.items ?? []) : (list.data?.items ?? []);
  const isLoading = q.length > 0 ? search.isLoading : list.isLoading;

  return (
    <div className="flex flex-col gap-5">
      <header>
        <h1 className="text-2xl font-semibold text-fg">{t('prompts.title')}</h1>
        <p className="text-sm text-fg-muted">{t('prompts.description')}</p>
      </header>

      <Card>
        <CardBody>
          <div className="relative">
            <Search
              size={14}
              className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-fg-muted"
            />
            <Input
              className="pl-9"
              placeholder={t('prompts.searchPlaceholder')}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
            />
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title={q.length > 0 ? t('prompts.resultsFor', { q }) : t('prompts.recent')}
          description={t('prompts.matches', { count: items.length })}
        />
        <CardBody className="space-y-3">
          {isLoading ? (
            <Skeleton className="h-32" />
          ) : items.length === 0 ? (
            <p className="text-sm text-fg-muted">{t('prompts.empty')}</p>
          ) : (
            items.map((p) => <PromptCard key={p.id} prompt={p} />)
          )}
        </CardBody>
      </Card>
    </div>
  );
}

function PromptCard({
  prompt,
}: {
  prompt: PromptRow & { snippet?: string };
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <article className="rounded-md border border-border bg-surface-2 p-3">
      <header className="flex items-center justify-between gap-2">
        <span className="font-mono text-xs text-fg-muted">{prompt.created_at ?? '—'}</span>
        {prompt.project ? <Badge tone="accent">{prompt.project}</Badge> : null}
      </header>
      {prompt.snippet ? (
        <p
          className="mt-2 whitespace-pre-wrap text-sm text-fg"
          dangerouslySetInnerHTML={{ __html: prompt.snippet }}
        />
      ) : (
        <p className="mt-2 line-clamp-4 whitespace-pre-wrap text-sm text-fg">
          {prompt.content ?? t('common.empty')}
        </p>
      )}
    </article>
  );
}
