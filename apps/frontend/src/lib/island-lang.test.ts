import { describe, expect, it } from 'vitest';
import { DEFAULT_LANG, resolveIslandLang } from './island-lang.ts';

// The server (internal/ui/i18n.go langForRequest) resolves the display
// language as cookie "lang" → Accept-Language → "en" and renders it into
// <html lang>. Islands must follow that decision, never their own detector.

describe('resolveIslandLang', () => {
  it('uses the server-rendered <html lang> first', () => {
    expect(resolveIslandLang({ htmlLang: 'es', cookie: 'lang=en' })).toBe('es');
    expect(resolveIslandLang({ htmlLang: 'en', cookie: 'lang=es' })).toBe('en');
  });

  it('normalizes region tags and case from <html lang>', () => {
    expect(resolveIslandLang({ htmlLang: 'es-AR', cookie: '' })).toBe('es');
    expect(resolveIslandLang({ htmlLang: 'EN', cookie: '' })).toBe('en');
  });

  it('falls back to the lang cookie when <html lang> is missing or unsupported', () => {
    expect(resolveIslandLang({ htmlLang: '', cookie: 'theme=dark; lang=es' })).toBe('es');
    expect(resolveIslandLang({ htmlLang: 'fr', cookie: 'lang=es; sidebar=collapsed' })).toBe('es');
  });

  it('ignores unsupported cookie values', () => {
    expect(resolveIslandLang({ htmlLang: '', cookie: 'lang=fr' })).toBe('en');
    expect(resolveIslandLang({ htmlLang: '', cookie: 'xlang=es' })).toBe('en');
  });

  it('falls back to the server default "en" when nothing is set', () => {
    expect(DEFAULT_LANG).toBe('en');
    expect(resolveIslandLang({ htmlLang: '', cookie: '' })).toBe('en');
    expect(resolveIslandLang({})).toBe('en');
  });
});
