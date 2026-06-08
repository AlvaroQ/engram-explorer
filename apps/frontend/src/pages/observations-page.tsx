import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { useNavigate, useSearch } from '@tanstack/react-router';
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Download, FileJson, Search, SlidersHorizontal, Trash2 } from 'lucide-react';
import { useEffect, useMemo, useRef, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '../components/ui/badge.tsx';
import { Button } from '../components/ui/button.tsx';
import { Input } from '../components/ui/input.tsx';
import { Select } from '../components/ui/select.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Drawer } from '../components/ui/drawer.tsx';
import { MultiSelectChips } from '../components/observations/multi-select-chips.tsx';
import { ObservationDetail } from '../components/observations/observation-detail.tsx';
import {
  api,
  type ObservationListParams,
  type ObservationRow,
  type ProjectStats,
} from '../lib/api.ts';
import { observationsRoute, type ObservationsSearch } from '../router.tsx';
import { downloadCsv, downloadJson } from '../lib/export.ts';

const PAGE_LIMIT = 100;

export function ObservationsPage(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate({ from: observationsRoute.fullPath });
  const search = useSearch({ from: observationsRoute.fullPath });
  const [drawerId, setDrawerId] = useState<number | null>(null);

  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: api.listProjects,
    staleTime: 5 * 60_000,
  });
  const projectOptions = (projectsQuery.data?.items ?? [])
    .map((p: ProjectStats) => p.project)
    .filter((p): p is string => typeof p === 'string');

  const params: ObservationListParams = useMemo(() => {
    const out: ObservationListParams = {
      limit: PAGE_LIMIT,
    };
    if (search.project && search.project.length > 0) out.project = search.project;
    if (search.type && search.type.length > 0) out.type = search.type;
    if (search.tool_name && search.tool_name.length > 0) out.tool_name = search.tool_name;
    if (search.scope && search.scope !== 'all') out.scope = search.scope;
    if (search.q) out.q = search.q;
    if (search.topic_key) out.topic_key = search.topic_key;
    if (search.deleted === 'only') out.only_deleted = true;
    if (search.deleted === 'all') out.include_deleted = true;
    return out;
  }, [search]);

  const observations = useInfiniteQuery({
    queryKey: ['observations', params],
    queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
      api.listObservations({ ...params, ...(pageParam ? { cursor: pageParam } : {}) }),
    getNextPageParam: (last) => last.nextCursor ?? undefined,
    initialPageParam: undefined,
  });

  const rows = useMemo<ObservationRow[]>(() => {
    if (!observations.data) return [];
    return observations.data.pages.flatMap((p) => p.items);
  }, [observations.data]);

  function setSearchPatch(patch: FilterPatch): void {
    void navigate({
      search: (old: ObservationsSearch) => normalizeSearch({ ...old, ...patch }),
    });
  }

  return (
    <div className="flex h-full flex-col gap-4">
      <header className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-fg">{t('observations.title')}</h1>
          <p className="text-sm text-fg-muted">{t('observations.description')}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="secondary"
            disabled={rows.length === 0}
            onClick={() => downloadJson(`observations-${nowSlug()}.json`, rows)}
          >
            <FileJson size={14} /> JSON
          </Button>
          <Button
            size="sm"
            variant="secondary"
            disabled={rows.length === 0}
            onClick={() =>
              downloadCsv(`observations-${nowSlug()}.csv`, rows, [
                'id',
                'created_at',
                'updated_at',
                'project',
                'type',
                'title',
                'tool_name',
                'topic_key',
                'revision_count',
                'duplicate_count',
                'session_id',
              ])
            }
          >
            <Download size={14} /> CSV
          </Button>
          <ResultMeta isLoading={observations.isLoading} count={rows.length} />
        </div>
      </header>

      <FilterBar
        search={search}
        projectOptions={projectOptions}
        onChange={(patch) => setSearchPatch(patch)}
      />

      <div className="flex-1 overflow-hidden rounded-lg border border-border bg-surface">
        {observations.isLoading ? (
          <Skeleton className="h-full" />
        ) : observations.isError ? (
          <ErrorBlock message={(observations.error as Error | undefined)?.message ?? 'unknown'} />
        ) : (
          <ObservationsTable
            rows={rows}
            onSelect={(id) => setDrawerId(id)}
            isFetchingNextPage={observations.isFetchingNextPage}
            hasNextPage={observations.hasNextPage ?? false}
            fetchNextPage={() => void observations.fetchNextPage()}
          />
        )}
      </div>

      <Drawer
        open={drawerId !== null}
        onOpenChange={(open) => {
          if (!open) setDrawerId(null);
        }}
        title={drawerId === null ? '' : t('observations.drawerTitle', { id: String(drawerId) })}
      >
        {drawerId !== null ? <ObservationDetail id={drawerId} /> : null}
      </Drawer>
    </div>
  );
}

