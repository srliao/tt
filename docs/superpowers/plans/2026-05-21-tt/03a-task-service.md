# Phase 03a — Task Domain Service

> Read `00-index.md` first. Commit after each task. Parallelizable with 03b (tags) and 03c (scripts).

**Goal:** Implement the task domain service: CRUD, state transitions with timestamp management, staging, drag-drop reorder via fractional keys, rebalance, dynamic filter/sort listing, and `ByScript` / `LatestBySpawningScript` lookups for the runtime.

**Dependencies:** Phase 02.

**Tech stack:** Go stdlib, sqlc-generated queries, `dbtest` helper.

**Parallelizable with:** 03b, 03c.

## Background (spec invariants)

- `priority REAL` orders the main list. `staged_order REAL` (nullable) orders the stage; `NULL` = unstaged.
- Drag-drop computes the new key as the **midpoint** of the moved item's new visible neighbors. Top/bottom edges use `neighbor ∓ 1.0`. When the two neighbor keys are within `1e-9` of each other, rebalance first.
- New tasks: `priority = COALESCE(MAX(priority), -1) + 1.0`. Staging: `staged_order = COALESCE(MAX(staged_order), -1) + 1.0`.
- State transitions manage `completed_at` / `cancelled_at` and **never touch `staged_order`** (spec §3 "Semantic rules"). Moving away from `done` clears `completed_at`. Same for cancelled.
- Tag attachment lives on the task service via `SetTagsByID([]int64)`; the tag service (03b) provides the name→id resolution.

## File map

```
internal/db/queries/tasks.sql       # replace stub
internal/task/
├── types.go
├── reorder.go
├── reorder_test.go
├── service.go
└── service_test.go
```

## Task 1: Replace `queries/tasks.sql`

**Files:** `internal/db/queries/tasks.sql`

The SQL is the load-bearing artifact for sqlc — write it verbatim:

```sql
-- name: CreateTask :one
INSERT INTO tasks (title, notes, due_date, priority, staged_order, spawned_by_script_id)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = ?;

-- name: GetTaskTags :many
SELECT t.id, t.name
FROM tags t JOIN task_tags tt ON tt.tag_id = t.id
WHERE tt.task_id = ?
ORDER BY t.name;

-- name: UpdateTaskFields :one
UPDATE tasks
SET title = ?, notes = ?, due_date = ?, updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: SetTaskState :one
UPDATE tasks
SET state = ?, completed_at = ?, cancelled_at = ?, updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: SetTaskStaged :one
UPDATE tasks SET staged_order = ?, updated_at = datetime('now')
WHERE id = ? RETURNING *;

-- name: SetTaskPriority :one
UPDATE tasks SET priority = ?, updated_at = datetime('now')
WHERE id = ? RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = ?;

-- name: ClearStage :exec
UPDATE tasks SET staged_order = NULL, updated_at = datetime('now')
WHERE staged_order IS NOT NULL;

-- name: ClearFinishedFromStage :exec
UPDATE tasks SET staged_order = NULL, updated_at = datetime('now')
WHERE staged_order IS NOT NULL AND state IN ('done','cancelled');

-- name: MaxPriority :one
SELECT COALESCE(MAX(priority), -1.0) FROM tasks;

-- name: MaxStagedOrder :one
SELECT COALESCE(MAX(staged_order), -1.0) FROM tasks WHERE staged_order IS NOT NULL;

-- name: ListAllPrioritiesAsc :many
SELECT id, priority FROM tasks ORDER BY priority ASC, id ASC;

-- name: ListAllStagedAsc :many
SELECT id, staged_order FROM tasks WHERE staged_order IS NOT NULL ORDER BY staged_order ASC, id ASC;

-- name: ListTasksByScript :many
SELECT * FROM tasks WHERE spawned_by_script_id = ?
ORDER BY created_at DESC, id DESC;

-- name: LatestTaskBySpawningScript :one
SELECT * FROM tasks WHERE spawned_by_script_id = ?
ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: AddTaskTag :exec
INSERT OR IGNORE INTO task_tags (task_id, tag_id) VALUES (?, ?);

-- name: ReplaceTaskTags :exec
DELETE FROM task_tags WHERE task_id = ?;
```

`List` uses a dynamic query built in Go (filter shape is too varied for sqlc), so no `ListTasks` here.

- [ ] Run `just db-gen && go build ./...` → clean.
- [ ] Commit:
  ```bash
  git add internal/db/queries/tasks.sql internal/db/sqlc/ && git commit -m "feat(db): add task queries"
  ```

## Task 2: Domain types

**Files:** `internal/task/types.go`

