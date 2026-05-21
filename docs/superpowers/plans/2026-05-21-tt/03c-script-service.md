# Phase 03c — Script Domain Service

> Read `00-index.md` first. Commit after each task. Parallelizable with 03a (tasks) and 03b (tags).

**Goal:** Script CRUD, schedule parsing + matching (Go-side), `DueAt(now)`, run lifecycle (`StartRun` → `AppendLog` → `FinishRun`), user-state read/write, FIFO retention pruning (≤500), and startup recovery of orphaned `running` rows.

**Dependencies:** Phase 02.

**Parallelizable with:** 03a, 03b.

## File map

```
internal/db/queries/scripts.sql     # replace stub
internal/db/queries/runs.sql        # replace stub (script_runs + script_logs)
internal/script/
├── types.go
├── schedule.go
├── schedule_test.go
├── service.go
└── service_test.go
```

## Task 1: Replace `queries/scripts.sql`

**Files:** `internal/db/queries/scripts.sql`

Copy verbatim:

```sql
-- name: CreateScript :one
INSERT INTO scripts (name, code, enabled, schedule_kind, schedule_config)
VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: GetScript :one
SELECT * FROM scripts WHERE id = ?;

-- name: ListScripts :many
SELECT * FROM scripts ORDER BY name ASC, id ASC;

-- name: ListEnabledScripts :many
SELECT * FROM scripts WHERE enabled = 1;

-- name: UpdateScript :one
UPDATE scripts
SET name = ?, code = ?, enabled = ?, schedule_kind = ?, schedule_config = ?,
    updated_at = datetime('now')
WHERE id = ? RETURNING *;

-- name: DeleteScript :exec
DELETE FROM scripts WHERE id = ?;

-- name: SetScriptLastRunAt :exec
UPDATE scripts SET last_run_at = ?, updated_at = datetime('now') WHERE id = ?;

-- name: SetScriptUserState :exec
UPDATE scripts SET user_state = ?, updated_at = datetime('now') WHERE id = ?;

-- name: GetScriptUserState :one
SELECT user_state FROM scripts WHERE id = ?;
```

## Task 2: Replace `queries/runs.sql`

**Files:** `internal/db/queries/runs.sql`

Copy verbatim:

```sql
-- name: CreateScriptRun :one
INSERT INTO script_runs (script_id, trigger, status)
VALUES (?, ?, 'running') RETURNING *;

-- name: FinishScriptRun :exec
UPDATE script_runs
SET finished_at = datetime('now'), status = ?, error_message = ?,
    spawned_task_ids = ?
WHERE id = ?;

-- name: GetScriptRun :one
SELECT * FROM script_runs WHERE id = ?;

-- name: ListScriptRunsByScript :many
SELECT * FROM script_runs WHERE script_id = ?
ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?;

-- name: ListAllScriptRuns :many
SELECT * FROM script_runs ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?;

-- name: CountScriptRuns :one
SELECT COUNT(*) FROM script_runs;

-- name: DeleteOldestScriptRuns :exec
DELETE FROM script_runs WHERE id IN (
  SELECT id FROM script_runs ORDER BY started_at ASC, id ASC LIMIT ?
);

-- name: MarkOrphanedRunsAsError :exec
UPDATE script_runs
SET status = 'error', error_message = 'interrupted (binary restart)',
    finished_at = datetime('now')
WHERE status = 'running';

-- name: AppendScriptLog :exec
INSERT INTO script_logs (script_run_id, level, message) VALUES (?, ?, ?);

-- name: ListScriptLogsByRun :many
SELECT * FROM script_logs WHERE script_run_id = ?
ORDER BY logged_at ASC, id ASC;
```

- [ ] Run `just db-gen && go build ./...` → clean.
- [ ] Commit:
  ```bash
  git add internal/db/queries/scripts.sql internal/db/queries/runs.sql internal/db/sqlc/ && \
    git commit -m "feat(db): add script and run queries"
  ```

## Task 3: Domain types

**Files:** `internal/script/types.go`

