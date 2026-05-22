/**
 * Tests for <InlineTagEditor>.
 *
 * The editor is mounted with a controlled `task` prop and disappears when
 * it goes back to null. The DOM anchor is resolved on each open via
 * `document.querySelector`, so the tests render a stub row with the
 * expected `data-task-id` / `data-tag-cell` attributes.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
import type { Task } from '@/types/task';
import { InlineTagEditor } from './inline-tag-editor';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function task(partial: Partial<Task> & { id: number; title: string }): Task {
  return {
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

function renderHarness(initial: Task | null, onClose = vi.fn()) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const utils = render(
    <ThemeProvider>
      <QueryClientProvider client={qc}>
        {initial ? (
          <table>
            <tbody>
              <tr data-task-id={initial.id}>
                <td data-tag-cell>cell</td>
              </tr>
            </tbody>
          </table>
        ) : null}
        <InlineTagEditor task={initial} onClose={onClose} />
      </QueryClientProvider>
    </ThemeProvider>,
  );
  return { onClose, ...utils };
}

describe('<InlineTagEditor>', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders nothing when task is null', () => {
    renderHarness(null);
    expect(document.querySelector('[data-slot="inline-tag-editor"]')).toBeNull();
  });

  it('mounts a popover anchored to the row tag cell when a task is provided', async () => {
    // Tag listing returns an empty array — the combobox just needs to render.
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    renderHarness(task({ id: 5, title: 'Hello', tags: ['x'] }));
    await waitFor(() => {
      expect(document.querySelector('[data-slot="inline-tag-editor"]')).not.toBeNull();
    });
  });

  it('Escape inside the combobox commits via PATCH and closes the editor', async () => {
    const fetchMock = vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'PATCH') return Promise.resolve(jsonResponse({}));
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);

    const onClose = vi.fn();
    renderHarness(task({ id: 9, title: 'Tagged', tags: ['old'] }), onClose);
    const editor = await waitFor(() => {
      const el = document.querySelector('[data-slot="inline-tag-editor"]');
      if (!el) throw new Error('editor not mounted');
      return el as HTMLElement;
    });
    // Simulate a tag change by mutating local state through removing the
    // existing chip via Backspace on the empty combobox input.
    const input = editor.querySelector(
      '[data-slot="tag-combobox-input"]',
    ) as HTMLInputElement | null;
    expect(input).not.toBeNull();
    if (!input) throw new Error('input missing');
    act(() => {
      input.focus();
      fireEvent.keyDown(input, { key: 'Backspace' });
    });
    // Now dismiss with Escape.
    act(() => {
      fireEvent.keyDown(input, { key: 'Escape' });
    });
    // Radix dispatches the close synchronously; assert the callback fired.
    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
    // And the PATCH went out with the cleared tag list.
    const patchCalls = fetchMock.mock.calls.filter(
      (c) => (c[1] as RequestInit | undefined)?.method === 'PATCH',
    );
    expect(patchCalls.length).toBeGreaterThanOrEqual(1);
    const body = JSON.parse((patchCalls[0][1] as RequestInit).body as string);
    expect(body.tags).toEqual([]);
  });

  it('keeps the editor open and surfaces an inline error when PATCH fails', async () => {
    // Silence the console.error we emit on the failure path.
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fetchMock = vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'PATCH') {
        return Promise.resolve(jsonResponse({ error: { message: 'server boom' } }, 500));
      }
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);

    const onClose = vi.fn();
    renderHarness(task({ id: 12, title: 'Errors', tags: ['old'] }), onClose);
    const editor = await waitFor(() => {
      const el = document.querySelector('[data-slot="inline-tag-editor"]');
      if (!el) throw new Error('editor not mounted');
      return el as HTMLElement;
    });
    const input = editor.querySelector(
      '[data-slot="tag-combobox-input"]',
    ) as HTMLInputElement | null;
    if (!input) throw new Error('input missing');
    act(() => {
      input.focus();
      fireEvent.keyDown(input, { key: 'Backspace' });
    });
    act(() => {
      fireEvent.keyDown(input, { key: 'Escape' });
    });
    // PATCH went out…
    await waitFor(() => {
      const patchCalls = fetchMock.mock.calls.filter(
        (c) => (c[1] as RequestInit | undefined)?.method === 'PATCH',
      );
      expect(patchCalls.length).toBeGreaterThanOrEqual(1);
    });
    // …but the editor stays open and renders an inline error.
    await waitFor(() => {
      expect(document.querySelector('[data-slot="inline-tag-editor-error"]')).not.toBeNull();
    });
    expect(onClose).not.toHaveBeenCalled();
    errSpy.mockRestore();
  });

  it('closing without changes does NOT send a PATCH', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse([]));
    vi.stubGlobal('fetch', fetchMock);

    const onClose = vi.fn();
    const { rerender } = renderHarness(
      task({ id: 11, title: 'NoChange', tags: ['a', 'b'] }),
      onClose,
    );
    await waitFor(() => {
      expect(document.querySelector('[data-slot="inline-tag-editor"]')).not.toBeNull();
    });
    // Tear down by flipping the task prop to null — emulates onClose path.
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    rerender(
      <ThemeProvider>
        <QueryClientProvider client={qc}>
          <InlineTagEditor task={null} onClose={onClose} />
        </QueryClientProvider>
      </ThemeProvider>,
    );
    const patchCalls = fetchMock.mock.calls.filter(
      (c) => (c[1] as RequestInit | undefined)?.method === 'PATCH',
    );
    expect(patchCalls.length).toBe(0);
  });
});
