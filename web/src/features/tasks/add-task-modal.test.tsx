import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
import { AddTaskModal } from './add-task-modal';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function Wrapper({ children }: { children: ReactNode }) {
  const [qc] = useState(
    () =>
      new QueryClient({
        defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
      }),
  );
  return (
    <ThemeProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </ThemeProvider>
  );
}

function Harness({ stageAfterCreate }: { stageAfterCreate?: boolean } = {}) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <AddTaskModal open={open} onOpenChange={setOpen} stageAfterCreate={stageAfterCreate} />
      <span data-testid="open-state">{open ? 'open' : 'closed'}</span>
    </>
  );
}

describe('AddTaskModal', () => {
  afterEach(() => vi.restoreAllMocks());

  it('exposes a title input plus an "Add tags" disclosure — no other fields or footer buttons', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      <Wrapper>
        <Harness />
      </Wrapper>,
    );

    await screen.findByLabelText('New task title');
    expect(screen.queryByLabelText('Notes')).toBeNull();
    expect(screen.queryByLabelText('Due date')).toBeNull();
    expect(screen.queryByRole('button', { name: '+ Add tags' })).not.toBeNull();
    expect(screen.queryByRole('button', { name: 'Cancel' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Create task' })).toBeNull();
    // The combobox should not be visible until the user clicks the disclosure.
    expect(document.querySelector('[data-slot="tag-combobox-trigger"]')).toBeNull();
  });

  it('POSTs only the title when Enter is pressed', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ id: 1, title: 'Buy milk' }, 201));
    vi.stubGlobal('fetch', fetchMock);

    render(
      <Wrapper>
        <Harness />
      </Wrapper>,
    );

    const input = await screen.findByLabelText('New task title');
    await act(async () => {
      fireEvent.change(input, { target: { value: 'Buy milk' } });
    });
    const form = input.closest('form');
    if (!form) throw new Error('expected the input to be in a form');
    await act(async () => {
      fireEvent.submit(form);
    });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/tasks',
        expect.objectContaining({ method: 'POST' }),
      );
    });
    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body).toEqual({ title: 'Buy milk' });
    await waitFor(() => {
      expect(screen.getByTestId('open-state').textContent).toBe('closed');
    });
  });

  it('does nothing when Enter is pressed on an empty / whitespace-only title', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    render(
      <Wrapper>
        <Harness />
      </Wrapper>,
    );

    const input = await screen.findByLabelText('New task title');
    await act(async () => {
      fireEvent.change(input, { target: { value: '   ' } });
    });
    const form = input.closest('form');
    if (!form) throw new Error('expected the input to be in a form');
    await act(async () => {
      fireEvent.submit(form);
    });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByTestId('open-state').textContent).toBe('open');
  });

  it('reveals the tag combobox when "+ Add tags" is clicked and submits tags', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (String(url).includes('/tags') && (!init || init.method === undefined)) {
        return Promise.resolve(
          jsonResponse([{ id: 1, name: 'home', count: 0, created_at: '2026-05-01T00:00:00Z' }]),
        );
      }
      if (String(url) === '/api/v1/tasks' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse({ id: 7, title: 'Buy milk' }, 201));
      }
      return Promise.resolve(jsonResponse({}));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <Wrapper>
        <Harness />
      </Wrapper>,
    );

    const titleInput = await screen.findByLabelText('New task title');
    await act(async () => {
      fireEvent.change(titleInput, { target: { value: 'Buy milk' } });
    });

    // Click "+ Add tags" to reveal the combobox
    const disclosure = screen.getByRole('button', { name: '+ Add tags' });
    await act(async () => {
      fireEvent.click(disclosure);
    });

    // Combobox is now mounted
    const comboInput = document.querySelector(
      '[data-slot="tag-combobox-input"]',
    ) as HTMLInputElement | null;
    expect(comboInput).not.toBeNull();
    act(() => {
      (comboInput as HTMLInputElement).focus();
    });

    // Click the 'home' option
    const homeOption = await screen.findByRole('option', { name: /home/i });
    await act(async () => {
      fireEvent.click(homeOption);
    });

    // Submit the form via the title input
    const form = titleInput.closest('form');
    if (!form) throw new Error('expected the input to be in a form');
    await act(async () => {
      fireEvent.submit(form);
    });

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([url, init]) =>
            String(url) === '/api/v1/tasks' && (init as RequestInit | undefined)?.method === 'POST',
        ),
      ).toBe(true);
    });
    const postCall = fetchMock.mock.calls.find(
      ([url, init]) =>
        String(url) === '/api/v1/tasks' && (init as RequestInit | undefined)?.method === 'POST',
    ) as [string, RequestInit];
    const body = JSON.parse(postCall[1].body as string);
    expect(body).toEqual({ title: 'Buy milk', tags: ['home'] });
  });

  it('stages the new task when stageAfterCreate is set', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/v1/tasks' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse({ id: 42, title: 'Buy milk' }, 201));
      }
      if (url === '/api/v1/tasks/42/stage' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse({ id: 42, title: 'Buy milk', staged_order: 1 }));
      }
      return Promise.resolve(jsonResponse({}));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <Wrapper>
        <Harness stageAfterCreate />
      </Wrapper>,
    );

    const input = await screen.findByLabelText('New task title');
    await act(async () => {
      fireEvent.change(input, { target: { value: 'Buy milk' } });
    });
    const form = input.closest('form');
    if (!form) throw new Error('expected the input to be in a form');
    await act(async () => {
      fireEvent.submit(form);
    });

    await waitFor(() => {
      const stageCall = fetchMock.mock.calls.find(
        ([url, init]) =>
          url === '/api/v1/tasks/42/stage' && (init as RequestInit | undefined)?.method === 'POST',
      );
      expect(stageCall).toBeTruthy();
    });
  });
});
