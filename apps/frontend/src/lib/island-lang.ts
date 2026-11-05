// Island language bridge (S7).
//
// The Go server owns the display language: internal/ui/i18n.go
// langForRequest resolves cookie "lang" → Accept-Language → "en" and renders
// the result into <html lang>. Islands must follow that decision so a page
// never mixes languages. They read <html lang> first, then the "lang" cookie
// (same cookie the server sets via POST /lang), and finally fall back to the
// server default. No localStorage or navigator detection: those can diverge
// from what the server rendered.

export const SUPPORTED_LANGS = ['es', 'en'] as const;
export type SupportedLang = (typeof SUPPORTED_LANGS)[number];

/** Must match the server fallback in internal/ui/i18n.go langForRequest. */
export const DEFAULT_LANG: SupportedLang = 'en';

export interface LangSources {
  /** document.documentElement.lang */
  htmlLang?: string | null;
  /** document.cookie */
  cookie?: string | null;
}

function normalize(value: string | null | undefined): SupportedLang | null {
  if (!value) return null;
  const code = value.trim().toLowerCase().split('-')[0] ?? '';
  return (SUPPORTED_LANGS as readonly string[]).includes(code) ? (code as SupportedLang) : null;
}

function cookieLang(cookie: string | null | undefined): SupportedLang | null {
  if (!cookie) return null;
  for (const part of cookie.split(';')) {
    const [name, ...rest] = part.trim().split('=');
    if (name === 'lang') return normalize(rest.join('='));
  }
  return null;
}

export function resolveIslandLang(sources: LangSources): SupportedLang {
  return normalize(sources.htmlLang) ?? cookieLang(sources.cookie) ?? DEFAULT_LANG;
}

/** Reads the live document; safe outside the browser (returns the default). */
export function detectIslandLang(): SupportedLang {
  if (typeof document === 'undefined') return DEFAULT_LANG;
  return resolveIslandLang({
    htmlLang: document.documentElement.lang,
    cookie: document.cookie,
  });
}
