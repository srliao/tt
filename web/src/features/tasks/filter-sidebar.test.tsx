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
import { ThemeProvider } from '@/components/theme-provider';
import { FilterSidebar } from './filter-sidebar';
import { taskSearchSchema } from './use-task-list-search';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

/**
 * Fetch mock that returns tag-with-count data on /tags?counts=1 and an empty
 * array everywhere else. Returned tags include both small and large lists so
 * tests can target specific behavior (e.g. the search-in-list trigger).
 */
function mockFetchWithTags(tags: Array<{ id: number; name: string; count: number }>) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (url.includes('/tags')) {
      return jsonResponse(tags);
    }
    return jsonResponse([]);
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
    <ThemeProvider>
      <QueryClientProvider client={qc}>
        <RouterProvider router={router as never} />
      </QueryClientProvider>
    </ThemeProvider>,
  );
  return { router, ...result };
}

describe('FilterSidebar', () => {
  afterEach(() => vi.restoreAllMocks());

  it('does not render an in-sidebar search field (Phase 5: moved to command palette)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    renderSidebar();
    // The sidebar mounts asynchronously; wait for any quick-filter button so
    // the absence assertion is meaningful.
    await screen.findByRole('button', { name: 'Overdue' });
    expect(screen.queryByPlaceholderText('Search tasks…')).toBeNull();
    expect(screen.queryByLabelText('Search tasks')).toBeNull();
    expect(document.querySelector('[data-search-input]')).toBeNull();
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

  it('renders the tag inline list without a popover, with per-tag counts', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    const workBtn = await within(aside).findByRole('button', { name: /work/i });
    // Count badge sits next to the tag chip inside the same row.
    expect(workBtn.textContent).toContain('3');

    // The inline list does not gate behind a popover — both tags are visible
    // immediately.
    expect(within(aside).getByRole('button', { name: /home/i })).toBeInTheDocument();
  });

  it('clicking a tag toggles selection and updates the URL', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    const { router } = renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    const workBtn = await within(aside).findByRole('button', { name: /work/i });
    await act(async () => {
      workBtn.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['work'] },
      });
    });

    // The same row should now report selected via data-selected.
    await waitFor(() => {
      const asides = screen.getAllByRole('complementary');
      const latest = asides[asides.length - 1];
      const row = latest.querySelector('[data-tag-name="work"]') as HTMLElement;
      expect(row).toHaveAttribute('data-selected', 'true');
    });
  });

  it('removing a selected chip also unselects the row in the list', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    const { router } = renderSidebar('/tasks');

    // Click the row to select it.
    const aside = await screen.findByRole('complementary');
    const workBtn = await within(aside).findByRole('button', { name: /work/i });
    await act(async () => {
      workBtn.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['work'] },
      });
    });

    // Now click the × on the chip and verify the URL clears tags.
    const latestAside = (() => {
      const asides = screen.getAllByRole('complementary');
      return asides[asides.length - 1];
    })();
    const chipBlock = within(latestAside).getByTestId('selected-tag-chips');
    const removeBtn = within(chipBlock).getByRole('button', { name: /remove work/i });
    await act(async () => {
      removeBtn.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).not.toHaveProperty('tag_filter');
    });
  });

  it('clicking the all toggle writes the new tag_filter mode to the URL', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    const { router } = renderSidebar('/tasks?tag_filter=any%3Awork');

    // The mode toggle is meaningful once at least one tag is selected — the
    // URL seeds a single-tag any-mode filter so the toggle has tags to gate.
    const aside = await screen.findByRole('complementary');
    const allToggle = within(aside).getByRole('button', { name: 'all' });
    await act(async () => {
      allToggle.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'all', tags: ['work'] },
      });
    });

    // Flipping back to "any" keeps the same tag set, just with mode=any.
    const asides = screen.getAllByRole('complementary');
    const latest = asides[asides.length - 1];
    const anyToggle = within(latest).getByRole('button', { name: 'any' });
    await act(async () => {
      anyToggle.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['work'] },
      });
    });
  });

  it('hides the in-list filter input when there are 8 or fewer tags', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags(
        Array.from({ length: 8 }, (_, i) => ({ id: i + 1, name: `tag-${i}`, count: i })),
      ),
    );
    renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    // Wait for the inline list to render before asserting the filter input is
    // absent — otherwise we might be checking before the tags have loaded.
    await within(aside).findByRole('button', { name: /tag-0/i });
    expect(within(aside).queryByLabelText('Filter tag list')).toBeNull();
  });

  it('renders the in-list filter input when there are more than 8 tags', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags(
        Array.from({ length: 9 }, (_, i) => ({ id: i + 1, name: `tag-${i}`, count: i })),
      ),
    );
    renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    expect(await within(aside).findByLabelText('Filter tag list')).toBeInTheDocument();
  });
});
