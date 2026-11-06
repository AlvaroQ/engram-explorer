import { describe, expect, it } from 'vitest';
import { islandStringsEn, islandStringsEs, islandT } from './island-strings.ts';

describe('islandT', () => {
  it('returns the Spanish label when the page language is es', () => {
    expect(islandT('noData', 'es')).toBe('Sin datos');
    expect(islandT('others', 'es')).toBe('Otros');
    expect(islandT('noActivity30d', 'es')).toBe('Sin actividad en los últimos 30 días.');
  });

  it('returns the English label when the page language is en', () => {
    expect(islandT('noData', 'en')).toBe('No data');
    expect(islandT('others', 'en')).toBe('Others');
    expect(islandT('noActivity30d', 'en')).toBe('No activity in the last 30 days.');
  });

  it('defaults to the server-rendered language (en outside a browser)', () => {
    expect(islandT('noData')).toBe('No data');
  });

  it('keeps es and en catalogs in sync', () => {
    expect(Object.keys(islandStringsEs).sort()).toEqual(Object.keys(islandStringsEn).sort());
    for (const value of Object.values(islandStringsEs)) expect(value).not.toBe('');
  });
});
