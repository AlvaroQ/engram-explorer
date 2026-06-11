import { useMemo, type JSX } from 'react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Legend,
  Line,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../lib/api.ts';
import { Skeleton } from '../ui/skeleton.tsx';

/**
 * Localized labels passed in from the Go templ side (via @Island props) so the
 * island stays out of the frontend i18n bundle. Data is fetched on its own from
 * /api/cc-sessions/stats (server-cached), so the heavy scan never blocks render.
 */
export interface CCUsageLabels {
  costByProject: string;
  tokensByModel: string;
  overTime: string;
  topSessions: string;
  sessions: string;
  tokens: string;
  cost: string;
  loading: string;
  empty: string;
  error: string;
  others: string;
  untitled: string;
  costEstimateNote: string;
}

const COLORS = [
  'hsl(343 76% 68%)', // love (pink)
  'hsl(189 43% 73%)', // foam
  'hsl(267 57% 78%)', // iris
  'hsl(35 88% 72%)',  // gold
  'hsl(197 49% 38%)', // pine
  'hsl(2 55% 83%)',   // rose
  'hsl(268 21% 57%)', // dawn iris
  'hsl(189 30% 48%)', // dawn foam
];
const FALLBACK_COLOR = 'hsl(248 15% 61%)';  // muted
const MODEL_COLORS: Record<string, string> = {
  opus: 'hsl(343 76% 68%)',   // love (pink)
  sonnet: 'hsl(189 43% 73%)', // foam
  haiku: 'hsl(35 88% 72%)',   // gold
  unknown: 'hsl(248 15% 61%)',// muted
};
const TOP_N = 8;

const tooltipStyle = {
  backgroundColor: 'hsl(var(--surface-2))',
  border: '1px solid hsl(var(--border))',
  borderRadius: 8,
  color: 'hsl(var(--fg))',
  fontSize: 12,
} as const;
// recharts renders tooltip item name/value with an inline black color by default
// (invisible on the dark tooltip box) — override item + label text explicitly.
const tooltipItemStyle = { color: 'hsl(var(--fg))' } as const;
const tooltipLabelStyle = { color: 'hsl(var(--fg-muted))' } as const;

function colorAt(i: number): string {
  return COLORS[i % COLORS.length] ?? FALLBACK_COLOR;
}

