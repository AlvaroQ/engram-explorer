import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiRequestError, api, type AssignProjectResponse, type ProjectStats } from '../../lib/api.ts';
import { Button } from '../ui/button.tsx';
import { Input } from '../ui/input.tsx';
import { Badge } from '../ui/badge.tsx';

interface AssignProjectModalProps {
  entity: 'observation' | 'session' | 'prompt';
  id: string | number;
  currentLabel: string;
  onClose: () => void;
  onAssigned?: (result: AssignProjectResponse) => void;
}

export function AssignProjectModal({
  entity,
  id,
  currentLabel,
  onClose,
  onAssigned,
}: AssignProjectModalProps): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState('');
  const [selected, setSelected] = useState<string | null>(null);

  const projects = useQuery({
    queryKey: ['projects'],
    queryFn: api.listProjects,
    staleTime: 60_000,
  });
  const sync = useQuery({
    queryKey: ['sync-projects'],
    queryFn: api.syncProjects,
    staleTime: 60_000,
  });

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  const projectOptions = useMemo(() => {
    const items = projects.data?.items ?? [];
    return items
      .filter((p: ProjectStats) => p.project && p.project.trim() !== '')
      .map((p) => p.project)
      .sort((a, b) => a.localeCompare(b));
  }, [projects.data]);

  const filtered = useMemo(() => {
    if (draft.trim().length === 0) return projectOptions.slice(0, 12);
    const needle = draft.toLowerCase();
    return projectOptions.filter((p) => p.toLowerCase().includes(needle)).slice(0, 12);
  }, [projectOptions, draft]);

  const enrolled = useMemo(() => {
    if (!selected || !sync.data) return false;
    return sync.data.projects.some((p) => p.project === selected && p.enrolled);
  }, [selected, sync.data]);

  const mutation = useMutation({
    mutationFn: (project: string) => api.assignProject(entity, id, project),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: ['orphans'] });
      void queryClient.invalidateQueries({ queryKey: ['observations'] });
      void queryClient.invalidateQueries({ queryKey: ['observation', id] });
      void queryClient.invalidateQueries({ queryKey: ['projects'] });
      void queryClient.invalidateQueries({ queryKey: ['overview'] });
      void queryClient.invalidateQueries({ queryKey: ['sync-projects'] });
      void queryClient.invalidateQueries({ queryKey: ['sync-issues'] });
      onAssigned?.(result);
      onClose();
    },
  });

  const errorMessage = mutation.error
    ? mutation.error instanceof ApiRequestError
      ? mutation.error.api.message
      : (mutation.error as Error).message
    : null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
    >
      <div className="w-full max-w-md rounded-lg border border-border bg-surface p-5 shadow-2xl">
        <h2 className="text-base font-semibold text-fg">{t('assign.modalTitle')}</h2>

        <div className="mt-4 grid grid-cols-2 gap-3 rounded-md border border-border bg-surface-2 p-3 text-xs">
          <div>
            <p className="uppercase tracking-wide text-fg-muted">{t('assign.preview.current')}</p>
            <p className="mt-1 truncate font-mono text-fg">{currentLabel || t('assign.preview.empty')}</p>
          </div>
          <div>
            <p className="uppercase tracking-wide text-fg-muted">{t('assign.preview.next')}</p>
            <p className="mt-1 truncate font-mono text-fg">
              {selected ?? <span className="text-fg-muted">—</span>}
            </p>
          </div>
        </div>

        <div className="mt-4">
          <label className="text-xs uppercase tracking-wide text-fg-muted" htmlFor="assign-search">
            {t('assign.selectLabel')}
          </label>
          <Input
            id="assign-search"
            className="mt-1"
            placeholder={t('assign.selectPlaceholder')}
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value);
              setSelected(null);
            }}
            autoFocus
          />
          <div className="mt-2 max-h-48 overflow-auto rounded-md border border-border bg-surface-2">
            {filtered.length === 0 ? (
              <p className="p-3 text-xs text-fg-muted">{t('assign.noMatch')}</p>
            ) : (
              <ul className="divide-y divide-border">
                {filtered.map((p) => (
                  <li key={p}>
                    <button
                      type="button"
                      className={
                        'flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-surface ' +
                        (selected === p ? 'bg-surface text-fg' : 'text-fg-muted')
                      }
                      onClick={() => setSelected(p)}
                    >
                      <span className="truncate">{p}</span>
                      {selected === p ? <Badge tone="accent">✓</Badge> : null}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>

        {selected ? (
          <p
            className={
              'mt-3 rounded-md border p-2 text-xs ' +
              (enrolled
                ? 'border-accent/40 bg-accent/10 text-fg'
                : 'border-warn/40 bg-warn/10 text-fg')
            }
          >
            {enrolled ? t('assign.noteEnrolled') : t('assign.noteNotEnrolled')}
          </p>
        ) : null}

        {errorMessage ? (
          <p className="mt-3 rounded-md border border-fail/40 bg-fail/10 p-2 text-xs text-fail">
            {t('assign.error', { message: errorMessage })}
          </p>
        ) : null}

        <div className="mt-5 flex justify-end gap-2">
          <Button variant="secondary" size="md" onClick={onClose} disabled={mutation.isPending}>
            {t('modal.cancel')}
          </Button>
          <Button
            variant="primary"
            size="md"
            disabled={!selected || mutation.isPending}
            onClick={() => {
              if (selected) mutation.mutate(selected);
            }}
          >
            {mutation.isPending ? t('assign.submitting') : t('assign.submit')}
          </Button>
        </div>
      </div>
    </div>
  );
}
