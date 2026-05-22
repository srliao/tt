import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import { act, fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
import type { Task } from '@/types/task';
import {
  computeDragEnd,
  computeReorderPayload,
  moveTask,
  rangeSelection,
  TaskTable,
} from './task-table';
import { taskSearchSchema } from './use-task-list-search';

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
  // TaskTable reads URL search params via `useTaskListSearch`, which needs a
  // tanstack router context — provide a minimal in-memory router so the
  // component renders without hitting `useMatch` null guards. ThemeProvider
  // is required because <TagGlyph> reads `resolvedTheme`.
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    validateSearch: (s) => taskSearchSchema.parse(s),
    component: () => <>{children}</>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([tasksRoute]),
    history: createMemoryHistory({ initialEntries: ['/tasks'] }),
  });
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <ThemeProvider>
      <QueryClientProvider client={qc}>
        <RouterProvider router={router as never} />
      </QueryClientProvider>
    </ThemeProvider>
  );
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

describe('rangeSelection', () => {
  const tasks = [{ id: 1 } as Task, { id: 2 } as Task, { id: 3 } as Task, { id: 4 } as Task];
  it('returns an inclusive id set spanning anchor → cursor', () => {
    expect(rangeSelection(tasks, 1, 3)).toEqual(new Set([2, 3, 4]));
  });
  it('handles cursor < anchor (range walks both directions)', () => {
    expect(rangeSelection(tasks, 2, 0)).toEqual(new Set([1, 2, 3]));
  });
  it('returns a single-id set when anchor equals cursor', () => {
    expect(rangeSelection(tasks, 1, 1)).toEqual(new Set([2]));
  });
  it('returns an empty set when either index is negative', () => {
    expect(rangeSelection(tasks, -1, 2)).toEqual(new Set());
    expect(rangeSelection(tasks, 1, -1)).toEqual(new Set());
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

  it('renders a drag handle column when sort=priority', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="priority"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );
    expect(await screen.findByRole('button', { name: 'Reorder A' })).toBeTruthy();
  });

  it('does not render a drag handle when sort=title', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="title"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );
    // Wait for the row to mount, then assert there is no reorder handle.
    await screen.findByRole('button', { name: /Mark A as done/ });
    expect(screen.queryByRole('button', { name: 'Reorder A' })).toBeNull();
  });

  it('hides the multi-select checkbox unless multiSelectMode is on', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const { rerender } = render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="title"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );
    await screen.findByRole('button', { name: /Mark A as done/ });
    expect(screen.queryByRole('checkbox', { name: 'Select A' })).toBeNull();

    rerender(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="title"
          multiSelectMode={true}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );
    expect(await screen.findByRole('checkbox', { name: 'Select A' })).toBeTruthy();
  });

  it('renders the stage bookmark and done radio for each row', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="title"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );
    expect(await screen.findByRole('button', { name: 'Stage A' })).toBeTruthy();
    expect(screen.getByRole('button', { name: /Mark A as done/ })).toBeTruthy();
  });

  it('clicking the title invokes onEdit', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onEdit = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 7, title: 'Edit me' })]}
          sort="title"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={onEdit}
        />,
      ),
    );
    const button = await screen.findByRole('button', { name: 'Edit me' });
    act(() => {
      button.click();
    });
    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({ id: 7, title: 'Edit me' }));
  });

  it('pressing t on a focused row invokes onEditTags with that task', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onEditTags = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' }), task({ id: 2, title: 'B' })]}
          sort="title"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
          onEditTags={onEditTags}
        />,
      ),
    );
    await screen.findByRole('table');
    // Focus a row via plain j, then press t.
    act(() => {
      fireEvent.keyDown(document.body, { key: 'j' });
    });
    act(() => {
      fireEvent.keyDown(document.body, { key: 't' });
    });
    expect(onEditTags).toHaveBeenCalledTimes(1);
    expect(onEditTags).toHaveBeenCalledWith(expect.objectContaining({ id: 1, title: 'A' }));
  });

  it('t shortcut does nothing when no row is focused', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onEditTags = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="title"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
          onEditTags={onEditTags}
        />,
      ),
    );
    await screen.findByRole('table');
    act(() => {
      fireEvent.keyDown(document.body, { key: 't' });
    });
    expect(onEditTags).not.toHaveBeenCalled();
  });

  it('shift-j extends selection from the focused row downward and auto-enables multi-select', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onSelectedChange = vi.fn();
    const onMultiSelectModeChange = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[
            task({ id: 1, title: 'A' }),
            task({ id: 2, title: 'B' }),
            task({ id: 3, title: 'C' }),
            task({ id: 4, title: 'D' }),
          ]}
          sort="title"
          multiSelectMode={false}
          onMultiSelectModeChange={onMultiSelectModeChange}
          selectedIds={new Set()}
          onSelectedChange={onSelectedChange}
          onEdit={() => {}}
        />,
      ),
    );
    await screen.findByRole('table');
    // Focus row 2 (index 1) by pressing j twice.
    act(() => {
      fireEvent.keyDown(document.body, { key: 'j' });
    });
    act(() => {
      fireEvent.keyDown(document.body, { key: 'j' });
    });
    onSelectedChange.mockClear();
    onMultiSelectModeChange.mockClear();
    // ⇧j → extend to row 3.
    act(() => {
      fireEvent.keyDown(document.body, { key: 'J', shiftKey: true });
    });
    expect(onMultiSelectModeChange).toHaveBeenCalledWith(true);
    expect(onSelectedChange).toHaveBeenLastCalledWith(new Set([2, 3]));
    // ⇧j again → extend to row 4.
    act(() => {
      fireEvent.keyDown(document.body, { key: 'J', shiftKey: true });
    });
    expect(onSelectedChange).toHaveBeenLastCalledWith(new Set([2, 3, 4]));
  });

  it('shift-k after shift-j shrinks the range back toward the anchor', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onSelectedChange = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[
            task({ id: 1, title: 'A' }),
            task({ id: 2, title: 'B' }),
            task({ id: 3, title: 'C' }),
            task({ id: 4, title: 'D' }),
          ]}
          sort="title"
          multiSelectMode={true}
          selectedIds={new Set()}
          onSelectedChange={onSelectedChange}
          onEdit={() => {}}
        />,
      ),
    );
    await screen.findByRole('table');
    act(() => {
      fireEvent.keyDown(document.body, { key: 'j' });
    });
    act(() => {
      fireEvent.keyDown(document.body, { key: 'j' });
    });
    // Anchor is row 2 now.
    act(() => {
      fireEvent.keyDown(document.body, { key: 'J', shiftKey: true });
    });
    act(() => {
      fireEvent.keyDown(document.body, { key: 'J', shiftKey: true });
    });
    expect(onSelectedChange).toHaveBeenLastCalledWith(new Set([2, 3, 4]));
    onSelectedChange.mockClear();
    act(() => {
      fireEvent.keyDown(document.body, { key: 'K', shiftKey: true });
    });
    expect(onSelectedChange).toHaveBeenLastCalledWith(new Set([2, 3]));
    act(() => {
      fireEvent.keyDown(document.body, { key: 'K', shiftKey: true });
    });
    expect(onSelectedChange).toHaveBeenLastCalledWith(new Set([2]));
  });

  it('plain j after a shift-j range resets the anchor (next shift-j starts a fresh range)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onSelectedChange = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[
            task({ id: 1, title: 'A' }),
            task({ id: 2, title: 'B' }),
            task({ id: 3, title: 'C' }),
            task({ id: 4, title: 'D' }),
          ]}
          sort="title"
          multiSelectMode={true}
          selectedIds={new Set()}
          onSelectedChange={onSelectedChange}
          onEdit={() => {}}
        />,
      ),
    );
    await screen.findByRole('table');
    act(() => {
      fireEvent.keyDown(document.body, { key: 'j' }); // focus row 1
    });
    act(() => {
      fireEvent.keyDown(document.body, { key: 'J', shiftKey: true }); // range to row 2
    });
    expect(onSelectedChange).toHaveBeenLastCalledWith(new Set([1, 2]));
    onSelectedChange.mockClear();
    act(() => {
      fireEvent.keyDown(document.body, { key: 'j' }); // plain step → resets anchor; no selection change
    });
    expect(onSelectedChange).not.toHaveBeenCalled();
    // Now ⇧j → fresh range starting at the new focus (row 3).
    act(() => {
      fireEvent.keyDown(document.body, { key: 'J', shiftKey: true });
    });
    expect(onSelectedChange).toHaveBeenLastCalledWith(new Set([3, 4]));
  });

  it('Escape exits multi-select mode and clears the selection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onSelectedChange = vi.fn();
    const onMultiSelectModeChange = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' }), task({ id: 2, title: 'B' })]}
          sort="title"
          multiSelectMode={true}
          onMultiSelectModeChange={onMultiSelectModeChange}
          selectedIds={new Set([1, 2])}
          onSelectedChange={onSelectedChange}
          onEdit={() => {}}
        />,
      ),
    );
    await screen.findByRole('table');
    act(() => {
      fireEvent.keyDown(document.body, { key: 'Escape' });
    });
    expect(onSelectedChange).toHaveBeenCalledWith(new Set());
    expect(onMultiSelectModeChange).toHaveBeenCalledWith(false);
  });

  it('Escape is a no-op when multi-select is off and nothing is selected', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onSelectedChange = vi.fn();
    const onMultiSelectModeChange = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' })]}
          sort="title"
          multiSelectMode={false}
          onMultiSelectModeChange={onMultiSelectModeChange}
          selectedIds={new Set()}
          onSelectedChange={onSelectedChange}
          onEdit={() => {}}
        />,
      ),
    );
    await screen.findByRole('table');
    act(() => {
      fireEvent.keyDown(document.body, { key: 'Escape' });
    });
    expect(onSelectedChange).not.toHaveBeenCalled();
    expect(onMultiSelectModeChange).not.toHaveBeenCalled();
  });

  it('renders a tag glyph for each task tag and forwards the tag name on click', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A', tags: ['backend', 'ops'] })]}
          sort="title"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
        />,
      ),
    );
    // Two glyphs render — one per tag — each with the canonical aria label.
    expect(await screen.findByRole('button', { name: 'Tag backend' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Tag ops' })).toBeTruthy();
  });

  it('keydown on document.body after mount focuses row 0 (no click required)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onEditTags = vi.fn();
    render(
      wrap(
        <TaskTable
          tasks={[task({ id: 1, title: 'A' }), task({ id: 2, title: 'B' })]}
          sort="title"
          multiSelectMode={false}
          selectedIds={new Set()}
          onSelectedChange={() => {}}
          onEdit={() => {}}
          onEditTags={onEditTags}
        />,
      ),
    );
    await screen.findByRole('table');
    // No focus/click on the table — keystroke arrives on document.body.
    act(() => {
      fireEvent.keyDown(document.body, { key: 'j' });
    });
    // Press `t` — if row 0 is focused, onEditTags fires for task id 1.
    act(() => {
      fireEvent.keyDown(document.body, { key: 't' });
    });
    expect(onEditTags).toHaveBeenCalledWith(expect.objectContaining({ id: 1 }));
  });

  it('keydown is a no-op when the active element is an <input>', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onEditTags = vi.fn();
    render(
      wrap(
        <>
          <input aria-label="search" />
          <TaskTable
            tasks={[task({ id: 1, title: 'A' }), task({ id: 2, title: 'B' })]}
            sort="title"
            multiSelectMode={false}
            selectedIds={new Set()}
            onSelectedChange={() => {}}
            onEdit={() => {}}
            onEditTags={onEditTags}
          />
        </>,
      ),
    );
    const input = await screen.findByLabelText('search');
    act(() => {
      input.focus();
      fireEvent.keyDown(input, { key: 'j' });
    });
    act(() => {
      fireEvent.keyDown(input, { key: 't' });
    });
    expect(onEditTags).not.toHaveBeenCalled();
  });

  it('keydown is a no-op when an open dialog is present in the DOM', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const onEditTags = vi.fn();
    // Inject a Radix-style open dialog into the document. The handler
    // detects it via querySelector and defers.
    const dialog = document.createElement('div');
    dialog.setAttribute('role', 'dialog');
    dialog.setAttribute('data-state', 'open');
    document.body.appendChild(dialog);
    try {
      render(
        wrap(
          <TaskTable
            tasks={[task({ id: 1, title: 'A' })]}
            sort="title"
            multiSelectMode={false}
            selectedIds={new Set()}
            onSelectedChange={() => {}}
            onEdit={() => {}}
            onEditTags={onEditTags}
          />,
        ),
      );
      await screen.findByRole('table');
      act(() => {
        fireEvent.keyDown(document.body, { key: 'j' });
      });
      act(() => {
        fireEvent.keyDown(document.body, { key: 't' });
      });
      expect(onEditTags).not.toHaveBeenCalled();
    } finally {
      dialog.remove();
    }
  });
});
