import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/task';
import { computeDragEnd, computeReorderPayload, moveTask, TaskTable } from './task-table';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function task(partial: Partial<Task> & { id: number; title: string }): Task {
  return {
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
    tags: [],
    ...partial,
  };
}

function wrap(children: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe('computeDragEnd', () => {
  it('returns null when overId is missing or same as activeId', () => {
    const t1 = task({ id: 1, title: 'a' });
    expect(computeDragEnd([t1], 1, null)).toBeNull();
    expect(computeDragEnd([t1], 1, 1)).toBeNull();
  });

  it('simulates a drop and produces the reorder payload', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    const result = computeDragEnd([t1, t2, t3], 1, 3);
    expect(result?.next.map((t) => t.id)).toEqual([2, 3, 1]);
    expect(result?.payload).toEqual({ task_id: 1, before_id: 3, after_id: null });
  });
});

describe('moveTask + computeReorderPayload (Alt-C semantics)', () => {
  it('moving t1 to the end of [t1,t2,t3] yields payload {before: t2, after: null}', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    const moved = moveTask([t1, t2, t3], 1, 3);
    expect(moved.map((t) => t.id)).toEqual([2, 3, 1]);
    expect(computeReorderPayload(moved, 1)).toEqual({
      task_id: 1,
      before_id: 3,
      after_id: null,
    });
  });

  it('moving t3 to the top yields {before: null, after: t1}', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    const moved = moveTask([t1, t2, t3], 3, 1);
    expect(moved.map((t) => t.id)).toEqual([3, 1, 2]);
    expect(computeReorderPayload(moved, 3)).toEqual({
      task_id: 3,
      before_id: null,
      after_id: 1,
    });
  });
});

describe('computeReorderPayload', () => {
  it('returns null neighbours for first row', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    expect(computeReorderPayload([t1, t2], 1)).toEqual({
      task_id: 1,
      before_id: null,
      after_id: 2,
    });
  });

  it('returns null neighbours for last row', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    expect(computeReorderPayload([t1, t2], 2)).toEqual({
      task_id: 2,
      before_id: 1,
      after_id: null,
    });
  });

  it('reports both neighbours for a middle row', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    expect(computeReorderPayload([t1, t2, t3], 2)).toEqual({
      task_id: 2,
      before_id: 1,
      after_id: 3,
    });
  });
});

describe('TaskTable', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders a drag handle column when sort=priority', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="priority"
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );
    expect(screen.getByRole('button', { name: 'Reorder A' })).toBeTruthy();
  });

  it('does not render a drag handle when sort=title', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="title"
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );
    expect(screen.queryByRole('button', { name: 'Reorder A' })).toBeNull();
  });

  it('clicking the title invokes onEdit', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onEdit = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 7, title: 'Edit me' })]}
          sort="title"
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={onEdit}
        />,
      ),
    );
    act(() => {
      screen.getByRole('button', { name: 'Edit me' }).click();
    });
    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({ id: 7, title: 'Edit me' }));
  });

  it('kebab "Mark done" calls POST /tasks/:id/state', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({}));
    vi.stubGlobal('fetch', fetchMock);
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 42, title: 'A' })]}
          sort="title"
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );

    // Radix opens its dropdown on pointerdown; simulate that explicitly.
    const trigger = screen.getByRole('button', { name: 'Actions for A' });
    await act(async () => {
      fireEvent.pointerDown(trigger, { button: 0, pointerType: 'mouse' });
      fireEvent.pointerUp(trigger, { button: 0, pointerType: 'mouse' });
      fireEvent.click(trigger);
    });

    const markDone = await screen.findByText('Mark done');
    await act(async () => {
      fireEvent.click(markDone);
    });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/tasks/42/state',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ state: 'done' }),
        }),
      );
    });
  });
});
