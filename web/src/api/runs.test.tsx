import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { buildRunQuery, useRun, useRuns } from './runs';

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
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, Wrapper };
}

describe('buildRunQuery', () => {
  it('serialises all filter fields', () => {
    expect(
      buildRunQuery({
        script_id: 7,
        status: 'ok',
        from: '2026-05-01T00:00:00Z',
        to: '2026-05-21T00:00:00Z',
        limit: 25,
        cursor: '50',
      }),
    ).toBe(
      '?script_id=7&status=ok&from=2026-05-01T00%3A00%3A00Z&to=2026-05-21T00%3A00%3A00Z&limit=25&cursor=50',
    );
  });

  it('returns empty string when no filters set', () => {
    expect(buildRunQuery({})).toBe('');
  });
});

describe('useRuns', () => {
  afterEach(() => vi.restoreAllMocks());

  it('GETs /runs with filters appended', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse([]));
    vi.stubGlobal('fetch', fetchMock);
    const { Wrapper } = wrap();
    const { result } = renderHook(() => useRuns({ status: 'error', limit: 10 }), {
      wrapper: Wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/runs?status=error&limit=10',
      expect.any(Object),
    );
  });
});

describe('useRun', () => {
  afterEach(() => vi.restoreAllMocks());

  it('GETs /runs/:id and returns the detail envelope', async () => {
    const detail = {
      id: 1,
      script_id: 2,
      started_at: '2026-05-21T00:00:00Z',
      finished_at: '2026-05-21T00:00:01Z',
      status: 'ok',
      error_message: '',
      spawned_task_ids: [],
      trigger: 'manual',
      logs: [],
      spawned_tasks: [],
    };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(detail));
    vi.stubGlobal('fetch', fetchMock);
    const { Wrapper } = wrap();
    const { result } = renderHook(() => useRun(1), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/runs/1', expect.any(Object));
    expect(result.current.data).toEqual(detail);
  });

  it('is disabled when id is undefined', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const { Wrapper } = wrap();
    renderHook(() => useRun(undefined), { wrapper: Wrapper });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