- [ ] Define:
  - `type Kind string` with `KindEveryTick`, `KindDaily`, `KindWeekly`, `KindMonthly`.
  - `type Weekday string` with lowercase `Monday`..`Sunday`.
  - `type Schedule struct { Kind Kind; Weekday Weekday; Day MonthlyDay }`.
  - `type MonthlyDay struct { N int; IsLast bool; Valid bool }` with custom JSON marshal/unmarshal handling both `int` and `"last"`.
  - `type Script { ID int64; Name, Code string; Enabled bool; Schedule Schedule; LastRunAt *time.Time; CreatedAt, UpdatedAt time.Time }`.
  - `CreateInput { Name, Code string; Enabled bool; Schedule Schedule }`. `UpdateInput = CreateInput`.
  - `type Trigger string` with `TriggerScheduled`, `TriggerManual`.
  - `type RunStatus string` with `RunStatusRunning`, `RunStatusOK`, `RunStatusError`, `RunStatusTimeout`.
  - `type Run { ID, ScriptID int64; StartedAt time.Time; FinishedAt *time.Time; Status RunStatus; ErrorMessage string; SpawnedTaskIDs []int64; Trigger Trigger }`.
  - `type LogLevel string` with `LogDebug`, `LogInfo`, `LogWarn`, `LogError`.
  - `type Log { ID, RunID int64; Level LogLevel; Message string; LoggedAt time.Time }`.
- [ ] Verify build.
- [ ] Commit:
  ```bash
  git add internal/script/types.go && git commit -m "feat(script): add domain types"
  ```

## Task 4: Schedule matching — failing tests

**Files:** `internal/script/schedule_test.go`

- [ ] Add a `at(s)` helper parsing `"2006-01-02 15:04"` in UTC.
- [ ] **Tests cover** (calling `Schedule.Matches(now time.Time, lastRunAt *time.Time) bool`):

| Test | Schedule | now | last | Expect |
|---|---|---|---|---|
| every_tick always | `{Kind: EveryTick}` | any | any | true |
| daily, last yesterday | `{Daily}` | 2026-05-21 09:30 | 2026-05-20 23:50 | true |
| daily, last today | `{Daily}` | 2026-05-21 09:30 | 2026-05-21 08:00 | false |
| daily, never | `{Daily}` | 2026-05-21 | nil | true |
| weekly Monday on Mon | `{Weekly, Monday}` | 2026-05-25 (Mon) | nil | true |
| weekly Monday on Tue | `{Weekly, Monday}` | 2026-05-26 | nil | false |
| weekly Monday last on same Mon | `{Weekly, Monday}` | 2026-05-25 09:00 | 2026-05-25 08:00 | false |
| monthly 15 on 15th | `{Monthly, Day=15}` | 2026-05-15 | nil | true |
| monthly 15 on 14th | `{Monthly, Day=15}` | 2026-05-14 | nil | false |
| monthly last on May 31 | `{Monthly, Last}` | 2026-05-31 | nil | true |
| monthly last on May 30 | `{Monthly, Last}` | 2026-05-30 | nil | false |
| monthly last on Feb 28 2026 | `{Monthly, Last}` | 2026-02-28 | nil | true |

- [ ] Run → undefined `Matches`.

## Task 5: Schedule — implementation

**Files:** `internal/script/schedule.go`

- [ ] Implement `Schedule.Matches(now time.Time, lastRunAt *time.Time) bool` using this truth table (spec §5):

```
every_tick → true
daily      → notRunToday(now, last)
weekly     → weekdayMatches(now, sch.Weekday) AND notRunToday(now, last)
monthly    → !sch.Day.Valid           → false
             sch.Day.IsLast           → isLastOfMonth(now) AND notRunToday(now, last)
             else                     → now.Day() == sch.Day.N AND notRunToday(now, last)
```

Helpers:
- `notRunToday(now, last)`: if `last == nil` true; else `!sameLocalDate(now, *last)`.
- `sameLocalDate(a, b)`: compare `.Year()` + `.YearDay()` on `.Local()`.
- `weekdayMatches(now, w)`: case-insensitive equality of `now.Weekday().String()` to `string(w)`.
- `isLastOfMonth(now)`: add one day and check `next.Month() != now.Month()`.

