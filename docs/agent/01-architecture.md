# Architecture

## Process model

Single Go binary, three concurrent subsystems sharing one SQLite file:

```
                ┌──────────────────────────────────────┐
                │              cmd/tt/main             │
                │   (only place that wires concretes)  │
                └──────────────────────────────────────┘
                                 │
        ┌────────────────────────┼─────────────────────────┐
        ▼                        ▼                         ▼
   HTTP server            Scheduler                  Runtime
   (chi router)           (15min ticker +            (goja JS engine,
   /api/v1/* + SPA         single worker)             one runtime per run)
        │                        │                         │
        └────────────┬───────────┴────────────┬────────────┘
                     ▼                        ▼
              ┌──────────────┐         ┌──────────────┐
              │ Domain svcs  │         │ Domain svcs  │
              │ task / tag / │         │ via consumer │
              │ script       │         │  interfaces  │
              └──────┬───────┘         └──────────────┘
                     ▼
              ┌──────────────┐
              │   db.Store   │
              │  (sqlc + WAL │
              │   SQLite)    │
              └──────────────┘
```

## Layer map (top-down dependencies)

```
cmd/tt              ── wires everything (only place with concrete types)
  └─ internal/httpapi      HTTP transport, validation, error envelope
        ├─ internal/scheduler   ticker + worker goroutines
        │     └─ internal/runtime    goja per-run isolated JS exec
        │           ├─ task svc
        │           ├─ tag svc
        │           └─ script svc
        ├─ internal/task         task domain (CRUD, fractional ordering, filter/sort)
        ├─ internal/tag          tag domain (CRUD + Resolve)
        ├─ internal/script       script domain (CRUD, schedule parsing, run lifecycle)
        └─ internal/web          embed.FS for the built SPA
              └─ internal/db          *sql.DB, sqlc-generated queries, goose migrations
                    ├─ migrations/    *.sql (goose Up/Down)
                    ├─ queries/       *.sql (sqlc input)
                    └─ sqlc/          generated Go (do not edit)
```

## Core design rules

These are load-bearing — violating them breaks tests or causes subtle bugs.

1. **Consumer declares the interface.** Each downstream package defines a narrow interface naming only the methods it uses. The producer's `Impl` satisfies it structurally. Search any service file for `type Service interface` to see this pattern. Examples:
   - `internal/scheduler/scheduler.go` declares `Runner`, `ScriptLookup`
   - `internal/httpapi/server.go` declares `TaskService`, `TagService`, `ScriptService`, `ManualRunEnqueuer`, `Pinger`
   - `internal/task/service.go` exposes the full `task.Service` interface as documentation; HTTP / runtime each declare narrower views

2. **Only `cmd/tt/main.go` knows concrete types.** Every other file accepts an interface. New services must follow this — never reach into another package's `*Impl`.

3. **`ctx` API never calls SQL directly.** Runtime bindings call `task.Service` / `tag.Service` / `script.Service`. The runtime owns its own in-memory **buffers** (state, queued tasks) that flush only on `RunStatusOK`. See [05-runtime.md](./05-runtime.md).

4. **Fractional ordering keys (`priority`, `staged_order`).** Drag-drop computes midpoint between neighbors. When neighbors are within `1e-9`, a rebalance pass re-spreads keys to integer values. See `internal/task/reorder.go` for `Midpoint` / `NeedsRebalance` / `EvenSpread`. Mutations live in `internal/task/service.go` (`ReorderMain`, `ReorderStage`, `RebalancePriority`, `RebalanceStage`).

5. **Single goja runtime per script run.** No sharing between runs. Sandboxing = `goja.New()` + deleted globals (`setTimeout`, `setInterval`, `fetch`, `process`, `require`) + 5s `Interrupt` budget.

6. **One scheduler worker.** All script execution is sequential — local single-user; race conditions in user scripts are not a concern. Queue is size-100; manual-trigger overflow returns 503, scheduled overflow is silently dropped + counted.

7. **All timestamps as TEXT.** Code uses SQLite's `datetime('now')` ("YYYY-MM-DD HH:MM:SS") OR RFC3339 — every parse path accepts both. See `parseSqliteTime` in `task/service.go`, `tag/service.go`, `script/service.go` (intentionally duplicated to keep packages self-contained).

8. **JSON error envelope.** All non-2xx responses share `{"error": {"code", "message", "details?"}}`. Codes are stable strings; see `internal/httpapi/errors.go`. The mapper is `writeServiceError` in `tasks.go` — string-based to avoid importing the scheduler/sqlite packages.

## Cross-cutting concerns

- **Logging**: `slog` text handler to stderr. Middleware emits one line per HTTP request with `request_id`. Script-internal logs go to `script_logs` table, NOT stderr.
- **Panic recovery**: at three boundaries — HTTP middleware, scheduler worker per-job, runtime `Run` (outside `execute`) AND `execute` itself.
- **Embedded assets**: `internal/web/dist/` (SPA, populated by `just build`) and `internal/db/migrations/*.sql` (always present).
- **Schema-ahead detection**: if DB migration version > what binary knows, goose aborts startup. Intentional.

## What's NOT here (designed-for but unbuilt)

Listed in the spec §10. Common gotchas to avoid wasting time:

- No `ctx.tasks.byTag/byState`, no `ctx.stage.*` mutations — only `ctx.queueTask`.
- No `ctx.dryRun`, no notifications, no SSE/WebSocket — UI refetches on user action.
- No auth, multi-user, mobile UI, recurring sub-tasks.
- No metrics, tracing, or external error reporting.
