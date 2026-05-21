/**
 * TanStack Query hooks for the `/scripts` endpoints + nested
 * `/scripts/:id/{runs,tasks,run}` resources.
 *
 * URL shape is documented in `internal/httpapi/scripts.go`. All script
 * mutations invalidate the `['scripts']` query key so list/detail views
 * refresh together. `useRunScript` additionally invalidates the
 * per-script run history and the global tasks list (since a successful
 * run may have queued tasks).
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Run } from '@/types/run';
import type { Script, ScriptCreateInput, ScriptUpdateInput } from '@/types/script';
import type { Task } from '@/types/task';

export function useScripts() {
  return useQuery<Script[]>({
    queryKey: ['scripts'],
    queryFn: () => api<Script[]>('/scripts'),
  });
}

export function useScript(id: number | undefined) {
  return useQuery<Script>({
    queryKey: ['scripts', id],
    queryFn: () => api<Script>(`/scripts/${id}`),
    enabled: id !== undefined && id !== null && !Number.isNaN(id),
  });
}

export function useCreateScript() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ScriptCreateInput) =>
      api<Script>('/scripts', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['scripts'] });
    },
  });
}

export function useUpdateScript() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: ScriptUpdateInput }) =>
      api<Script>(`/scripts/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['scripts'] });
      void qc.invalidateQueries({ queryKey: ['scripts', vars.id] });
    },
  });
}

export function useDeleteScript() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/scripts/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['scripts'] });
    },
  });
}

export interface RunScriptResponse {
  run_id: number;
}

export function useRunScript(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<RunScriptResponse>(`/scripts/${id}/run`, { method: 'POST' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['scripts', id, 'runs'] });
      void qc.invalidateQueries({ queryKey: ['scripts', id, 'tasks'] });
      // A successful run may have spawned tasks; refresh the global list too.
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

export interface ScriptRunsParams {
  limit?: number;
  /** Cursor — RFC3339 timestamp used by the server to paginate backwards. */
  before?: string;
}

export function buildRunsQuery(params: ScriptRunsParams): string {
  const sp = new URLSearchParams();
  if (params.limit !== undefined) sp.set('limit', String(params.limit));
  if (params.before) sp.set('before', params.before);
  const qs = sp.toString();
  return qs ? `?${qs}` : '';
}

export function useScriptRuns(id: number | undefined, params: ScriptRunsParams = {}) {
  return useQuery<Run[]>({
    queryKey: ['scripts', id, 'runs', params],
    queryFn: () => api<Run[]>(`/scripts/${id}/runs${buildRunsQuery(params)}`),
    enabled: id !== undefined && id !== null && !Number.isNaN(id),
    // Polling per spec §6 — surface fresh statuses while the editor is open.
    refetchInterval: 5_000,
  });
}

export interface SpawnedTasksParams {
  limit?: number;
  /** Opaque cursor returned by the server (e.g. last seen task id/timestamp). */
  cursor?: string;
}

export function buildSpawnedTasksQuery(params: SpawnedTasksParams): string {
  const sp = new URLSearchParams();
  if (params.limit !== undefined) sp.set('limit', String(params.limit));
  if (params.cursor) sp.set('cursor', params.cursor);
  const qs = sp.toString();
  return qs ? `?${qs}` : '';
}

export function useSpawnedTasks(id: number | undefined, params: SpawnedTasksParams = {}) {
  return useQuery<Task[]>({
    queryKey: ['scripts', id, 'tasks', params],
    queryFn: () => api<Task[]>(`/scripts/${id}/tasks${buildSpawnedTasksQuery(params)}`),
    enabled: id !== undefined && id !== null && !Number.isNaN(id),
  });
}
