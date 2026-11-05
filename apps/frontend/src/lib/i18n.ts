import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import en from '../locales/en.json';
import es from '../locales/es.json';
import { DEFAULT_LANG, SUPPORTED_LANGS, detectIslandLang } from './island-lang.ts';

export { SUPPORTED_LANGS, type SupportedLang } from './island-lang.ts';

// Islands follow the server-rendered language (<html lang> → "lang" cookie →
// server default). No browser language detector: a localStorage or navigator
// guess could disagree with the page the server already rendered (S7).
void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    es: { translation: es },
  },
  lng: detectIslandLang(),
  fallbackLng: DEFAULT_LANG,
  supportedLngs: SUPPORTED_LANGS,
  interpolation: {
    escapeValue: false,
  },
});

export default i18n;
