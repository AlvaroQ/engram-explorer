import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { ChevronLeft } from 'lucide-react';
import { useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Badge } from '../components/ui/badge.tsx';
import { Button } from '../components/ui/button.tsx';
import { ConfirmModal } from '../components/ui/confirm-modal.tsx';
import { AssignProjectModal } from '../components/orphans/assign-project-modal.tsx';
import {
  ApiRequestError,
  api,
  type OrphanObservation,
  type OrphanPrompt,
  type OrphanSession,
} from '../lib/api.ts';

type EntityKind = 'observation' | 'session' | 'prompt';

type AssignTarget = { entity: EntityKind; id: string | number; label: string } | null;
type DeleteTarget = { entity: EntityKind; id: string | number; label: string } | null;

export function OrphansPage(): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [assignTarget, setAssignTarget] = useState<AssignTarget>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget>(null);
  const [flash, setFlash] = useState<string | null>(null);

  const orphans = useQuery({
    queryKey: ['orphans'],
    queryFn: api.listOrphans,
    staleTime: 30_000,
  });

  const deletion = useMutation({
    mutationFn: (target: { entity: EntityKind; id: string | number }) =>
      api.deleteEntity(target.entity, target.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['orphans'] });
      void queryClient.invalidateQueries({ queryKey: ['observations'] });
      void queryClient.invalidateQueries({ queryKey: ['projects'] });
      void queryClient.invalidateQueries({ queryKey: ['overview'] });
      void queryClient.invalidateQueries({ queryKey: ['sync-projects'] });
      setDeleteTarget(null);
      setFlash(null);
    },
    onError: (err) => {
      const msg = err instanceof ApiRequestError ? err.api.message : (err as Error).message;
      setFlash(msg);
    },
  });

  function describeDelete(entity: EntityKind): string {
    if (entity === 'observation') return t('orphans.deleteConfirm.descObservation');
    if (entity === 'session') return t('orphans.deleteConfirm.descSession');
    return t('orphans.deleteConfirm.descPrompt');
  }

  return (
    <div className="flex flex-col gap-5">
      <header>
        <Link
          to="/projects"
          className="mb-2 inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-fg-muted hover:text-fg"
        >
          <ChevronLeft size={12} /> {t('projectDetail.allProjects')}
        </Link>
        <h1 className="text-2xl font-semibold text-fg">{t('orphans.title')}</h1>
        <p className="text-sm text-fg-muted">{t('orphans.description')}</p>
      </header>

      {orphans.isLoading ? (
        <Skeleton className="h-32" />
      ) : orphans.data &&
        orphans.data.totals.observations === 0 &&
        orphans.data.totals.sessions === 0 &&
        orphans.data.totals.prompts === 0 ? (
        <Card>
          <CardBody>
            <p className="text-center text-sm text-ok">{t('orphans.empty')}</p>
          </CardBody>
        </Card>
      ) : orphans.data ? (
        <>
          {orphans.data.totals.observations > 0 ? (
            <ObservationsSection
              rows={orphans.data.observations}
              onAssign={(o) =>
                setAssignTarget({
                  entity: 'observation',
                  id: o.id,
                  label: o.title ?? `#${String(o.id)}`,
                })
              }
              onDelete={(o) =>
                setDeleteTarget({
                  entity: 'observation',
                  id: o.id,
                  label: o.title ?? `#${String(o.id)}`,
                })
              }
            />
          ) : null}
          {orphans.data.totals.sessions > 0 ? (
            <SessionsSection
              rows={orphans.data.sessions}
              onAssign={(s) =>
                setAssignTarget({ entity: 'session', id: s.id, label: s.directory ?? s.id })
              }
              onDelete={(s) =>
                setDeleteTarget({ entity: 'session', id: s.id, label: s.directory ?? s.id })
              }
            />
          ) : null}
          {orphans.data.totals.prompts > 0 ? (
            <PromptsSection
              rows={orphans.data.prompts}
              onAssign={(p) =>
                setAssignTarget({
                  entity: 'prompt',
                  id: p.id,
                  label: (p.content ?? '').slice(0, 60) || `#${String(p.id)}`,
                })
              }
              onDelete={(p) =>
                setDeleteTarget({
                  entity: 'prompt',
                  id: p.id,
                  label: (p.content ?? '').slice(0, 60) || `#${String(p.id)}`,
                })
              }
            />
          ) : null}
        </>
      ) : null}

      {assignTarget ? (
        <AssignProjectModal
          entity={assignTarget.entity}
          id={assignTarget.id}
          currentLabel={assignTarget.label}
          onClose={() => setAssignTarget(null)}
        />
      ) : null}

      {deleteTarget ? (
        <ConfirmModal
          title={
            <span>
              {t('orphans.deleteConfirm.title')}
              <span className="ml-2 font-mono text-xs text-fg-muted">{deleteTarget.label}</span>
            </span>
          }
          description={
            <>
              <p>{describeDelete(deleteTarget.entity)}</p>
              {flash ? (
                <p className="mt-2 rounded-md border border-fail/40 bg-fail/10 p-2 text-xs text-fail">
                  {flash}
                </p>
              ) : null}
            </>
          }
          confirmLabel={t('orphans.deleteConfirm.confirm')}
          tone="danger"
          onConfirm={() =>
            deletion.mutate({ entity: deleteTarget.entity, id: deleteTarget.id })
          }
          onCancel={() => {
            setDeleteTarget(null);
            setFlash(null);
          }}
        />
      ) : null}
    </div>
  );
}

