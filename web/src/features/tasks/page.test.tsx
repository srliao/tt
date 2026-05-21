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
import { TasksPage } from './page';
import { taskSearchSchema } from './use-task-list-search';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function renderPage(initial = '/tasks') {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    validateSearch: (s) => taskSearchSchema.parse(s),
    component: TasksPage,
  });
  // Required so <Link to="/scripts" /> in the empty state can resolve.
  const scriptsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts',
    component: () => null,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([tasksRoute, scriptsRoute]),
    history: createMemoryHistory({ initialEntries: [initial] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  );
}

describe('TasksPage', () => {
  afterEach(() => vi.restoreAllMocks());

  it('shows the empty state when the server returns no tasks and no filters are active', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([]))),
    );
    renderPage();
    expect(await screen.findByRole('button', { name: /Create your first task/ })).toBeTruthy();
  });

  it('renders the task table when tasks are present', async () => {
    const taskRow = {
      id: 1,
      title: 'Existing',
      notes: '',
      state: 'not_done' as const,
      due_date: null,
      priority: 0,
      staged_order: null,
      spawned_by_script_id: null,
      created_at: '2026-05-01T00:00:00Z',
      completed_at: null,
      cancelled_at: null,
      updated_at: '2026-05-01T00:00:00Z',
      tags: [],
    };
    // `Response` bodies can only be consumed once, so build a fresh Response
    // per call rather than reusing the same instance via mockResolvedValue.
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/tags')) return Promise.resolve(jsonResponse([]));
        return Promise.resolve(jsonResponse([taskRow]));
      }),
    );
    renderPage();
    expect(await screen.findByText('Existing', undefined, { timeout: 2000 })).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Create your first task/ })).toBeNull();
  });
});
