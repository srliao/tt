import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  buildRunsQuery,
  buildSpawnedTasksQuery,
  useCreateScript,
  useRunScript,
  useScripts,
} from './scripts';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function wrap() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, Wrapper };
}

describe('buildRunsQuery / buildSpawnedTasksQuery', () => {
  it('serialises limit + before', () => {
    expect(buildRunsQuery({ limit: 20, before: '2026-05-21T00:00:00Z' })).toBe(
      '?limit=20&before=2026-05-21T00%3A00%3A00Z',
    );
  });
  it('returns "" when empty', () => {
    expect(buildRunsQuery({})).toBe('');
    expect(buildSpawnedTasksQuery({})).toBe('');
  });
});

describe('useScripts', () => {
  afterEach(() => vi.restoreAllMocks());
  it('GETs /scripts and returns the array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse([{ id: 1, name: 'A' }]));
    vi.stubGlobal('fetch', fetchMock);
    const { Wrapper } = wrap();
    const { result } = renderHook(() => useScripts(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/scripts', expect.any(Object));
    expect(result.current.data).toEqual([{ id: 1, name: 'A' }]);
  });
});

describe('useCreateScript', () => {
  afterEach(() => vi.restoreAllMocks());
  it('POSTs JSON and invalidates the scripts cache on success', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ id: 9 }, 201));
    vi.stubGlobal('fetch', fetchMock);
    const { qc, Wrapper } = wrap();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');
    const { result } = renderHook(() => useCreateScript(), { wrapper: Wrapper });
    await act(async () => {
      await result.current.mutateAsync({
        name: 'Weekly',
        code: 'ctx.queueTask({title:"x"})',
        enabled: true,
        schedule: { kind: 'daily' },
      });
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/scripts',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['scripts'] });
  });
});

describe('useRunScript', () => {
  afterEach(() => vi.restoreAllMocks());
  it('POSTs /scripts/:id/run and returns the run_id', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ run_id: 42 }));
    vi.stubGlobal('fetch', fetchMock);
    const { Wrapper } = wrap();
    const { result } = renderHook(() => useRunScript(7), { wrapper: Wrapper });
    let response: { run_id: number } | undefined;
    await act(async () => {
      response = await result.current.mutateAsync();
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/scripts/7/run',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(response?.run_id).toBe(42);
  });
});