- [ ] Define:
  - `type State string` with constants `StateNotDone`, `StateDone`, `StateCancelled`. Add `(s State) IsValid() bool` and `ValidStates() []State`.
  - `type Task struct { ... }` — fields per spec §3 + a `Tags []string`. Use pointer types for nullable timestamps (`*time.Time`) and nullable strings (`*string`) and the nullable `StagedOrder *float64`, `SpawnedByScriptID *int64`. Add `json` tags matching snake_case.
  - `CreateInput { Title, Notes string; DueDate *string; Tags []string; SpawnedByScriptID *int64 }`.
  - `UpdateInput { Title, Notes string; DueDate *string; Tags []string }`.
  - `FilterSort { States []State; TagIDs []int64; Due DueRange; Search string; Sort SortAxis; Ascending bool; Limit, Offset int }`.
  - `type DueRange string` with constants `DueAny ("")`, `DueOverdue`, `DueToday`, `DueThisWeek`, `DueNone`.
  - `type SortAxis string` with constants `SortPriority`, `SortDueDate`, `SortCreatedAt`, `SortTitle`.
- [ ] Verify build: `go build ./internal/task/...`.
- [ ] Commit:
  ```bash
  git add internal/task/types.go && git commit -m "feat(task): add domain types"
  ```

## Task 3: Reorder helpers — failing test

**Files:** `internal/task/reorder_test.go`

- [ ] Add a test for `Midpoint(before, after *float64) float64`. Table-driven, covering: both nil → `0.0`; before nil, `after=1.0` → `0.0`; `before=5.0`, after nil → `6.0`; `before=1.0, after=3.0` → `2.0`; `before=1.0, after=1.5` → `1.25`.
- [ ] Add a test for `NeedsRebalance(a, b float64) bool`: gap of `1e-10` triggers rebalance (true); gap of `1e-12` triggers rebalance (true); gap of `1.0` is healthy (false). (Threshold is `< 1e-9` per spec §3.)
- [ ] Run `go test ./internal/task/...` → `undefined: Midpoint`.

## Task 4: Reorder helpers — implementation

**Files:** `internal/task/reorder.go`

- [ ] Implement:
  - `const rebalanceEpsilon = 1e-9`.
  - `Midpoint(before, after *float64) float64` per the table above.
  - `NeedsRebalance(a, b float64) bool` returning `math.Abs(b-a) < rebalanceEpsilon`.
  - `EvenSpread(n int) []float64` returning `[0, 1, …, n-1]` (used by the rebalance pass).
- [ ] Run `go test ./internal/task/... -v -run TestMidpoint -run TestNeedsRebalance` → green.
- [ ] Commit:
  ```bash
  git add internal/task/reorder.go internal/task/reorder_test.go && git commit -m "feat(task): add fractional-key reorder helpers"
  ```

## Task 5: Service skeleton + Create — failing test

**Files:** `internal/task/service_test.go`, `internal/task/service.go` (created in Task 6)

- [ ] Add a helper `newService(t)` that builds an in-memory `dbtest.New(t)` and `task.New(store)`.
- [ ] **Tests cover:**
  - `Create` returns ascending priority for sequential creates (`b.Priority > a.Priority`).
  - New tasks default to `StateNotDone`.
  - New tasks have `StagedOrder == nil`.
  - Empty title returns an error.
  - Invalid `DueDate` (not `YYYY-MM-DD`) returns an error.
- [ ] Run: build error `undefined: task.New`.

## Task 6: Service — Create + helpers

**Files:** `internal/task/service.go`

- [ ] Define the `Service` interface listing every method to be implemented across this phase: `Create`, `Update`, `SetState`, `Get`, `Delete`, `Stage`, `Unstage`, `ClearStage`, `ClearFinishedFromStage`, `ReorderMain`, `ReorderStage`, `RebalancePriority`, `RebalanceStage`, `List`, `ByScript`, `LatestBySpawningScript`, `SetTagsByID`.
- [ ] Define `Impl` wrapping `*db.Store` + `*sqlcgen.Queries`; `New(store)` constructs.
- [ ] Implement `Create`:
  - Trim title, error if empty.
  - If `DueDate` non-nil/non-empty, parse `"2006-01-02"` and error on parse failure.
  - Query `MaxPriority`, set `newPriority = max + 1.0`.
  - Call `q.CreateTask(...)` with `sql.NullString` / `sql.NullFloat64` / `sql.NullInt64` wrappers as needed.
  - Tag attachment is deferred; `Create` does **not** attach tags (callers use `SetTagsByID` after resolving names).
  - Return `rowToTask(row, nil)`.