function ResultMeta({
  isLoading,
  count,
}: {
  isLoading: boolean;
  count: number;
}): JSX.Element {
  const { t } = useTranslation();
  if (isLoading) return <Skeleton className="h-5 w-32" />;
  return <p className="text-xs text-fg-muted">{t('observations.loaded', { count })}</p>;
}

function nowSlug(): string {
  const d = new Date();
  return d.toISOString().replace(/[:.]/g, '-').slice(0, 19);
}

function ErrorBlock({ message }: { message: string }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="flex h-full items-center justify-center p-6">
      <p className="text-sm text-fail">{t('observations.loadError', { message })}</p>
    </div>
  );
}

type FilterPatch = {
  q?: string | undefined;
  project?: string[] | undefined;
  type?: string[] | undefined;
  tool_name?: string[] | undefined;
  scope?: 'project' | 'personal' | 'all' | undefined;
  topic_key?: string | undefined;
  deleted?: 'active' | 'all' | 'only' | undefined;
};

function normalizeSearch(s: FilterPatch): ObservationsSearch {
  const out: ObservationsSearch = {};
  if (s.q && s.q.length > 0) out.q = s.q;
  if (s.project && s.project.length > 0) out.project = s.project;
  if (s.type && s.type.length > 0) out.type = s.type;
  if (s.tool_name && s.tool_name.length > 0) out.tool_name = s.tool_name;
  if (s.scope && s.scope !== 'all') out.scope = s.scope;
  if (s.topic_key) out.topic_key = s.topic_key;
  if (s.deleted && s.deleted !== 'active') out.deleted = s.deleted;
  return out;
}

const TYPE_OPTIONS = [
  'architecture',
  'bugfix',
  'decision',
  'discovery',
  'session_summary',
  'config',
  'ui-fix',
  'pattern',
  'preference',
  'insight',
  'project',
  'learning',
  'manual',
  'feature',
  'command',
];

interface FilterBarProps {
  search: ObservationsSearch;
  projectOptions: string[];
  onChange: (patch: FilterPatch) => void;
}

