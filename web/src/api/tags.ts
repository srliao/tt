/**
 * TanStack Query hooks for the `/tags` endpoints.
 *
 * URL shape is documented in `internal/httpapi/tags.go`. All mutations
 * invalidate the `['tags']` query key on success so list views refresh.
 * `useDeleteTag` additionally invalidates `['tasks']` because deleting a tag
 * cascades through `task_tags`, which changes the chips rendered on the
 * /tasks and /stage pages.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Tag } from '@/types/tag';

export function useTags() {
  return useQuery<Tag[]>({
    queryKey: ['tags'],
    queryFn: () => api<Tag[]>('/tags'),
  });
}

export function useCreateTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api<Tag>('/tags', { method: 'POST', body: JSON.stringify({ name }) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tags'] });
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
      void qc.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
}