- [ ] Implement helpers:
  - `rowToTask(r sqlcgen.Task, tags []string) Task` — converts nullable fields to pointers; calls `parseSqliteTime` on each timestamp.
  - `parseSqliteTime(s string) time.Time` — tries `"2006-01-02 15:04:05"` then `time.RFC3339`, returns UTC.
- [ ] Run `go test ./internal/task/... -v -run TestCreate` → green.
- [ ] Commit:
  ```bash
  git add internal/task/service.go internal/task/service_test.go && git commit -m "feat(task): add Create with priority assignment"
  ```

## Task 7: State transitions

**Files:** `internal/task/service.go`, `internal/task/service_test.go`

- [ ] **Tests cover:**
  - `SetState(StateDone)` sets `CompletedAt` to a non-nil time.
  - `SetState(StateDone)` followed by `SetState(StateNotDone)` clears `CompletedAt`.
  - `SetState(StateCancelled)` sets `CancelledAt`; moving away clears it.
  - **Critical:** staging a task, then calling `SetState(StateDone)`, leaves `StagedOrder` unchanged.
  - `SetState` with an invalid state string returns an error.
- [ ] Implement `SetState(ctx, id, st State)`:
  - Validate `st.IsValid()`.
  - Build `completedAt sql.NullString` and `cancelledAt sql.NullString` based on `st`:
    - `StateDone`: completedAt = now (UTC, `"2006-01-02 15:04:05"`), cancelledAt = invalid.
    - `StateCancelled`: cancelledAt = now, completedAt = invalid.
    - `StateNotDone`: both invalid.
  - Call `q.SetTaskState(...)`. Reload tags via `loadTags`. Return `rowToTask`.
- [ ] Implement `loadTags(ctx, taskID)` calling `q.GetTaskTags` and mapping to a `[]string` of names.
- [ ] Run `go test ./internal/task/... -v -run TestSetState` → green.
- [ ] Commit:
  ```bash
  git add internal/task/service.go internal/task/service_test.go && git commit -m "feat(task): add state transitions with timestamp management"
  ```

## Task 8: Stage / Unstage / Clear

**Files:** `internal/task/service.go`, `internal/task/service_test.go`

- [ ] **Tests cover:**
  - `Stage(id)` sets `StagedOrder` to `MaxStagedOrder + 1.0`; calling twice on different tasks gives ascending values.
  - `Unstage(id)` returns `StagedOrder == nil`.
  - `ClearStage()` clears all staged tasks regardless of state.
  - `ClearFinishedFromStage()` clears only `done` / `cancelled` staged rows; `not_done` staged rows survive.
- [ ] Implement `Stage`, `Unstage`, `ClearStage`, `ClearFinishedFromStage` — each one-liners over the sqlc methods.
- [ ] Run `go test ./internal/task/... -v -run TestStage -run TestClear` → green.
- [ ] Commit:
  ```bash
  git add internal/task/service.go internal/task/service_test.go && git commit -m "feat(task): add stage/unstage and stage-clear actions"
  ```

## Task 9: Get / Update / Delete

**Files:** `internal/task/service.go`, `internal/task/service_test.go`

- [ ] **Tests cover:**
  - `Get` returns the persisted task.
  - `Update` replaces title/notes/due_date.
  - `Update` with empty title errors.
  - `Update` with invalid due_date errors.
  - `Delete` removes the row; subsequent `Get` errors.
- [ ] Implement, mirroring the validation rules from `Create` for title/due_date.
- [ ] Run tests → green.
- [ ] Commit:
  ```bash
  git add internal/task/service.go internal/task/service_test.go && git commit -m "feat(task): add get/update/delete"
  ```

## Task 10: Reorder main + stage

**Files:** `internal/task/service.go`, `internal/task/service_test.go`

- [ ] **Tests cover:**
  - `ReorderMain` between two neighbors: new priority lies strictly between the neighbors' priorities.
  - `ReorderMain` to top (`beforeID = nil`, `afterID = first.ID`): new priority < first's.
  - `ReorderMain` to bottom (`beforeID = last.ID`, `afterID = nil`): new priority > last's.
  - `ReorderStage` mirrors the above on `staged_order`.
- [ ] Implement:
  - `neighborPriorities(ctx, beforeID, afterID, useStage)` looks up each neighbor via `q.GetTask`, returns `(*float64, *float64)` of their priority (or staged_order). Errors if a referenced neighbor doesn't have the requested key (e.g. unstaged passed as a stage neighbor).
  - `ReorderMain` / `ReorderStage`: fetch neighbor keys; if both non-nil and `NeedsRebalance(*bp, *ap)`, call `RebalancePriority` (or `RebalanceStage`), then re-fetch neighbor keys; compute `Midpoint(bp, ap)`; persist via `q.SetTaskPriority` / `q.SetTaskStaged`; reload tags; return.
