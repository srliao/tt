/**
 * TanStack Query hooks for the `/tasks` endpoints.
 *
 * URL shape is documented in `internal/httpapi/tasks.go`; all hooks invalidate
 * the `['tasks']` query key on success so list views refresh automatically.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type {
  Task,
  TaskCreateInput,
  TaskDueRange,
  TaskSortAxis,
  TaskState,
  TaskUpdateInput,
} from '@/types/task';

/** Shape of search params we send to the server. */
export interface TaskListParams {
  states?: TaskState[];
  /** Tag names (server resolves to IDs). */
  tags?: string[];
  /**
   * Tag names to exclude. Serialised as a single CSV `tags_exclude=a,b,c`
   * query param to keep the URL short for the common alt-click case.
   */
  tagsExclude?: string[];
  /**
   * How multiple tag filters combine on the server. The backend defaults to
   * `all` when the field is unset, so the UI sends `any` explicitly whenever
   * `tags` is non-empty (the UI default is `any` — see filter sidebar).
   */
  tagMode?: 'any' | 'all';
  due?: TaskDueRange;
  q?: string;
  sort?: TaskSortAxis;
  asc?: boolean;
  limit?: number;
  offset?: number;
}

export function buildTaskListQuery(params: TaskListParams): string {
  const sp = new URLSearchParams();
  for (const s of params.states ?? []) sp.append('state', s);
  for (const t of params.tags ?? []) sp.append('tag', t);
  if (params.tagMode && (params.tags?.length ?? 0) > 0) {
    sp.set('tag_mode', params.tagMode);
  }
  if (params.tagsExclude && params.tagsExclude.length > 0) {
    sp.set('tags_exclude', params.tagsExclude.join(','));
  }
  if (params.due) sp.set('due', params.due);
  if (params.q) sp.set('q', params.q);
  if (params.sort) sp.set('sort', params.sort);
  if (params.asc !== undefined) sp.set('asc', String(params.asc));
  if (params.limit !== undefined) sp.set('limit', String(params.limit));
  if (params.offset !== undefined) sp.set('offset', String(params.offset));
  const qs = sp.toString();
  return qs ? `?${qs}` : '';
}

export function useTasks(params: TaskListParams) {
  return useQuery<Task[]>({
    queryKey: ['tasks', params],
    queryFn: () => api<Task[]>(`/tasks${buildTaskListQuery(params)}`),
  });
}

export function useCreateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: TaskCreateInput) =>
      api<Task>('/tasks', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

export function useUpdateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: TaskUpdateInput }) =>
      api<Task>(`/tasks/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

export function useDeleteTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/tasks/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

export function useSetTaskState() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, state }: { id: number; state: TaskState }) =>
      api<Task>(`/tasks/${id}/state`, {
        method: 'POST',
        body: JSON.stringify({ state }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

export function useStageTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<Task>(`/tasks/${id}/stage`, { method: 'POST' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

export function useUnstageTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<Task>(`/tasks/${id}/stage`, { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

export interface BulkTagInput {
  ids: number[];
  op: 'add' | 'remove' | 'set';
  tags: string[];
}

/**
 * STUB — Phase 7 will replace this with a single POST /tasks/bulk-tag
 * round-trip. The current backend has no per-task /tags endpoint, so the
 * only way to mutate tags client-side is PATCH /tasks/:id with the full
 * tags list. We fetch each task, compute the new tag list, and PATCH it.
 * Slow (2N round-trips for N tasks) but functionally complete.
 *
 * NOTE: partial-failure window — earlier PATCHes commit before a later one
 * rejects. Phase 7 replaces this with a single-transaction endpoint.
 */
export function useBulkTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: BulkTagInput) => {
      const tasks = await Promise.all(body.ids.map((id) => api<Task>(`/tasks/${id}`)));
      await Promise.all(
        tasks.map((t) => {
          const current = new Set(t.tags);
          let next: string[];
          if (body.op === 'set') {
            next = body.tags.slice();
          } else if (body.op === 'add') {
            for (const tag of body.tags) current.add(tag);
            next = [...current];
          } else {
            for (const tag of body.tags) current.delete(tag);
            next = [...current];
          }
          return api<Task>(`/tasks/${t.id}`, {
            method: 'PATCH',
            body: JSON.stringify({
              title: t.title,
              notes: t.notes,
              due_date: t.due_date,
              tags: next,
            }),
          });
        }),
      );
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
      void qc.invalidateQueries({ queryKey: ['tags'] });
    },
  });
}

export interface ReorderInput {
  task_id: number;
  before_id: number | null;
  after_id: number | null;
}

export function useReorderMain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ReorderInput) =>
      api<void>('/tasks/reorder', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}
