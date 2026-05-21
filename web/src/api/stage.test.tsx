import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useClearFinishedFromStage, useClearStage, useReorderStage, useStagedTasks } from './stage';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const w = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, wrapper: w };
}

describe('useStagedTasks', () => {
  afterEach(() => vi.restoreAllMocks());

  it('filters to staged tasks and sorts by staged_order ascending', async () => {
    const tasks = [
      { id: 1, title: 'a', staged_order: 2.0 },
      { id: 2, title: 'b', staged_order: null },
      { id: 3, title: 'c', staged_order: 1.0 },
    ];
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(jsonResponse(tasks)));

    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useStagedTasks(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.map((t) => t.id)).toEqual([3, 1]);
  });
});

describe('useReorderStage', () => {
  afterEach(() => vi.restoreAllMocks());

  it('POSTs to /stage/reorder and invalidates [tasks] on success', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ id: 5 }));
    vi.stubGlobal('fetch', fetchMock);

    const { qc, wrapper } = makeWrapper();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');
    const { result } = renderHook(() => useReorderStage(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ task_id: 5, before_id: 1, after_id: null });
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/stage/reorder',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ task_id: 5, before_id: 1, after_id: null }),
      }),
    );
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['tasks'] });
  });
});

describe('useClearStage', () => {
  afterEach(() => vi.restoreAllMocks());

  it('DELETEs /stage and invalidates [tasks]', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    const { qc, wrapper } = makeWrapper();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');
    const { result } = renderHook(() => useClearStage(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/stage',
      expect.objectContaining({ method: 'DELETE' }),
    );
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['tasks'] });
  });
});

describe('useClearFinishedFromStage', () => {
  afterEach(() => vi.restoreAllMocks());

  it('DELETEs /stage/finished and invalidates [tasks]', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    const { qc, wrapper } = makeWrapper();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');
    const { result } = renderHook(() => useClearFinishedFromStage(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/stage/finished',
      expect.objectContaining({ method: 'DELETE' }),
    );
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['tasks'] });
  });
});
