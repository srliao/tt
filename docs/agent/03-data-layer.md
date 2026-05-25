# Data Layer

SQLite (pure-Go `modernc.org/sqlite`, no cgo) + `goose` migrations + `sqlc`-generated query code.

## Tables (snapshot)

```
tasks        ── id, title, notes, state, due_date, priority (REAL),
                staged_order (REAL nullable), spawned_by_script_id,
                created_at, completed_at, cancelled_at, updated_at
tags         ── id, name UNIQUE, color_hue (INTEGER NOT NULL DEFAULT 0), created_at
task_tags    ── (task_id, tag_id) PK, both ON DELETE CASCADE
scripts      ── id, name, code, enabled, schedule_kind, schedule_config JSON,
                user_state JSON, last_run_at, created_at, updated_at
script_runs  ── id, script_id FK, started_at, finished_at, status, error_message,
                spawned_task_ids JSON, trigger
script_logs  ── id, script_run_id FK CASCADE, level, message, logged_at
```

DDL source: `internal/db/migrations/0001_init.sql`. Read it directly before touching schema — it has inline comments on the load-bearing rules (fractional ordering, ON DELETE CASCADE/SET NULL choices, etc.).

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
- Use `s.q.WithTx(tx)` to bind queries inside a transaction (see `RebalancePriority`, `SetTagsByID`, `BulkTag`).
- `tags.color_hue` is the persisted per-tag chip color (HSL hue, 0-330 in 30° steps). The canonical 12-hue palette is **duplicated** between `internal/tag/types.go` (`HuePalette`) and `web/src/lib/tag-color.ts` (`HUES`) — keep both in sync if expanded. Service `Create`/`Resolve(autoCreate=true)` assign via `pickLeastUsedHue` (see [04-backend-services.md](./04-backend-services.md)).
- Multi-task tag mutations go through `task.Impl.BulkTag` (one tx for the whole selection). Handler in `internal/httpapi/tasks.go:handleBulkTag` resolves names → ids (autoCreate for add/set, `ResolveExisting` for remove so unknown names are silently ignored), service operates on ids only. The slice-based `DeleteTaskTagsForTask` query uses `sqlc.slice('tag_ids')` for the remove path. Service validation requires non-empty `TagIDs` only for `add`; `remove` with empty `TagIDs` is a silent no-op (the all-unknown case, returns 200 with unchanged tasks), and `set` with empty `TagIDs` is the explicit clear-all pathway. The handler still rejects an empty raw `tags` array at the boundary.

## When dynamic SQL is needed

`task.Impl.List` builds the query as a string because the filter shape (states, tags, tag exclusion, due range, search, sort axis, asc/desc, limit/offset) is too varied for sqlc to model cleanly. Pattern: build into `strings.Builder`, push args into `[]any`, run via `s.store.DB().QueryContext`, then re-fetch each row through `s.q.GetTask` for typed projection.

Tag inclusion is parameterised by `FilterSort.Tags` (`task.TagFilter{Mode, RealTagIDs, IncludeUntagged}`):

- `Any` + ids: `id IN (SELECT … tag_id IN (…))`.
- `All` + ids: same sub-select with `GROUP BY task_id HAVING COUNT(DISTINCT tag_id) = N`.
- `IncludeUntagged` alone: `NOT EXISTS (SELECT 1 FROM task_tags …)`.
- `Any` + ids + `IncludeUntagged`: union of the two clauses with OR.
- `All` + ids + `IncludeUntagged`: impossible set, short-circuits to `0 = 1`.

The HTTP layer resolves tag NAMES → ids and strips the `@untagged` sentinel before constructing the `TagFilter` (see `parseTagFilter` in `internal/httpapi/tasks.go`). The service never touches tag-name resolution. `FilterSort.TagExcludeIDs` adds a `NOT IN (SELECT task_id FROM task_tags WHERE tag_id IN (...))` clause and composes with inclusion via AND.

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