- [ ] Implement `ParseSchedule(kind, configJSON string) (Schedule, error)`:
  - `every_tick` / `daily`: no extras.
  - `weekly`: unmarshal `{weekday: string}`; validate against the seven weekday constants.
  - `monthly`: unmarshal `{day: ...}` (using `MonthlyDay`'s custom unmarshal); validate `IsLast` or `1 ≤ N ≤ 31`.
- [ ] Implement `(s Schedule) MarshalConfig() (string, error)` returning the JSON shape that round-trips through `ParseSchedule`:
  - every_tick / daily → `"{}"`.
  - weekly → `{"weekday":"<wd>"}`.
  - monthly → `{"day":<n or "last">}`.
- [ ] Run schedule tests → all green.
- [ ] Commit:
  ```bash
  git add internal/script/schedule.go internal/script/schedule_test.go && \
    git commit -m "feat(script): add schedule parsing and matching"
  ```

## Task 6: Service — failing tests

**Files:** `internal/script/service_test.go`

- [ ] **Tests cover:**
  - `Create` + `Get` round-trips name, code, schedule (including weekly weekday).
  - `Update` replaces fields including switching schedule kind.
  - `DueAt(monday)` returns only enabled scripts whose schedule matches; disabled scripts excluded.
  - Run lifecycle:
    - `StartRun` returns a run with `Status == RunStatusRunning`, `StartedAt` set.
    - `AppendLog(runID, LogInfo, "hello")` then `GetLogs(runID)` returns one log with that message.
    - `FinishRun(runID, RunStatusOK, "", []int64{42})` updates the row; `GetRun` reflects status and `SpawnedTaskIDs == [42]`.
  - `RecoverOrphanedRuns()` after `StartRun` (no Finish) sets the run's status to `RunStatusError` with the recovery message.
  - `WriteUserState(id, []byte("{\"k\":1}"))` then `ReadUserState(id)` returns the same bytes.
  - Retention: insert 510 runs (each `Start` + `Finish`), call `PruneRuns(500)`, then `CountRuns() == 500`.
- [ ] Run → undefined symbols.

## Task 7: Service — implementation

**Files:** `internal/script/service.go`

- [ ] Define the `Service` interface and `Impl` struct. The full method list:
  - CRUD: `Create`, `Update`, `Delete`, `Get`, `List`.
  - Scheduling: `DueAt(now)`, `SetLastRunAt(id, t)`.
  - State: `ReadUserState`, `WriteUserState`.
  - Run lifecycle: `StartRun`, `FinishRun`, `AppendLog`, `GetRun`, `GetLogs`, `ListRunsByScript`, `ListAllRuns`, `CountRuns`, `PruneRuns`, `RecoverOrphanedRuns`.
- [ ] Implementation notes:
  - On Create/Update: trim name; validate non-empty; call `Schedule.MarshalConfig` for the stored JSON.
  - `Enabled` stored as 0/1 via a `boolToInt` helper; `Script.Enabled = r.Enabled == 1`.
  - `DueAt`: load `ListEnabledScripts`, loop, call `sc.Schedule.Matches(now, sc.LastRunAt)`.
  - `SetLastRunAt`: store as `"2006-01-02 15:04:05"` UTC.
  - `WriteUserState`: empty blob → store `"{}"`.
  - `FinishRun`: marshal `spawnedIDs` to JSON (nil → `[]`); `errMsg == ""` → invalid `NullString`.
  - `rowToRun`: unmarshal `SpawnedTaskIDs` from JSON; default to `[]int64{}` on empty.
  - `PruneRuns(keep)`: `CountRuns()` → if `keep >= count` return; else `q.DeleteOldestScriptRuns(count - keep)`. Cascade removes logs.
  - `parseSqliteTime` mirrors the task/tag versions.
- [ ] Add compile-time check: `var _ Service = (*Impl)(nil)`.
- [ ] Run `go test ./internal/script/... -v` → all green.
- [ ] Commit:
  ```bash
  git add internal/script/service.go internal/script/service_test.go && \
    git commit -m "feat(script): add script CRUD, run lifecycle, retention, recovery"
  ```

## Phase completion checklist

- [ ] `go test ./internal/script/... -v` all pass.
- [ ] `go build ./...` clean.
- [ ] `var _ Service = (*Impl)(nil)` compiles.
