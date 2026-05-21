import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
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
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

function Harness() {
  const [open, setOpen] = useState(true);
  return <AddTaskModal open={open} onOpenChange={setOpen} />;
}

describe('AddTaskModal', () => {
  afterEach(() => vi.restoreAllMocks());

  it('shows validation error when title is empty', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      <Wrapper>
        <Harness />
      </Wrapper>,
    );

    const submit = await screen.findByRole('button', { name: 'Create task' });
    await act(async () => {
      fireEvent.click(submit);
    });

    expect(await screen.findByRole('alert')).toHaveTextContent('Title is required');
  });

  it('submits the create-task request with the entered title', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ id: 1, title: 'Buy milk' }, 201));
    vi.stubGlobal('fetch', fetchMock);
    render(
      <Wrapper>
        <Harness />
      </Wrapper>,
    );

    const title = await screen.findByLabelText('Title');
    await act(async () => {
      fireEvent.change(title, { target: { value: 'Buy milk' } });
    });

    const tags = screen.getByLabelText('Tags');
    await act(async () => {
      fireEvent.change(tags, { target: { value: 'home, errand' } });
    });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Create task' }));
    });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/tasks',
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });

    const call = fetchMock.mock.calls[0];
    const body = JSON.parse(call[1].body as string);
    expect(body).toMatchObject({ title: 'Buy milk', tags: ['home', 'errand'] });
  });
});
