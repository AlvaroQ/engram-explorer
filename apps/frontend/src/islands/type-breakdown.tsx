/**
 * Island: type-breakdown
 *
 * Renders a donut PieChart (recharts) showing observation counts by type.
 * Exported mount() is the standard island contract — called by loader.ts.
 *
 * Props are passed via JSON in the data-props attribute of the host div:
 *   data-props='{"by_type":[{"type":"decision","count":42},...]}'
 */
import { createRoot } from 'react-dom/client';
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts';

export interface TypeBreakdownEntry {
  type: string;
  count: number;
}

export interface TypeBreakdownProps {
  by_type: TypeBreakdownEntry[];
}

const PIE_COLORS = [
  'hsl(224 90% 70%)',
  'hsl(142 76% 60%)',
  'hsl(38 92% 60%)',
  'hsl(0 84% 67%)',
  'hsl(280 80% 70%)',
  'hsl(190 80% 60%)',
  'hsl(330 70% 65%)',
  'hsl(60 80% 60%)',
];

const MAX_VISIBLE = 6;

function TypeBreakdownChart({ by_type }: TypeBreakdownProps) {
  const sorted = [...by_type].sort((a, b) => b.count - a.count);
  const visible = sorted.slice(0, MAX_VISIBLE);
  const rest = sorted.slice(MAX_VISIBLE);

  const slices: Array<{ type: string; count: number; isOthers?: boolean }> =
    rest.length > 0
      ? [...visible, { type: 'Others', count: rest.reduce((s, d) => s + d.count, 0), isOthers: true }]
      : visible;

  const total = slices.reduce((sum, d) => sum + d.count, 0);

  // Column-first ordering for legend
  const half = Math.ceil(slices.length / 2);
  const legend: Array<{ item: (typeof slices)[number]; colorIdx: number }> = [];
  for (let i = 0; i < half; i++) {
    const left = slices[i];
    if (left) legend.push({ item: left, colorIdx: i });
    const right = slices[i + half];
    if (right) legend.push({ item: right, colorIdx: i + half });
  }

  if (slices.length === 0) {
    return <p style={{ fontSize: 14, color: 'hsl(var(--fg-muted, 215 20% 55%))' }}>No data</p>;
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12, height: '100%' }}>
      <div style={{ flex: 1, minHeight: 0 }}>
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={slices}
              dataKey="count"
              nameKey="type"
              innerRadius="55%"
              outerRadius="90%"
              paddingAngle={2}
              stroke="hsl(var(--surface, 222 20% 13%))"
              strokeWidth={2}
            >
              {slices.map((_, i) => (
                <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
              ))}
            </Pie>
            <Tooltip
              formatter={(value: number, name: string) => {
                const pct = total > 0 ? ((value / total) * 100).toFixed(1) : '0';
                return [`${value} (${pct}%)`, name];
              }}
              contentStyle={{
                backgroundColor: 'hsl(var(--surface-2, 222 20% 18%))',
                border: '1px solid hsl(var(--border, 222 20% 25%))',
                borderRadius: 8,
                color: 'hsl(var(--fg, 210 20% 90%))',
                fontSize: 12,
              }}
              itemStyle={{ color: 'hsl(var(--fg, 210 20% 90%))' }}
              labelStyle={{ color: 'hsl(var(--fg, 210 20% 90%))' }}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      <ul
        style={{
          marginTop: 'auto',
          display: 'grid',
          gridTemplateColumns: '1fr 1fr',
          columnGap: 16,
          rowGap: 4,
          fontSize: 12,
          listStyle: 'none',
          padding: 0,
          margin: 0,
        }}
      >
        {legend.map(({ item, colorIdx }) => (
          <li key={item.type} style={{ display: 'flex', alignItems: 'center', gap: 8 }} title={item.type}>
            <span
              style={{
                width: 8,
                height: 8,
                borderRadius: 2,
                flexShrink: 0,
                backgroundColor: PIE_COLORS[colorIdx % PIE_COLORS.length],
              }}
            />
            <span
              style={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                color: item.isOthers
                  ? 'hsl(var(--fg-muted, 215 20% 55%))'
                  : 'hsl(var(--fg, 210 20% 90%))',
                fontStyle: item.isOthers ? 'italic' : undefined,
              }}
            >
              {item.type}
            </span>
            <span
              style={{
                marginLeft: 'auto',
                fontFamily: 'monospace',
                color: 'hsl(var(--fg-muted, 215 20% 55%))',
              }}
            >
              {item.count}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Island contract: called by loader.ts with the host element and parsed props.
 * Creates a React root inside el and renders the chart.
 */
export function mount(el: HTMLElement, props: TypeBreakdownProps): void {
  createRoot(el).render(<TypeBreakdownChart {...props} />);
}
