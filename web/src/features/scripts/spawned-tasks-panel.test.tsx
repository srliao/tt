import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/task';
import { SpawnedTasksPanel, unwrapTaskPage } from './spawned-tasks-panel';

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
    spawned_by_script_id: 1,
    created_at: '2026-05-01T00:00:00Z',
    completed_at: null,
    cancelled_at: null,
    updated_at: '2026-05-01T00:00:00Z',
    tags: [],
    ...partial,
  };
}

function renderPanel(scriptId = 1) {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts/$id',
    component: () => <SpawnedTasksPanel scriptId={scriptId} />,
  });
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    component: () => null,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([route, tasksRoute]),
    history: createMemoryHistory({ initialEntries: [`/scripts/${scriptId}`] }),
  });
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  );
}

describe('unwrapTaskPage', () => {
  it('handles array shape', () => {
    expect(unwrapTaskPage([task({ id: 1, title: 'A' })])).toEqual({
      tasks: [task({ id: 1, title: 'A' })],
      nextCursor: null,
    });
  });

  it('handles object shape with next_cursor', () => {
    expect(unwrapTaskPage({ tasks: [task({ id: 1, title: 'A' })], next_cursor: 'abc' })).toEqual({
      tasks: [task({ id: 1, title: 'A' })],
      nextCursor: 'abc',
    });
  });
});

describe('SpawnedTasksPanel', () => {
  afterEach(() => vi.restoreAllMocks());

  it('shows the empty hint when the server returns no tasks', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([]))),
    );
    renderPanel();
    expect(await screen.findByText(/No spawned tasks yet\./)).toBeTruthy();
  });

  it('renders one row per task', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse([
            task({ id: 1, title: 'Spawned 1' }),
            task({ id: 2, title: 'Spawned 2', state: 'done' }),
          ]),
        ),
      ),
    );
    renderPanel();
    expect(await screen.findByText('Spawned 1')).toBeTruthy();
    expect(screen.getByText('Spawned 2')).toBeTruthy();
    expect(screen.getByText('Done')).toBeTruthy();
  });
});
