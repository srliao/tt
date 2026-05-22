# Data Layer

SQLite (pure-Go `modernc.org/sqlite`, no cgo) + `goose` migrations + `sqlc`-generated query code.

## Tables (snapshot)

```
tasks        ── id, title, notes, state, due_date, priority (REAL),
                staged_order (REAL nullable), spawned_by_script_id,
                created_at, completed_at, cancelled_at, updated_at
tags         ── id, name UNIQUE, created_at
task_tags    ── (task_id, tag_id) PK, both ON DELETE CASCADE
scripts      ── id, name, code, enabled, schedule_kind, schedule_config JSON,
                user_state JSON, last_run_at, created_at, updated_at
script_runs  ── id, script_id FK, started_at, finished_at, status, error_message,
                spawned_task_ids JSON, trigger
script_logs  ── id, script_run_id FK CASCADE, level, message, logged_at
```

DDL source: `internal/db/migrations/0001_init.sql`. Spec semantic rules are at `docs/superpowers/specs/2026-05-21-task-tracker-design.md` §3 — read those before touching schema.

## Adding columns or tables

1. **New migration**: `internal/db/migrations/000N_<name>.sql` with `-- +goose Up` / `-- +goose Down` markers. Numbered sequentially.
2. **Update sqlc queries** if needed in `internal/db/queries/*.sql`. The schema source for sqlc is the migrations directory (see `internal/db/sqlc.yaml`).
3. **Regen**: `just db-gen`.
4. **Update domain types** in `internal/{task,tag,script}/types.go` and `rowTo*` projectors in the service file.
5. **Update TypeScript mirrors** in `web/src/types/*.ts` if it's user-facing.

Migrations are embedded via `internal/db/migrations/embed.go`. They run on every startup; failures abort startup with a clear error.

## sqlc conventions

- Queries grouped by table: `tasks.sql`, `tags.sql`, `scripts.sql`, `runs.sql`.
- Generated code lives in `internal/db/sqlc/` — **do not edit by hand**. Always re-run `just db-gen`.
- Config: `emit_pointers_for_null_types: true` → nullable columns map to `*string` / `*float64` / `*int64`.
- `RETURNING *` is used everywhere a row is needed back after a write.
- Use `s.q.WithTx(tx)` to bind queries inside a transaction (see `RebalancePriority`, `SetTagsByID`).

## When dynamic SQL is needed

`task.Impl.List` builds the query as a string because the filter shape (states, tags, due range, search, sort axis, asc/desc, limit/offset) is too varied for sqlc to model cleanly. Pattern: build into `strings.Builder`, push args into `[]any`, run via `s.store.DB().QueryContext`, then re-fetch each row through `s.q.GetTask` for typed projection.

Don't replicate this for simple filters — prefer adding a named query.

## Timestamps

- Stored as TEXT in SQLite's `datetime('now')` layout (`2006-01-02 15:04:05`, UTC, second precision).
- Some code paths may write RFC3339 (e.g., `time.Time.Format` defaults). Every parser must accept both — see `parseSqliteTime` in `task/service.go`, `tag/service.go`, `script/service.go`. **Duplicated intentionally** so packages don't cross-depend; keep them in sync if you fix one.
- `due_date` is `YYYY-MM-DD` only (no time component).

## Fractional ordering keys

- `tasks.priority` (main-list order, NOT NULL DEFAULT 0).
- `tasks.staged_order` (stage order, NULL when not staged).
- Created at next `MAX(...) + 1.0`.
- Reorder picks midpoint between visible neighbors. See `internal/task/reorder.go`:
  - `Midpoint(before, after *float64) float64` — nil = top/bottom edge.
  - `NeedsRebalance(a, b float64) bool` — true when `|b-a| < 1e-9`.
  - `RebalancePriority` / `RebalanceStage` — re-spread to `0, 1, 2, ...` inside a tx.

State transitions **do not change `staged_order`** — done/cancelled tasks stay visible in the stage so users see progress through the batch.

## Run log retention

- Cap: 500 rows in `script_runs`.
- Pruned at the end of every `runtime.Runner.Run` invocation (see `runRetention` in `internal/runtime/runner.go`).
- Pruning oldest cascades through `script_logs` automatically (FK with `ON DELETE CASCADE`).

## Startup recovery

On binary start, `script.Service.RecoverOrphanedRuns` marks any `status='running'` rows as `'error'` with message `interrupted (binary restart)`. Called by `scheduler.Start` before the ticker begins. See `internal/db/queries/runs.sql:MarkOrphanedRunsAsError`.
