/**
 * TanStack Query hooks for the `/runs` endpoints.
 *
 * URL shape is documented in `internal/httpapi/runs.go`.
 *
 * - `GET /runs?script_id=&status=&from=&to=&limit=&cursor=` returns a flat
 *   array of `Run`. `cursor` is a numeric offset; clients pass back the row
 *   count they've already pulled to fetch the next page.
 * - `GET /runs/:id` returns the run fields flattened with an additional
 *   `logs` array and a `spawned_tasks` summary array. Deleted spawned tasks
 *   are silently dropped server-side, so `spawned_tasks.length` may be less
 *   than `spawned_task_ids.length` — the detail UI uses that gap to render a
 *   "deleted" chip without an extra round-trip.
 */

import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Log, Run, RunStatus } from '@/types/run';
import type { TaskState } from '@/types/task';

export interface RunListFilters {
  script_id?: number;
  status?: RunStatus;
  /** RFC3339 lower bound (inclusive) on `started_at`. */
  from?: string;
  /** RFC3339 upper bound (inclusive) on `started_at`. */
  to?: string;
  limit?: number;
  /** Numeric offset cursor returned by clients tracking pagination. */
  cursor?: string;
}

export function buildRunQuery(filters: RunListFilters): string {
  const sp = new URLSearchParams();
  if (filters.script_id !== undefined) sp.set('script_id', String(filters.script_id));
  if (filters.status) sp.set('status', filters.status);
  if (filters.from) sp.set('from', filters.from);
  if (filters.to) sp.set('to', filters.to);
  if (filters.limit !== undefined) sp.set('limit', String(filters.limit));
  if (filters.cursor) sp.set('cursor', filters.cursor);
  const qs = sp.toString();
  return qs ? `?${qs}` : '';
}

export function useRuns(filters: RunListFilters = {}) {
  const qs = buildRunQuery(filters);
  return useQuery<Run[]>({
    queryKey: ['runs', filters],
    queryFn: () => api<Run[]>(`/runs${qs}`),
  });
}

export interface SpawnedTaskSummary {
  id: number;
  title: string;
  state: TaskState;
}

/**
 * Server response shape: `script.Run` fields embedded at the top level plus
 * `logs` and `spawned_tasks`. Mirrors `runDetail` in `internal/httpapi/runs.go`.
 */
export interface RunDetail extends Run {
  logs: Log[];
  spawned_tasks: SpawnedTaskSummary[];
}

export interface UseRunOptions {
  /** When provided, override the polling interval. Default = no polling. */
  refetchInterval?: number | false;
}

export function useRun(id: number | undefined, opts: UseRunOptions = {}) {
  return useQuery<RunDetail>({
    queryKey: ['runs', id],
    queryFn: () => api<RunDetail>(`/runs/${id}`),
    enabled: id !== undefined && id !== null && !Number.isNaN(id),
    refetchInterval: opts.refetchInterval ?? false,
  });
}
