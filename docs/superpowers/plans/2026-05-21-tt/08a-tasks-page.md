# Phase 08a — `/tasks` Page

> Read `00-index.md` first. Commit after each task. Parallelizable with 08b–08e.

**Goal:** Implement the main `/tasks` page per spec §6: left sidebar with filters + quick filters, table with state pills/tag chips/due-date warnings/drag handles, add-task modal, bulk actions, search with debounce, and drag-drop reorder semantics (Alternative C — recompute the moved task's key between visible neighbors).

**Dependencies:** 06 (task + tag endpoints), 07 (frontend bootstrap).

**Tech stack:** TanStack Query + Router (URL search params), dnd-kit, react-hook-form + zod, date-fns, shadcn components, lucide-react.

**Parallelizable with:** 08b, 08c, 08d, 08e.

## File map

```
web/src/api/
├── tasks.ts                 # query/mutation hooks for tasks
└── tags.ts                  # used by filter combobox + tag chips
web/src/features/tasks/
├── page.tsx                 # the /tasks page assembly
├── filter-sidebar.tsx
├── task-table.tsx
├── task-row.tsx
├── add-task-modal.tsx
├── bulk-action-bar.tsx
├── use-task-list-search.ts  # URL search-param sync helper
└── *.test.tsx
web/src/routes/tasks.tsx     # replace placeholder; renders <TasksPage />
```

## URL search-params shape (spec §6)

`/tasks?states=not_done,done&tags=work,urgent&due=today&q=milk&sort=priority&asc=true&quick=overdue`

- Encoded via TanStack Router's `search` schema (zod-validated).
- All filters live in the URL — refresh-stable, shareable.

## Task 1: API hooks

**Files:** `web/src/api/tasks.ts`, `web/src/api/tags.ts`

- [ ] Implement TanStack Query hooks (each calls `api<T>` from phase 07):
  - `useTasks(params)` → `useQuery({queryKey: ['tasks', params], queryFn: () => api(`/tasks?${qs}`)})`.
  - `useCreateTask()` → `useMutation`, invalidates `['tasks']` on success.
  - `useUpdateTask()`, `useDeleteTask()`, `useSetTaskState()`, `useStageTask()`, `useUnstageTask()`, `useReorderMain()`. All invalidate `['tasks']`.
  - `useTags()` for the filter combobox + chip display.
- [ ] **Tests cover:** at least one of each (`useTasks`, `useCreateTask`) using `QueryClientProvider` + `msw` or a manual `fetch` mock. Assert correct query keys for cache invalidation.
- [ ] Commit:
  ```bash
  git add web/src/api/tasks.ts web/src/api/tags.ts && \
    git commit -m "feat(web): add task and tag query/mutation hooks"
  ```

## Task 2: URL search-params hook

**Files:** `web/src/features/tasks/use-task-list-search.ts`

- [ ] Implement `useTaskListSearch()` that wraps `useSearch()` from TanStack Router and returns:
  - `{ states, tags, due, q, sort, asc, quick }` parsed via a zod schema.
  - A `setSearch(updates)` function that calls `navigate({search: prev => ({...prev, ...updates})})`.
- [ ] Define the search schema on the route (in `web/src/routes/tasks.tsx` via `validateSearch`).
- [ ] Quick-filter presets (per spec §6) — when `quick` is set, derive the underlying filters at read time:
  - `all-open` → `states: ['not_done']` (default).
  - `overdue` → `states: ['not_done'], due: 'overdue'`.
  - `due-today` → `states: ['not_done'], due: 'today'`.
  - `recently-completed` → `states: ['done'], sort: 'completed_at', asc: false, dueRange: last 7 days`. (Note: spec asks for "last 7 days"; encode as a derived query.)
  - `cancelled` → `states: ['cancelled']`.
- [ ] **Tests cover:** preset application produces the expected URL search-params object.
- [ ] Commit:
  ```bash
  git add web/src/features/tasks/use-task-list-search.ts web/src/routes/tasks.tsx && \
    git commit -m "feat(tasks): URL-driven filter/sort state"
  ```

## Task 3: Filter sidebar

**Files:** `web/src/features/tasks/filter-sidebar.tsx`

- [ ] Renders, top to bottom:
  - **Quick filters** as clickable rows: "All open" (default selected), "Overdue", "Due today", "Recently completed", "Cancelled". Clicking sets the `quick` URL param.
  - **State** multi-checkbox (`not_done`, `done`, `cancelled`). Defaults to `not_done` only.
  - **Tags** multi-select combobox backed by `useTags()` (shadcn `<Combobox>`). AND semantics.
  - **Due** select: None, Overdue, Today, This Week, No due date.
  - **Search** input (debounced 300ms; updates `q` param). Shows "Searching open tasks only" hint when state filter is restricted (per spec §6).
  - Mark the search input with `data-search-input` so the global `/` shortcut focuses it.
- [ ] **Tests cover:** clicking a quick filter updates the URL params; checking a state checkbox updates `states`.
- [ ] Commit:
  ```bash
  git add web/src/features/tasks/filter-sidebar.tsx && \
    git commit -m "feat(tasks): add filter sidebar with quick filters and search"
  ```

## Task 4: Task table + row

**Files:** `web/src/features/tasks/task-table.tsx`, `web/src/features/tasks/task-row.tsx`

- [ ] `TaskTable` displays:
  - Headers per spec §6 row content.
  - Drag handles rendered **only when `sort === 'priority'`**; otherwise the column is hidden.
  - Each `TaskRow` renders: checkbox (for bulk select), drag handle, title (click → edit modal), state pill, tag chips, due date (red text + warning icon when overdue), staged indicator (small "·staged" badge), kebab menu (edit, state→done/not_done/cancelled, stage/unstage, delete).
  - Kebab menu uses shadcn `<DropdownMenu>`.
- [ ] `j` / `k` move row focus down/up; `enter` opens the edit modal; `e` edits; `s` stages; `space` toggles bulk-select; `d` sets `done`. Implement via a `useTableShortcuts(rows)` hook bound to the table container; only active when the page is "focused" (use a `useEffect` that adds/removes listeners on mount/unmount).
- [ ] **Tests cover:**
  - Row click opens edit modal.
  - Kebab "Mark done" calls `useSetTaskState` mutation.
  - When `sort=priority` drag handle is rendered; when `sort=title` it is not.
- [ ] Commit:
  ```bash
  git add web/src/features/tasks/task-table.tsx web/src/features/tasks/task-row.tsx && \
    git commit -m "feat(tasks): add task table with row actions"
  ```

## Task 5: Drag-drop reorder (Alternative C semantics)

**Files:** `web/src/features/tasks/task-table.tsx` (extend)

- [ ] Wrap the rows in dnd-kit's `<DndContext>` + `<SortableContext>` (vertical strategy). Only active when `sort === 'priority'`.
- [ ] On drop, compute `beforeID` and `afterID` from the moved row's new visible neighbors (the rows immediately above and below it in the post-drop visible list). Send `POST /api/v1/tasks/reorder` with `{task_id, before_id, after_id}` via the mutation hook.
- [ ] Optimistic update: while the mutation is in flight, render the new order.
- [ ] When any filter is active, render a subtle note in the sort/filter bar: "Filtered view — hidden tasks unchanged".
- [ ] **Tests cover:** A test that simulates a drop (call the `onDragEnd` handler directly with synthetic events) and asserts that the mutation hook was invoked with the expected `{task_id, before_id, after_id}`.
- [ ] Commit:
  ```bash
  git add web/src/features/tasks/task-table.tsx && \
    git commit -m "feat(tasks): add dnd-kit reorder with Alt-C semantics"
  ```

## Task 6: Add-task modal

**Files:** `web/src/features/tasks/add-task-modal.tsx`

- [ ] Shadcn `<Dialog>` containing a react-hook-form + zod-validated form: title (required), notes (textarea), tags (combobox creating new tags inline), due_date (date picker). Submits via `useCreateTask`. Cmd/Ctrl-Enter submits; Esc closes.
- [ ] Listens for the global `tt:new-task` event (from the `n` shortcut) and opens.
- [ ] **Tests cover:** Submitting fills the mutation payload correctly; empty title shows validation error.
- [ ] Commit:
  ```bash
  git add web/src/features/tasks/add-task-modal.tsx && \
    git commit -m "feat(tasks): add task creation modal"
  ```

## Task 7: Bulk action bar

**Files:** `web/src/features/tasks/bulk-action-bar.tsx`

- [ ] Sticky floating bar at the bottom of the table, shown when ≥1 row is selected.
- [ ] Buttons: "Mark done", "Stage selected", "Delete selected" (with confirm dialog).
- [ ] Calls the appropriate mutation hooks in a loop (or a single bulk endpoint if 06 added one — v1 spec does not require one).
- [ ] **Tests cover:** clicking "Mark done" with two rows selected calls `useSetTaskState` twice.
- [ ] Commit:
  ```bash
  git add web/src/features/tasks/bulk-action-bar.tsx && \
    git commit -m "feat(tasks): add bulk action bar"
  ```

## Task 8: Assemble + empty state

**Files:** `web/src/features/tasks/page.tsx`, `web/src/routes/tasks.tsx`

- [ ] `TasksPage` wires sidebar + table + add-task modal + bulk-action-bar.
- [ ] Empty state per spec §6: when `useTasks()` returns no rows AND no filters active: big "Create your first task" CTA, paragraph explaining tasks/stage/scripts, link to `/scripts`.
- [ ] Replace the placeholder in `web/src/routes/tasks.tsx` with `<TasksPage />`.
- [ ] **Tests cover:** empty state visible when no data; otherwise table renders.
- [ ] Commit:
  ```bash
  git add web/src/features/tasks/page.tsx web/src/routes/tasks.tsx && \
    git commit -m "feat(tasks): assemble /tasks page with empty state"
  ```

## Phase completion checklist

- [ ] `cd web && pnpm run test` passes.
- [ ] Manual smoke: `/tasks` lets you create, edit, stage, mark done, delete; filters work; search debounces; drag-drop only available under sort=priority; bulk actions work; `n` opens the modal; `?` shows the cheatsheet.
