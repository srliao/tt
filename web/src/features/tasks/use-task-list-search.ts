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
import { type MouseEvent, useCallback } from 'react';
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
export const TAG_MODES = ['any', 'all'] as const;

export type QuickFilter = (typeof QUICK_FILTERS)[number];
export type TagMode = (typeof TAG_MODES)[number];

/**
 * Zod schema for the /tasks URL search params. Used both by the
 * route's `validateSearch` and the hook below.
 */
export const taskSearchSchema = z
  .object({
    states: z.array(z.enum(TASK_STATES)).optional(),
    tags: z.array(z.string()).optional(),
    tagsExclude: z.array(z.string()).optional(),
    tagMode: z.enum(TAG_MODES).optional(),
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
  // We always send tag_mode when tags are non-empty so the server respects
  // the UI default of "any" — see comments on TaskListParams.tagMode.
  const tagsPresent = search.tags && search.tags.length > 0;
  const base: TaskListParams = {
    states: search.states && search.states.length > 0 ? search.states : undefined,
    tags: tagsPresent ? search.tags : undefined,
    tagMode: tagsPresent ? (search.tagMode ?? 'any') : undefined,
    due: search.due,
    q: search.q,
    sort: search.sort,
    asc: search.asc,
  };

  if (!quick) {
    // Mirror the sidebar's default-checked "Not done" state so the request
    // matches what the user sees in the UI.
    return { ...base, states: base.states ?? ['not_done'] };
  }

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
    tagMode: base.tagMode,
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

/** Click-modifier semantics for `<TagGlyph>` / row tag interactions. */
export type TagFilterMode = 'replace' | 'add' | 'exclude';

/**
 * Translate a React MouseEvent's modifier keys into a filter mutation mode.
 * Shared by row tag glyphs and any future click-to-filter affordance.
 *
 *   bare click   → replace (single-tag focus)
 *   shift+click  → add to the current `tags` filter
 *   alt+click    → add to the `tagsExclude` filter
 */
export function clickModeFromEvent(e: MouseEvent): TagFilterMode {
  if (e.altKey) return 'exclude';
  if (e.shiftKey) return 'add';
  return 'replace';
}

/**
 * Returns a stable callback that mutates the URL's tag filter according to
 * the given mode. `replace` clears any prior exclusions so the user lands
 * on a clean single-tag view.
 */
export function useTagFilterMutator() {
  const { search, setSearch } = useTaskListSearch();
  return useCallback(
    (name: string, mode: TagFilterMode) => {
      if (mode === 'replace') {
        setSearch({ tags: [name], tagsExclude: undefined });
        return;
      }
      if (mode === 'add') {
        const set = new Set(search.tags ?? []);
        set.add(name);
        setSearch({ tags: [...set] });
        return;
      }
      // exclude
      const set = new Set(search.tagsExclude ?? []);
      set.add(name);
      setSearch({ tagsExclude: [...set] });
    },
    [search, setSearch],
  );
}
