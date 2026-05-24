/**
 * URL-driven filter/sort state for the /tasks page.
 *
 * The search-params shape is validated on the route definition in
 * `routes/tasks.tsx` so this hook can assume parsed values. Quick filters
 * (`quick=...`) are stored as-is and translated to effective filters via
 * `applyQuickFilter()` at read time — that keeps the URL short ("share me")
 * while still letting the user temporarily override one axis of the preset.
 *
 * Tag filtering uses a single structured `tag_filter=<mode>:<name>,<name>,…`
 * URL param. The raw URL value is a string; `taskSearchSchema` transforms it
 * into a `TagFilter` object so consumers can read `search.tag_filter` as a
 * structured value. `setSearch` re-serialises back to the string form so the
 * URL stays `tag_filter=any:work` rather than `[object Object]`.
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

export const TAG_MATCH_MODES = ['any', 'all'] as const;
export type TagMatchMode = (typeof TAG_MATCH_MODES)[number];

/**
 * Reserved sentinel for the "untagged" pseudo-tag. Real tags can't contain
 * `@` (validated server-side) so the sentinel cannot collide with a real
 * tag name.
 */
export const UNTAGGED_TOKEN = '@untagged';

export type QuickFilter = (typeof QUICK_FILTERS)[number];

/**
 * Structured tag filter — the parsed form of the `tag_filter=<mode>:<name>,…`
 * URL param. `tags` may include `UNTAGGED_TOKEN` to mean "untagged tasks".
 */
export const tagFilterSchema = z.object({
  mode: z.enum(TAG_MATCH_MODES),
  tags: z.array(z.string().min(1)).min(1),
});

export type TagFilter = z.infer<typeof tagFilterSchema>;

/**
 * Parse a raw `tag_filter` URL value of the shape `mode:name,name,…` into a
 * structured `TagFilter`. Returns `undefined` for any malformed input (bad
 * mode, missing `:`, empty tag list) so the route validator can drop the
 * param silently rather than throw.
 */
export function parseTagFilter(raw: string): TagFilter | undefined {
  const idx = raw.indexOf(':');
  if (idx < 0) return undefined;
  const mode = raw.slice(0, idx);
  if (mode !== 'any' && mode !== 'all') return undefined;
  const tags = Array.from(
    new Set(
      raw
        .slice(idx + 1)
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
    ),
  );
  if (tags.length === 0) return undefined;
  return { mode, tags };
}

/**
 * Serialise a `TagFilter` back to its URL string form. Returns `undefined`
 * when the filter has no tags so callers can omit the param entirely
 * (never write `tag_filter=any:`).
 */
export function serializeTagFilter(f: TagFilter | undefined): string | undefined {
  if (!f || f.tags.length === 0) return undefined;
  return `${f.mode}:${f.tags.join(',')}`;
}

/**
 * Zod schema for the /tasks URL search params. Used both by the
 * route's `validateSearch` and the hook below.
 *
 * `tag_filter` arrives as a string from the URL and is transformed to a
 * `TagFilter | undefined`. Consumers read `search.tag_filter` as the parsed
 * object; `setSearch` calls `serializeTagFilter` to write the string form
 * back to the URL.
 */