function fmtTokens(n: number): string {
  if (n <= 0) return '0';
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`;
  return `${(n / 1_000_000).toFixed(2)}M`;
}

function fmtCost(n: number): string {
  if (n <= 0) return '$0';
  if (n < 0.01) return '<$0.01';
  return `$${n.toFixed(2)}`;
}

export function CCUsageCharts({ labels }: { labels: CCUsageLabels }): JSX.Element {
  const query = useQuery({
    queryKey: ['cc-sessions-stats'],
    queryFn: () => api.ccSessionsStats(),
  });
  const data = query.data;

  const projectBars = useMemo(() => {
    if (!data) return [] as Array<{ name: string; cost: number }>;
    const sorted = [...data.by_project].sort((a, b) => b.usage.cost_usd - a.usage.cost_usd);
    const top = sorted.slice(0, TOP_N).map((p) => ({
      name: p.project,
      cost: Number(p.usage.cost_usd.toFixed(4)),
    }));
    const tail = sorted.slice(TOP_N);
    if (tail.length > 0) {
      const cost = tail.reduce((s, p) => s + p.usage.cost_usd, 0);
      top.push({
        name: `${labels.others} (${String(tail.length)})`,
        cost: Number(cost.toFixed(4)),
      });
    }
    return top;
  }, [data, labels.others]);

  const modelPie = useMemo(() => {
    if (!data) return [] as Array<{ name: string; value: number }>;
    return data.by_model
      .filter((m) => m.usage.total_tokens > 0)
      .map((m) => ({ name: m.model, value: m.usage.total_tokens }));
  }, [data]);

  const overTime = useMemo(() => {
    if (!data) return [] as Array<{ day: string; sessions: number; cost: number }>;
    return data.over_time.map((d) => ({
      day: d.day,
      sessions: d.sessions,
      cost: Number(d.usage.cost_usd.toFixed(4)),
    }));
  }, [data]);

  if (query.isLoading) return <Skeleton className="h-60" />;
  if (query.isError) return <p className="text-sm text-fail">{labels.error}</p>;
  if (!data || data.sessions === 0) return <p className="text-sm text-fg-muted">{labels.empty}</p>;

  const costSuffix = data.has_unknown_model ? '~' : '';

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap gap-8">
        <Stat label={labels.sessions} value={String(data.sessions)} />
        <Stat label={labels.tokens} value={fmtTokens(data.totals.total_tokens)} />
        <Stat label={labels.cost} value={`${fmtCost(data.totals.cost_usd)}${costSuffix}`} />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div>
          <p className="mb-2 text-xs font-medium text-fg-muted">{labels.costByProject}</p>
          <div style={{ height: 260 }}>
            <ResponsiveContainer width="100%" height="100%">
              <BarChart
                data={projectBars}
                layout="vertical"
                margin={{ top: 4, right: 16, left: 8, bottom: 0 }}
              >
                <CartesianGrid
                  strokeDasharray="3 3"
                  stroke="hsl(var(--border))"
                  horizontal={false}
                />
                <XAxis
                  type="number"
                  stroke="hsl(var(--fg-muted))"
                  tick={{ fill: 'hsl(var(--fg-muted))', fontSize: 11 }}
                  tickFormatter={(v) => fmtCost(Number(v))}
                />
                <YAxis
                  type="category"
                  dataKey="name"
                  width={120}
                  stroke="hsl(var(--fg-muted))"
                  tick={{ fill: 'hsl(var(--fg-muted))', fontSize: 11 }}
                />
                <Tooltip
                  contentStyle={tooltipStyle}
                  itemStyle={tooltipItemStyle}
                  labelStyle={tooltipLabelStyle}
                  cursor={{ fill: 'hsl(var(--surface-2))', opacity: 0.4 }}
                  formatter={(v) => fmtCost(Number(v))}
                />
                <Bar dataKey="cost" radius={[0, 2, 2, 0]}>
                  {projectBars.map((entry, i) => (
                    <Cell key={entry.name} fill={colorAt(i)} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div>
          <p className="mb-2 text-xs font-medium text-fg-muted">{labels.tokensByModel}</p>
          <div style={{ height: 260 }}>
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={modelPie}
                  dataKey="value"
                  nameKey="name"
                  cx="50%"
                  cy="50%"
                  innerRadius={55}
                  outerRadius={90}
                  paddingAngle={2}
                >
                  {modelPie.map((entry, i) => (
                    <Cell key={entry.name} fill={MODEL_COLORS[entry.name] ?? colorAt(i)} />
                  ))}
                </Pie>
                <Tooltip
                  contentStyle={tooltipStyle}
                  itemStyle={tooltipItemStyle}
                  labelStyle={tooltipLabelStyle}
                  formatter={(v) => fmtTokens(Number(v))}
                />
                <Legend wrapperStyle={{ fontSize: 11 }} iconType="circle" iconSize={8} />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>

      {overTime.length > 0 ? (
        <div>
          <p className="mb-2 text-xs font-medium text-fg-muted">{labels.overTime}</p>
          <div style={{ height: 240 }}>
            <ResponsiveContainer width="100%" height="100%">
              <ComposedChart data={overTime} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
                <XAxis
                  dataKey="day"
                  stroke="hsl(var(--fg-muted))"
                  tick={{ fill: 'hsl(var(--fg-muted))', fontSize: 11 }}
                  tickFormatter={(v) => String(v).slice(5)}
                  interval="preserveStartEnd"
                />
                <YAxis
                  yAxisId="left"
                  stroke="hsl(var(--fg-muted))"
                  tick={{ fill: 'hsl(var(--fg-muted))', fontSize: 11 }}
                  allowDecimals={false}
                  width={32}
                />
                <YAxis
                  yAxisId="right"
                  orientation="right"
                  stroke="hsl(var(--fg-muted))"
                  tick={{ fill: 'hsl(var(--fg-muted))', fontSize: 11 }}
                  tickFormatter={(v) => fmtCost(Number(v))}
                  width={48}
                />
                <Tooltip
                  contentStyle={tooltipStyle}
                  itemStyle={tooltipItemStyle}
                  labelStyle={tooltipLabelStyle}
                  cursor={{ fill: 'hsl(var(--surface-2))', opacity: 0.4 }}
                  formatter={(value, name) =>
                    name === labels.cost ? fmtCost(Number(value)) : String(value)
                  }
                />
                <Legend
                  wrapperStyle={{ fontSize: 11, paddingTop: 4 }}
                  iconType="square"
                  iconSize={8}
                />
                <Bar
                  yAxisId="left"
                  dataKey="sessions"
                  name={labels.sessions}
                  fill="hsl(343 76% 68%)"
                  radius={[2, 2, 0, 0]}
                />
                <Line
                  yAxisId="right"
                  type="monotone"
                  dataKey="cost"
                  name={labels.cost}
                  stroke="hsl(35 88% 72%)"
                  strokeWidth={2}
                  dot={false}
                />
              </ComposedChart>
            </ResponsiveContainer>
          </div>
        </div>
      ) : null}

      {data.top_sessions.length > 0 ? (
        <div>
          <p className="mb-2 text-xs font-medium text-fg-muted">{labels.topSessions}</p>
          <ul className="flex flex-col divide-y divide-border">
            {data.top_sessions.map((s, i) => (
              <li key={s.id} className="flex items-center gap-3 py-1.5 text-sm">
                <span className="w-5 shrink-0 text-right text-xs text-fg-muted tabular-nums">
                  {i + 1}
                </span>
                <span className="w-16 shrink-0 text-right font-mono text-xs text-fg tabular-nums">
                  {fmtCost(s.usage.cost_usd)}
                </span>
                <span className="shrink-0 rounded bg-surface-2 px-1.5 py-0.5 text-xs text-fg-muted">
                  {s.project}
                </span>
                <a
                  href={`/cc-sessions/${encodeURIComponent(s.project_folder)}/${s.id}`}
                  className="min-w-0 flex-1 truncate text-fg-muted hover:text-accent"
                  title={s.first_prompt}
                >
                  {s.first_prompt || labels.untitled}
                </a>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {data.has_unknown_model ? (
        <p className="text-xs text-fg-muted">{labels.costEstimateNote}</p>
      ) : null}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <div>
      <div className="text-xs text-fg-muted">{label}</div>
      <div className="text-lg font-semibold text-fg tabular-nums">{value}</div>
    </div>
  );
}
