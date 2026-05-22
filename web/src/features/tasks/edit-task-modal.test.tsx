import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/task';
import { EditTaskModal } from './edit-task-modal';

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

function makeTask(partial: Partial<Task> & { id: number; title: string }): Task {
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

function Harness({ task }: { task: Task }) {
  const [current, setCurrent] = useState<Task | null>(task);
  return <EditTaskModal task={current} onOpenChange={(next) => !next && setCurrent(null)} />;
}

describe('EditTaskModal', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders all editable fields prefilled from the task', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    const task = makeTask({
      id: 9,
      title: 'Wash car',
      notes: 'soap',
      due_date: '2026-06-01',
      tags: ['home', 'errand'],
    });

    render(
      <Wrapper>
        <Harness task={task} />
      </Wrapper>,
    );

    expect((await screen.findByLabelText('Title')) as HTMLInputElement).toHaveProperty(
      'value',
      'Wash car',
    );
    expect((screen.getByLabelText('Notes') as HTMLTextAreaElement).value).toBe('soap');
    expect((screen.getByLabelText('Due date') as HTMLInputElement).value).toBe('2026-06-01');
    expect((screen.getByLabelText('Tags') as HTMLInputElement).value).toBe('home, errand');
  });

  it('PATCHes the task with the new values on save', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ id: 9, title: 'Wash car' }));
    vi.stubGlobal('fetch', fetchMock);

    render(
      <Wrapper>
        <Harness task={makeTask({ id: 9, title: 'Wash car' })} />
      </Wrapper>,
    );

    const tags = await screen.findByLabelText('Tags');
    await act(async () => {
      fireEvent.change(tags, { target: { value: 'home, errand' } });
    });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/tasks/9',
        expect.objectContaining({ method: 'PATCH' }),
      );
    });

    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body).toMatchObject({ title: 'Wash car', tags: ['home', 'errand'] });
  });

  it('shows a validation error when title is cleared', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      <Wrapper>
        <Harness task={makeTask({ id: 1, title: 'Wash car' })} />
      </Wrapper>,
    );

    const title = await screen.findByLabelText('Title');
    await act(async () => {
      fireEvent.change(title, { target: { value: '   ' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    });

    expect(await screen.findByRole('alert')).toHaveTextContent('Title is required');
  });
});
