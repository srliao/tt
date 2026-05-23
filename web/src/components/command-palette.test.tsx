import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
import { __resetSelectionStoreForTests } from '@/features/tasks/use-selection';
import { taskSearchSchema } from '@/features/tasks/use-task-list-search';
import { CommandPalette } from './command-palette';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function makeFetchMock(
  opts: {
    tasks?: Array<{ id: number; title: string }>;
    tags?: Array<{ id: number; name: string; count: number }>;
  } = {},
) {
  const tasks = (opts.tasks ?? []).map((t) => ({
    id: t.id,
    title: t.title,
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
  }));
  const tags = opts.tags ?? [];
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (url.includes('/tags')) return jsonResponse(tags);
    if (url.includes('/tasks')) return jsonResponse(tasks);
    return jsonResponse([]);
  });
}

function renderPalette(opts: Parameters<typeof makeFetchMock>[0] = {}, initial = '/tasks') {
  vi.stubGlobal('fetch', makeFetchMock(opts));

  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    validateSearch: (s) => taskSearchSchema.parse(s),
    component: () => (
      <>
        <table data-task-table tabIndex={-1}>
          <tbody>
            <tr>
              <td>table</td>
            </tr>
          </tbody>
        </table>
        <CommandPalette />
      </>
    ),
  });
  const stageRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/stage',
    component: () => (
      <>
        <div>stage page</div>
        <CommandPalette />
      </>
    ),
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([tasksRoute, stageRoute]),
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

function dispatchKey(target: EventTarget, init: KeyboardEventInit) {
  // Use a real KeyboardEvent dispatched at the target so it bubbles up to the
  // palette's `document.addEventListener('keydown')`. `fireEvent.keyDown`
  // does this for us via Testing Library's wrapper.
  fireEvent.keyDown(target, init);
}

