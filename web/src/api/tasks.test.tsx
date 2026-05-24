import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/task';
import { buildTaskListQuery, useBulkTag, useCreateTask, useTasks } from './tasks';

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
  it('serialises repeated state params plus structured tag_filter', () => {
    const qs = buildTaskListQuery({
      states: ['not_done', 'done'],
      tag_filter: { mode: 'any', tags: ['work', 'urgent'] },
      due: 'today',
      q: 'milk',
      sort: 'priority',
      asc: true,
    });
    expect(qs).toBe(
      '?state=not_done&state=done&tag_filter=any%3Awork%2Curgent&due=today&q=milk&sort=priority&asc=true',
    );
  });

  it('serialises tag_filter with the @untagged sentinel', () => {
    const qs = buildTaskListQuery({
      tag_filter: { mode: 'any', tags: ['work', '@untagged'] },
    });
    // URLSearchParams percent-encodes ':' '@' and ',' — the un-encoded form
    // is `tag_filter=any:work,@untagged`.
    expect(qs).toBe('?tag_filter=any%3Awork%2C%40untagged');
  });

  it('omits tag_filter when tags array is empty', () => {
    expect(buildTaskListQuery({ tag_filter: { mode: 'any', tags: [] } })).toBe('');
  });

  it('returns empty string when no params', () => {
    expect(buildTaskListQuery({})).toBe('');
  });

  it('serialises tagsExclude as a single CSV tags_exclude param', () => {
    const qs = buildTaskListQuery({
      tagsExclude: ['skip', 'later'],
    });
    expect(qs).toBe('?tags_exclude=skip%2Clater');
  });

  it('omits tags_exclude when empty', () => {
    expect(buildTaskListQuery({ tagsExclude: [] })).toBe('');
  });
});

describe('useTasks', () => {
  afterEach(() => vi.restoreAllMocks());

  it('fetches tasks and exposes the array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse([{ id: 1, title: 'A' }]));
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useTasks({ states: ['not_done'] }), { wrapper: wrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([{ id: 1, title: 'A' }]);
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tasks?state=not_done', expect.any(Object));
  });
});

function fullTask(partial: Partial<Task> & { id: number }): Task {
  return {
    title: partial.title ?? `task ${partial.id}`,
    notes: '',
    state: 'not_done',
    due_date: null,
    priority: 0,
    staged_order: null,
    spawned_by_script_id: null,
    created_at: '2026-05-01T00:00:00Z',
    completed_at: null,
    cancelled_at: null,
    updated_at: '2026-05-01T00:00:00Z',
    tags: partial.tags ?? [],
    ...partial,
  };
}

describe('useBulkTag', () => {
  afterEach(() => vi.restoreAllMocks());

  it('POSTs to /tasks/bulk-tag and patches every cached tasks list', async () => {
    const updated = [fullTask({ id: 1, tags: ['x'] }), fullTask({ id: 2, tags: ['x'] })];
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(updated));
    vi.stubGlobal('fetch', fetchMock);

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    // Seed two task list caches with stale tag data so we can verify the
    // splice patches both without a refetch.
    const seedA: Task[] = [fullTask({ id: 1, tags: [] }), fullTask({ id: 3, tags: ['k'] })];
    const seedB: Task[] = [fullTask({ id: 2, tags: [] })];
    qc.setQueryData(['tasks', { states: ['not_done'] }], seedA);
    qc.setQueryData(['tasks', { states: ['done'] }], seedB);

    const invalidate = vi.spyOn(qc, 'invalidateQueries');
    const w = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useBulkTag(), { wrapper: w });
    await act(async () => {
      await result.current.mutateAsync({ ids: [1, 2], op: 'add', tags: ['x'] });
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/tasks/bulk-tag',
      expect.objectContaining({ method: 'POST' }),
    );
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect(JSON.parse(init.body as string)).toEqual({
      ids: [1, 2],
      op: 'add',
      tags: ['x'],
    });

    // Both caches got the new tag spliced in; non-targeted tasks were left
    // alone.
    const patchedA = qc.getQueryData<Task[]>(['tasks', { states: ['not_done'] }]);
    expect(patchedA?.find((t) => t.id === 1)?.tags).toEqual(['x']);
    expect(patchedA?.find((t) => t.id === 3)?.tags).toEqual(['k']);
    const patchedB = qc.getQueryData<Task[]>(['tasks', { states: ['done'] }]);
    expect(patchedB?.find((t) => t.id === 2)?.tags).toEqual(['x']);

    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['tags', 'with-counts'] });
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
