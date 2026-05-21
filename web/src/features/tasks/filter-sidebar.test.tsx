import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FilterSidebar } from './filter-sidebar';
import { taskSearchSchema } from './use-task-list-search';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function renderSidebar(initial = '/tasks') {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    validateSearch: (s) => taskSearchSchema.parse(s),
    component: () => <FilterSidebar />,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([tasksRoute]),
    history: createMemoryHistory({ initialEntries: [initial] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const result = render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  );
  return { router, ...result };
}

describe('FilterSidebar', () => {
  afterEach(() => vi.restoreAllMocks());

  it('marks the search input with data-search-input', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    renderSidebar();
    expect(await screen.findByPlaceholderText('Search tasks…')).toHaveAttribute(
      'data-search-input',
    );
  });

  it('clicking a quick filter updates the URL search-params', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    renderSidebar('/tasks');

    // The aside is the FilterSidebar root; scope queries to the latest copy
    // because TanStack Router keeps the previous match mounted across
    // navigations under memory history.
    const initialAside = await screen.findByRole('complementary');
    const btn = within(initialAside).getByRole('button', { name: 'Overdue' });
    await act(async () => {
      btn.click();
    });

    await waitFor(() => {
      const asides = screen.getAllByRole('complementary');
      const latest = asides[asides.length - 1];
      expect(within(latest).getByRole('button', { name: 'Overdue' })).toHaveAttribute(
        'data-active',
        'true',
      );
    });
  });

  it('checking a state checkbox updates the URL states array', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    renderSidebar('/tasks');

    const initialAside = await screen.findByRole('complementary');
    const cb = within(initialAside).getByTestId('state-checkbox-done');
    await act(async () => {
      cb.click();
    });

    await waitFor(() => {
      const asides = screen.getAllByRole('complementary');
      const latest = asides[asides.length - 1];
      expect(within(latest).getByTestId('state-checkbox-done')).toHaveAttribute(
        'aria-checked',
        'true',
      );
    });
  });
});
