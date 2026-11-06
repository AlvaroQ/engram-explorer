import { describe, expect, it } from 'vitest';
import en from './en.json';
import es from './es.json';

// Flattens a catalog into dotted keys ("brain.loading.label") with their
// values; anything that is neither an object nor a string is kept as-is so
// the type check below reports it.
function flatten(
  tree: unknown,
  prefix = '',
  out = new Map<string, unknown>(),
): Map<string, unknown> {
  if (tree !== null && typeof tree === 'object' && !Array.isArray(tree)) {
    for (const [k, v] of Object.entries(tree)) flatten(v, prefix ? `${prefix}.${k}` : k, out);
  } else {
    out.set(prefix, tree);
  }
  return out;
}

describe('island locale catalogs', () => {
  const flatEn = flatten(en);
  const flatEs = flatten(es);

  it('define the same keys in en and es', () => {
    const missingInEs = [...flatEn.keys()].filter((k) => !flatEs.has(k)).sort();
    const missingInEn = [...flatEs.keys()].filter((k) => !flatEn.has(k)).sort();
    expect(missingInEs).toEqual([]);
    expect(missingInEn).toEqual([]);
    expect(flatEn.size).toBeGreaterThan(0);
  });

  it('have a non-empty string for every key', () => {
    const bad: string[] = [];
    for (const [lang, flat] of [
      ['en', flatEn],
      ['es', flatEs],
    ] as const) {
      for (const [k, v] of flat) {
        if (typeof v !== 'string' || v.trim() === '') bad.push(`${lang}: ${k}`);
      }
    }
    expect(bad).toEqual([]);
  });
});
