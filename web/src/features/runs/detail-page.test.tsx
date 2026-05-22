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
import type { RunDetail } from '@/api/runs';
import { pollInterval, RunDetailPage } from './detail-page';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function detail(partial: Partial<RunDetail> & { id: number }): RunDetail {
  return {
    script_id: 1,
    started_at: '2026-05-21T10:00:00Z',
    finished_at: '2026-05-21T10:00:02Z',
    status: 'ok',
    error_message: '',
    spawned_task_ids: [],
    trigger: 'manual',
    logs: [],
    spawned_tasks: [],
    ...partial,
  };
}

function renderPage(id: number) {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const here = createRoute({
    getParentRoute: () => rootRoute,
    path: '/runs/$id',
    component: () => <RunDetailPage id={id} />,
  });
  const runsList = createRoute({
    getParentRoute: () => rootRoute,
    path: '/runs',
    component: () => null,
  });
  const scriptDetail = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts/$id',
    component: () => null,
  });
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    component: () => null,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([here, runsList, scriptDetail, tasksRoute]),
    history: createMemoryHistory({ initialEntries: [`/runs/${id}`] }),
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

describe('pollInterval', () => {
  it('returns 2000ms while running', () => {
    expect(pollInterval('running')).toBe(2000);
  });

  it('returns false for any terminal status', () => {
    expect(pollInterval('ok')).toBe(false);
    expect(pollInterval('error')).toBe(false);
    expect(pollInterval('timeout')).toBe(false);
  });

  it('returns false when the status is unknown', () => {
    expect(pollInterval(undefined)).toBe(false);
  });
});

describe('RunDetailPage', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders the run header and logs section', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.endsWith('/api/v1/runs/7')) {
          return Promise.resolve(
            jsonResponse(
              detail({
                id: 7,
                status: 'ok',
                logs: [
                  {
                    id: 100,
                    run_id: 7,
                    level: 'info',
                    logged_at: '2026-05-21T10:00:00.500Z',
                    message: 'hello world',
                  },
                ],
              }),
            ),
          );
        }
        if (url.includes('/api/v1/scripts/1')) {
          return Promise.resolve(
            jsonResponse({
              id: 1,
              name: 'Weekly review',
              code: '',
              enabled: true,
              schedule: { kind: 'daily' },
              created_at: '2026-05-01T00:00:00Z',
              updated_at: '2026-05-01T00:00:00Z',
            }),
          );
        }
        return Promise.resolve(jsonResponse({}, 404));
      }),
    );
    renderPage(7);
    expect(await screen.findByText('Run #7')).toBeTruthy();
    expect(await screen.findByText('hello world')).toBeTruthy();
    expect(screen.getByText(/Logs \(1\)/)).toBeTruthy();
  });

  it('renders an error block when the status is not ok', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse(
            detail({
              id: 9,
              status: 'error',
              error_message: 'TypeError: x is undefined',
            }),
          ),
        ),
      ),
    );
    renderPage(9);
    expect(await screen.findByText('TypeError: x is undefined')).toBeTruthy();
  });

  it('renders spawned task chips when the run queued tasks', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.endsWith('/api/v1/runs/5')) {
          return Promise.resolve(
            jsonResponse(
              detail({
                id: 5,
                spawned_task_ids: [11, 12, 13],
                spawned_tasks: [
                  { id: 11, title: 'Alpha task', state: 'not_done' },
                  { id: 12, title: 'Beta task', state: 'done' },
                ],
              }),
            ),
          );
        }
        return Promise.resolve(jsonResponse({}, 404));
      }),
    );
    renderPage(5);
    expect(await screen.findByText('Alpha task')).toBeTruthy();
    expect(screen.getByText('Beta task')).toBeTruthy();
    // The third id should render as a deleted chip.
    expect(screen.getByText('Task #13')).toBeTruthy();
  });
});
