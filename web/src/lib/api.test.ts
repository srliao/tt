import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, api } from './api';

function jsonResponse(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('api()', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('returns parsed JSON for 2xx', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ id: 1, title: 'hello' }, 200));
    vi.stubGlobal('fetch', fetchMock);

    const result = await api<{ id: number; title: string }>('/tasks');

    expect(result).toEqual({ id: 1, title: 'hello' });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/tasks',
      expect.objectContaining({
        headers: expect.any(Headers),
      }),
    );
  });

  it('throws ApiError with .code on 400 validation envelope', async () => {
    const envelope = {
      error: {
        code: 'validation_failed',
        message: 'title is required',
        details: { field: 'title' },
      },
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(jsonResponse(envelope, 400)));

    await expect(api('/tasks', { method: 'POST', body: '{}' })).rejects.toMatchObject({
      name: 'ApiError',
      code: 'validation_failed',
      message: 'title is required',
      status: 400,
      details: { field: 'title' },
    });
  });

  it('resolves to undefined on 204 No Content', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 })));

    const result = await api<void>('/tasks/1', { method: 'DELETE' });

    expect(result).toBeUndefined();
  });

  it('throws ApiError with http_error code when body is not JSON', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce(new Response('plain text', { status: 500 })),
    );

    const err = (await api('/tasks').catch((e: unknown) => e)) as ApiError;

    expect(err).toBeInstanceOf(ApiError);
    expect(err.code).toBe('http_error');
    expect(err.status).toBe(500);
  });
});
