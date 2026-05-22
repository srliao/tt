import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
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
  return (
    <ThemeProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </ThemeProvider>
  );
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
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (String(url).includes('/tags')) {
          return Promise.resolve(
            jsonResponse([
              { id: 1, name: 'home', count: 3, created_at: '2026-05-01T00:00:00Z' },
              { id: 2, name: 'errand', count: 1, created_at: '2026-05-01T00:00:00Z' },
            ]),
          );
        }
        return Promise.resolve(jsonResponse({}));
      }),
    );
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
    // Tag chips are rendered inside the combobox trigger.
    const trigger = document.querySelector(
      '[data-slot="tag-combobox-trigger"]',
    ) as HTMLElement | null;
    expect(trigger).not.toBeNull();
    expect(within(trigger as HTMLElement).getByText('home')).toBeInTheDocument();
    expect(within(trigger as HTMLElement).getByText('errand')).toBeInTheDocument();
  });

  it('PATCHes the task with the new values on save', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (String(url).includes('/tags') && (!init || init.method === undefined)) {
        return Promise.resolve(
          jsonResponse([
            { id: 1, name: 'home', count: 3, created_at: '2026-05-01T00:00:00Z' },
            { id: 2, name: 'errand', count: 1, created_at: '2026-05-01T00:00:00Z' },
          ]),
        );
      }
      if (String(url).includes('/api/v1/tasks/9') && init?.method === 'PATCH') {
        return Promise.resolve(jsonResponse({ id: 9, title: 'Wash car' }));
      }
      return Promise.resolve(jsonResponse({}));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <Wrapper>
        <Harness task={makeTask({ id: 9, title: 'Wash car' })} />
      </Wrapper>,
    );

    // Open the combobox and click both options.
    const input = document.querySelector(
      '[data-slot="tag-combobox-input"]',
    ) as HTMLInputElement | null;
    expect(input).not.toBeNull();
    act(() => {
      (input as HTMLInputElement).focus();
    });

    // Click 'home' then 'errand' from the dropdown.
    const homeOption = await screen.findByRole('option', { name: /home/i });
    await act(async () => {
      fireEvent.click(homeOption);
    });
    const errandOption = await screen.findByRole('option', { name: /errand/i });
    await act(async () => {
      fireEvent.click(errandOption);
    });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    });

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([url, init]) =>
            String(url) === '/api/v1/tasks/9' &&
            (init as RequestInit | undefined)?.method === 'PATCH',
        ),
      ).toBe(true);
    });

    const patchCall = fetchMock.mock.calls.find(
      ([url, init]) =>
        String(url) === '/api/v1/tasks/9' && (init as RequestInit | undefined)?.method === 'PATCH',
    ) as [string, RequestInit];
    const body = JSON.parse(patchCall[1].body as string);
    expect(body).toMatchObject({ title: 'Wash car', tags: ['home', 'errand'] });
  });

  it('shows a validation error when title is cleared', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (String(url).includes('/tags')) return Promise.resolve(jsonResponse([]));
        return Promise.resolve(jsonResponse({}));
      }),
    );
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
