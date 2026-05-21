/**
 * TanStack Query hooks for the stage subsystem (`/api/v1/stage/*` +
 * `/tasks?staged=...` client-side filtering).
 *
 * Cross-phase contract: the `/tasks` endpoint does NOT support a
 * `?staged=true` filter on the server. Stage views fetch the full list and
 * filter client-side by `staged_order !== null`. See `00-index.md`.
 *
 * All mutations invalidate the `['tasks']` query key (the same one used by
 * `useTasks` and `useStagedTasks`) so the table and badge refresh together.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Task } from '@/types/task';

/**
 * Tasks currently on the stage, sorted by `staged_order` ascending. The
 * filtering is client-side per the resolved cross-phase contract.
 */
export function useStagedTasks() {
  return useQuery<Task[]>({
    queryKey: ['tasks', 'staged'],
    queryFn: async () => {
      const all = await api<Task[]>('/tasks');
      return all
        .filter((t) => t.staged_order !== null && t.staged_order !== undefined)
        .sort((a, b) => (a.staged_order ?? 0) - (b.staged_order ?? 0));
    },
  });
}

export interface StageReorderInput {
  task_id: number;
  before_id?: number | null;
  after_id?: number | null;
}

export function useReorderStage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: StageReorderInput) =>
      api<Task>('/stage/reorder', {
        method: 'POST',
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

/** Removes every task from the stage. */
export function useClearStage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<void>('/stage', { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

/** Removes only `done` + `cancelled` tasks from the stage. */
export function useClearFinishedFromStage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<void>('/stage/finished', { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}
