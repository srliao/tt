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
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
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
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ id: 7, name: 'errands', created_at: '' }));
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
    const call = fetchMock.mock.calls[0];
    expect(JSON.parse(call[1].body as string)).toEqual({ name: 'errands' });
  });

  it('pressing Escape cancels the rename without firing a request', async () => {
    const fetchMock = vi.fn();
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
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByText('home')).toBeTruthy();
  });

  it('Enter with an empty name shows an inline error and does not call PATCH', async () => {
    const fetchMock = vi.fn();
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
    expect(fetchMock).not.toHaveBeenCalled();
    expect(await screen.findByRole('alert')).toHaveTextContent('Tag name is required');
  });

  it('clicking Delete opens the cascade-confirm and DELETEs only after confirm', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/v1/tags/7' && init?.method === 'DELETE') {
        return Promise.resolve(new Response(null, { status: 204 }));
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
    expect(desc.textContent).toContain('This will remove the tag from any tasks that use it');

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
});
