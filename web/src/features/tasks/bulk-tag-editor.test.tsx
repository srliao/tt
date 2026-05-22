/**
 * Tests for <BulkTagEditor>.
 *
 * Renders the editor with a controlled `open` prop; the anchor is a stub
 * button so the Radix popover has somewhere to mount. fetch is stubbed —
 * the tag list endpoint returns a known shape, mutations are recorded.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, waitFor } from '@testing-library/react';
import { createRef } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
import type { Task } from '@/types/task';
import { BulkTagEditor } from './bulk-tag-editor';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function task(partial: Partial<Task> & { id: number }): Task {
  return {
    id: partial.id,
    title: partial.title ?? `task ${partial.id}`,
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
    ...partial,
  };
}

interface HarnessOpts {
  selectedTasks: Task[];
  onOpenChange?: (next: boolean) => void;
  onApplied?: () => void;
  /** Tags returned by `/tags?counts=1`. Default: derived from selected tasks. */
  availableTags?: { name: string; count?: number }[];
}

function renderEditor(opts: HarnessOpts) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const anchorRef = createRef<HTMLButtonElement>();
  const onOpenChange = opts.onOpenChange ?? vi.fn();
  const utils = render(
    <ThemeProvider>
      <QueryClientProvider client={qc}>
        <button ref={anchorRef} type="button">
          Anchor
        </button>
        <BulkTagEditor
          selectedTasks={opts.selectedTasks}
          open
          onOpenChange={onOpenChange}
          anchorRef={anchorRef as React.RefObject<HTMLElement | null>}
          onApplied={opts.onApplied}
        />
      </QueryClientProvider>
    </ThemeProvider>,
  );
  return { ...utils, onOpenChange };
}

function findInput(): HTMLInputElement {
  const el = document.querySelector('[data-slot="bulk-tag-editor-input"]');
  if (!el) throw new Error('editor input missing');
  return el as HTMLInputElement;
}

async function waitForEditor() {
  await waitFor(() => {
    if (!document.querySelector('[data-slot="bulk-tag-editor"]')) {
      throw new Error('editor not mounted');
    }
  });
}

