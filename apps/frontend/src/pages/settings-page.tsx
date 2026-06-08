import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Check,
  Cloud,
  Cpu,
  Database,
  Download,
  Languages,
  Moon,
  Palette,
  Server,
  Sun,
  Upload,
} from 'lucide-react';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiRequestError, api, type DbImportResponse } from '../lib/api.ts';
import { Card, CardBody, CardHeader } from '../components/ui/card.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { Badge } from '../components/ui/badge.tsx';
import { Button } from '../components/ui/button.tsx';
import { ConfirmModal } from '../components/ui/confirm-modal.tsx';
import { cn } from '../lib/cn.ts';
import { useTheme, type Theme } from '../lib/theme.ts';
import { SUPPORTED_LANGS, type SupportedLang } from '../lib/i18n.ts';

import type { ChangeEvent, JSX, ReactNode } from 'react';

export function SettingsPage(): JSX.Element {
  const { t, i18n } = useTranslation();
  const [theme, setTheme] = useTheme();
  const health = useQuery({ queryKey: ['health'], queryFn: api.health, staleTime: 5_000 });
  const meta = useQuery({ queryKey: ['meta'], queryFn: api.meta, staleTime: 60_000 });
  const caps = useQuery({
    queryKey: ['cloud-capabilities'],
    queryFn: api.cloudCapabilities,
    staleTime: 60_000,
  });

  const currentLang = (SUPPORTED_LANGS as readonly string[]).includes(i18n.resolvedLanguage ?? '')
    ? (i18n.resolvedLanguage as SupportedLang)
    : 'es';

  return (
    <div className="flex flex-col gap-5">
      <header>
        <h1 className="text-2xl font-semibold text-fg">{t('settings.title')}</h1>
      </header>

      <Section icon={<Palette size={14} />} title={t('settings.section.appearance')}>
        <Card>
          <CardBody className="divide-y divide-border/60 px-5 py-0">
            <PreferenceRow
              icon={<Languages size={16} />}
              label={t('settings.language')}
              description={t('settings.languageDescription')}
            >
              {SUPPORTED_LANGS.map((lang) => (
                <Button
                  key={lang}
                  variant={currentLang === lang ? 'primary' : 'secondary'}
                  size="sm"
                  onClick={() => void i18n.changeLanguage(lang)}
                  aria-pressed={currentLang === lang}
                >
                  {currentLang === lang ? <Check size={14} /> : null}
                  {t(`lang.${lang}`)}
                </Button>
              ))}
            </PreferenceRow>

            <PreferenceRow icon={<Sun size={16} />} label={t('settings.theme')}>
              <ThemeButton current={theme} value="dark" onSelect={setTheme} icon={<Moon size={14} />} label={t('settings.themeDark')} />
              <ThemeButton current={theme} value="light" onSelect={setTheme} icon={<Sun size={14} />} label={t('settings.themeLight')} />
            </PreferenceRow>
          </CardBody>
        </Card>
      </Section>

      <Section icon={<Server size={14} />} title={t('settings.section.system')}>
        <DatabaseCard
          loading={health.isLoading}
          path={health.data?.db.path}
          ok={health.data?.db.ok}
        />

        <div className="grid gap-4 sm:grid-cols-2">
          <Card>
            <CardHeader title={<TitleWithIcon icon={<Server size={14} />}>{t('settings.daemon')}</TitleWithIcon>} />
            <CardBody className="py-1.5">
              {health.isLoading ? (
                <div className="py-2">
                  <Skeleton className="h-4 w-full" />
                </div>
              ) : (
                <RowGroup>
                  <Row label={t('settings.row.url')} value={health.data?.daemon.url ?? '—'} mono />
                  <Row
                    label={t('settings.row.status')}
                    node={
                      <Badge tone={health.data?.daemon.ok ? 'ok' : 'fail'}>
                        {health.data?.daemon.ok ? t('settings.badge.reachable') : t('settings.badge.unreachable')}
                      </Badge>
                    }
                  />
                  {!health.data?.daemon.ok && health.data?.daemon.error ? (
                    <p className="py-2 text-xs text-fail">
                      {health.data.daemon.error.code}: {health.data.daemon.error.message}
                    </p>
                  ) : null}
                </RowGroup>
              )}
            </CardBody>
          </Card>

          <Card>
            <CardHeader title={<TitleWithIcon icon={<Cpu size={14} />}>{t('settings.runtime')}</TitleWithIcon>} />
            <CardBody className="py-1.5">
              <RowGroup>
                <Row label={t('settings.row.dashboard')} value={meta.data ? `${meta.data.name} ${meta.data.version}` : '—'} />
                <Row label={t('settings.row.node')} value={meta.data?.node ?? '—'} />
                <Row label={t('settings.row.uptime')} value={health.data ? `${String(health.data.uptime_s)}s` : '—'} />
              </RowGroup>
            </CardBody>
          </Card>
        </div>
      </Section>

      <Section icon={<Cloud size={14} />} title={t('settings.section.cloud')}>
        <Card>
          <CardHeader title={<TitleWithIcon icon={<Cloud size={14} />}>{t('settings.cloudCli')}</TitleWithIcon>} description={t('settings.cloudCliDesc')} />
          <CardBody className="py-1.5">
            {caps.isLoading ? (
              <div className="py-2">
                <Skeleton className="h-4 w-full" />
              </div>
            ) : caps.isError ? (
              <p className="py-2 text-sm text-fail">{t('settings.cloudCliError')}</p>
            ) : (
              <RowGroup>
                <Row
                  label={t('settings.row.cloudEnroll')}
                  node={
                    <Badge tone={caps.data?.enroll ? 'ok' : 'fail'}>
                      {caps.data?.enroll ? t('settings.badge.available') : t('settings.badge.missing')}
                    </Badge>
                  }
                />
                <Row
                  label={t('settings.row.cloudUnenroll')}
                  node={
                    <Badge tone={caps.data?.unenroll ? 'ok' : 'warn'}>
                      {caps.data?.unenroll ? t('settings.badge.available') : t('settings.badge.notInCli')}
                    </Badge>
                  }
                />
                {caps.data && !caps.data.unenroll ? (
                  <p className="py-2 text-xs text-fg-muted">{t('settings.cloudCliTip')}</p>
                ) : null}
              </RowGroup>
            )}
          </CardBody>
        </Card>
      </Section>
    </div>
  );
}

