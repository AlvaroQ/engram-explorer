import { describe, expect, it } from 'vitest';
import { islandModuleUrl } from './island-url.ts';

describe('islandModuleUrl', () => {
  it('maps a valid island name to its stable bundle path', () => {
    expect(islandModuleUrl('type-breakdown')).toBe('/islands/type-breakdown.js');
    expect(islandModuleUrl('brain-preview-hero')).toBe('/islands/brain-preview-hero.js');
    expect(islandModuleUrl('chart2')).toBe('/islands/chart2.js');
  });

  it('rejects names outside ^[a-z0-9-]+$ (no path, scheme, query or case tricks)', () => {
    for (const name of [
      '',
      '../evil',
      'a/b',
      'https://evil.example/x',
      '//evil.example/x',
      'Brain',
      'brain.js',
      'brain?x=1',
      'brain#x',
      'brain ',
      'bra\\in',
      'brain\n',
    ]) {
      expect(islandModuleUrl(name), JSON.stringify(name)).toBeNull();
    }
  });
});
