/**
 * TanStack Query hooks for the `/drawings` endpoints.
 *
 * URL shape is documented in `internal/httpapi/drawings.go`. Scene saves go
 * through `useSaveDrawingScene`, which takes an already-serialized scene
 * string so the editor can skip no-op saves by string comparison.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Drawing, DrawingSummary } from '@/types/drawing';

/**
 * Builds a JSON body embedding an already-serialized scene verbatim, so a
 * multi-MB scene isn't parsed and re-stringified just to be wrapped.
 * Exported for tests.
 */
export function bodyWithScene(fields: Record<string, unknown>, scene: string): string {
  const head = JSON.stringify(fields).slice(0, -1);
  return `${head}${head.length > 1 ? ',' : ''}"scene":${scene}}`;
}

export function useDrawings() {
  return useQuery<DrawingSummary[]>({
    queryKey: ['drawings'],
    queryFn: () => api<DrawingSummary[]>('/drawings'),
  });
}

/**
 * Loads one drawing including its scene. Background refetches are disabled:
 * the canvas is seeded once from this data and then owns the scene, so a
 * refetch could only ever hand back a stale copy of what's on screen.
 */
export function useDrawing(id: number) {
  return useQuery<Drawing>({
    queryKey: ['drawings', id],
    queryFn: () => api<Drawing>(`/drawings/${id}`),
    staleTime: Number.POSITIVE_INFINITY,
    refetchOnWindowFocus: false,
  });
}

/**
 * Creates a drawing. `scene` is an already-serialized Excalidraw scene
 * (import); omit it for a blank canvas.
 */
export function useCreateDrawing() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ name, scene }: { name: string; scene?: string }) =>
      api<Drawing>('/drawings', {
        method: 'POST',
        body: scene === undefined ? JSON.stringify({ name }) : bodyWithScene({ name }, scene),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['drawings'], exact: true });
    },
  });
}

export function useRenameDrawing() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) =>
      api<Drawing>(`/drawings/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) }),
    onSuccess: (d) => {
      // Patch just the name into the open drawing's cache; its scene copy is
      // the mount-time seed and must not be replaced.
      qc.setQueryData<Drawing>(['drawings', d.id], (old) => old && { ...old, name: d.name });
      void qc.invalidateQueries({ queryKey: ['drawings'], exact: true });
    },
  });
}

export function useSaveDrawingScene() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, scene }: { id: number; scene: string }) =>
      api<Drawing>(`/drawings/${id}`, { method: 'PATCH', body: bodyWithScene({}, scene) }),
    onSuccess: () => {
      // Only the list (for updated_at ordering); the open drawing's cache
      // entry is deliberately left alone — see useDrawing.
      void qc.invalidateQueries({ queryKey: ['drawings'], exact: true });
    },
  });
}

export function useDeleteDrawing() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/drawings/${id}`, { method: 'DELETE' }),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: ['drawings', id] });
      void qc.invalidateQueries({ queryKey: ['drawings'], exact: true });
    },
  });
}