function FilterBar({ search, projectOptions, onChange }: FilterBarProps): JSX.Element {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(true);
  const [qDraft, setQDraft] = useState(search.q ?? '');

  // Debounce q updates into URL
  useEffect(() => {
    const tm = window.setTimeout(() => {
      if ((search.q ?? '') !== qDraft) {
        onChange({ q: qDraft || undefined });
      }
    }, 250);
    return () => window.clearTimeout(tm);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [qDraft]);

  // Mirror URL→draft when navigating externally
  useEffect(() => {
    setQDraft(search.q ?? '');
  }, [search.q]);

  const hasFilters =
    Boolean(search.q) ||
    (search.project?.length ?? 0) > 0 ||
    (search.type?.length ?? 0) > 0 ||
    (search.tool_name?.length ?? 0) > 0 ||
    (search.scope && search.scope !== 'all') ||
    Boolean(search.topic_key) ||
    (search.deleted && search.deleted !== 'active');

  return (
    <div className="rounded-lg border border-border bg-surface">
      <div className="flex items-center justify-between gap-2 px-3 py-2">
        <div className="relative flex-1">
          <Search
            size={14}
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-fg-muted"
          />
          <Input
            className="pl-9"
            placeholder={t('observations.searchPlaceholder')}
            value={qDraft}
            onChange={(e) => setQDraft(e.target.value)}
          />
        </div>
        <Button variant="secondary" size="md" onClick={() => setExpanded((v) => !v)}>
          <SlidersHorizontal size={14} />
          {t('observations.filters')}
        </Button>
        {hasFilters ? (
          <Button
            variant="ghost"
            size="md"
            onClick={() =>
              onChange({
                q: undefined,
                project: undefined,
                type: undefined,
                tool_name: undefined,
                scope: undefined,
                topic_key: undefined,
                deleted: undefined,
              })
            }
          >
            <Trash2 size={14} /> {t('observations.clear')}
          </Button>
        ) : null}
      </div>
      {expanded ? (
        <div className="grid grid-cols-1 gap-3 border-t border-border px-3 py-3 sm:grid-cols-2 lg:grid-cols-3">
          <MultiSelectChips
            label={t('observations.labels.project')}
            values={search.project ?? []}
            options={projectOptions}
            onChange={(vals) => onChange({ project: vals.length > 0 ? vals : undefined })}
          />
          <MultiSelectChips
            label={t('observations.labels.type')}
            values={search.type ?? []}
            options={TYPE_OPTIONS}
            onChange={(vals) => onChange({ type: vals.length > 0 ? vals : undefined })}
          />
          <MultiSelectChips
            label={t('observations.labels.toolName')}
            values={search.tool_name ?? []}
            onChange={(vals) => onChange({ tool_name: vals.length > 0 ? vals : undefined })}
            placeholder={t('observations.placeholders.toolName')}
          />
          <div className="flex flex-col gap-1">
            <label className="text-xs uppercase tracking-wide text-fg-muted" htmlFor="scope-select">
              {t('observations.labels.scope')}
            </label>
            <Select
              id="scope-select"
              value={search.scope ?? 'all'}
              onChange={(e) =>
                onChange({
                  scope: e.target.value === 'all' ? undefined : (e.target.value as 'project' | 'personal'),
                })
              }
            >
              <option value="all">{t('observations.scope.all')}</option>
              <option value="project">{t('observations.scope.project')}</option>
              <option value="personal">{t('observations.scope.personal')}</option>
            </Select>
          </div>
          <div className="flex flex-col gap-1">
            <label className="text-xs uppercase tracking-wide text-fg-muted" htmlFor="topic-input">
              {t('observations.labels.topicKey')}
            </label>
            <Input
              id="topic-input"
              placeholder={t('observations.placeholders.topicKey')}
              value={search.topic_key ?? ''}
              onChange={(e) => onChange({ topic_key: e.target.value || undefined })}
            />
          </div>
          <div className="flex flex-col gap-1">
            <label className="text-xs uppercase tracking-wide text-fg-muted" htmlFor="deleted-select">
              {t('observations.labels.deletedRows')}
            </label>
            <Select
              id="deleted-select"
              value={search.deleted ?? 'active'}
              onChange={(e) =>
                onChange({
                  deleted: e.target.value === 'active' ? undefined : (e.target.value as 'all' | 'only'),
                })
              }
            >
              <option value="active">{t('observations.deleted.active')}</option>
              <option value="all">{t('observations.deleted.all')}</option>
              <option value="only">{t('observations.deleted.only')}</option>
            </Select>
          </div>
        </div>
      ) : null}
    </div>
  );
}

interface ObservationsTableProps {
  rows: ObservationRow[];
  onSelect: (id: number) => void;
  isFetchingNextPage: boolean;
  hasNextPage: boolean;
  fetchNextPage: () => void;
}

function ObservationsTable({
  rows,
  onSelect,
  isFetchingNextPage,
  hasNextPage,
  fetchNextPage,
}: ObservationsTableProps): JSX.Element {
  const { t } = useTranslation();
  const columns = useMemo<ColumnDef<ObservationRow>[]>(
    () => [
      {
        header: t('observations.table.created'),
        accessorFn: (r) => r.updated_at ?? r.created_at ?? '',
        cell: (ctx) => <span className="font-mono text-xs">{ctx.getValue<string>()}</span>,
        size: 160,
      },
      {
        header: t('observations.table.project'),
        accessorFn: (r) => r.project ?? '',
        cell: (ctx) => {
          const v = ctx.getValue<string>();
          return v === '' ? (
            <Badge tone="fail">{t('observations.table.empty')}</Badge>
          ) : (
            <span className="text-xs">{v}</span>
          );
        },
        size: 160,
      },
      {
        header: t('observations.table.type'),
        accessorFn: (r) => r.type,
        cell: (ctx) => <Badge tone="accent">{ctx.getValue<string>()}</Badge>,
        size: 110,
      },
      {
        header: t('observations.table.title'),
        accessorFn: (r) => r.title ?? '',
        cell: (ctx) => (
          <span className="block truncate text-sm text-fg">{ctx.getValue<string>() || '—'}</span>
        ),
        size: 360,
      },
      {
        header: t('observations.table.tool'),
        accessorFn: (r) => r.tool_name ?? '',
        cell: (ctx) => <span className="text-xs text-fg-muted">{ctx.getValue<string>() || '—'}</span>,
        size: 90,
      },
      {
        header: t('observations.table.topic'),
        accessorFn: (r) => r.topic_key ?? '',
        cell: (ctx) => (
          <span className="block truncate font-mono text-xs text-fg-muted">
            {ctx.getValue<string>() || '—'}
          </span>
        ),
        size: 180,
      },
      {
        header: t('observations.table.rev'),
        accessorFn: (r) => r.revision_count ?? 0,
        cell: (ctx) => <span className="text-xs text-fg-muted">{ctx.getValue<number>()}</span>,
        size: 50,
      },
      {
        header: t('observations.table.dup'),
        accessorFn: (r) => r.duplicate_count ?? 0,
        cell: (ctx) => <span className="text-xs text-fg-muted">{ctx.getValue<number>()}</span>,
        size: 50,
      },
    ],
    [t],
  );

  const table = useReactTable({
    data: rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

  const parentRef = useRef<HTMLDivElement>(null);
  const tableRows = table.getRowModel().rows;
  const virtualizer = useVirtualizer({
    count: tableRows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 36,
    overscan: 12,
  });

  const virtualItems = virtualizer.getVirtualItems();
  const totalSize = virtualizer.getTotalSize();
  const lastItem = virtualItems[virtualItems.length - 1];

  useEffect(() => {
    if (!hasNextPage || isFetchingNextPage) return;
    if (!lastItem) return;
    if (lastItem.index >= tableRows.length - 20) {
      fetchNextPage();
    }
  }, [hasNextPage, isFetchingNextPage, lastItem, tableRows.length, fetchNextPage]);

  return (
    <div className="flex h-full flex-col">
      <div
        className="grid border-b border-border bg-surface-2 px-3 py-2 text-xs uppercase tracking-wide text-fg-muted"
        style={{ gridTemplateColumns: gridTemplate(columns) }}
      >
        {table.getHeaderGroups().map((hg) =>
          hg.headers.map((h) => (
            <span key={h.id} className="truncate">
              {flexRender(h.column.columnDef.header, h.getContext())}
            </span>
          )),
        )}
      </div>
      <div ref={parentRef} className="flex-1 overflow-auto">
        {tableRows.length === 0 ? (
          <p className="p-6 text-center text-sm text-fg-muted">{t('observations.table.noMatch')}</p>
        ) : (
          <div style={{ height: totalSize, position: 'relative' }}>
            {virtualItems.map((vi) => {
              const row = tableRows[vi.index];
              if (!row) return null;
              return (
                <button
                  key={row.id}
                  type="button"
                  onClick={() => onSelect(row.original.id)}
                  className="absolute left-0 top-0 grid w-full cursor-pointer items-center gap-1 border-b border-border/50 bg-transparent px-3 text-left hover:bg-surface-2"
                  style={{
                    transform: `translateY(${String(vi.start)}px)`,
                    height: vi.size,
                    gridTemplateColumns: gridTemplate(columns),
                  }}
                >
                  {row.getVisibleCells().map((cell) => (
                    <div key={cell.id} className="min-w-0 truncate">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </div>
                  ))}
                </button>
              );
            })}
          </div>
        )}
        {isFetchingNextPage ? (
          <p className="p-3 text-center text-xs text-fg-muted">{t('observations.table.loadingMore')}</p>
        ) : null}
      </div>
    </div>
  );
}

function gridTemplate(columns: ColumnDef<ObservationRow>[]): string {
  return columns.map((c) => `${String(c.size ?? 100)}px`).join(' ');
}