describe('<BulkTagEditor>', () => {
  afterEach(() => vi.restoreAllMocks());

  it('computes tri-state from selectedTasks: shared tag → all, partial → some', async () => {
    // Selection: ['a','b'], ['b'], ['b','c']  → b=all(3), a=some(1), c=some(1).
    const tags = [
      { name: 'a', count: 1 },
      { name: 'b', count: 3 },
      { name: 'c', count: 1 },
    ];
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(tags)));

    renderEditor({
      selectedTasks: [
        task({ id: 1, tags: ['a', 'b'] }),
        task({ id: 2, tags: ['b'] }),
        task({ id: 3, tags: ['b', 'c'] }),
      ],
    });

    await waitForEditor();

    const itemA = await waitFor(() => {
      const el = document.querySelector('[data-slot="bulk-tag-editor-item"][data-tag-name="a"]');
      if (!el) throw new Error('a row missing');
      return el as HTMLElement;
    });
    const itemB = document.querySelector(
      '[data-slot="bulk-tag-editor-item"][data-tag-name="b"]',
    ) as HTMLElement | null;
    const itemC = document.querySelector(
      '[data-slot="bulk-tag-editor-item"][data-tag-name="c"]',
    ) as HTMLElement | null;
    expect(itemB?.getAttribute('data-presence')).toBe('all');
    expect(itemA.getAttribute('data-presence')).toBe('some');
    expect(itemC?.getAttribute('data-presence')).toBe('some');

    expect(itemA.textContent).toContain('1 of 3');
    expect(itemB?.textContent).toContain('all 3');
  });

  it('selecting an indeterminate row stages it as a chip', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([{ name: 'a', count: 1 }])));

    renderEditor({
      selectedTasks: [task({ id: 1, tags: ['a'] }), task({ id: 2, tags: [] })],
    });
    await waitForEditor();

    const row = await waitFor(() => {
      const el = document.querySelector('[data-slot="bulk-tag-editor-item"][data-tag-name="a"]');
      if (!el) throw new Error('row missing');
      return el as HTMLElement;
    });
    expect(row.getAttribute('data-presence')).toBe('some');

    await act(async () => {
      row.click();
    });

    const staged = document.querySelector('[data-slot="bulk-tag-editor-staged"]');
    expect(staged).not.toBeNull();
    expect(staged?.textContent).toContain('a');
    expect(staged?.textContent).toContain('1 staged');
  });

  it('Apply (⌘↵) calls bulkTag mutation with {ids, op, tags}', async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({ url, init });
      if (url === '/api/v1/tasks/bulk-tag' && init?.method === 'POST') {
        const body = JSON.parse(init.body as string) as {
          ids: number[];
          tags: string[];
        };
        return Promise.resolve(jsonResponse(body.ids.map((id) => task({ id, tags: body.tags }))));
      }
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);

    const onOpenChange = vi.fn();
    renderEditor({
      selectedTasks: [task({ id: 10, tags: [] }), task({ id: 11, tags: [] })],
      onOpenChange,
    });
    await waitForEditor();

    // Type a new tag name + create.
    const input = findInput();
    act(() => {
      input.focus();
      fireEvent.change(input, { target: { value: 'newtag' } });
    });
    const createRow = await waitFor(() => {
      const el = document.querySelector('[data-slot="bulk-tag-editor-create"]');
      if (!el) throw new Error('create row missing');
      return el as HTMLElement;
    });
    await act(async () => {
      createRow.click();
    });

    // Apply via ⌘↵.
    const popover = document.querySelector('[data-slot="bulk-tag-editor"]') as HTMLElement;
    await act(async () => {
      fireEvent.keyDown(popover, { key: 'Enter', metaKey: true });
    });

    // One bulk-tag POST carries every id + the staged tag list.
    await waitFor(() => {
      const bulkCalls = calls.filter(
        (c) => c.url === '/api/v1/tasks/bulk-tag' && c.init?.method === 'POST',
      );
      expect(bulkCalls.length).toBe(1);
    });

    const bulk = calls.find((c) => c.url === '/api/v1/tasks/bulk-tag');
    const body = JSON.parse(bulk?.init?.body as string);
    expect(body.ids.sort()).toEqual([10, 11]);
    expect(body.op).toBe('add');
    expect(body.tags).toEqual(['newtag']);

    // Editor closed via onOpenChange(false).
    await waitFor(() => {
      const lastCall = onOpenChange.mock.calls[onOpenChange.mock.calls.length - 1];
      expect(lastCall?.[0]).toBe(false);
    });
  });

  it('Esc closes the editor and discards staged tags', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    const onOpenChange = vi.fn();
    renderEditor({
      selectedTasks: [task({ id: 1, tags: [] })],
      onOpenChange,
    });
    await waitForEditor();

    const input = findInput();
    act(() => {
      input.focus();
      fireEvent.change(input, { target: { value: 'staging' } });
    });
    // Stage via the create row.
    const createRow = await waitFor(() => {
      const el = document.querySelector('[data-slot="bulk-tag-editor-create"]');
      if (!el) throw new Error('create row missing');
      return el as HTMLElement;
    });
    await act(async () => {
      createRow.click();
    });
    expect(document.querySelector('[data-slot="bulk-tag-editor-staged"]')).not.toBeNull();

    // Esc inside the input — fires onOpenChange(false).
    act(() => {
      fireEvent.keyDown(input, { key: 'Escape' });
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it('Set mode requires the confirm checkbox before Apply enables', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    renderEditor({ selectedTasks: [task({ id: 1, tags: ['old'] })] });
    await waitForEditor();

    // Switch to Set mode.
    const setTab = document.querySelector(
      '[data-slot="bulk-tag-editor-mode"][data-mode="set"]',
    ) as HTMLButtonElement | null;
    expect(setTab).not.toBeNull();
    await act(async () => {
      setTab?.click();
    });

    // Apply should be disabled until the confirm checkbox is ticked.
    const apply = document.querySelector(
      '[data-slot="bulk-tag-editor-apply"]',
    ) as HTMLButtonElement | null;
    expect(apply).not.toBeNull();
    expect(apply?.disabled).toBe(true);

    const confirm = document.querySelector(
      '[data-slot="bulk-tag-editor-set-confirm"] input',
    ) as HTMLInputElement | null;
    expect(confirm).not.toBeNull();
    await act(async () => {
      if (confirm) {
        confirm.click();
      }
    });

    expect(apply?.disabled).toBe(false);
  });

  it('Set + empty staged + confirm = clear-all-tags', async () => {
    // Set mode with empty staged + confirm should call the bulk endpoint
    // with op:'set', tags:[] so every selected task ends up tagless.
    const calls: { url: string; init?: RequestInit }[] = [];
    const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({ url, init });
      if (url === '/api/v1/tasks/bulk-tag' && init?.method === 'POST') {
        const body = JSON.parse(init.body as string) as { ids: number[] };
        return Promise.resolve(jsonResponse(body.ids.map((id) => task({ id, tags: [] }))));
      }
      return Promise.resolve(jsonResponse([{ name: 'legacy', count: 2 }]));
    });
    vi.stubGlobal('fetch', fetchMock);

    const onOpenChange = vi.fn();
    renderEditor({
      selectedTasks: [
        task({ id: 20, tags: ['legacy'] }),
        task({ id: 21, tags: ['legacy', 'extra'] }),
      ],
      onOpenChange,
    });
    await waitForEditor();

    // Switch to Set mode.
    const setTab = document.querySelector(
      '[data-slot="bulk-tag-editor-mode"][data-mode="set"]',
    ) as HTMLButtonElement | null;
    await act(async () => {
      setTab?.click();
    });

    // Tick the confirm checkbox without staging anything.
    const confirm = document.querySelector(
      '[data-slot="bulk-tag-editor-set-confirm"] input',
    ) as HTMLInputElement | null;
    expect(confirm).not.toBeNull();
    await act(async () => {
      confirm?.click();
    });

    // No staged chips strip should be present.
    expect(document.querySelector('[data-slot="bulk-tag-editor-staged"]')).toBeNull();

    // Apply — should be enabled because Set requires only the confirm.
    const apply = document.querySelector(
      '[data-slot="bulk-tag-editor-apply"]',
    ) as HTMLButtonElement | null;
    expect(apply?.disabled).toBe(false);
    await act(async () => {
      apply?.click();
    });

    await waitFor(() => {
      const bulkCalls = calls.filter(
        (c) => c.url === '/api/v1/tasks/bulk-tag' && c.init?.method === 'POST',
      );
      expect(bulkCalls.length).toBe(1);
    });

    const bulk = calls.find((c) => c.url === '/api/v1/tasks/bulk-tag');
    const body = JSON.parse(bulk?.init?.body as string);
    expect(body.ids.sort()).toEqual([20, 21]);
    expect(body.op).toBe('set');
    expect(body.tags).toEqual([]);

    await waitFor(() => {
      const lastCall = onOpenChange.mock.calls[onOpenChange.mock.calls.length - 1];
      expect(lastCall?.[0]).toBe(false);
    });
  });

  it('error path keeps editor open and staged chips intact, then Esc closes', async () => {
    // bulk-tag rejects with a 500 envelope. After the failed Apply the
    // popover must remain mounted, the staged chip still visible, and an
    // error message must render.
    const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/v1/tasks/bulk-tag' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse({ error: { code: 'internal', message: 'boom' } }, 500));
      }
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);

    const onOpenChange = vi.fn();
    renderEditor({
      selectedTasks: [task({ id: 30, tags: [] })],
      onOpenChange,
    });
    await waitForEditor();

    // Stage a tag via the create row.
    const input = findInput();
    act(() => {
      input.focus();
      fireEvent.change(input, { target: { value: 'failtag' } });
    });
    const createRow = await waitFor(() => {
      const el = document.querySelector('[data-slot="bulk-tag-editor-create"]');
      if (!el) throw new Error('create row missing');
      return el as HTMLElement;
    });
    await act(async () => {
      createRow.click();
    });

    expect(document.querySelector('[data-slot="bulk-tag-editor-staged"]')).not.toBeNull();

    // Trigger Apply via ⌘↵.
    const popover = document.querySelector('[data-slot="bulk-tag-editor"]') as HTMLElement;
    await act(async () => {
      fireEvent.keyDown(popover, { key: 'Enter', metaKey: true });
    });

    // Error message appears.
    await waitFor(() => {
      const err = document.querySelector('[data-slot="bulk-tag-editor-error"]');
      if (!err) throw new Error('error not rendered');
      expect(err.textContent ?? '').toMatch(/./);
    });

    // (i) onOpenChange(false) was NOT called.
    expect(onOpenChange.mock.calls.some(([next]) => next === false)).toBe(false);
    // (ii) Editor is still mounted.
    expect(document.querySelector('[data-slot="bulk-tag-editor"]')).not.toBeNull();
    // (iii) Staged chip is still visible.
    const staged = document.querySelector('[data-slot="bulk-tag-editor-staged"]');
    expect(staged).not.toBeNull();
    expect(staged?.textContent).toContain('failtag');

    // Pressing Esc inside the input closes the popover.
    const input2 = findInput();
    act(() => {
      fireEvent.keyDown(input2, { key: 'Escape' });
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it('Paste with commas splits and stages all parts', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    renderEditor({ selectedTasks: [task({ id: 1, tags: [] })] });
    await waitForEditor();

    const input = findInput();
    act(() => {
      input.focus();
      // jsdom's clipboard event needs a getData implementation.
      fireEvent.paste(input, {
        clipboardData: { getData: () => 'x, y, z' },
      });
    });

    const staged = await waitFor(() => {
      const el = document.querySelector('[data-slot="bulk-tag-editor-staged"]');
      if (!el) throw new Error('staged chips missing');
      return el as HTMLElement;
    });
    expect(staged.textContent).toContain('x');
    expect(staged.textContent).toContain('y');
    expect(staged.textContent).toContain('z');
    expect(staged.textContent).toContain('3 staged');
  });
});