describe('CommandPalette', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    // Selection is sessionStorage-backed AND has a module-level store
    // cache; wipe both between tests so Bulk-group state from one test
    // doesn't leak into another.
    sessionStorage.clear();
    __resetSelectionStoreForTests();
  });

  // The palette attaches its keydown listener inside a useEffect, so we have
  // to wait for the first effect pass before dispatching keys. The sr-only
  // DialogHeader is rendered unconditionally and gives us a cheap mounted
  // signal independent of the dialog's open state.
  const waitForMount = () => screen.findByText('Command palette');

  it('opens on `/` when nothing is focused', async () => {
    renderPalette({ tasks: [{ id: 1, title: 'Alpha' }] });
    await waitForMount();

    await act(async () => {
      dispatchKey(document.body, { key: '/' });
    });

    expect(await screen.findByPlaceholderText(/Search tasks, tags/)).toBeInTheDocument();
  });

  it('opens on ⌘K', async () => {
    renderPalette();
    await waitForMount();
    await act(async () => {
      dispatchKey(document.body, { key: 'k', metaKey: true });
    });
    expect(await screen.findByPlaceholderText(/Search tasks, tags/)).toBeInTheDocument();
  });

  it('does not open on `/` when the user is typing in an input', async () => {
    renderPalette();
    await waitForMount();
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    await act(async () => {
      dispatchKey(input, { key: '/' });
    });

    // Wait a tick to be sure the dialog didn't open asynchronously.
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByPlaceholderText(/Search tasks, tags/)).toBeNull();
    input.remove();
  });

  it('closes on Escape', async () => {
    renderPalette();
    await waitForMount();
    await act(async () => {
      dispatchKey(document.body, { key: '/' });
    });
    const input = await screen.findByPlaceholderText(/Search tasks, tags/);
    expect(input).toBeInTheDocument();

    await act(async () => {
      fireEvent.keyDown(input, { key: 'Escape' });
    });

    await waitFor(() => {
      expect(screen.queryByPlaceholderText(/Search tasks, tags/)).toBeNull();
    });
  });

  it('⌘↵ applies the typed query as a ?q= filter', async () => {
    const { router } = renderPalette();
    await waitForMount();
    await act(async () => {
      dispatchKey(document.body, { key: '/' });
    });
    const input = await screen.findByPlaceholderText(/Search tasks, tags/);
    await act(async () => {
      fireEvent.change(input, { target: { value: 'urgent' } });
    });
    await act(async () => {
      fireEvent.keyDown(input, { key: 'Enter', metaKey: true });
    });

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({ q: 'urgent' });
    });
  });

  it('selecting a task navigates to /tasks with ?open=<id>', async () => {
    const { router } = renderPalette({ tasks: [{ id: 42, title: 'Alpha task' }] }, '/stage');

    await waitForMount();
    await act(async () => {
      dispatchKey(document.body, { key: '/' });
    });
    const input = await screen.findByPlaceholderText(/Search tasks, tags/);
    await act(async () => {
      fireEvent.change(input, { target: { value: 'Alpha' } });
    });

    // The task result is rendered as a CommandItem (role="option"). Match by
    // accessible name so we don't trip on the highlight() wrapper splitting
    // the title across <mark> and surrounding text nodes.
    const taskItem = await screen.findByRole('option', { name: /Alpha task/i });
    await act(async () => {
      fireEvent.click(taskItem);
    });

    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/tasks');
      expect(router.state.location.search).toMatchObject({ open: 42 });
    });
  });

  it('selecting a tag navigates to /tasks with tag_filter=any:<name>', async () => {
    const { router } = renderPalette(
      {
        tags: [
          { id: 1, name: 'work', count: 2 },
          { id: 2, name: 'home', count: 1 },
        ],
      },
      '/stage',
    );

    await waitForMount();
    await act(async () => {
      dispatchKey(document.body, { key: '/' });
    });
    const input = await screen.findByPlaceholderText(/Search tasks, tags/);
    await act(async () => {
      fireEvent.change(input, { target: { value: 'work' } });
    });

    // Two CommandItems match the "work" query: the "Show only tasks matching"
    // filter item, and the "work" tag chip. Disambiguate by data-value, which
    // uniquely identifies the tag entry as `tag-work`.
    const tagItem = await waitFor(() => {
      const el = document.querySelector('[data-value="tag-work"]');
      if (!el) throw new Error('tag-work item not found yet');
      return el as HTMLElement;
    });
    await act(async () => {
      fireEvent.click(tagItem);
    });

    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/tasks');
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['work'] },
      });
    });
  });

  it('typing "untag" surfaces Untagged at the top of the Tags group and selecting it sets tag_filter to any:@untagged', async () => {
    const { router } = renderPalette(
      {
        tags: [
          { id: 1, name: 'work', count: 2 },
          { id: 2, name: 'home', count: 1 },
        ],
      },
      '/stage',
    );

    await waitForMount();
    await act(async () => {
      dispatchKey(document.body, { key: '/' });
    });
    const input = await screen.findByPlaceholderText(/Search tasks, tags/);
    await act(async () => {
      fireEvent.change(input, { target: { value: 'untag' } });
    });

    // Only the synthetic Untagged entry should appear under "Tags · 0"
    // (no real tag named "untag" exists in the mock). The CommandItem's
    // value="untagged" wins it the role=option accessible name.
    const untaggedItem = await screen.findByRole('option', { name: /untagged/i });
    await act(async () => {
      fireEvent.click(untaggedItem);
    });

    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/tasks');
      expect(router.state.location.search).toMatchObject({
        tag_filter: { mode: 'any', tags: ['@untagged'] },
      });
    });
  });

  describe('Bulk group', () => {
    it('does not render the Bulk group when selection is empty', async () => {
      renderPalette({ tasks: [{ id: 1, title: 'Alpha' }] });
      await waitForMount();
      await act(async () => {
        dispatchKey(document.body, { key: '/' });
      });
      await screen.findByPlaceholderText(/Search tasks, tags/);
      expect(screen.queryByText(/Bulk ·/)).toBeNull();
    });

    it('renders the Bulk group with the correct count when selection > 0', async () => {
      sessionStorage.setItem('tt:selection', JSON.stringify([1, 2, 3, 4, 5]));
      __resetSelectionStoreForTests();
      renderPalette({
        tasks: [
          { id: 1, title: 'Alpha' },
          { id: 2, title: 'Beta' },
          { id: 3, title: 'Gamma' },
          { id: 4, title: 'Delta' },
          { id: 5, title: 'Epsilon' },
        ],
      });
      await waitForMount();
      await act(async () => {
        dispatchKey(document.body, { key: '/' });
      });
      await screen.findByPlaceholderText(/Search tasks, tags/);
      expect(await screen.findByText(/Bulk · 5 tasks selected/i)).toBeInTheDocument();
      expect(await screen.findByText(/Tag 5 tasks…/i)).toBeInTheDocument();
      expect(await screen.findByText(/Stage 5 tasks/i)).toBeInTheDocument();
      expect(await screen.findByText(/Mark 5 tasks done/i)).toBeInTheDocument();
    });

    it('clicking "Tag N tasks…" navigates with openBulkTagEditor=1', async () => {
      sessionStorage.setItem('tt:selection', JSON.stringify([1, 2, 3, 4, 5]));
      __resetSelectionStoreForTests();
      const { router } = renderPalette(
        {
          tasks: [
            { id: 1, title: 'Alpha' },
            { id: 2, title: 'Beta' },
            { id: 3, title: 'Gamma' },
            { id: 4, title: 'Delta' },
            { id: 5, title: 'Epsilon' },
          ],
        },
        '/stage',
      );
      await waitForMount();
      await act(async () => {
        dispatchKey(document.body, { key: '/' });
      });
      await screen.findByPlaceholderText(/Search tasks, tags/);
      const tagItem = await screen.findByRole('option', { name: /Tag 5 tasks…/i });
      await act(async () => {
        fireEvent.click(tagItem);
      });
      await waitFor(() => {
        expect(router.state.location.pathname).toBe('/tasks');
        expect(router.state.location.search).toMatchObject({ openBulkTagEditor: true });
      });
    });

    it('clicking "Stage N tasks" fires a stage mutation per id and closes the palette', async () => {
      sessionStorage.setItem('tt:selection', JSON.stringify([1, 2, 3, 4, 5]));
      __resetSelectionStoreForTests();
      // Use a tracking fetch mock so we can assert POSTs to /stage.
      const stageCalls: number[] = [];
      vi.stubGlobal(
        'fetch',
        vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
          const url =
            typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
          const method = init?.method ?? 'GET';
          if (method === 'POST') {
            const match = url.match(/\/tasks\/(\d+)\/stage$/);
            if (match) {
              stageCalls.push(Number(match[1]));
              return jsonResponse({
                id: Number(match[1]),
                title: `task ${match[1]}`,
                notes: '',
                state: 'not_done',
                due_date: null,
                priority: 0,
                staged_order: 1,
                spawned_by_script_id: null,
                created_at: '2026-05-01T00:00:00Z',
                completed_at: null,
                cancelled_at: null,
                updated_at: '2026-05-01T00:00:00Z',
                tags: [],
              });
            }
          }
          if (url.includes('/tags')) return jsonResponse([]);
          if (url.includes('/tasks')) {
            return jsonResponse(
              [1, 2, 3, 4, 5].map((id) => ({
                id,
                title: `task ${id}`,
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
              })),
            );
          }
          return jsonResponse([]);
        }),
      );

      // We're already stubbing fetch above; pass empty opts so renderPalette
      // doesn't overwrite our stub.
      // renderPalette's first line is vi.stubGlobal('fetch', …) — call it with
      // tasks: [] and then re-stub to take precedence.
      const rootRoute = createRootRoute({ component: () => <Outlet /> });
      const tasksRoute = createRoute({
        getParentRoute: () => rootRoute,
        path: '/tasks',
        validateSearch: (s) => taskSearchSchema.parse(s),
        component: () => <CommandPalette />,
      });
      const router = createRouter({
        routeTree: rootRoute.addChildren([tasksRoute]),
        history: createMemoryHistory({ initialEntries: ['/tasks'] }),
      });
      const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
      render(
        <ThemeProvider>
          <QueryClientProvider client={qc}>
            <RouterProvider router={router as never} />
          </QueryClientProvider>
        </ThemeProvider>,
      );
      await waitForMount();
      await act(async () => {
        dispatchKey(document.body, { key: '/' });
      });
      await screen.findByPlaceholderText(/Search tasks, tags/);
      const stageItem = await screen.findByRole('option', { name: /Stage 5 tasks/i });
      await act(async () => {
        fireEvent.click(stageItem);
      });

      await waitFor(() => {
        expect(stageCalls.sort()).toEqual([1, 2, 3, 4, 5]);
      });
      await waitFor(() => {
        expect(screen.queryByPlaceholderText(/Search tasks, tags/)).toBeNull();
      });
    });

    it('clicking "Clear selection" empties selection and closes the palette', async () => {
      sessionStorage.setItem('tt:selection', JSON.stringify([1, 2]));
      __resetSelectionStoreForTests();
      renderPalette({
        tasks: [
          { id: 1, title: 'Alpha' },
          { id: 2, title: 'Beta' },
        ],
      });
      await waitForMount();
      await act(async () => {
        dispatchKey(document.body, { key: '/' });
      });
      await screen.findByPlaceholderText(/Search tasks, tags/);
      const clearItem = await screen.findByRole('option', { name: /Clear selection/i });
      await act(async () => {
        fireEvent.click(clearItem);
      });
      await waitFor(() => {
        expect(screen.queryByPlaceholderText(/Search tasks, tags/)).toBeNull();
      });
      expect(sessionStorage.getItem('tt:selection')).toBeNull();
    });
  });
});
