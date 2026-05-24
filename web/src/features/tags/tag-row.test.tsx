import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Tag } from '@/types/tag';
import { TagRow } from './tag-row';

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

const tag: Tag = { id: 7, name: 'home', created_at: '2026-05-01T00:00:00Z' };

describe('TagRow', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders the tag name', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])));
    render(
      <Wrapper>
        <ul>
          <TagRow tag={tag} />
        </ul>
      </Wrapper>,
    );
    expect(screen.getByText('home')).toBeTruthy();
  });

  it('clicking Edit shows an input that PATCHes on Enter', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/v1/tags/7' && init?.method === 'PATCH') {
        return Promise.resolve(jsonResponse({ id: 7, name: 'errands', created_at: '' }));
      }
      // The row also fires /tags?counts=1 for the delete-confirm copy.
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(
      <Wrapper>
        <ul>
          <TagRow tag={tag} />
        </ul>
      </Wrapper>,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Edit home' }));
    });

    const input = (await screen.findByRole('textbox', { name: 'Rename home' })) as HTMLInputElement;
    await act(async () => {
      fireEvent.change(input, { target: { value: 'errands' } });
    });
    await act(async () => {
      fireEvent.keyDown(input, { key: 'Enter' });
    });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/tags/7',
        expect.objectContaining({ method: 'PATCH' }),
      );
    });
    const patchCall = fetchMock.mock.calls.find(
      ([url, init]) =>
        url === '/api/v1/tags/7' && (init as RequestInit | undefined)?.method === 'PATCH',
    );
    expect(patchCall).toBeTruthy();
    expect(JSON.parse((patchCall![1] as RequestInit).body as string)).toEqual({ name: 'errands' });
  });

  it('pressing Escape cancels the rename without firing a request', async () => {
    // The row also fires /tags?counts=1 for the delete-confirm copy — return
    // an empty list so it resolves cleanly. The assertion below filters out
    // those reads.
    const fetchMock = vi.fn((_url: string, _init?: RequestInit) =>
      Promise.resolve(jsonResponse([])),
    );
    vi.stubGlobal('fetch', fetchMock);
    render(
      <Wrapper>
        <ul>
          <TagRow tag={tag} />
        </ul>
      </Wrapper>,
    );
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Edit home' }));
    });
    const input = (await screen.findByRole('textbox', { name: 'Rename home' })) as HTMLInputElement;
    await act(async () => {
      fireEvent.change(input, { target: { value: 'newname' } });
    });
    await act(async () => {
      fireEvent.keyDown(input, { key: 'Escape' });
    });
    // No write requests (PATCH/DELETE) should have fired — the only call we
    // tolerate is the count-fetch.
    const writes = fetchMock.mock.calls.filter(([_url, init]) => {
      const method = (init as RequestInit | undefined)?.method;
      return method === 'PATCH' || method === 'DELETE' || method === 'POST';
    });
    expect(writes).toHaveLength(0);
    expect(screen.getByText('home')).toBeTruthy();
  });

  it('Enter with an empty name shows an inline error and does not call PATCH', async () => {
    const fetchMock = vi.fn((_url: string, _init?: RequestInit) =>
      Promise.resolve(jsonResponse([])),
    );
    vi.stubGlobal('fetch', fetchMock);
    render(
      <Wrapper>
        <ul>
          <TagRow tag={tag} />
        </ul>
      </Wrapper>,
    );
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Edit home' }));
    });
    const input = (await screen.findByRole('textbox', { name: 'Rename home' })) as HTMLInputElement;
    await act(async () => {
      fireEvent.change(input, { target: { value: '   ' } });
    });
    await act(async () => {
      fireEvent.keyDown(input, { key: 'Enter' });
    });
    const writes = fetchMock.mock.calls.filter(([_url, init]) => {
      const method = (init as RequestInit | undefined)?.method;
      return method === 'PATCH' || method === 'DELETE' || method === 'POST';
    });
    expect(writes).toHaveLength(0);
    expect(await screen.findByRole('alert')).toHaveTextContent('Tag name is required');
  });

  it('clicking Delete opens the cascade-confirm and DELETEs only after confirm', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/v1/tags/7' && init?.method === 'DELETE') {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (url.includes('/tags?counts=1')) {
        return Promise.resolve(jsonResponse([{ id: 7, name: 'home', created_at: '', count: 3 }]));
      }
      return Promise.resolve(jsonResponse({}));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <Wrapper>
        <ul>
          <TagRow tag={tag} />
        </ul>
      </Wrapper>,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Delete home' }));
    });

    // Radix portals the dialog body outside our tree — query the document.
    const desc = await waitFor(() => {
      const el = document.body.querySelector('[data-slot="alert-dialog-description"]');
      if (!el) throw new Error('dialog body not rendered');
      return el;
    });
    // Wait for the counts query to land so the copy reflects the real count.
    await waitFor(() => {
      expect(desc.textContent).toContain('3 tasks');
    });

    // No DELETE call before the user confirms.
    expect(
      fetchMock.mock.calls.find(
        ([url, init]) =>
          url === '/api/v1/tags/7' && (init as RequestInit | undefined)?.method === 'DELETE',
      ),
    ).toBeFalsy();

    const confirm = await waitFor(() => {
      const buttons = Array.from(document.querySelectorAll('button')) as HTMLButtonElement[];
      const candidate = buttons.find((b) => b.textContent?.trim() === 'Delete' && !b.ariaLabel);
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
            url === '/api/v1/tags/7' && (init as RequestInit | undefined)?.method === 'DELETE',
        ),
      ).toBeTruthy();
    });
  });

  it('delete confirm uses singular wording when exactly one task is tagged', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/tags?counts=1')) {
          return Promise.resolve(jsonResponse([{ id: 7, name: 'home', created_at: '', count: 1 }]));
        }
        return Promise.resolve(jsonResponse({}));
      }),
    );

    render(
      <Wrapper>
        <ul>
          <TagRow tag={tag} />
        </ul>
      </Wrapper>,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Delete home' }));
    });

    const desc = await waitFor(() => {
      const el = document.body.querySelector('[data-slot="alert-dialog-description"]');
      if (!el) throw new Error('dialog body not rendered');
      return el;
    });
    await waitFor(() => {
      expect(desc.textContent).toContain('1 task');
      expect(desc.textContent).not.toContain('1 tasks');
    });
  });

  it('delete confirm shows the "no tasks" copy when count is zero', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/tags?counts=1')) {
          return Promise.resolve(jsonResponse([{ id: 7, name: 'home', created_at: '', count: 0 }]));
        }
        return Promise.resolve(jsonResponse({}));
      }),
    );

    render(
      <Wrapper>
        <ul>
          <TagRow tag={tag} />
        </ul>
      </Wrapper>,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Delete home' }));
    });

    const desc = await waitFor(() => {
      const el = document.body.querySelector('[data-slot="alert-dialog-description"]');
      if (!el) throw new Error('dialog body not rendered');
      return el;
    });
    await waitFor(() => {
      expect(desc.textContent).toContain('No tasks currently use this tag');
    });
  });
});