export const taskSearchSchema = z
  .object({
    states: z.array(z.enum(TASK_STATES)).optional(),
    /**
     * Accepts either the raw URL string form (`'any:work,errand'`) or a
     * pre-parsed `TagFilter` object. The URL serialiser always writes the
     * string; programmatic callers (e.g. tests calling `router.navigate`
     * directly with structured search) pass the object form.
     */
    tag_filter: z
      .union([z.string(), tagFilterSchema])
      .optional()
      .transform((v) => {
        if (v === undefined) return undefined;
        if (typeof v === 'string') return parseTagFilter(v);
        return v;
      })
      .pipe(tagFilterSchema.optional()),
    /**
     * Tag exclusions are orthogonal to the include filter — alt-click on a
     * row tag adds to this list. Phase 1 leaves this shape unchanged; the
     * tag-filter refactor only touches the include side.
     */
    tagsExclude: z.array(z.string()).optional(),
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
    /**
     * Transient signal used by the command palette to ask the /tasks page to
     * open the bulk tag editor. The page consumes and clears immediately.
     * Same race-avoidance rationale as `open`: a URL signal beats a
     * CustomEvent because the URL is settled before the page commits.
     */
    openBulkTagEditor: z.coerce.boolean().optional(),
    /** Transient signal: open the bulk delete confirm dialog. */
    confirmBulkDelete: z.coerce.boolean().optional(),
    /** Transient signal: open the bulk cancel confirm dialog. */
    confirmBulkCancel: z.coerce.boolean().optional(),
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
  const tagsExcludePresent = search.tagsExclude && search.tagsExclude.length > 0;
  const base: TaskListParams = {
    states: search.states && search.states.length > 0 ? search.states : undefined,
    tag_filter: search.tag_filter,
    tagsExclude: tagsExcludePresent ? search.tagsExclude : undefined,
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
    tag_filter: base.tag_filter,
    tagsExclude: base.tagsExclude,
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
      search.tag_filter ||
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
 *
 * `tag_filter` is special: callers pass the structured `TagFilter` object,
 * which is serialised back to its URL string form before navigating. That
 * way `search.tag_filter` is always the parsed shape and the URL is always
 * the canonical `tag_filter=any:work` string.
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
          // Strip empty/undefined values so the URL stays short, and
          // re-serialise `tag_filter` back to its URL string form so the
          // navigate call writes `tag_filter=any:work` rather than the
          // JSON-encoded object form.
          const cleaned: Record<string, unknown> = {};
          for (const [key, value] of Object.entries(merged)) {
            if (value === undefined || value === '' || value === null) continue;
            if (Array.isArray(value) && value.length === 0) continue;
            if (key === 'tag_filter') {
              const serialised = serializeTagFilter(value as TagFilter | undefined);
              if (serialised) cleaned[key] = serialised;
              continue;
            }
            cleaned[key] = value;
          }
          return cleaned as TaskSearch;
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
 *   shift+click  → add to the current tag filter
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
 *
 * `add` preserves the existing match mode (defaulting to `'any'` when no
 * filter is set yet) so a shift-click from the row never silently flips the
 * user's chosen Any/All toggle.
 */
export function useTagFilterMutator() {
  const { search, setSearch } = useTaskListSearch();
  return useCallback(
    (name: string, mode: TagFilterMode) => {
      if (mode === 'replace') {
        setSearch({
          tag_filter: { mode: 'any', tags: [name] },
          tagsExclude: undefined,
        });
        return;
      }
      if (mode === 'add') {
        const current = search.tag_filter;
        const tags = current?.tags ?? [];
        if (tags.includes(name)) {
          // Already present — nothing to do; avoid a no-op URL write.
          return;
        }
        setSearch({
          tag_filter: {
            mode: current?.mode ?? 'any',
            tags: [...tags, name],
          },
        });
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
 * - `tag_filter` (mode='any', default): at least one of the filter tags
 *   appears on the task. The `@untagged` sentinel matches tasks with zero
 *   tags, so `any:@untagged,work` returns the union (untagged ∪ tagged-with-work).
 * - `tag_filter` (mode='all'): every filter tag appears on the task. When
 *   `@untagged` is in the list together with real tag names, the result is
 *   always empty (no task can be both untagged AND have other tags).
 * - `tagsExclude`: no excluded tag appears on the task.
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

  if (filter.tag_filter && filter.tag_filter.tags.length > 0) {
    const taskTags = new Set(task.tags);
    const { mode, tags } = filter.tag_filter;
    const wantsUntagged = tags.includes(UNTAGGED_TOKEN);
    const realTags = tags.filter((t) => t !== UNTAGGED_TOKEN);
    const isUntagged = taskTags.size === 0;

    if (mode === 'all') {
      // All + Untagged with any real tag is unsatisfiable — a task can't be
      // both untagged AND carry the real tags.
      if (wantsUntagged && realTags.length > 0) return false;
      if (wantsUntagged) {
        if (!isUntagged) return false;
      } else {
        for (const t of realTags) if (!taskTags.has(t)) return false;
      }
    } else {
      // any: union of (untagged tasks) ∪ (tasks matching at least one real tag)
      let matched = false;
      if (wantsUntagged && isUntagged) matched = true;
      if (!matched) {
        for (const t of realTags) {
          if (taskTags.has(t)) {
            matched = true;
            break;
          }
        }
      }
      if (!matched) return false;
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
