import { describe, expect, it } from 'vitest';
import { friendlyModelName } from './model-labels.ts';

describe('friendlyModelName', () => {
  it('names current Claude model ids (family-major-minor-date)', () => {
    expect(friendlyModelName('claude-opus-4-5-20251101')).toBe('Claude Opus 4.5');
    expect(friendlyModelName('claude-sonnet-4-5-20250929')).toBe('Claude Sonnet 4.5');
    expect(friendlyModelName('claude-haiku-4-5')).toBe('Claude Haiku 4.5');
    expect(friendlyModelName('claude-opus-4-1-20250805')).toBe('Claude Opus 4.1');
  });

  it('does not mistake the date stamp for a minor version', () => {
    expect(friendlyModelName('claude-sonnet-4-20250514')).toBe('Claude Sonnet 4');
    expect(friendlyModelName('claude-opus-4')).toBe('Claude Opus 4');
  });

  it('names legacy Claude 3.x ids (version before family)', () => {
    expect(friendlyModelName('claude-3-5-sonnet-20241022')).toBe('Claude 3.5 Sonnet');
    expect(friendlyModelName('claude-3-7-sonnet-20250219')).toBe('Claude 3.7 Sonnet');
    expect(friendlyModelName('claude-3-haiku-20240307')).toBe('Claude 3 Haiku');
  });

  it('names Codex / OpenAI GPT ids', () => {
    expect(friendlyModelName('gpt-5-codex')).toBe('GPT-5 Codex');
    expect(friendlyModelName('gpt-5.1-codex-max')).toBe('GPT-5.1 Codex Max');
    expect(friendlyModelName('gpt-5')).toBe('GPT-5');
    expect(friendlyModelName('gpt-5-mini')).toBe('GPT-5 mini');
    expect(friendlyModelName('gpt-4o-2024-08-06')).toBe('GPT-4o');
    expect(friendlyModelName('gpt-4.1')).toBe('GPT-4.1');
  });

  it('is case-insensitive and trims whitespace', () => {
    expect(friendlyModelName('  Claude-Opus-4-5  ')).toBe('Claude Opus 4.5');
    expect(friendlyModelName('GPT-5-Codex')).toBe('GPT-5 Codex');
  });

  it('passes unknown ids through unchanged', () => {
    expect(friendlyModelName('o3')).toBe('o3');
    expect(friendlyModelName('<synthetic>')).toBe('<synthetic>');
    expect(friendlyModelName('some-local-model')).toBe('some-local-model');
    expect(friendlyModelName('')).toBe('');
  });
});
