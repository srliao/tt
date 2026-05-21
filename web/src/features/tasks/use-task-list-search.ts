/**
 * URL-driven filter/sort state for the /tasks page.
 *
 * The search-params shape is validated on the route definition in
 * `routes/tasks.tsx` so this hook can assume parsed values. Quick filters
 * (`quick=...`) are stored as-is and translated to effective filters via
 * `applyQuickFilter()` at read time — that keeps the URL short ("share me")
 * while still letting the user temporarily override one axis of the preset.
 */

import { useNavigate, useSearch } from '@tanstack/react-router';
import { useCallback } from 'react';
import { z } from 'zod';
import type { TaskListParams } from '@/api/tasks';
import type { TaskDueRange, TaskSortAxis, TaskState } from '@/types/task';

export const TASK_STATES = ['not_done', 'done', 'cancelled'] as const;
export const TASK_DUE_RANGES = ['', 'overdue', 'today', 'this_week', 'none'] as const;
export const TASK_SORTS = ['priority', 'due_date', 'created_at', 'title'] as const;
export const QUICK_FILTERS = [
  'all-open',
  'overdue',
  'due-today',
  'recently-completed',
  'cancelled',
] as const;

export type QuickFilter = (typeof QUICK_FILTERS)[number];

/**
 * Zod schema for the /tasks URL search params. Used both by the
 * route's `validateSearch` and the hook below.
 */
export const taskSearchSchema = z
  .object({
    states: z.array(z.enum(TASK_STATES)).optional(),
    tags: z.array(z.string()).optional(),
    due: z.enum(TASK_DUE_RANGES).optional(),
    q: z.string().optional(),
    sort: z.enum(TASK_SORTS).optional(),
    asc: z.boolean().optional(),
    quick: z.enum(QUICK_FILTERS).optional(),
  })
  .partial();

export type TaskSearch = z.infer<typeof taskSearchSchema>;

/**
 * Given a quick-filter preset and the user's raw URL params, return the
 * effective filter sent to the server. Explicit user choices override the
 * preset (e.g. preset says `states: ['not_done']` but the user has also
 * checked `done` — honour the URL).
 */
export function applyQuickFilter(search: TaskSearch): TaskListParams {
  const quick = search.quick;
  const base: TaskListParams = {
    states: search.states && search.states.length > 0 ? search.states : undefined,
    tags: search.tags && search.tags.length > 0 ? search.tags : undefined,
    due: search.due,
    q: search.q,
    sort: search.sort,
    asc: search.asc,
  };

  if (!quick) return base;

  let preset: TaskListParams = {};
  switch (quick) {
    case 'all-open':
      preset = { states: ['not_done'] };
      break;
    case 'overdue':
      preset = { states: ['not_done'], due: 'overdue' };
      break;
    case 'due-today':
      preset = { states: ['not_done'], due: 'today' };
      break;
    case 'recently-completed':
      preset = { states: ['done'], sort: 'created_at', asc: false };
      break;
    case 'cancelled':
      preset = { states: ['cancelled'] };
      break;
  }

  return {
    states: base.states ?? preset.states,
    tags: base.tags,
    due: base.due ?? preset.due,
    q: base.q,
    sort: base.sort ?? preset.sort,
    asc: base.asc ?? preset.asc,
  };
}

/** True when no filter or sort is active and no quick preset is selected. */
export function hasActiveFilters(search: TaskSearch): boolean {
  return Boolean(
    (search.states && search.states.length > 0) ||
      (search.tags && search.tags.length > 0) ||
      search.due ||
      search.q ||
      search.quick,
  );
}

/**
 * Hook bound to the `/tasks` route. Returns the parsed search object plus a
 * `setSearch(updates)` helper that performs a shallow merge into the URL.
 * Pass `undefined` to clear a single field.
 */
export function useTaskListSearch() {
  // `strict: false` lets this hook be exercised under test routers that
  // don't have the file-route's `from: '/tasks'` identifier registered.
  const search = useSearch({ strict: false }) as TaskSearch;
  const navigate = useNavigate();

  const setSearch = useCallback(
    (updates: Partial<TaskSearch>) => {
      void navigate({
        to: '.',
        search: (prev) => {
          const merged: TaskSearch = { ...(prev as TaskSearch), ...updates };
          // Strip empty/undefined values so the URL stays short.
          const cleaned: TaskSearch = {};
          for (const [key, value] of Object.entries(merged)) {
            if (value === undefined || value === '' || value === null) continue;
            if (Array.isArray(value) && value.length === 0) continue;
            (cleaned as Record<string, unknown>)[key] = value;
          }
          return cleaned;
        },
      });
    },
    [navigate],
  );

  return { search, setSearch };
}

// Re-export the underlying types so callers don't have to dig.
export type { TaskDueRange, TaskSortAxis, TaskState };
