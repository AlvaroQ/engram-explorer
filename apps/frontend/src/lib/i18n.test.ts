import { afterEach, describe, expect, it, vi } from 'vitest';

// The islands i18n instance must start in the language the server rendered
// into <html lang>, even when a stale localStorage "engram.lang" says otherwise.

const g = globalThis as Record<string, unknown>;

afterEach(() => {
  vi.resetModules();
  delete g['document'];
  delete g['localStorage'];
});

function stubBrowser(htmlLang: string, cookie: string, stored: string | null): void {
  g['document'] = { documentElement: { lang: htmlLang }, cookie };
  g['localStorage'] = {
    getItem: (k: string) => (k === 'engram.lang' ? stored : null),
    setItem: () => undefined,
    removeItem: () => undefined,
  };
}

describe('islands i18n instance', () => {
  it('follows <html lang> over a stale localStorage value', async () => {
    stubBrowser('es', '', 'en');
    const { default: i18n } = await import('./i18n.ts');
    expect(i18n.language).toBe('es');
  });

  it('follows <html lang="en"> over a stale localStorage "es"', async () => {
    stubBrowser('en', 'lang=en', 'es');
    const { default: i18n } = await import('./i18n.ts');
    expect(i18n.language).toBe('en');
  });

  it('falls back to the server default "en" without a document', async () => {
    const { default: i18n } = await import('./i18n.ts');
    expect(i18n.language).toBe('en');
  });
});
