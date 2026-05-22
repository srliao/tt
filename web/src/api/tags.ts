/**
 * TanStack Query hooks for the `/tags` endpoints.
 *
 * URL shape is documented in `internal/httpapi/tags.go`. All mutations
 * invalidate the `['tags']` query key on success so list views refresh.
 * `useDeleteTag` additionally invalidates `['tasks']` because deleting a tag
 * cascades through `task_tags`, which changes the chips rendered on the
 * /tasks and /stage pages.
 *
 * The `['tags', 'with-counts']` key is also invalidated by every mutation so
 * the count-aware filter sidebar / command palette (see phase 0 design)
 * stays in sync after create / rename / delete.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Tag, TagWithCount } from '@/types/tag';

export function useTags() {
  return useQuery<Tag[]>({
    queryKey: ['tags'],
    queryFn: () => api<Tag[]>('/tags'),
  });
}

/**
 * Same data as `useTags()` plus a `count` of tasks referencing each tag.
 * Backed by `GET /tags?counts=1`. Cached separately because it requires an
 * extra join — pages that only need names should keep using `useTags()`.
 */
export function useTagsWithCounts() {
  return useQuery<TagWithCount[]>({
    queryKey: ['tags', 'with-counts'],
    queryFn: () => api<TagWithCount[]>('/tags?counts=1'),
    staleTime: 30_000,
  });
}

export function useCreateTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api<Tag>('/tags', { method: 'POST', body: JSON.stringify({ name }) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tags'] });
      void qc.invalidateQueries({ queryKey: ['tags', 'with-counts'] });
    },
  });
}

export function useRenameTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) =>
      api<Tag>(`/tags/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tags'] });
      void qc.invalidateQueries({ queryKey: ['tags', 'with-counts'] });
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}

export function useDeleteTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/tags/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tags'] });
      void qc.invalidateQueries({ queryKey: ['tags', 'with-counts'] });
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}
