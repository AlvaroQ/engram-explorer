import { describe, expect, it } from 'vitest';
import type { AgentDayStat, AgentGroupStat, AgentUsageDTO } from './api.ts';
import {
  agentSummaryText,
  costText,
  formatCost,
  formatTokens,
  isUnpriced,
  notPricedHint,
  overTimeRows,
  projectBars,
  unpricedModels,
  usageMetric,
} from './agent-usage-format.ts';

const NBSP = '\u00a0';

function usage(tokens: number, cost: number, unknown: boolean): AgentUsageDTO {
  return {
    input_tokens: tokens,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    output_tokens: 0,
    reasoning_tokens: 0,
    cost_usd: cost,
    unknown_model: unknown,
  };
}

function group(key: string, tokens: number, cost: number, unknown = false): AgentGroupStat {
  return { key, sessions: 1, usage: usage(tokens, cost, unknown) };
}

describe('isUnpriced', () => {
  it('is true only when every model lacks a price (cost 0 + unknown model)', () => {
    expect(isUnpriced(usage(500, 0, true))).toBe(true);
    expect(isUnpriced(usage(500, 1.2, true))).toBe(false); // partially priced
    expect(isUnpriced(usage(500, 1.2, false))).toBe(false);
    expect(isUnpriced(usage(0, 0, false))).toBe(false); // nothing spent
  });
});

describe('usageMetric', () => {
  it('keeps cost charts when the scope has any priced cost (Claude-only, mixed)', () => {
    expect(usageMetric(usage(1000, 6613.07, false))).toBe('cost');
    expect(usageMetric(usage(1000, 12.5, true))).toBe('cost');
  });
  it('switches to tokens when nothing in the scope is priced (Codex on unknown models)', () => {
    expect(usageMetric(usage(1000, 0, true))).toBe('tokens');
    expect(usageMetric(usage(1000, 0, false))).toBe('tokens');
  });
});

describe('formatTokens', () => {
  it('formats with locale separators and k/M units', () => {
    expect(formatTokens(0, 'en')).toBe('0');
    expect(formatTokens(950, 'en')).toBe('950');
    expect(formatTokens(12_345, 'en')).toBe('12.3k');
    expect(formatTokens(8_060_450_000, 'en')).toBe('8,060.45M');
    expect(formatTokens(12_345, 'es')).toBe('12,3k');
    expect(formatTokens(18_060_450_000, 'es')).toBe('18.060,45M');
  });
});

describe('formatCost and costText', () => {
  it('formats USD per locale', () => {
    expect(formatCost(6613.07, 'en')).toBe('$6,613.07');
    expect(formatCost(16613.07, 'es')).toBe(`16.613,07${NBSP}$`);
    expect(formatCost(0.004, 'en')).toBe('<$0.01');
  });
  it('reads "Not priced" for unpriced usage, "~" for a lower bound, plain amount otherwise', () => {
    expect(costText(usage(500, 0, true), 'en', 'Not priced')).toBe('Not priced');
    expect(costText(usage(500, 0, true), 'es', 'Sin precio')).toBe('Sin precio');
    expect(costText(usage(500, 1.5, true), 'en', 'Not priced')).toBe('$1.50~');
    expect(costText(usage(500, 1.5, false), 'en', 'Not priced')).toBe('$1.50');
  });
});

describe('unpricedModels and notPricedHint', () => {
  const byModel = [
    group('claude-opus-4-8', 10, 3),
    group('gpt-5.6-sol', 10, 0, true),
    group('gpt-5.6-terra', 10, 0, true),
  ];
  it('lists the models without a known price', () => {
    expect(unpricedModels(byModel)).toEqual(['gpt-5.6-sol', 'gpt-5.6-terra']);
  });
  it('names up to three models in the hint, with a fallback when none is known', () => {
    const tpl = 'No price is known for these models ({{models}}).';
    expect(notPricedHint(tpl, 'fallback', ['gpt-5.6-sol', 'gpt-5.6-terra'])).toBe(
      'No price is known for these models (gpt-5.6-sol, gpt-5.6-terra).',
    );
    expect(notPricedHint(tpl, 'fallback', ['a', 'b', 'c', 'd'])).toBe(
      'No price is known for these models (a, b, c, …).',
    );
    expect(notPricedHint(tpl, 'fallback', [])).toBe('fallback');
  });
});

describe('agentSummaryText', () => {
  it('labels sessions, tokens and cost (en)', () => {
    const tpl = '{{sessions}} sessions · {{tokens}} tokens · {{cost}}';
    expect(
      agentSummaryText(tpl, 1586, usage(8_060_450_000, 6613.07, false), 'en', 'Not priced'),
    ).toBe('1,586 sessions · 8,060.45M tokens · $6,613.07');
  });
  it('labels sessions, tokens and cost (es), "Sin precio" when unpriced', () => {
    const tpl = '{{sessions}} sesiones · {{tokens}} tokens · {{cost}}';
    expect(agentSummaryText(tpl, 95, usage(312_400_000, 0, true), 'es', 'Sin precio')).toBe(
      '95 sesiones · 312,40M tokens · Sin precio',
    );
  });
});

describe('projectBars', () => {
  const byProject = [group('a', 100, 0, true), group('b', 900, 0, true), group('c', 50, 2)];
  it('ranks by cost in cost mode', () => {
    expect(projectBars(byProject, 'cost', 'Others', 8)).toEqual([
      { name: 'c', value: 2 },
      { name: 'a', value: 0 },
      { name: 'b', value: 0 },
    ]);
  });
  it('ranks by total tokens in tokens mode and folds the tail into Others', () => {
    expect(projectBars(byProject, 'tokens', 'Others', 2)).toEqual([
      { name: 'b', value: 900 },
      { name: 'a', value: 100 },
      { name: 'Others (1)', value: 50 },
    ]);
  });
});

describe('overTimeRows', () => {
  const days: AgentDayStat[] = [
    {
      day: '2026-11-01',
      sessions: 3,
      usage: usage(700, 0, true),
      by_agent: { codex: usage(700, 0, true) },
    },
  ];
  it('plots cost per agent in cost mode and tokens per agent in tokens mode', () => {
    expect(overTimeRows(days, ['codex'], 'cost')).toEqual([
      { day: '2026-11-01', sessions: 3, codex: 0 },
    ]);
    expect(overTimeRows(days, ['codex'], 'tokens')).toEqual([
      { day: '2026-11-01', sessions: 3, codex: 700 },
    ]);
  });
});
