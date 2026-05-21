import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { buildTaskListQuery, useCreateTask, useTasks } from './tasks';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe('buildTaskListQuery', () => {
  it('serialises repeated state and tag params', () => {
    const qs = buildTaskListQuery({
      states: ['not_done', 'done'],
      tags: ['work', 'urgent'],
      due: 'today',
      q: 'milk',
      sort: 'priority',
      asc: true,
    });
    expect(qs).toBe('?state=not_done&state=done&tag=work&tag=urgent&due=today&q=milk&sort=priority&asc=true');
  });

  it('returns empty string when no params', () => {
    expect(buildTaskListQuery({})).toBe('');
  });
});

describe('useTasks', () => {
  afterEach(() => vi.restoreAllMocks());

  it('fetches tasks and exposes the array', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse([{ id: 1, title: 'A' }]));
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useTasks({ states: ['not_done'] }), { wrapper: wrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([{ id: 1, title: 'A' }]);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/tasks?state=not_done',
      expect.any(Object),
    );
  });
});

describe('useCreateTask', () => {
  afterEach(() => vi.restoreAllMocks());

  it('POSTs JSON and invalidates the tasks cache on success', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ id: 9, title: 'New' }, 201));
    vi.stubGlobal('fetch', fetchMock);

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidate = vi.spyOn(qc, 'invalidateQueries');
    const w = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useCreateTask(), { wrapper: w });
    await act(async () => {
      await result.current.mutateAsync({ title: 'New' });
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/tasks',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['tasks'] });
  });
});