- [ ] Run tests → green.
- [ ] Commit:
  ```bash
  git add internal/task/service.go internal/task/service_test.go && git commit -m "feat(task): add drag-drop reorder for main list and stage"
  ```

## Task 11: Rebalance

**Files:** `internal/task/service.go`, `internal/task/service_test.go`

- [ ] **Tests cover:**
  - Insert 5 tasks, call `RebalancePriority`. Subsequent `List` returns tasks with `Priority == 0.0, 1.0, 2.0, 3.0, 4.0` in order.
  - (Similar for `RebalanceStage` with 5 staged tasks.)
- [ ] Implement `RebalancePriority` and `RebalanceStage`:
  - List rows in ascending key order.
  - `BeginTx`, use `q.WithTx(tx)`, loop assigning `float64(i)` to each.
  - Commit; defer rollback on error.
- [ ] Run tests → green.
- [ ] Commit:
  ```bash
  git add internal/task/service.go internal/task/service_test.go && git commit -m "feat(task): add rebalance pass for fractional keys"
  ```

## Task 12: List with dynamic filter/sort

**Files:** `internal/task/service.go`, `internal/task/service_test.go`

- [ ] **Tests cover:**
  - Default sort (no filter): returns all rows ordered by `priority ASC, id ASC`.
  - `States: [StateNotDone]` filters out done/cancelled.
  - `Search: "MILK"` (uppercase) matches a task with `"milk"` in notes (case-insensitive).
  - Sort by `SortTitle` returns alphabetical order.
  - Filter `Due: DueToday` matches a task whose `due_date == today's local date`.
- [ ] Implement: build SQL dynamically into a `strings.Builder`, append params to `[]any`:
  - Base: `SELECT id FROM tasks WHERE 1=1`.
  - State: `AND state IN (?, ?, …)` when `len(States) > 0`.
  - Tags (AND semantics): `AND id IN (SELECT task_id FROM task_tags WHERE tag_id IN (?,…) GROUP BY task_id HAVING COUNT(DISTINCT tag_id) = ?)`.
  - Due ranges via `date('now')` / `date('now', '+7 days')`.
  - Search: `AND (LOWER(title) LIKE ? OR LOWER(notes) LIKE ?)` with `%lower(search)%`.
  - Sort: priority asc default; due_date sort puts NULLs last (`due_date IS NULL, due_date ASC`); title uses `LOWER(title)`. Honor `Ascending` (only for non-priority axes).
  - Apply `LIMIT` / `OFFSET` when set.
  - For each returned id, call `q.GetTask` + `loadTags` and assemble `Task`.
- [ ] Run tests → green.
- [ ] Commit:
  ```bash
  git add internal/task/service.go internal/task/service_test.go && git commit -m "feat(task): add list with dynamic filter/sort"
  ```

## Task 13: ByScript / LatestBySpawningScript / SetTagsByID

**Files:** `internal/task/service.go`, `internal/task/service_test.go`

- [ ] **Tests cover:**
  - Create one task with `SpawnedByScriptID = &sid`, one without; `ByScript(sid, 10, 0)` returns only the spawned one.
  - Create two tasks with the same `SpawnedByScriptID` separated by `time.Sleep(1100ms)` (so `created_at` differs at second granularity); `LatestBySpawningScript(sid)` returns the newer one.
  - `LatestBySpawningScript` on a script with no spawned tasks returns `(nil, nil)`.
  - `SetTagsByID(taskID, []int64{tagA, tagB})` then `Get(taskID)` returns those tag names (via the join). Calling again with a different set replaces the previous.
- [ ] Implement:
  - `ByScript` calls `q.ListTasksByScript`, slices by `offset`/`limit` in Go (sqlc query doesn't take them).
  - `LatestBySpawningScript` calls `q.LatestTaskBySpawningScript`; on `sql.ErrNoRows` return `(nil, nil)`.
  - `SetTagsByID` calls `q.ReplaceTaskTags(taskID)`, then loops `q.AddTaskTag` for each id.
- [ ] At the end of `service.go` add a compile-time interface check: `var _ Service = (*Impl)(nil)`.
- [ ] Run `go test ./internal/task/... -v` → all green.
- [ ] Commit:
  ```bash
  git add internal/task/service.go internal/task/service_test.go && git commit -m "feat(task): add ByScript lookup and tag attachment"
  ```

## Phase completion checklist

- [ ] `go test ./internal/task/... -v` all pass.
- [ ] `go build ./...` clean.
- [ ] `var _ Service = (*Impl)(nil)` compiles in `service.go`.
- [ ] All commits land with conventional messages.
