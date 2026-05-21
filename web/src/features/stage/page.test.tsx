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
import type { Task } from '@/types/task';
import { StagePage } from './page';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function task(partial: Partial<Task> & { id: number; title: string; staged_order: number }): Task {
  return {
    notes: '',
    state: 'not_done',
    due_date: null,
    priority: 0,
    spawned_by_script_id: null,
    created_at: '2026-05-01T00:00:00Z',
    completed_at: null,
    cancelled_at: null,
    updated_at: '2026-05-01T00:00:00Z',
    tags: [],
    ...partial,
  };
}

function renderPage(initial = '/stage') {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const stageRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/stage',
    component: StagePage,
  });
  // Required so <Link to="/tasks" /> in the empty state / top bar can resolve.
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    component: () => null,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([stageRoute, tasksRoute]),
    history: createMemoryHistory({ initialEntries: [initial] }),
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

describe('StagePage', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it('shows the empty state when no tasks are staged', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([]))),
    );
    renderPage();
    expect(
      await screen.findByText(/Pick a few tasks from your list to focus on now\./),
    ).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Go to tasks' })).toBeTruthy();
  });

  it('renders staged rows and a count', async () => {
    const staged = [
      task({ id: 1, title: 'First', staged_order: 1 }),
      task({ id: 2, title: 'Second', staged_order: 2 }),
    ];
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse(staged))),
    );
    renderPage();
    expect(await screen.findByText('First', undefined, { timeout: 2000 })).toBeTruthy();
    expect(screen.getByText('Second')).toBeTruthy();
    expect(screen.getByTestId('stage-count').textContent).toContain('2 staged');
  });

  it('"Clear finished" calls DELETE /stage/finished without a confirm', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') return Promise.resolve(new Response(null, { status: 204 }));
      return Promise.resolve(
        jsonResponse([task({ id: 1, title: 'A', state: 'done', staged_order: 1 })]),
      );
    });
    vi.stubGlobal('fetch', fetchMock);

    renderPage();
    // Wait until the staged task is visible — the "Clear finished" button is
    // disabled while the list is empty.
    await screen.findByText('A');
    const button = await waitFor(() => {
      const b = screen.getByRole('button', { name: 'Clear finished' }) as HTMLButtonElement;
      if (b.disabled) throw new Error('still disabled');
      return b;
    });
    await act(async () => {
      fireEvent.click(button);
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/stage/finished',
        expect.objectContaining({ method: 'DELETE' }),
      );
    });
  });

  it('"Clear stage" only fires after the confirm is accepted', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/v1/stage' && init?.method === 'DELETE') {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(jsonResponse([task({ id: 1, title: 'A', staged_order: 1 })]));
    });
    vi.stubGlobal('fetch', fetchMock);

    renderPage();
    await screen.findByText('A');
    const trigger = await waitFor(() => {
      const b = screen.getByRole('button', { name: 'Clear stage' }) as HTMLButtonElement;
      if (b.disabled) throw new Error('still disabled');
      return b;
    });

    // Open the confirm — at this point no DELETE /stage call should have fired.
    await act(async () => {
      fireEvent.click(trigger);
    });
    expect(
      fetchMock.mock.calls.find(
        ([url, init]) =>
          url === '/api/v1/stage' && (init as RequestInit | undefined)?.method === 'DELETE',
      ),
    ).toBeFalsy();

    // The AlertDialog body renders into a portal; query the whole document for it.
    const confirm = await waitFor(() => {
      const buttons = Array.from(document.querySelectorAll('button')) as HTMLButtonElement[];
      // The dialog has a "Clear stage" action button distinct from the trigger.
      const candidate = buttons.find(
        (b) => b.textContent?.trim() === 'Clear stage' && b !== trigger,
      );
      if (!candidate) throw new Error('confirm not rendered');
      return candidate;
    });

    await act(async () => {
      fireEvent.click(confirm);
    });

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.find(
          ([url, init]) =>
            url === '/api/v1/stage' && (init as RequestInit | undefined)?.method === 'DELETE',
        ),
      ).toBeTruthy();
    });
  });

  it('shows the soft-cap hint when staged count > 7', async () => {
    const many = Array.from({ length: 8 }, (_, i) =>
      task({ id: i + 1, title: `T${i + 1}`, staged_order: i + 1 }),
    );
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse(many))),
    );
    renderPage();
    expect(await screen.findByTestId('stage-soft-cap-hint')).toBeTruthy();
  });

  it('does not show the soft-cap hint at exactly 7', async () => {
    const many = Array.from({ length: 7 }, (_, i) =>
      task({ id: i + 1, title: `T${i + 1}`, staged_order: i + 1 }),
    );
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse(many))),
    );
    renderPage();
    await screen.findByText('T1');
    expect(screen.queryByTestId('stage-soft-cap-hint')).toBeNull();
  });
});
