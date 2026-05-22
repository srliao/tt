import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Run } from '@/types/run';
import { RunsListPage, runsSearchSchema, toRunListFilters } from './list-page';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function run(partial: Partial<Run> & { id: number }): Run {
  return {
    script_id: 1,
    started_at: '2026-05-21T10:00:00Z',
    finished_at: '2026-05-21T10:00:01Z',
    status: 'ok',
    error_message: '',
    spawned_task_ids: [],
    trigger: 'manual',
    ...partial,
  };
}

function renderPage(initialUrl = '/runs') {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const runsIndexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/runs',
    validateSearch: (s) => runsSearchSchema.parse(s),
    component: RunsListPage,
  });
  const runDetailRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/runs/$id',
    component: () => null,
  });
  const scriptsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts',
    component: () => null,
  });
  const scriptDetailRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts/$id',
    component: () => null,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      runsIndexRoute,
      runDetailRoute,
      scriptsRoute,
      scriptDetailRoute,
    ]),
    history: createMemoryHistory({ initialEntries: [initialUrl] }),
  });
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return {
    qc,
    router,
    ...render(
      <QueryClientProvider client={qc}>
        <RouterProvider router={router as never} />
      </QueryClientProvider>,
    ),
  };
}

describe('toRunListFilters', () => {
  it('expands YYYY-MM-DD to RFC3339 boundaries', () => {
    expect(toRunListFilters({ from: '2026-05-01', to: '2026-05-21', status: 'ok' }, 25)).toEqual({
      limit: 25,
      status: 'ok',
      from: '2026-05-01T00:00:00Z',
      to: '2026-05-21T23:59:59Z',
    });
  });

  it('omits unset filters', () => {
    expect(toRunListFilters({}, 50)).toEqual({ limit: 50 });
  });
});

describe('RunsListPage', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders the empty state when no runs exist', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/scripts')) return Promise.resolve(jsonResponse([]));
        return Promise.resolve(jsonResponse([]));
      }),
    );
    renderPage();
    expect(
      await screen.findByText(/Manually trigger a script or wait for its schedule\./),
    ).toBeTruthy();
  });

  it('lists runs returned by the server', async () => {
    const scripts = [
      {
        id: 1,
        name: 'Weekly review',
        code: '',
        enabled: true,
        schedule: { kind: 'daily' },
        created_at: '2026-05-01T00:00:00Z',
        updated_at: '2026-05-01T00:00:00Z',
      },
    ];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/v1/scripts')) return Promise.resolve(jsonResponse(scripts));
        if (url.includes('/api/v1/runs')) {
          return Promise.resolve(jsonResponse([run({ id: 7, script_id: 1, status: 'error' })]));
        }
        return Promise.resolve(jsonResponse([]));
      }),
    );
    renderPage();
    expect(await screen.findByText('Weekly review')).toBeTruthy();
    expect(screen.getByText('error')).toBeTruthy();
  });

  it('refetches with new query key when a filter changes', async () => {
    const fetchMock = vi.fn((url: string) => {
      if (url.includes('/api/v1/scripts')) return Promise.resolve(jsonResponse([]));
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPage();

    // Wait for the first /runs call to land.
    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((c) => c[0] as string);
      expect(calls.some((c) => c.startsWith('/api/v1/runs'))).toBe(true);
    });
    const initialRunsCalls = fetchMock.mock.calls
      .map((c) => c[0] as string)
      .filter((c) => c.startsWith('/api/v1/runs')).length;

    // Type a `from` date — should trigger a new /runs request with the bound.
    fireEvent.change(screen.getByLabelText('From'), { target: { value: '2026-05-10' } });

    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((c) => c[0] as string);
      const newRunCalls = calls.filter((c) => c.startsWith('/api/v1/runs'));
      expect(newRunCalls.length).toBeGreaterThan(initialRunsCalls);
      expect(newRunCalls.some((c) => c.includes('from=2026-05-10'))).toBe(true);
    });
  });
});
