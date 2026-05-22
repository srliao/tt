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
import type { Task, TaskDueRange, TaskSortAxis, TaskState } from '@/types/task';

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
    /**
     * Transient signal used by the command palette to ask the /tasks page to
     * open the edit modal for a given task id. NOT a filter — it doesn't
     * affect the task list query and is excluded from `hasActiveFilters`.
     * The page clears it after consuming so refresh/back doesn't reopen.
     */
    open: z.coerce.number().optional(),
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
  const tagsExcludePresent = search.tagsExclude && search.tagsExclude.length > 0;
  const base: TaskListParams = {
    states: search.states && search.states.length > 0 ? search.states : undefined,
    tags: tagsPresent ? search.tags : undefined,
    tagsExclude: tagsExcludePresent ? search.tagsExclude : undefined,
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
    tagsExclude: base.tagsExclude,
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
      (search.tagsExclude && search.tagsExclude.length > 0) ||
      search.due ||
      search.q ||
      search.quick,
  );
}

/**
 * True when the effective state filter hides at least one canonical state.
 *
 * The "default" view (no `states` in the URL) mirrors the sidebar's
 * default-checked "Not done" — see `applyQuickFilter` — so it is considered
 * restricted and the active-filter strip surfaces the "Open only · include
 * done?" affordance even on a fresh `/tasks` visit. An empty `states` array
 * is treated identically to the unset case.
 *
 * Only when all three canonical states are explicitly selected is the view
 * considered unrestricted (no affordance).
 */
export function isStateRestricted(search: TaskSearch): boolean {
  // Default view (states unset or empty) = filtered to not_done only.
  if (!search.states || search.states.length === 0) return true;
  // Restricted iff at least one canonical state is missing from the selection.
  return TASK_STATES.some((s) => !search.states?.includes(s));
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

/**
 * Client-side mirror of the server's task-list predicates, used by the
 * ⇧⌘A "select all matching" shortcut so the page can resolve an off-screen
 * selection set without an extra round-trip.
 *
 * Mirror semantics — kept aligned with `internal/task/service.go`:
 *
 * - `states`: task.state must be in `filter.states` (no-op when unset/empty).
 * - `tags` + `tagMode='any'` (default): at least one of the filter tags
 *   appears on the task.
 * - `tags` + `tagMode='all'`: every filter tag appears on the task.
 * - `tagsExclude`: no filter tag appears on the task.
 * - `due`:
 *     - `'overdue'`  → due_date strictly before today AND state !== 'done'
 *       (the spec narrows the server predicate so "select all overdue"
 *       won't grab tasks the user already completed today).
 *     - `'today'`    → due_date equals today (local).
 *     - `'this_week'`→ due_date within [today, today + 7 days] (local).
 *     - `'none'`     → due_date is null.
 *     - `''`/undef   → no constraint.
 * - `q`: case-insensitive substring of title OR notes.
 *
 * Dates use the user's local timezone (Date constructors), which matches the
 * server's `date('now', 'localtime')` calls in practice for single-user mode.
 */
export function matchesFilter(task: Task, filter: TaskListParams): boolean {
  if (filter.states && filter.states.length > 0) {
    if (!filter.states.includes(task.state)) return false;
  }

  if (filter.tags && filter.tags.length > 0) {
    const taskTags = new Set(task.tags);
    const mode = filter.tagMode ?? 'any';
    if (mode === 'all') {
      for (const t of filter.tags) if (!taskTags.has(t)) return false;
    } else {
      let any = false;
      for (const t of filter.tags) {
        if (taskTags.has(t)) {
          any = true;
          break;
        }
      }
      if (!any) return false;
    }
  }

  if (filter.tagsExclude && filter.tagsExclude.length > 0) {
    const taskTags = new Set(task.tags);
    for (const t of filter.tagsExclude) if (taskTags.has(t)) return false;
  }

  if (filter.due) {
    if (!matchesDue(task, filter.due)) return false;
  }

  if (filter.q) {
    const needle = filter.q.toLowerCase();
    const hay = `${task.title}\n${task.notes}`.toLowerCase();
    if (!hay.includes(needle)) return false;
  }

  return true;
}

/** Format a Date as `YYYY-MM-DD` in the local timezone. */
function localDateKey(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

function matchesDue(task: Task, due: TaskDueRange): boolean {
  if (due === '') return true;
  if (due === 'none') return task.due_date == null;
  if (task.due_date == null) return false;
  const today = new Date();
  const todayKey = localDateKey(today);
  // due_date is YYYY-MM-DD; compare as strings (ISO sort-order safe).
  if (due === 'today') return task.due_date === todayKey;
  if (due === 'overdue') return task.due_date < todayKey && task.state !== 'done';
  if (due === 'this_week') {
    const plus7 = new Date(today);
    plus7.setDate(plus7.getDate() + 7);
    return task.due_date >= todayKey && task.due_date <= localDateKey(plus7);
  }
  return true;
}

/**
 * Returns the ids of every task in `tasks` that satisfies `filter`. Pure;
 * exported so the /tasks page can resolve the ⇧⌘A "select all matching"
 * shortcut without an extra server round-trip.
 */
export function computeAllMatchingIds(tasks: Task[], filter: TaskListParams): number[] {
  const out: number[] = [];
  for (const t of tasks) if (matchesFilter(t, filter)) out.push(t.id);
  return out;
}
