/**
 * TanStack Query hooks for the `/tags` endpoints.
 */

import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Tag } from '@/types/tag';

export function useTags() {
  return useQuery<Tag[]>({
    queryKey: ['tags'],
    queryFn: () => api<Tag[]>('/tags'),
  });
}
