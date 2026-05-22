import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CheatsheetTags } from './cheatsheet-tags';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function wrap() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe('CheatsheetTags', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders the empty hint when no tags exist', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([]))),
    );
    render(<CheatsheetTags />, { wrapper: wrap() });
    expect(await screen.findByText(/No tags yet/)).toBeTruthy();
  });

  it('copies a quoted name to the clipboard on click', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([{ id: 1, name: 'weekly' }]))),
    );

    const writeText = vi.fn().mockResolvedValueOnce(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });

    render(<CheatsheetTags />, { wrapper: wrap() });
    const button = await screen.findByRole('button', { name: /Copy "weekly"/ });
    await act(async () => {
      fireEvent.click(button);
    });
    expect(writeText).toHaveBeenCalledWith('"weekly"');
  });
});
