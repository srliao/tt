import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Tag } from '@/types/tag';
import { TagsPage } from './page';

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

describe('TagsPage', () => {
  afterEach(() => vi.restoreAllMocks());

  it('shows the empty-state placeholder when no tags exist', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([] as Tag[])));
    render(
      <Wrapper>
        <TagsPage />
      </Wrapper>,
    );
    expect(await screen.findByText('No tags yet. Add one to start organizing tasks.')).toBeTruthy();
    // The add form is always present.
    expect(screen.getByPlaceholderText('New tag name')).toBeTruthy();
  });

  it('lists tags alphabetically', async () => {
    const tags: Tag[] = [
      { id: 1, name: 'work', color_hue: 0, created_at: '' },
      { id: 2, name: 'home', color_hue: 30, created_at: '' },
      { id: 3, name: 'errands', color_hue: 60, created_at: '' },
    ];
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(tags)));
    render(
      <Wrapper>
        <TagsPage />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByText('errands')).toBeTruthy();
    });
    const list = screen.getByRole('list', { name: 'Tags' });
    const items = Array.from(list.querySelectorAll('[data-tag-id]')).map(
      (el) => el.textContent?.replace(/\s+/g, ' ').trim().split(' ')[0],
    );
    expect(items).toEqual(['errands', 'home', 'work']);
  });
});
