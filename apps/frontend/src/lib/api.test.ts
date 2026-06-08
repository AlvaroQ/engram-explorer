import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { api } from './api.ts';

// Regression guard for the singular/plural mismatch that caused a 405 on
// move-project and delete: the backend write routes are PLURAL
// (/api/observations|sessions|prompts/...), so the client must pluralize the
// (singular) entity argument. A singular path falls through to the catch-all
// `GET /` and Go's mux answers 405 Method Not Allowed.
describe('api client write routes hit the plural backend paths', () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockReset();
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({}) } as Response);
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const lastCall = () => {
    const call = fetchMock.mock.calls.at(-1);
    return { url: String(call?.[0]), init: (call?.[1] ?? {}) as RequestInit };
  };

  const entities = [
    ['observation', 'observations'],
    ['session', 'sessions'],
    ['prompt', 'prompts'],
  ] as const;

  it('assignProject pluralizes the entity for all three kinds', async () => {
    for (const [entity, plural] of entities) {
      fetchMock.mockClear();
      await api.assignProject(entity, 42, 'ai-ready-kmp');
      const { url, init } = lastCall();
      expect(url).toBe(`/api/${plural}/42/project`);
      expect(init.method).toBe('PATCH');
    }
  });

  it('deleteEntity pluralizes the entity for all three kinds', async () => {
    for (const [entity, plural] of entities) {
      fetchMock.mockClear();
      await api.deleteEntity(entity, 7);
      const { url, init } = lastCall();
      expect(url).toBe(`/api/${plural}/7`);
      expect(init.method).toBe('DELETE');
    }
  });

  it('renameProject posts to the plural projects rename route', async () => {
    await api.renameProject('old-proj', { target: 'new-proj', mode: 'rename' });
    const { url, init } = lastCall();
    expect(url).toBe('/api/projects/old-proj/rename');
    expect(init.method).toBe('POST');
  });
});
