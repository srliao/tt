import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AddTagForm } from './add-tag-form';

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

describe('AddTagForm', () => {
  afterEach(() => vi.restoreAllMocks());

  it('shows a validation error when the name is empty', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})));
    render(
      <Wrapper>
        <AddTagForm />
      </Wrapper>,
    );
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Add tag/ }));
    });
    expect(await screen.findByRole('alert')).toHaveTextContent('Tag name is required');
  });

  it('submits POST /tags with the entered name and clears the input on success', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ id: 1, name: 'home', created_at: '' }));
    vi.stubGlobal('fetch', fetchMock);
    render(
      <Wrapper>
        <AddTagForm />
      </Wrapper>,
    );
    const input = screen.getByPlaceholderText('New tag name') as HTMLInputElement;
    await act(async () => {
      fireEvent.change(input, { target: { value: 'home' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Add tag/ }));
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/tags',
        expect.objectContaining({ method: 'POST' }),
      );
    });
    const call = fetchMock.mock.calls[0];
    expect(JSON.parse(call[1].body as string)).toEqual({ name: 'home' });
    await waitFor(() => expect(input.value).toBe(''));
  });

  it('surfaces a 409 conflict inline as "Tag already exists"', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({ error: { code: 'conflict', message: 'tag already exists' } }, 409),
      );
    vi.stubGlobal('fetch', fetchMock);
    render(
      <Wrapper>
        <AddTagForm />
      </Wrapper>,
    );
    const input = screen.getByPlaceholderText('New tag name') as HTMLInputElement;
    await act(async () => {
      fireEvent.change(input, { target: { value: 'home' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Add tag/ }));
    });
    expect(await screen.findByText('Tag already exists')).toBeTruthy();
    // The input is not cleared so the user can correct it.
    expect(input.value).toBe('home');
  });
});
