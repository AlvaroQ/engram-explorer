// Pure helpers of the agent-usage-charts island: honest cost text for
// unpriced models, per-locale number formatting and the chart metric (cost
// when the scope has any priced cost, tokens otherwise).
//
// Cost is estimated server-side only for models with a known price; a usage
// whose models are all unpriced has cost 0 + unknown_model and must read
// "Not priced", never "$0".
import {
  agentUsageTotal,
  type AgentDayStat,
  type AgentGroupStat,
  type AgentUsageDTO,
} from './api.ts';

export type UsageMetric = 'cost' | 'tokens';

const HINT_MODELS = 3;

/** True when tokens were spent only on models without a known price. */
export function isUnpriced(u: AgentUsageDTO): boolean {
  return u.unknown_model && u.cost_usd <= 0;
}

/**
 * Metric of the cost-shaped charts for a scope: cost while anything in it is
 * priced (Claude-only and mixed scopes), tokens when nothing is (an all-zero
 * cost chart would draw no bars).
 */
export function usageMetric(totals: AgentUsageDTO): UsageMetric {
  return totals.cost_usd > 0 ? 'cost' : 'tokens';
}

function decimal(lang: string, digits: number, n: number): string {
  return new Intl.NumberFormat(lang, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(n);
}

/** Compact token count with locale separators: 950, 12.3k, 8,060.45M (en). */
export function formatTokens(n: number, lang: string): string {
  if (n <= 0) return '0';
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${decimal(lang, 1, n / 1000)}k`;
  return `${decimal(lang, 2, n / 1_000_000)}M`;
}

function usd(n: number, lang: string): string {
  return new Intl.NumberFormat(lang, {
    style: 'currency',
    currency: 'USD',
    currencyDisplay: 'narrowSymbol',
  }).format(n);
}

/** USD amount per locale ($6,613.07 in en, 6613,07 $ in es); tiny amounts read <$0.01. */
export function formatCost(n: number, lang: string): string {
  if (n > 0 && n < 0.01) return `<${usd(0.01, lang)}`;
  return usd(Math.max(n, 0), lang);
}

/** Cost of a usage: "Not priced", a "~" lower bound, or the plain amount. */
export function costText(u: AgentUsageDTO, lang: string, notPriced: string): string {
  if (isUnpriced(u)) return notPriced;
  return `${formatCost(u.cost_usd, lang)}${u.unknown_model ? '~' : ''}`;
}

/** Model ids without a known price (by_model entries flagged unknown). */
export function unpricedModels(byModel: AgentGroupStat[]): string[] {
  return byModel.filter((m) => m.usage.unknown_model).map((m) => m.key);
}

/** Tooltip of a "Not priced" value; {{models}} lists up to three ids. */
export function notPricedHint(template: string, fallback: string, models: string[]): string {
  if (models.length === 0) return fallback;
  const shown = models.slice(0, HINT_MODELS);
  if (models.length > HINT_MODELS) shown.push('…');
  return template.replace('{{models}}', shown.join(', '));
}

/** "586 sessions · 8,060.45M tokens · $6,613.07" from the localized template. */
export function agentSummaryText(
  template: string,
  sessions: number,
  u: AgentUsageDTO,
  lang: string,
  notPriced: string,
): string {
  return template
    .replace('{{sessions}}', new Intl.NumberFormat(lang).format(sessions))
    .replace('{{tokens}}', formatTokens(agentUsageTotal(u), lang))
    .replace('{{cost}}', costText(u, lang, notPriced));
}

function metricValue(u: AgentUsageDTO, metric: UsageMetric): number {
  return metric === 'cost' ? Number(u.cost_usd.toFixed(4)) : agentUsageTotal(u);
}

/** Top projects by the metric, the tail folded into "Others (n)". */
export function projectBars(
  byProject: AgentGroupStat[],
  metric: UsageMetric,
  others: string,
  topN: number,
): Array<{ name: string; value: number }> {
  const sorted = [...byProject].sort(
    (a, b) => metricValue(b.usage, metric) - metricValue(a.usage, metric),
  );
  const top = sorted
    .slice(0, topN)
    .map((p) => ({ name: p.key, value: metricValue(p.usage, metric) }));
  const tail = sorted.slice(topN);
  if (tail.length > 0) {
    const value = tail.reduce((s, p) => s + metricValue(p.usage, metric), 0);
    top.push({
      name: `${others} (${String(tail.length)})`,
      value: metric === 'cost' ? Number(value.toFixed(4)) : value,
    });
  }
  return top;
}

/** One row per day: the session count plus the metric of each agent. */
export function overTimeRows(
  overTime: AgentDayStat[],
  agentKeys: string[],
  metric: UsageMetric,
): Array<Record<string, string | number>> {
  return overTime.map((d) => {
    const row: Record<string, string | number> = { day: d.day, sessions: d.sessions };
    for (const a of agentKeys) {
      const u = d.by_agent[a];
      row[a] = u ? metricValue(u, metric) : 0;
    }
    return row;
  });
}
