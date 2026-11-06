import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import { cssColorFromToken, readThemeColor } from './theme-colors.ts';

describe('cssColorFromToken', () => {
  it('turns a bare HSL triplet token into a comma hsl() THREE.Color can parse', () => {
    const css = cssColorFromToken('267 57% 78%', '#000000');
    expect(css).toBe('hsl(267, 57%, 78%)');
    const c = new THREE.Color(css);
    expect(c.getHexString()).toBe(new THREE.Color('hsl(267, 57%, 78%)').getHexString());
    expect(c.getHexString()).not.toBe('ffffff');
  });

  it('accepts decimals and surrounding whitespace', () => {
    expect(cssColorFromToken('  248 12.5% 42%  ', '#000')).toBe('hsl(248, 12.5%, 42%)');
  });

  it('passes full CSS colors through', () => {
    expect(cssColorFromToken('#c4a7e7', '#000')).toBe('#c4a7e7');
    expect(cssColorFromToken('rgb(1, 2, 3)', '#000')).toBe('rgb(1, 2, 3)');
  });

  it('falls back for empty or unparseable values', () => {
    expect(cssColorFromToken('', '#6e6a86')).toBe('#6e6a86');
    expect(cssColorFromToken('var(--x)', '#6e6a86')).toBe('#6e6a86');
  });
});

describe('readThemeColor', () => {
  it('reads the custom property through the provided reader', () => {
    const read = (name: string) => (name === '--info' ? '267 57% 78%' : '');
    expect(readThemeColor('--info', '#c4a7e7', read)).toBe('hsl(267, 57%, 78%)');
  });

  it('uses the fallback when the token is not defined', () => {
    expect(readThemeColor('--missing', '#c4a7e7', () => '')).toBe('#c4a7e7');
  });

  it('uses the fallback outside a browser', () => {
    expect(readThemeColor('--info', '#c4a7e7')).toBe('#c4a7e7');
  });
});
