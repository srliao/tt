import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { BulkActionBar } from './bulk-action-bar';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function wrap(children: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe('BulkActionBar', () => {
  afterEach(() => vi.restoreAllMocks());

  it('is hidden when nothing is selected', () => {
    vi.stubGlobal('fetch', vi.fn());
    render(wrap(<BulkActionBar selectedIds={new Set()} onClear={() => {}} />));
    expect(screen.queryByRole('region', { name: 'Bulk actions' })).toBeNull();
  });

  it('"Mark done" fires POST /tasks/:id/state once per selected row', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}));
    vi.stubGlobal('fetch', fetchMock);
    render(wrap(<BulkActionBar selectedIds={new Set([7, 9])} onClear={() => {}} />));

    await act(async () => {
      screen.getByRole('button', { name: 'Mark done' }).click();
    });

    const urls = (fetchMock.mock.calls as Array<[string, unknown]>)
      .map(([url]) => url)
      .filter((url) => url.includes('/state'));
    expect(urls).toHaveLength(2);
    expect(urls).toContain('/api/v1/tasks/7/state');
    expect(urls).toContain('/api/v1/tasks/9/state');
  });
});
