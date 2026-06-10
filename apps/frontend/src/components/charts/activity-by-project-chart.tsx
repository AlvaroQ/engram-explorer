import { useMemo, useState, type JSX } from 'react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type ActivityRange, type ActivityResponse } from '../../lib/api.ts';
import { Skeleton } from '../ui/skeleton.tsx';
import { cn } from '../../lib/cn.ts';

const PROJECT_COLORS = [
  'hsl(224 90% 70%)',
  'hsl(142 76% 60%)',
  'hsl(38 92% 60%)',
  'hsl(280 80% 70%)',
  'hsl(190 80% 60%)',
];
const OTHERS_COLOR = 'hsl(220 8% 55%)';
const ORPHANS_COLOR = 'hsl(0 0% 45% / 0.55)';
const ORPHANS_RAW = '(orphans)';
const TOP_N = 5;

type WidePoint = { day: string } & Record<string, number>;

interface PivotResult {
  data: WidePoint[];
  series: Array<{
    key: string;
    label: string;
    color: string;
    isOthers: boolean;
    isOrphans: boolean;
  }>;
}

function pivot(
  rows: ActivityResponse['rows'],
  orphansLabel: string,
  othersLabel: string,
): PivotResult {
  if (rows.length === 0) return { data: [], series: [] };

  const totals = new Map<string, number>();
  for (const r of rows) {
    totals.set(r.project, (totals.get(r.project) ?? 0) + r.count);
  }

  const projectsByTotal = [...totals.entries()].sort((a, b) => b[1] - a[1]).map(([k]) => k);
  const orphansPresent = projectsByTotal.includes(ORPHANS_RAW);
  const ranked = projectsByTotal.filter((p) => p !== ORPHANS_RAW);
  const top = ranked.slice(0, TOP_N);
  const tail = ranked.slice(TOP_N);
  const inTop = new Set(top);

  const dayMap = new Map<string, WidePoint>();
  for (const r of rows) {
    const existing = dayMap.get(r.day);
    const point = existing ?? ({ day: r.day } as WidePoint);
    let bucket: string;
    if (r.project === ORPHANS_RAW) {
      bucket = ORPHANS_RAW;
    } else if (inTop.has(r.project)) {
      bucket = r.project;
    } else {
      bucket = '__others__';
    }
    point[bucket] = (point[bucket] ?? 0) + r.count;
    if (!existing) dayMap.set(r.day, point);
  }

  const data = [...dayMap.values()].sort((a, b) => a.day.localeCompare(b.day));

  const series: PivotResult['series'] = top.map((p, i) => ({
    key: p,
    label: p,
    color: PROJECT_COLORS[i % PROJECT_COLORS.length] ?? PROJECT_COLORS[0]!,
    isOthers: false,
    isOrphans: false,
  }));
  if (tail.length > 0) {
    series.push({
      key: '__others__',
      label: `${othersLabel} (${String(tail.length)})`,
      color: OTHERS_COLOR,
      isOthers: true,
      isOrphans: false,
    });
  }
  if (orphansPresent) {
    series.push({
      key: ORPHANS_RAW,
      label: orphansLabel,
      color: ORPHANS_COLOR,
      isOthers: false,
      isOrphans: true,
    });
  }

  return { data, series };
}

export function ActivityByProjectChart(): JSX.Element {
  const { t } = useTranslation();
  const [range, setRange] = useState<ActivityRange>('30d');

  const query = useQuery({
    queryKey: ['activity', range],
    queryFn: () => api.activityByProject(range),
  });

  const pivoted = useMemo(
    () =>
      pivot(query.data?.rows ?? [], t('overview.activity.orphans'), t('overview.activity.others')),
    [query.data, t],
  );

  return (
    <div className="flex h-full flex-col gap-3">
      <div className="flex items-center justify-end">
        <RangeToggle value={range} onChange={setRange} />
      </div>
      <div className="min-h-0 flex-1">
        {query.isLoading ? (
          <Skeleton className="h-full" />
        ) : query.isError ? (
          <p className="text-sm text-fail">{t('overview.activity.loadError')}</p>
        ) : pivoted.data.length === 0 ? (
          <p className="text-sm text-fg-muted">{t('overview.activity.empty')}</p>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={pivoted.data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
              <XAxis
                dataKey="day"
                stroke="hsl(var(--fg-muted))"
                tick={{ fill: 'hsl(var(--fg-muted))', fontSize: 11 }}
                tickFormatter={(v: string) => v.slice(5)}
                interval="preserveStartEnd"
              />
              <YAxis
                stroke="hsl(var(--fg-muted))"
                tick={{ fill: 'hsl(var(--fg-muted))', fontSize: 11 }}
                allowDecimals={false}
                width={32}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: 'hsl(var(--surface-2))',
                  border: '1px solid hsl(var(--border))',
                  borderRadius: 8,
                  color: 'hsl(var(--fg))',
                  fontSize: 12,
                }}
                cursor={{ fill: 'hsl(var(--surface-2))', opacity: 0.4 }}
                itemSorter={(item) => -(item.value as number)}
              />
              <Legend
                wrapperStyle={{ fontSize: 11, paddingTop: 4 }}
                iconType="square"
                iconSize={8}
                formatter={(value) => {
                  const s = pivoted.series.find((x) => x.label === value);
                  return (
                    <span
                      className={cn(
                        'text-fg',
                        s?.isOrphans || s?.isOthers ? 'italic text-fg-muted' : null,
                      )}
                    >
                      {value}
                    </span>
                  );
                }}
              />
              {pivoted.series.map((s) => (
                <Bar
                  key={s.key}
                  dataKey={s.key}
                  name={s.label}
                  stackId="a"
                  fill={s.color}
                  radius={[2, 2, 0, 0]}
                />
              ))}
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  );
}

function RangeToggle({
  value,
  onChange,
}: {
  value: ActivityRange;
  onChange: (next: ActivityRange) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const options: ActivityRange[] = ['7d', '30d', '90d'];
  return (
    <div
      role="group"
      aria-label={t('overview.activity.range.ariaLabel')}
      className="inline-flex rounded-md border border-border bg-surface p-0.5"
    >
      {options.map((opt) => {
        const active = opt === value;
        return (
          <button
            key={opt}
            type="button"
            onClick={() => onChange(opt)}
            aria-pressed={active}
            className={cn(
              'rounded px-2.5 py-1 text-xs font-medium tabular-nums transition-colors',
              active ? 'bg-accent text-white' : 'text-fg-muted hover:text-fg',
            )}
          >
            {t(`overview.activity.range.${opt}` as const)}
          </button>
        );
      })}
    </div>
  );
}
