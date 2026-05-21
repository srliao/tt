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
import type { Script } from '@/types/script';
import { humanizeSchedule, ScriptsListPage } from './list-page';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function script(partial: Partial<Script> & { id: number; name: string }): Script {
  return {
    code: '',
    enabled: true,
    schedule: { kind: 'daily' },
    last_run_at: null,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    ...partial,
  };
}

function renderPage() {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const indexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts',
    component: ScriptsListPage,
  });
  const newRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts/new',
    component: () => null,
  });
  const detailRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts/$id',
    component: () => null,
  });
  const runRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/runs/$id',
    component: () => null,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([indexRoute, newRoute, detailRoute, runRoute]),
    history: createMemoryHistory({ initialEntries: ['/scripts'] }),
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

describe('humanizeSchedule', () => {
  it('handles all kinds', () => {
    expect(humanizeSchedule({ kind: 'every_tick' })).toBe('Every 15 min');
    expect(humanizeSchedule({ kind: 'daily' })).toBe('Daily');
    expect(humanizeSchedule({ kind: 'weekly', weekday: 'monday' })).toBe('Weekly on Mon');
    expect(humanizeSchedule({ kind: 'monthly', day: 15 })).toBe('Monthly day 15');
    expect(humanizeSchedule({ kind: 'monthly', day: 'last' })).toBe('Monthly last day');
  });
});

describe('ScriptsListPage', () => {
  afterEach(() => vi.restoreAllMocks());

  it('shows the empty state when no scripts exist', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([]))),
    );
    renderPage();
    expect(
      await screen.findByText(/Userscripts let you auto-create tasks on a schedule\./),
    ).toBeTruthy();
    expect(screen.getByRole('link', { name: /Create your first script/i })).toBeTruthy();
  });

  it('lists scripts with humanized schedules', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse([
            script({
              id: 1,
              name: 'Weekly review',
              schedule: { kind: 'weekly', weekday: 'monday' },
            }),
            script({ id: 2, name: 'Monthly bills', schedule: { kind: 'monthly', day: 1 } }),
          ]),
        ),
      ),
    );
    renderPage();
    expect(await screen.findByText('Weekly review')).toBeTruthy();
    expect(screen.getByText('Weekly on Mon')).toBeTruthy();
    expect(screen.getByText('Monthly day 1')).toBeTruthy();
  });
});