function ObservationsSection({
  rows,
  onAssign,
  onDelete,
}: {
  rows: OrphanObservation[];
  onAssign: (o: OrphanObservation) => void;
  onDelete: (o: OrphanObservation) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader
        title={t('orphans.section.observations')}
        description={t('orphans.section.count', { count: rows.length })}
      />
      <CardBody className="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-surface-2 text-left text-xs uppercase tracking-wide text-fg-muted">
              <tr>
                <th className="px-3 py-2">{t('orphans.table.id')}</th>
                <th className="px-3 py-2">{t('orphans.table.type')}</th>
                <th className="px-3 py-2">{t('orphans.table.title')}</th>
                <th className="px-3 py-2">{t('orphans.table.topic')}</th>
                <th className="px-3 py-2">{t('orphans.table.created')}</th>
                <th className="px-3 py-2">{t('orphans.table.session')}</th>
                <th className="px-3 py-2 text-right">{t('orphans.table.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {rows.map((o) => (
                <tr key={o.id} className="hover:bg-surface-2">
                  <td className="px-3 py-1.5 font-mono text-xs text-fg-muted">{o.id}</td>
                  <td className="px-3 py-1.5">
                    <Badge tone="accent">{o.type}</Badge>
                  </td>
                  <td className="px-3 py-1.5 max-w-md truncate text-fg">
                    {o.title ?? <span className="text-fg-muted">{t('common.untitled')}</span>}
                  </td>
                  <td className="px-3 py-1.5 truncate font-mono text-xs text-fg-muted">
                    {o.topic_key ?? '—'}
                  </td>
                  <td className="px-3 py-1.5 font-mono text-xs text-fg-muted">
                    {o.updated_at ?? o.created_at ?? '—'}
                  </td>
                  <td className="px-3 py-1.5 truncate font-mono text-xs text-fg-muted">
                    {o.session_id ?? '—'}
                  </td>
                  <td className="px-3 py-1.5 text-right">
                    <div className="flex justify-end gap-2">
                      <Button size="sm" variant="primary" onClick={() => onAssign(o)}>
                        {t('orphans.table.assign')}
                      </Button>
                      <Button size="sm" variant="danger" onClick={() => onDelete(o)}>
                        {t('orphans.table.delete')}
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardBody>
    </Card>
  );
}

function SessionsSection({
  rows,
  onAssign,
  onDelete,
}: {
  rows: OrphanSession[];
  onAssign: (s: OrphanSession) => void;
  onDelete: (s: OrphanSession) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader
        title={t('orphans.section.sessions')}
        description={t('orphans.section.count', { count: rows.length })}
      />
      <CardBody className="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-surface-2 text-left text-xs uppercase tracking-wide text-fg-muted">
              <tr>
                <th className="px-3 py-2">{t('orphans.table.id')}</th>
                <th className="px-3 py-2">{t('orphans.table.directory')}</th>
                <th className="px-3 py-2">{t('orphans.table.started')}</th>
                <th className="px-3 py-2">{t('orphans.table.ended')}</th>
                <th className="px-3 py-2">{t('orphans.table.summary')}</th>
                <th className="px-3 py-2 text-right">{t('orphans.table.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {rows.map((s) => (
                <tr key={s.id} className="hover:bg-surface-2">
                  <td className="px-3 py-1.5 truncate font-mono text-xs text-fg">{s.id}</td>
                  <td className="px-3 py-1.5 truncate font-mono text-xs text-fg-muted">
                    {s.directory ?? '—'}
                  </td>
                  <td className="px-3 py-1.5 font-mono text-xs text-fg-muted">{s.started_at ?? '—'}</td>
                  <td className="px-3 py-1.5 font-mono text-xs text-fg-muted">{s.ended_at ?? '—'}</td>
                  <td className="px-3 py-1.5 max-w-md truncate text-fg-muted">{s.summary ?? '—'}</td>
                  <td className="px-3 py-1.5 text-right">
                    <div className="flex justify-end gap-2">
                      <Button size="sm" variant="primary" onClick={() => onAssign(s)}>
                        {t('orphans.table.assign')}
                      </Button>
                      <Button size="sm" variant="danger" onClick={() => onDelete(s)}>
                        {t('orphans.table.delete')}
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardBody>
    </Card>
  );
}

function PromptsSection({
  rows,
  onAssign,
  onDelete,
}: {
  rows: OrphanPrompt[];
  onAssign: (p: OrphanPrompt) => void;
  onDelete: (p: OrphanPrompt) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader
        title={t('orphans.section.prompts')}
        description={t('orphans.section.count', { count: rows.length })}
      />
      <CardBody className="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-surface-2 text-left text-xs uppercase tracking-wide text-fg-muted">
              <tr>
                <th className="px-3 py-2">{t('orphans.table.id')}</th>
                <th className="px-3 py-2">{t('orphans.table.created')}</th>
                <th className="px-3 py-2">{t('orphans.table.session')}</th>
                <th className="px-3 py-2">{t('orphans.table.content')}</th>
                <th className="px-3 py-2 text-right">{t('orphans.table.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {rows.map((p) => (
                <tr key={p.id} className="hover:bg-surface-2">
                  <td className="px-3 py-1.5 font-mono text-xs text-fg-muted">{p.id}</td>
                  <td className="px-3 py-1.5 font-mono text-xs text-fg-muted">
                    {p.created_at ?? '—'}
                  </td>
                  <td className="px-3 py-1.5 truncate font-mono text-xs text-fg-muted">
                    {p.session_id ?? '—'}
                  </td>
                  <td className="px-3 py-1.5 max-w-md truncate text-fg">
                    {p.content ?? <span className="text-fg-muted">{t('common.empty')}</span>}
                  </td>
                  <td className="px-3 py-1.5 text-right">
                    <div className="flex justify-end gap-2">
                      <Button size="sm" variant="primary" onClick={() => onAssign(p)}>
                        {t('orphans.table.assign')}
                      </Button>
                      <Button size="sm" variant="danger" onClick={() => onDelete(p)}>
                        {t('orphans.table.delete')}
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardBody>
    </Card>
  );
}
