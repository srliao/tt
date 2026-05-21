# Phase 08e — `/runs` Global Log + `/runs/$id` Detail

> Read `00-index.md` first. Commit after each task. Parallelizable with 08a/b/c/d.

**Goal:** A global, paginated, filterable run log (`/runs`) and a per-run detail page (`/runs/$id`) showing status, timing, logs table, and chips for each spawned task.

**Dependencies:** 06 (runs endpoints), 07.

**Tech stack:** shadcn `<Table>`, `<Badge>`, lucide-react. date-fns for formatting.

**Parallelizable with:** 08a, 08b, 08c, 08d.

## File map

```
web/src/api/runs.ts
web/src/features/runs/
├── list-page.tsx
├── detail-page.tsx
├── status-pill.tsx
├── logs-table.tsx
├── spawned-tasks-chips.tsx
└── *.test.tsx
web/src/routes/runs.index.tsx     # replace placeholder
web/src/routes/runs.$id.tsx       # replace placeholder
```

## Task 1: API hooks

**Files:** `web/src/api/runs.ts`

- [ ] Implement:
  - `useRuns(filters)` → `GET /runs?script_id=&status=&from=&to=&limit=&cursor=`. Filters live in the URL search params (zod-validated route schema).
  - `useRun(id)` → `GET /runs/:id`. Returns `{run, logs, spawned_tasks}`.
- [ ] Commit:
  ```bash
  git add web/src/api/runs.ts && git commit -m "feat(web): add run query hooks"
  ```

## Task 2: Status pill

**Files:** `web/src/features/runs/status-pill.tsx`

- [ ] Maps `running|ok|error|timeout` to a shadcn `<Badge>` variant with an icon (lucide):
  - running → blue, `Loader2` spin.
  - ok → green, `Check`.
  - error → red, `X`.
  - timeout → orange, `Clock`.
- [ ] Commit:
  ```bash
  git add web/src/features/runs/status-pill.tsx && \
    git commit -m "feat(runs): add status pill"
  ```

## Task 3: List page

**Files:** `web/src/features/runs/list-page.tsx`, `web/src/routes/runs.index.tsx`

- [ ] Top filter bar: script select (`useScripts()` for options), status select (any|ok|error|timeout|running), date range (two date inputs). All in URL params.
- [ ] Paginated table with columns: script name (link → `/scripts/$id`), started_at, duration, status pill, spawned task count, trigger. Each row clickable → `/runs/$id`.
- [ ] "Load more" button using `cursor` from the API response.
- [ ] Empty state per spec: "No script runs yet. Manually trigger a script or wait for its schedule."
- [ ] Replace `runs.index.tsx` placeholder.
- [ ] **Tests cover:** filter changes update the URL params and trigger a refetch with the new query key.
- [ ] Commit:
  ```bash
  git add web/src/features/runs/list-page.tsx web/src/routes/runs.index.tsx && \
    git commit -m "feat(runs): add list page with filters"
  ```

## Task 4: Logs table

**Files:** `web/src/features/runs/logs-table.tsx`

- [ ] Columns: relative time (e.g. "+0.123s"), absolute time, level badge (debug/info/warn/error), message (monospace; preserves whitespace).
- [ ] Filter input at the top (case-insensitive substring against message).
- [ ] Auto-scroll to bottom on first render; "Pause auto-scroll" button when the user scrolls up.
- [ ] **Tests cover:** filter input narrows the visible rows.
- [ ] Commit:
  ```bash
  git add web/src/features/runs/logs-table.tsx && \
    git commit -m "feat(runs): add logs table"
  ```

## Task 5: Spawned-task chips

**Files:** `web/src/features/runs/spawned-tasks-chips.tsx`

- [ ] Given the run's `spawned_task_ids`, fetch summaries via the bulk endpoint or N individual `GET /tasks/:id` calls (start with N — optimize later if needed). Render each as a chip showing title + state pill, clickable → opens the global edit modal.
- [ ] If a task has been deleted since the run, render the chip with strikethrough and a "deleted" badge.
- [ ] Commit:
  ```bash
  git add web/src/features/runs/spawned-tasks-chips.tsx && \
    git commit -m "feat(runs): add spawned task chips"
  ```

## Task 6: Detail page

**Files:** `web/src/features/runs/detail-page.tsx`, `web/src/routes/runs.$id.tsx`

- [ ] Header: script name (link), trigger, started/finished, duration, status pill.
- [ ] If status != `ok`: error block with the `error_message` in a monospace block.
- [ ] Below: `<LogsTable />`.
- [ ] Below: `<SpawnedTasksChips />` (only when len > 0).
- [ ] Auto-refetch every 2s while status === `running`; stop polling once terminal.
- [ ] **Tests cover:** when status === `running`, refetch is enabled; when terminal, refetch is disabled.
- [ ] Replace `runs.$id.tsx` placeholder.
- [ ] Commit:
  ```bash
  git add web/src/features/runs/detail-page.tsx web/src/routes/runs.\$id.tsx && \
    git commit -m "feat(runs): add detail page with logs and spawned chips"
  ```

## Phase completion checklist

- [ ] `pnpm run test` passes.
- [ ] Manual smoke: `/runs` lists runs; clicking a row goes to `/runs/$id`; logs render; if a script is still running, the page auto-refreshes; spawned task chips link to tasks.
