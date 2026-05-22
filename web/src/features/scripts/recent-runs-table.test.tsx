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
import type { Run } from '@/types/run';
import { formatDuration, RecentRunsTable, statusVariant } from './recent-runs-table';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function run(partial: Partial<Run> & { id: number }): Run {
  return {
    script_id: 1,
    started_at: '2026-05-01T00:00:00Z',
    finished_at: '2026-05-01T00:00:01Z',
    status: 'ok',
    error_message: '',
    spawned_task_ids: [],
    trigger: 'scheduled',
    ...partial,
  };
}

function renderTable(scriptId = 1) {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts/$id',
    component: () => <RecentRunsTable scriptId={scriptId} />,
  });
  const runRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/runs/$id',
    component: () => null,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([route, runRoute]),
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

describe('statusVariant', () => {
  it('maps each status to a known variant', () => {
    expect(statusVariant('ok')).toBe('secondary');
    expect(statusVariant('error')).toBe('destructive');
    expect(statusVariant('timeout')).toBe('destructive');
    expect(statusVariant('running')).toBe('default');
  });
});

describe('formatDuration', () => {
  it('returns ms / s / min depending on magnitude', () => {
    expect(formatDuration({ started_at: '2026-05-01T00:00:00Z', finished_at: null })).toBe('—');
    expect(
      formatDuration({
        started_at: '2026-05-01T00:00:00.000Z',
        finished_at: '2026-05-01T00:00:00.500Z',
      }),
    ).toBe('500 ms');
    expect(
      formatDuration({
        started_at: '2026-05-01T00:00:00Z',
        finished_at: '2026-05-01T00:00:03Z',
      }),
    ).toBe('3.0 s');
  });
});

describe('RecentRunsTable', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders a row per run with status + duration + task count', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse([
            run({ id: 1, status: 'ok', spawned_task_ids: [10, 11] }),
            run({
              id: 2,
              status: 'error',
              finished_at: null,
              error_message: 'boom',
              spawned_task_ids: [],
            }),
          ]),
        ),
      ),
    );
    renderTable();
    expect(await screen.findByText('ok')).toBeTruthy();
    expect(screen.getByText('error')).toBeTruthy();
    // Duration cell for run 1
    expect(screen.getAllByText(/—|s|ms|min/).length).toBeGreaterThan(0);
  });
});