function Section({
  icon,
  title,
  children,
}: {
  icon: ReactNode;
  title: string;
  children: ReactNode;
}): JSX.Element {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center gap-2 text-fg-muted">
        {icon}
        <h2 className="text-xs font-semibold">{title}</h2>
      </div>
      <div className="flex flex-col gap-4">{children}</div>
    </section>
  );
}

function TitleWithIcon({ icon, children }: { icon: ReactNode; children: ReactNode }): JSX.Element {
  return <span className="inline-flex items-center gap-2">{icon} {children}</span>;
}

function PreferenceRow({
  icon,
  label,
  description,
  children,
}: {
  icon: ReactNode;
  label: string;
  description?: string;
  children: ReactNode;
}): JSX.Element {
  return (
    <div className="flex items-center justify-between gap-4 py-4">
      <div className="flex min-w-0 items-center gap-2.5">
        <span className="shrink-0 text-fg-muted">{icon}</span>
        <div className="min-w-0">
          <p className="text-sm font-medium text-fg">{label}</p>
          {description ? <p className="text-xs text-fg-muted">{description}</p> : null}
        </div>
      </div>
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">{children}</div>
    </div>
  );
}

function RowGroup({ children }: { children: ReactNode }): JSX.Element {
  return <div className="divide-y divide-border/60 text-sm">{children}</div>;
}

function ThemeButton({
  current,
  value,
  onSelect,
  icon,
  label,
}: {
  current: Theme;
  value: Theme;
  onSelect: (t: Theme) => void;
  icon: JSX.Element;
  label: string;
}): JSX.Element {
  return (
    <Button
      variant={current === value ? 'primary' : 'secondary'}
      size="sm"
      onClick={() => onSelect(value)}
      aria-pressed={current === value}
    >
      {icon}
      {label}
    </Button>
  );
}

function Row({
  label,
  value,
  node,
  mono,
}: {
  label: string;
  value?: string;
  node?: JSX.Element;
  mono?: boolean;
}): JSX.Element {
  return (
    <div className="flex items-center justify-between gap-3 py-2.5">
      <span className="shrink-0 text-fg-muted">{label}</span>
      {node ?? <span className={mono ? 'truncate font-mono text-fg' : 'text-fg'}>{value}</span>}
    </div>
  );
}

