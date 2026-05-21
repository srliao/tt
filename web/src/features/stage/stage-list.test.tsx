import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/task';
import { computeDragEnd, computeStagePayload, moveStaged, StageList } from './stage-list';

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
    staged_order: 1,
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

describe('moveStaged', () => {
  it('returns the same array when from and to are equal', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const arr = [t1, t2];
    expect(moveStaged(arr, 1, 1)).toBe(arr);
  });

  it('moves a row down into the target slot', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    const moved = moveStaged([t1, t2, t3], 1, 3);
    expect(moved.map((t) => t.id)).toEqual([2, 3, 1]);
  });

  it('moves a row up into the target slot', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    const moved = moveStaged([t1, t2, t3], 3, 1);
    expect(moved.map((t) => t.id)).toEqual([3, 1, 2]);
  });
});

describe('computeStagePayload', () => {
  it('returns null before for the first row', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    expect(computeStagePayload([t1, t2], 1)).toEqual({
      task_id: 1,
      before_id: null,
      after_id: 2,
    });
  });

  it('returns null after for the last row', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    expect(computeStagePayload([t1, t2], 2)).toEqual({
      task_id: 2,
      before_id: 1,
      after_id: null,
    });
  });

  it('reports both neighbours for a middle row', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    expect(computeStagePayload([t1, t2, t3], 2)).toEqual({
      task_id: 2,
      before_id: 1,
      after_id: 3,
    });
  });
});

describe('computeDragEnd', () => {
  it('returns null when overId is missing or matches activeId', () => {
    const t1 = task({ id: 1, title: 'a' });
    expect(computeDragEnd([t1], 1, null)).toBeNull();
    expect(computeDragEnd([t1], 1, 1)).toBeNull();
  });

  it('produces the post-drop list and reorder payload for a top→bottom drop', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    const result = computeDragEnd([t1, t2, t3], 1, 3);
    expect(result?.next.map((t) => t.id)).toEqual([2, 3, 1]);
    expect(result?.payload).toEqual({ task_id: 1, before_id: 3, after_id: null });
  });

  it('produces the right payload for a middle drop', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    const result = computeDragEnd([t1, t2, t3], 3, 2);
    expect(result?.next.map((t) => t.id)).toEqual([1, 3, 2]);
    expect(result?.payload).toEqual({ task_id: 3, before_id: 1, after_id: 2 });
  });

  it('produces the right payload for a bottom→top drop', () => {
    const t1 = task({ id: 1, title: 'a' });
    const t2 = task({ id: 2, title: 'b' });
    const t3 = task({ id: 3, title: 'c' });
    const result = computeDragEnd([t1, t2, t3], 3, 1);
    expect(result?.next.map((t) => t.id)).toEqual([3, 1, 2]);
    expect(result?.payload).toEqual({ task_id: 3, before_id: null, after_id: 1 });
  });
});

describe('StageList', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders a row + drag handle per task', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      wrap(
        <StageList
          tasks={[task({ id: 1, title: 'A' }), task({ id: 2, title: 'B' })]}
          focusedId={null}
          onEdit={() => {}}
        />,
      ),
    );
    expect(screen.getByRole('button', { name: 'Reorder A' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Reorder B' })).toBeTruthy();
  });
});
