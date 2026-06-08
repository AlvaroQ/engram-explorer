import { useQuery } from '@tanstack/react-query';
import { Copy, ExternalLink } from 'lucide-react';
import { useCallback, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { api, type ObservationRow } from '../../lib/api.ts';
import { Badge } from '../ui/badge.tsx';
import { Button } from '../ui/button.tsx';
import { Skeleton } from '../ui/skeleton.tsx';

interface ObservationDetailProps {
  id: number;
}

export function ObservationDetail({ id }: ObservationDetailProps): JSX.Element {
  const { t } = useTranslation();
  const detail = useQuery({
    queryKey: ['observation', id],
    queryFn: () => api.getObservation(id),
    staleTime: 60_000,
  });

  if (detail.isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-6 w-3/4" />
        <Skeleton className="h-24" />
      </div>
    );
  }
  if (detail.isError || !detail.data) {
    return <p className="text-sm text-fail">{t('observations.detail.loadError')}</p>;
  }
  return <ObservationDetailContent data={detail.data} />;
}

function ObservationDetailContent({
  data,
}: {
  data: { observation: ObservationRow; revisions: ObservationRow[] };
}): JSX.Element {
  const { t } = useTranslation();
  const { observation: o, revisions } = data;

  return (
    <div className="space-y-5 text-sm">
      <section className="flex flex-wrap items-center gap-2">
        <Badge tone="accent">{o.type}</Badge>
        {o.scope ? <Badge tone="neutral">{t('observations.detail.scope', { scope: o.scope })}</Badge> : null}
        {o.tool_name ? <Badge tone="neutral">{o.tool_name}</Badge> : null}
        {o.deleted_at ? <Badge tone="fail">{t('observations.detail.deleted')}</Badge> : null}
      </section>

      <section>
        <h3 className="text-base font-semibold text-fg">{o.title ?? t('common.untitled')}</h3>
        <dl className="mt-2 grid grid-cols-1 gap-x-4 gap-y-1 text-xs sm:grid-cols-2">
          <Field label={t('observations.detail.fields.project')} value={o.project} />
          <Field label={t('observations.detail.fields.topicKey')} value={o.topic_key} mono />
          <Field label={t('observations.detail.fields.created')} value={o.created_at} />
          <Field label={t('observations.detail.fields.updated')} value={o.updated_at} />
          <Field label={t('observations.detail.fields.revisions')} value={String(o.revision_count ?? 0)} />
          <Field label={t('observations.detail.fields.duplicates')} value={String(o.duplicate_count ?? 0)} />
          <Field label={t('observations.detail.fields.session')} value={o.session_id} mono />
          <Field label={t('observations.detail.fields.syncId')} value={o.sync_id} mono />
          {o.deleted_at ? <Field label={t('observations.detail.fields.deletedAt')} value={o.deleted_at} /> : null}
        </dl>
      </section>

      <section>
        <h4 className="mb-2 text-xs uppercase tracking-wide text-fg-muted">{t('observations.detail.content')}</h4>
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap rounded-md border border-border bg-surface-2 p-3 text-xs text-fg">
          {o.content ?? t('common.empty')}
        </pre>
      </section>

      {revisions.length > 0 ? (
        <section>
          <h4 className="mb-2 text-xs uppercase tracking-wide text-fg-muted">
            {t('observations.detail.revisions', { count: revisions.length })}
          </h4>
          <ul className="divide-y divide-border rounded-md border border-border bg-surface-2">
            {revisions.map((r) => (
              <li key={r.id} className="flex items-center justify-between gap-3 p-2 text-xs">
                <div className="min-w-0">
                  <p className="truncate text-fg">{r.title ?? t('common.untitled')}</p>
                  <p className="text-fg-muted">{r.updated_at ?? r.created_at ?? '—'}</p>
                </div>
                <Badge tone="neutral">{r.type}</Badge>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <ExportButtons observation={o} />
    </div>
  );
}

function Field({
  label,
  value,
  mono,
}: {
  label: string;
  value: string | null;
  mono?: boolean;
}): JSX.Element {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="text-fg-muted">{label}</dt>
      <dd className={mono ? 'truncate font-mono text-fg' : 'truncate text-fg'}>{value ?? '—'}</dd>
    </div>
  );
}

function ExportButtons({ observation }: { observation: ObservationRow }): JSX.Element {
  const { t } = useTranslation();
  const [copied, setCopied] = useState<string | null>(null);

  const copy = useCallback(async (label: string, text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(label);
      window.setTimeout(() => setCopied(null), 1200);
    } catch {
      setCopied(t('observations.detail.clipboardError'));
    }
  }, [t]);

  const json = JSON.stringify(observation, null, 2);
  const sql = `-- engram-explorer export
SELECT * FROM observations WHERE id = ${String(observation.id)};`;

  return (
    <section className="flex flex-wrap items-center gap-2">
      <Button size="sm" onClick={() => copy('json', json)}>
        <Copy size={14} /> {t('observations.detail.copyJson')}
      </Button>
      <Button size="sm" onClick={() => copy('sql', sql)}>
        <Copy size={14} /> {t('observations.detail.copySql')}
      </Button>
      {observation.session_id ? (
        <a
          href={`/sessions/${observation.session_id}`}
          className="inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-fg-muted hover:text-fg"
        >
          <ExternalLink size={12} /> {t('observations.detail.goToSession')}
        </a>
      ) : null}
      {copied ? (
        <span className="text-xs text-ok">{t('observations.detail.copied', { label: copied })}</span>
      ) : null}
    </section>
  );
}
