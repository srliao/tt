/**
 * TanStack Query hooks for the `/tasks` endpoints.
 *
 * URL shape is documented in `internal/httpapi/tasks.go`; all hooks invalidate
 * the `['tasks']` query key on success so list views refresh automatically.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { TagFilter } from '@/features/tasks/use-task-list-search';
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
  /**
   * Structured tag filter — replaces `tags` + `tagMode` (Phase 3 will
   * remove the old fields). Added in Phase 1 of the tag-filter refactor
   * so consumers can begin migrating; the API client is still wiring
   * `tags`/`tagMode` to the server until Phase 3.
   */
  tag_filter?: TagFilter;
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
 * Bulk-mutate tags across a multi-selection in a single transactional
 * request. The server returns the updated DTOs for every supplied id, which
 * we splice into every `['tasks', ...]` cache so list views update without a
 * refetch. Tag counts can change (auto-create on add/set), so the
 * `['tags', 'with-counts']` query is invalidated.
 */
export function useBulkTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BulkTagInput) =>
      api<Task[]>('/tasks/bulk-tag', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: (updated) => {
      // Patch every cached task list (filtered or not) without a refetch.
      qc.setQueriesData<Task[]>({ queryKey: ['tasks'] }, (prev) => {
        if (!prev) return prev;
        const map = new Map(updated.map((t) => [t.id, t] as const));
        return prev.map((t) => map.get(t.id) ?? t);
      });
      void qc.invalidateQueries({ queryKey: ['tags', 'with-counts'] });
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
