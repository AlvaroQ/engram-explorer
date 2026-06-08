/**
 * Island: project-activity-chart
 *
 * Renders a BarChart (recharts) showing daily observation counts for a single
 * project over the last 30 days. Exported mount() is the standard island
 * contract — called by loader.ts.
 *
 * Props are passed via JSON in the data-props attribute of the host div:
 *   data-props='{"activity_30d":[{"day":"2024-01-01","count":3},...]}'
 */
import { createRoot } from 'react-dom/client';
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

export interface ActivityDay {
  day: string;
  count: number;
}

export interface ProjectActivityChartProps {
  activity_30d: ActivityDay[];
}

function ActivityChart({ activity_30d }: ProjectActivityChartProps) {
  if (activity_30d.length === 0) {
    return (
      <p style={{ fontSize: 14, color: 'hsl(var(--fg-muted, 215 20% 55%))' }}>
        No activity in the last 30 days.
      </p>
    );
  }

  return (
    <div style={{ width: '100%', height: 260 }}>
      <ResponsiveContainer>
        <BarChart data={activity_30d} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid
            strokeDasharray="3 3"
            stroke="hsl(var(--border, 222 20% 25%))"
            vertical={false}
          />
          <XAxis
            dataKey="day"
            stroke="hsl(var(--fg-muted, 215 20% 55%))"
            tick={{ fill: 'hsl(var(--fg-muted, 215 20% 55%))', fontSize: 11 }}
            tickFormatter={(v: string) => v.slice(5)}
            interval="preserveStartEnd"
          />
          <YAxis
            stroke="hsl(var(--fg-muted, 215 20% 55%))"
            tick={{ fill: 'hsl(var(--fg-muted, 215 20% 55%))', fontSize: 11 }}
            allowDecimals={false}
            width={32}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: 'hsl(var(--surface-2, 222 20% 18%))',
              border: '1px solid hsl(var(--border, 222 20% 25%))',
              borderRadius: 8,
              color: 'hsl(var(--fg, 210 20% 90%))',
              fontSize: 12,
            }}
            cursor={{ fill: 'hsl(var(--surface-2, 222 20% 18%))', opacity: 0.4 }}
          />
          <Bar dataKey="count" fill="hsl(var(--accent, 224 90% 70%))" radius={[4, 4, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

/**
 * Island contract: called by loader.ts with the host element and parsed props.
 * Creates a React root inside el and renders the bar chart.
 */
export function mount(el: HTMLElement, props: ProjectActivityChartProps): void {
  createRoot(el).render(<ActivityChart {...props} />);
}
