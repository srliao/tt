import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/task';
import { BulkActionBar } from './bulk-action-bar';
import type { UseSelectionResult } from './use-selection';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function wrap(children: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

function makeSelection(ids: number[], offScreen = 0): UseSelectionResult {
  const set = new Set(ids);
  return {
    selected: set,
    visibleCount: ids.length - offScreen,
    offScreenCount: offScreen,
    toggle: vi.fn(),
    add: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
    setAll: vi.fn(),
    has: (id: number) => set.has(id),
  };
}

function makeTask(id: number, overrides: Partial<Task> = {}): Task {
  return {
    id,
    title: `task ${id}`,
    notes: '',
    state: 'not_done',
    due_date: null,
    priority: 0,
    staged_order: null,
    spawned_by_script_id: null,
    created_at: '2026-01-01T00:00:00Z',
    completed_at: null,
    cancelled_at: null,
    updated_at: '2026-01-01T00:00:00Z',
    tags: [],
    ...overrides,
  };
}

describe('BulkActionBar', () => {
  afterEach(() => vi.restoreAllMocks());

  it('is hidden when nothing is selected', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    render(
      wrap(<BulkActionBar selection={makeSelection([])} filter={{}} onOpenTagEditor={() => {}} />),
    );
    expect(screen.queryByRole('region', { name: 'Bulk actions' })).toBeNull();
  });

  it('renders the counter without off-screen badge when offScreenCount is 0', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    render(
      wrap(
        <BulkActionBar
          selection={makeSelection([1, 2, 3])}
          filter={{}}
          onOpenTagEditor={() => {}}
        />,
      ),
    );
    const region = screen.getByRole('region', { name: 'Bulk actions' });
    expect(region).toHaveTextContent('3');
    expect(region).toHaveTextContent('selected');
    expect(region).not.toHaveTextContent('off-screen');
  });

  it('renders the off-screen badge when offScreenCount > 0', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    render(
      wrap(
        <BulkActionBar
          selection={makeSelection([1, 2, 3, 4, 5], 2)}
          filter={{}}
          onOpenTagEditor={() => {}}
        />,
      ),
    );
    const region = screen.getByRole('region', { name: 'Bulk actions' });
    expect(region).toHaveTextContent('off-screen');
    expect(region).toHaveTextContent('2');
  });

  it('clicking "Tag…" calls onOpenTagEditor', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    const onOpenTagEditor = vi.fn();
    render(
      wrap(
        <BulkActionBar
          selection={makeSelection([1])}
          filter={{}}
          onOpenTagEditor={onOpenTagEditor}
        />,
      ),
    );
    await act(async () => {
      screen.getByRole('button', { name: /Tag…/ }).click();
    });
    expect(onOpenTagEditor).toHaveBeenCalledTimes(1);
  });

  it('"Mark done" fires POST /tasks/:id/state once per selected row', async () => {
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url.endsWith('/tasks') || url.includes('/tasks?')) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(
      wrap(
        <BulkActionBar selection={makeSelection([7, 9])} filter={{}} onOpenTagEditor={() => {}} />,
      ),
    );

    await act(async () => {
      screen.getByRole('button', { name: /Mark done/ }).click();
    });

    const urls = (fetchMock.mock.calls as Array<[string, unknown]>)
      .map(([url]) => url)
      .filter((url) => url.includes('/state'));
    expect(urls).toHaveLength(2);
    expect(urls).toContain('/api/v1/tasks/7/state');
    expect(urls).toContain('/api/v1/tasks/9/state');
  });

  it('"Stage" fires POST /tasks/:id/stage for each selected row', async () => {
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      // useTasks() expects an array; mutations expect any object.
      if (url.endsWith('/tasks') || url.includes('/tasks?')) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(
      wrap(<BulkActionBar selection={makeSelection([3])} filter={{}} onOpenTagEditor={() => {}} />),
    );

    await act(async () => {
      screen.getByRole('button', { name: /Stage/ }).click();
    });

    const urls = (fetchMock.mock.calls as Array<[string, unknown]>)
      .map(([url]) => url)
      .filter((url) => url.includes('/stage'));
    expect(urls).toContain('/api/v1/tasks/3/stage');
  });

  it('clicking Delete with off-screen > 0 shows AlertDialog naming the off-screen count', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    render(
      wrap(
        <BulkActionBar
          selection={makeSelection([1, 2, 3, 4, 5], 2)}
          filter={{}}
          onOpenTagEditor={() => {}}
        />,
      ),
    );

    await act(async () => {
      screen.getByRole('button', { name: 'Delete' }).click();
    });

    // AlertDialog renders an alertdialog role.
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog).toHaveTextContent('5');
    expect(dialog).toHaveTextContent('not visible');
    expect(dialog).toHaveTextContent('2');
  });

  it('"Select all matching · N" appears when filter has unselected tasks', async () => {
    const matching = [makeTask(1), makeTask(2), makeTask(3), makeTask(4)];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(matching));
    vi.stubGlobal('fetch', fetchMock);

    const sel = makeSelection([1]);
    render(wrap(<BulkActionBar selection={sel} filter={{}} onOpenTagEditor={() => {}} />));

    const btn = await screen.findByRole('button', { name: /Select all matching · 4/ });
    await act(async () => {
      btn.click();
    });

    expect(sel.add).toHaveBeenCalledTimes(1);
    const arg = (sel.add as ReturnType<typeof vi.fn>).mock.calls[0][0] as Set<number>;
    expect(arg).toBeInstanceOf(Set);
    expect([...arg].sort()).toEqual([1, 2, 3, 4]);
  });

  it('hides "Select all matching" when every matching task is already selected', async () => {
    const matching = [makeTask(1), makeTask(2)];
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(matching)));

    render(
      wrap(
        <BulkActionBar selection={makeSelection([1, 2])} filter={{}} onOpenTagEditor={() => {}} />,
      ),
    );

    // Let the useTasks query resolve.
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.queryByRole('button', { name: /Select all matching/ })).toBeNull();
  });
});
