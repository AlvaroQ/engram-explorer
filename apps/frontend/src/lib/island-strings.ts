// Lightweight es/en catalog for the small chart islands (type-breakdown,
// project-activity-chart) that intentionally do not bundle i18next. The
// language comes from the island language bridge, so these labels always
// match the server-rendered page (S6, S7).
import { detectIslandLang, type SupportedLang } from './island-lang.ts';

export const islandStringsEn = {
  noData: 'No data',
  others: 'Others',
  noActivity30d: 'No activity in the last 30 days.',
} as const;

export type IslandStringKey = keyof typeof islandStringsEn;

export const islandStringsEs: Record<IslandStringKey, string> = {
  noData: 'Sin datos',
  others: 'Otros',
  noActivity30d: 'Sin actividad en los últimos 30 días.',
};

const catalogs: Record<SupportedLang, Record<IslandStringKey, string>> = {
  en: islandStringsEn,
  es: islandStringsEs,
};

export function islandT(key: IslandStringKey, lang: SupportedLang = detectIslandLang()): string {
  return catalogs[lang][key];
}