function errMessage(err: unknown): string {
  if (err instanceof ApiRequestError) return err.api.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

function DatabaseCard({
  loading,
  path,
  ok,
}: {
  loading: boolean;
  path: string | undefined;
  ok: boolean | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [exporting, setExporting] = useState(false);
  const [pendingFile, setPendingFile] = useState<File | null>(null);
  const [flash, setFlash] = useState<{ tone: 'ok' | 'fail'; message: string } | null>(null);
  const [result, setResult] = useState<DbImportResponse | null>(null);

  const importMutation = useMutation<DbImportResponse, unknown, File>({
    mutationFn: (file) => api.importDb(file),
    onSuccess: (res) => {
      setResult(res);
      setFlash(null);
      for (const key of ['health', 'overview', 'observations', 'graph', 'projects', 'sync-projects', 'topics', 'prompts']) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err) => {
      setResult(null);
      setFlash({ tone: 'fail', message: t('settings.db.importError', { message: errMessage(err) }) });
    },
  });

  const importing = importMutation.isPending;

  async function handleExport(): Promise<void> {
    setExporting(true);
    setFlash(null);
    setResult(null);
    try {
      const blob = await api.exportDb();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `engram-backup-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.db`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      setFlash({ tone: 'ok', message: t('settings.db.exported') });
    } catch (err) {
      setFlash({ tone: 'fail', message: t('settings.db.exportError', { message: errMessage(err) }) });
    } finally {
      setExporting(false);
    }
  }

  function onFilePicked(e: ChangeEvent<HTMLInputElement>): void {
    const file = e.target.files?.[0] ?? null;
    e.target.value = ''; // allow re-selecting the same file later
    if (file) setPendingFile(file);
  }

  return (
    <Card>
      <CardHeader
        title={<TitleWithIcon icon={<Database size={14} />}>{t('settings.database')}</TitleWithIcon>}
        description={t('settings.db.description')}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" onClick={() => void handleExport()} disabled={exporting || importing}>
              <Download size={14} />
              {exporting ? t('settings.db.exporting') : t('settings.db.export')}
            </Button>
            <Button size="sm" onClick={() => fileInputRef.current?.click()} disabled={importing || exporting}>
              <Upload size={14} />
              {importing ? t('settings.db.importing') : t('settings.db.import')}
            </Button>
          </div>
        }
      />
      <CardBody className="py-1.5">
        {loading ? (
          <div className="py-2">
            <Skeleton className="h-4 w-full" />
          </div>
        ) : (
          <RowGroup>
            <Row label={t('settings.row.path')} value={path ?? '—'} mono />
            <Row
              label={t('settings.row.status')}
              node={
                <Badge tone={ok ? 'ok' : 'fail'}>
                  {ok ? t('settings.badge.connected') : t('settings.badge.unavailable')}
                </Badge>
              }
            />
          </RowGroup>
        )}

        {result ? (
          <div className="mt-3 rounded-md border border-ok/40 bg-ok/10 px-3 py-2 text-xs">
            <p className="font-medium text-ok">
              {t('settings.db.imported', {
                observations: String(result.inserted.observations),
                sessions: String(result.inserted.sessions),
                prompts: String(result.inserted.prompts),
                relations: String(result.inserted.relations),
              })}
            </p>
            <p className="mt-1 break-all font-mono text-fg-muted">
              {t('settings.db.backupSaved', { path: result.backupPath })}
            </p>
          </div>
        ) : flash ? (
          <p className={cn('mt-3 text-xs', flash.tone === 'ok' ? 'text-ok' : 'text-fail')}>{flash.message}</p>
        ) : null}

        <input
          ref={fileInputRef}
          type="file"
          accept=".db,.sqlite,.sqlite3,application/octet-stream"
          className="hidden"
          onChange={onFilePicked}
        />
      </CardBody>

      {pendingFile ? (
        <ConfirmModal
          title={t('settings.db.confirmTitle')}
          description={
            <span>
              {t('settings.db.confirmBody')}
              <span className="mt-2 block break-all font-mono text-xs text-fg">{pendingFile.name}</span>
            </span>
          }
          confirmLabel={t('settings.db.import')}
          onConfirm={() => {
            const file = pendingFile;
            setPendingFile(null);
            if (file) importMutation.mutate(file);
          }}
          onCancel={() => setPendingFile(null)}
        />
      ) : null}
    </Card>
  );
}
