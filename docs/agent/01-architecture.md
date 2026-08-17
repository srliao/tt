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

4. **Fractional ordering keys (`priority`, `staged_order`).** Lists render **ascending**, but new keys are minted at the **low** end — `Create` uses `MIN(priority) - 1.0` and `Stage` uses `MIN(staged_order) - 1.0`, so the newest item sorts first. Keys drift negative over time; that is intended and harmless for float64. Drag-drop computes midpoint between neighbors; when neighbors are within `1e-9`, a rebalance pass re-spreads keys to `0..n-1` ascending. See `internal/task/reorder.go` for `Midpoint` / `NeedsRebalance` / `EvenSpread`. Mutations live in `internal/task/service.go` (`ReorderMain`, `ReorderStage`, `RebalancePriority`, `RebalanceStage`). **Corollary:** insertion (rowid) order is the reverse of list order, so never use `id ASC` as a proxy for display order — see the reverse flush in `Runner.flushQueue` ([05-runtime.md](./05-runtime.md)).

5. **Single goja runtime per script run.** No sharing between runs. Sandboxing = `goja.New()` + deleted globals (`setTimeout`, `setInterval`, `fetch`, `process`, `require`) + 5s `Interrupt` budget.

6. **One scheduler worker.** All script execution is sequential — local single-user; race conditions in user scripts are not a concern. Queue is size-100; manual-trigger overflow returns 503, scheduled overflow is silently dropped + counted.

7. **All timestamps as TEXT.** Code uses SQLite's `datetime('now')` ("YYYY-MM-DD HH:MM:SS") OR RFC3339 — every parse path accepts both. See `parseSqliteTime` in `task/service.go`, `tag/service.go`, `script/service.go` (intentionally duplicated to keep packages self-contained).

8. **JSON error envelope.** All non-2xx responses share `{"error": {"code", "message", "details?"}}`. Codes are stable strings; see `internal/httpapi/errors.go`. The mapper is `writeServiceError` in `tasks.go` — string-based to avoid importing the scheduler/sqlite packages.

9. **Task selection lives in `sessionStorage`, not the URL.** `useSelection` in `web/src/features/tasks/use-selection.ts` is a module-level store + `useSyncExternalStore` subscription, persisted under key `tt:selection`. Every caller (`<TaskTable>`, `<BulkActionBar>`, `<BulkTagEditor>`, `<CommandPalette>`) shares the same snapshot so cross-component mutations stay consistent. URL is reserved for filters and transient *signals* (`open`, `openBulkTagEditor`, `confirmBulkDelete`, `confirmBulkCancel`) — never the selection set itself.

10. **Bulk-tag is one transaction.** `task.Impl.BulkTag` (see `internal/task/service.go`) opens a single tx and runs INSERT OR IGNORE / DELETE / REPLACE per op across the supplied task ids, then reloads the affected rows in request-order. Tag name → id resolution happens at the HTTP boundary (`handleBulkTag` in `internal/httpapi/tasks.go`): `tag.Resolve` (auto-create) for add/set, `tag.ResolveExisting` (silently drop unknown) for remove. Service operates on ids only.

11. **Document-level keydown for the task table.** `useTableShortcuts` in `web/src/features/tasks/task-table.tsx` binds to `document` (not the table element) so j/k/x/⌘A fire regardless of focus. Guards: a `disabled` flag, `isEditableTarget(target)`, and an open Radix dialog probe (`[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"]`). Replicate this guard set when adding any document-scoped shortcut on a page.

## Cross-cutting concerns

- **Logging**: `slog` text handler to stderr. Middleware emits one line per HTTP request with `request_id`. Script-internal logs go to `script_logs` table, NOT stderr.
- **Panic recovery**: at three boundaries — HTTP middleware, scheduler worker per-job, runtime `Run` (outside `execute`) AND `execute` itself.
- **Embedded assets**: `internal/web/dist/` (SPA, populated by `just build`) and `internal/db/migrations/*.sql` (always present).
- **Schema-ahead detection**: if DB migration version > what binary knows, goose aborts startup. Intentional.

## Container deployment mode

In production the binary runs inside a container image (`Dockerfile`) with Litestream streaming SQLite to Cloudflare R2. The process model is unchanged — single binary, same three subsystems — but startup is wrapped:

```
docker/entrypoint.sh
  └─ litestream replicate -exec "tt --data-dir /data --port 8080"
         └─ (replication side-channel to R2)
```

The image is multi-arch (linux/amd64 + linux/arm64), built by `.github/workflows/build.yml` and pushed to `ghcr.io/srliao/tt`. `docker-compose.yml` adds a `cloudflared` sidecar for tunnel access; no host ports are exposed. See `docs/deployment.md` for the operator runbook.

## What's NOT here (designed-for but unbuilt)

Listed in the spec §10. Common gotchas to avoid wasting time:

- No `ctx.tasks.byTag/byState`, no `ctx.stage.*` mutations — only `ctx.queueTask`.
- No `ctx.dryRun`, no notifications, no SSE/WebSocket — UI refetches on user action.
- No auth, multi-user, mobile UI, recurring sub-tasks.
- No metrics, tracing, or external error reporting.
