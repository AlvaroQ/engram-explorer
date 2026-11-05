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
import {
  agentSessionDetailPath,
  agentUsageTotal,
  api,
  type AgentSourceError,
} from '../../lib/api.ts';
import { BrainLoadingOverlay } from '../brain/brain-loading-overlay.tsx';

/**
 * Localized labels passed in from the Go templ side (via @Island props) so the
 * island stays out of the frontend i18n bundle. Data is fetched on its own from
 * /api/agent-sessions/stats (server-cached), so the heavy scan never blocks render.
 */
export interface AgentUsageLabels {
  costByAgent: string;
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
  tokenNote: string;
  /** Contains {{source}} and {{agent}} placeholders. */
  sourceError: string;
}

const COLORS = [
  'hsl(343 76% 68%)', // love (pink)
  'hsl(189 43% 73%)', // foam
  'hsl(267 57% 78%)', // iris
  'hsl(35 88% 72%)', // gold
  'hsl(197 49% 38%)', // pine
  'hsl(2 55% 83%)', // rose
  'hsl(268 21% 57%)', // dawn iris
  'hsl(189 30% 48%)', // dawn foam
];
const FALLBACK_COLOR = 'hsl(248 15% 61%)'; // muted
const AGENT_COLORS: Record<string, string> = {
  'claude-code': 'hsl(343 76% 68%)', // love (pink)
  codex: 'hsl(189 43% 73%)', // foam
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

function agentColor(agent: string, i: number): string {
  return AGENT_COLORS[agent] ?? colorAt(i);
}

function sourceErrorText(template: string, e: AgentSourceError, agentName: string): string {
  return template.replace('{{source}}', e.source).replace('{{agent}}', agentName);
}

export function AgentUsageCharts({
  labels,
  agents,
}: {
  labels: AgentUsageLabels;
  agents: Record<string, string>;
}): JSX.Element {
  const query = useQuery({
    queryKey: ['agent-sessions-stats'],
    queryFn: () => api.agentSessionsStats(),
  });
  const data = query.data;
  const agentName = (a: string): string => agents[a] ?? a;

  const agentKeys = useMemo(() => (data ? data.by_agent.map((g) => g.key) : []), [data]);

  const agentBars = useMemo(() => {
    if (!data) return [] as Array<{ name: string; key: string; cost: number; tokens: number }>;
    return data.by_agent.map((g) => ({
      name: agents[g.key] ?? g.key,
      key: g.key,
      cost: Number(g.usage.cost_usd.toFixed(4)),
      tokens: agentUsageTotal(g.usage),
    }));
  }, [data, agents]);

  const projectBars = useMemo(() => {
    if (!data) return [] as Array<{ name: string; cost: number }>;
    const sorted = [...data.by_project].sort((a, b) => b.usage.cost_usd - a.usage.cost_usd);
    const top = sorted.slice(0, TOP_N).map((p) => ({
      name: p.key,
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
      .map((m) => ({ name: m.key, value: agentUsageTotal(m.usage) }))
      .filter((m) => m.value > 0);
  }, [data]);

  // One cost series per agent (stacked bars) plus the session count line.
  const overTime = useMemo(() => {
    if (!data) return [] as Array<Record<string, string | number>>;
    return data.over_time.map((d) => {
      const row: Record<string, string | number> = { day: d.day, sessions: d.sessions };
      for (const a of agentKeys) {
        row[a] = Number((d.by_agent[a]?.cost_usd ?? 0).toFixed(4));
      }
      return row;
    });
  }, [data, agentKeys]);

  // While the heavy stats scan runs, show the Engram elephant loading
  // animation (relative wrapper bounds the overlay's absolute inset-0 to this
  // card region instead of the whole page).
  if (query.isLoading) {
    return (
      <div className="relative h-60 overflow-hidden rounded-md">
        <BrainLoadingOverlay label={labels.loading} logoSrc="/static/engram.png" />
      </div>
    );
  }
  if (query.isError) return <p className="text-sm text-fail">{labels.error}</p>;
  const errors = (data?.errors ?? []).map((e) => (
    <p key={e.source} className="text-sm text-fail" role="alert">
      {sourceErrorText(labels.sourceError, e, agentName(e.agent))}
    </p>
  ));
  if (!data || data.sessions === 0) {
    return (
      <div className="flex flex-col gap-2">
        {errors}
        <p className="text-sm text-fg-muted">{labels.empty}</p>
      </div>
    );
  }

  const costSuffix = data.has_unknown_model ? '~' : '';

  return (
    <div className="flex flex-col gap-5">
      {errors}
      <div className="flex flex-wrap gap-8">
        <Stat label={labels.sessions} value={String(data.sessions)} />
        <Stat label={labels.tokens} value={fmtTokens(agentUsageTotal(data.totals))} />
        <Stat label={labels.cost} value={`${fmtCost(data.totals.cost_usd)}${costSuffix}`} />
        {data.by_agent.map((g) => (
          <Stat
            key={g.key}
            label={agentName(g.key)}
            value={`${String(g.sessions)} · ${fmtTokens(agentUsageTotal(g.usage))} · ${fmtCost(g.usage.cost_usd)}${g.usage.unknown_model ? '~' : ''}`}
          />
        ))}
      </div>

      {agentBars.length > 1 ? (
        <div>
          <p className="mb-2 text-xs font-medium text-fg-muted">{labels.costByAgent}</p>
          <div style={{ height: 40 + agentBars.length * 36 }}>
            <ResponsiveContainer width="100%" height="100%">
              <BarChart
                data={agentBars}
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
                  {agentBars.map((entry, i) => (
                    <Cell key={entry.key} fill={agentColor(entry.key, i)} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      ) : null}

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
                    <Cell key={entry.name} fill={colorAt(i)} />
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
                    name === labels.sessions ? String(value) : fmtCost(Number(value))
                  }
                />
                <Legend
                  wrapperStyle={{ fontSize: 11, paddingTop: 4 }}
                  iconType="square"
                  iconSize={8}
                />
                {agentKeys.map((a, i) => (
                  <Bar
                    key={a}
                    yAxisId="right"
                    dataKey={a}
                    name={agentName(a)}
                    stackId="cost"
                    fill={agentColor(a, i)}
                  />
                ))}
                <Line
                  yAxisId="left"
                  type="monotone"
                  dataKey="sessions"
                  name={labels.sessions}
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
              <li key={`${s.source}/${s.key}`} className="flex items-center gap-3 py-1.5 text-sm">
                <span className="w-5 shrink-0 text-right text-xs text-fg-muted tabular-nums">
                  {i + 1}
                </span>
                <span className="w-16 shrink-0 text-right font-mono text-xs text-fg tabular-nums">
                  {fmtCost(s.usage.cost_usd)}
                </span>
                <span className="shrink-0 rounded bg-surface-2 px-1.5 py-0.5 text-xs text-fg-muted">
                  {agentName(s.agent)}
                </span>
                <span className="shrink-0 rounded bg-surface-2 px-1.5 py-0.5 text-xs text-fg-muted">
                  {s.project}
                </span>
                <a
                  href={agentSessionDetailPath(s.source, s.key)}
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
      <p className="text-xs text-fg-muted">{labels.tokenNote}</p>
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
