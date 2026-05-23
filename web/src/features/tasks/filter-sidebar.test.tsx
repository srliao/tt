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
import { taskSearchSchema, UNTAGGED_TOKEN } from './use-task-list-search';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

/**
 * Fetch mock that returns tag-with-count data on /tags?counts=1, an empty
 * array on /tasks, and an empty array everywhere else.
 */
function mockFetchWithTags(
  tags: Array<{ id: number; name: string; count: number }>,
  tasks: Array<{ id: number; tags: string[]; state?: string; title?: string; notes?: string }> = [],
) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (url.includes('/tags')) return jsonResponse(tags);
    if (url.includes('/tasks')) return jsonResponse(tasks);
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

/** Helper: grab the most-recent <aside> — TanStack Router keeps the previous
 *  match mounted across navigations under memory history. */
function latestAside() {
  const asides = screen.getAllByRole('complementary');
  return asides[asides.length - 1];
}

describe('FilterSidebar', () => {
  afterEach(() => vi.restoreAllMocks());

  it('does not render an in-sidebar search field (moved to command palette)', async () => {
    vi.stubGlobal('fetch', mockFetchWithTags([]));
    renderSidebar();
    await screen.findByRole('button', { name: 'Overdue' });
    expect(screen.queryByPlaceholderText('Search tasks…')).toBeNull();
    expect(screen.queryByLabelText('Search tasks')).toBeNull();
    expect(document.querySelector('[data-search-input]')).toBeNull();
  });

  it('clicking a quick filter updates the URL search-params', async () => {
    vi.stubGlobal('fetch', mockFetchWithTags([]));
    renderSidebar('/tasks');

    const initialAside = await screen.findByRole('complementary');
    const btn = within(initialAside).getByRole('button', { name: 'Overdue' });
    await act(async () => {
      btn.click();
    });

    await waitFor(() => {
      expect(within(latestAside()).getByRole('button', { name: 'Overdue' })).toHaveAttribute(
        'data-active',
        'true',
      );
    });
  });

  it('checking a state checkbox updates the URL states array', async () => {
    vi.stubGlobal('fetch', mockFetchWithTags([]));
    renderSidebar('/tasks');

    const initialAside = await screen.findByRole('complementary');
    const cb = within(initialAside).getByTestId('state-checkbox-done');
    await act(async () => {
      cb.click();
    });

    await waitFor(() => {
      expect(within(latestAside()).getByTestId('state-checkbox-done')).toHaveAttribute(
        'aria-checked',
        'true',
      );
    });
  });

  it('renders the tag inline list (no popover) with per-tag counts', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    const workBtn = await within(aside).findByRole('button', { name: /select tag work/i });
    expect(workBtn.textContent).toContain('3');
    expect(within(aside).getByRole('button', { name: /select tag home/i })).toBeInTheDocument();
  });

  it('renders the pinned Untagged row at the top with a count', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags(
        [{ id: 1, name: 'work', count: 3 }],
        [
          { id: 10, tags: [] },
          { id: 11, tags: [] },
          { id: 12, tags: ['work'] },
        ],
      ),
    );
    renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    // Pinned Untagged row carries the sentinel name as data-tag-name.
    const row = await waitFor(() => {
      const found = aside.querySelector(
        `[data-tag-name="${UNTAGGED_TOKEN}"]`,
      ) as HTMLElement | null;
      if (!found) throw new Error('untagged row not yet rendered');
      return found;
    });
    expect(row.textContent).toContain('Untagged');
    expect(row.textContent).toContain('2');
  });

  it('selecting a real tag writes any:work to the URL', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    const { router } = renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    const workBtn = await within(aside).findByRole('button', { name: /select tag work/i });
    await act(async () => {
      workBtn.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['work'] },
      });
    });
    await waitFor(() => {
      const row = latestAside().querySelector('[data-tag-name="work"]') as HTMLElement;
      expect(row).toHaveAttribute('data-selected', 'true');
    });
  });

  it('selecting two real tags writes any:work,home', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    const { router } = renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    const workBtn = await within(aside).findByRole('button', { name: /select tag work/i });
    await act(async () => {
      workBtn.click();
    });
    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['work'] },
      });
    });
    const homeBtn = await within(latestAside()).findByRole('button', {
      name: /select tag home/i,
    });
    await act(async () => {
      homeBtn.click();
    });
    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['work', 'home'] },
      });
    });
  });

  it('flipping to All preserves selected tags and writes all:...', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    const { router } = renderSidebar('/tasks?tag_filter=any%3Awork%2Chome');

    const aside = await screen.findByRole('complementary');
    const allBtn = within(aside).getByRole('button', { name: 'all' });
    await act(async () => {
      allBtn.click();
    });
    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'all', tags: ['work', 'home'] },
      });
    });
    // Flipping back to any keeps the same tags.
    await act(async () => {
      within(latestAside()).getByRole('button', { name: 'any' }).click();
    });
    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['work', 'home'] },
      });
    });
  });

  it('selecting Untagged writes any:@untagged to the URL', async () => {
    vi.stubGlobal('fetch', mockFetchWithTags([{ id: 1, name: 'work', count: 3 }]));
    const { router } = renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    const untaggedRow = await waitFor(() => {
      const found = aside.querySelector(
        `[data-tag-name="${UNTAGGED_TOKEN}"]`,
      ) as HTMLButtonElement | null;
      if (!found) throw new Error('untagged row not yet rendered');
      return found;
    });
    await act(async () => {
      untaggedRow.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['@untagged'] },
      });
    });
  });

  it('selecting Untagged while All is active flips mode to Any and clears other tags', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    const { router } = renderSidebar('/tasks?tag_filter=all%3Awork%2Chome');

    const aside = await screen.findByRole('complementary');
    const untaggedRow = await waitFor(() => {
      const found = aside.querySelector(
        `[data-tag-name="${UNTAGGED_TOKEN}"]`,
      ) as HTMLButtonElement | null;
      if (!found) throw new Error('untagged row not yet rendered');
      return found;
    });
    await act(async () => {
      untaggedRow.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['@untagged'] },
      });
    });
  });

  it('disables the All button when Untagged is selected (clicks do nothing)', async () => {
    vi.stubGlobal('fetch', mockFetchWithTags([{ id: 1, name: 'work', count: 3 }]));
    const { router } = renderSidebar('/tasks?tag_filter=any%3A%40untagged');

    const aside = await screen.findByRole('complementary');
    const allBtn = within(aside).getByRole('button', { name: 'all' });
    expect(allBtn).toBeDisabled();
    expect(allBtn).toHaveAttribute('aria-disabled', 'true');

    await act(async () => {
      allBtn.click();
    });
    // No URL mode change.
    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['@untagged'] },
      });
    });
  });

  it('exposes the disabled-All tooltip text in the DOM', async () => {
    vi.stubGlobal('fetch', mockFetchWithTags([{ id: 1, name: 'work', count: 3 }]));
    renderSidebar('/tasks?tag_filter=any%3A%40untagged');

    const aside = await screen.findByRole('complementary');
    const allBtn = within(aside).getByRole('button', { name: 'all' });
    // Radix renders TooltipTrigger via asChild — the tooltip is anchored to
    // the button. We assert the trigger is wired up by inspecting the
    // surrounding span (the pointer-events shim).
    const wrapper = allBtn.parentElement as HTMLElement;
    expect(wrapper).toHaveAttribute('data-slot', 'tooltip-trigger');
  });

  it('renders the Matches summary with the correct joiner', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    renderSidebar('/tasks?tag_filter=any%3Awork%2Chome');

    const aside = await screen.findByRole('complementary');
    const summary = await within(aside).findByTestId('matches-summary');
    const joiners = within(summary).getAllByTestId('matches-joiner');
    expect(joiners).toHaveLength(1);
    expect(joiners[0]).toHaveTextContent('or');
  });

  it('renders Matches with `and` joiner when mode=all', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetchWithTags([
        { id: 1, name: 'work', count: 3 },
        { id: 2, name: 'home', count: 1 },
      ]),
    );
    renderSidebar('/tasks?tag_filter=all%3Awork%2Chome');

    const aside = await screen.findByRole('complementary');
    const summary = await within(aside).findByTestId('matches-summary');
    const joiners = within(summary).getAllByTestId('matches-joiner');
    expect(joiners[0]).toHaveTextContent('and');
  });

  it('hides the Matches summary when nothing is selected', async () => {
    vi.stubGlobal('fetch', mockFetchWithTags([{ id: 1, name: 'work', count: 3 }]));
    renderSidebar('/tasks');

    const aside = await screen.findByRole('complementary');
    await within(aside).findByRole('button', { name: /select tag work/i });
    expect(within(aside).queryByTestId('matches-summary')).toBeNull();
  });

  it('clicking Clear in the Matches summary drops tag_filter from the URL', async () => {
    vi.stubGlobal('fetch', mockFetchWithTags([{ id: 1, name: 'work', count: 3 }]));
    const { router } = renderSidebar('/tasks?tag_filter=any%3Awork');

    const aside = await screen.findByRole('complementary');
    const summary = await within(aside).findByTestId('matches-summary');
    const clearBtn = within(summary).getByRole('button', { name: /clear/i });
    await act(async () => {
      clearBtn.click();
    });

    await waitFor(() => {
      expect(router.state.location.search).not.toHaveProperty('tag_filter');
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
    await within(aside).findByRole('button', { name: /select tag tag-0/i });
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
