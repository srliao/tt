# Navigation — Where does X live?

Quick locator tables. When you need to read code, start here.

## Backend by concept

| Concept | File(s) |
|---|---|
| Process entry, dependency wiring, signal handling | `cmd/tt/main.go` |
| CLI flags, data-dir resolution | `internal/config/config.go` |
| SQLite open + WAL + goose migrations | `internal/db/store.go` |
| Schema (DDL) | `internal/db/migrations/0001_init.sql` |
| sqlc queries | `internal/db/queries/{tasks,tags,scripts,runs}.sql` |
| Generated sqlc Go (read-only) | `internal/db/sqlc/*.sql.go` |
| Test helper (in-memory store) | `internal/db/dbtest/dbtest.go` |
| Task CRUD, reorder, list with filters | `internal/task/service.go` |
| Task fractional-key math (Midpoint, rebalance) | `internal/task/reorder.go` |
| Task domain types, enums (State, DueRange, SortAxis) | `internal/task/types.go` |
| Tag CRUD + Resolve (auto-create) | `internal/tag/service.go` |
| Tag domain types | `internal/tag/types.go` |
| Script CRUD, run lifecycle, retention pruning | `internal/script/service.go` |
| Script schedule parsing + matching | `internal/script/schedule.go` |
| Script domain types (Kind, Trigger, RunStatus, LogLevel) | `internal/script/types.go` |
| Runtime entry (one run lifecycle) | `internal/runtime/runner.go` |
| `ctx` object installer | `internal/runtime/ctx.go` |
| `ctx.today/weekday/parseDate/addDays/...` | `internal/runtime/ctx_dates.go` |
| `ctx.state.{get,set,delete,all}` + buffer | `internal/runtime/ctx_state.go` |
| `ctx.queueTask` + buffer | `internal/runtime/ctx_queue.go` |
| `ctx.log.*` + `console.*` install | `internal/runtime/ctx_log.go` |
| Scheduler ticker / worker / queue | `internal/scheduler/scheduler.go`, `internal/scheduler/worker.go` |
| HTTP server wiring + consumer interfaces | `internal/httpapi/server.go` |
| Task handlers + filter parsing + service-error mapper | `internal/httpapi/tasks.go` |
| Stage handlers (reorder, clear, clear-finished) | `internal/httpapi/stage.go` |
| Tag handlers | `internal/httpapi/tags.go` |
| Script handlers + manual run | `internal/httpapi/scripts.go` |
| Run-log handlers | `internal/httpapi/runs.go` |
| Request-id / log / panic-recovery middleware | `internal/httpapi/middleware.go` |
| SPA fallback handler + cache headers | `internal/httpapi/spa.go` |
| `/health`, `/version` | `internal/httpapi/health.go` |
| Error envelope, code constants | `internal/httpapi/errors.go` |
| Embed.FS for SPA bundle | `internal/web/assets.go` |

## Frontend by concept

| Concept | File(s) |
|---|---|
| Entry point | `web/src/main.tsx` |
| TanStack Router setup | `web/src/router.tsx` |
| File-based routes | `web/src/routes/*.tsx` |
| Generated route tree (do not edit) | `web/src/routeTree.gen.ts` |
| Global layout / top nav / stage badge | `web/src/components/layout.tsx` |
| Theme provider + toggle | `web/src/components/theme-{provider,toggle}.tsx` |
| Keyboard shortcut cheatsheet (`?`) | `web/src/components/shortcut-cheatsheet.tsx` |
| Global keyboard shortcuts (`n`, `g X`, `?`) | `web/src/lib/shortcuts.ts` |
| Command palette (`/`, ⌘K) — tasks/tags search + nav | `web/src/components/command-palette.tsx` |
| fetch wrapper + `ApiError` | `web/src/lib/api.ts` |
| TanStack Query client | `web/src/lib/query.ts` |
| shadcn/ui primitives | `web/src/components/ui/*.tsx` |
| Type mirrors of Go DTOs | `web/src/types/{task,tag,script,run}.ts` |
| API hooks (one file per resource) | `web/src/api/{tasks,tags,scripts,runs,stage}.ts` |
| Tasks page + table + row + filters + bulk bar + inline tag editor | `web/src/features/tasks/*` |
| Stage page + list + row + soft-cap hint | `web/src/features/stage/*` |
| Tags page | `web/src/features/tags/*` |
| Scripts list + editor + cheatsheets + spawned-tasks panel | `web/src/features/scripts/*` |
| Runs list + detail + logs table + status pill | `web/src/features/runs/*` |
| URL-driven filter state for /tasks | `web/src/features/tasks/use-task-list-search.ts` |
| Active-filter strip (chips above the task table) | `web/src/features/tasks/active-filter-strip.tsx` |

## Build / dev / tooling

| Concept | File |
|---|---|
| Justfile (single source of dev commands) | `justfile` |
| Go module | `go.mod` |
| sqlc config | `internal/db/sqlc.yaml` |
| Vite config + `/api` proxy | `web/vite.config.ts` |
| Vitest setup | `web/vitest.config.ts`, `web/vitest.setup.ts` |
| Biome lint/format | `web/biome.json` |
| TanStack Router config | `web/tsr.config.json` |
| shadcn components.json | `web/components.json` |

## Tests by location

Every Go file has a `_test.go` sibling. Every frontend `.tsx`/`.ts` typically has a `.test.tsx`/`.test.ts` sibling. Examples:

- `internal/task/service_test.go` — task service unit tests against in-memory SQLite
- `internal/runtime/runner_test.go` — full runtime exercise: every `ctx.*` method, timeout, state buffering, queue flush
- `internal/scheduler/scheduler_test.go` — fake clock + fake runner, schedule matching
- `internal/httpapi/*_test.go` — `httptest` + service stubs OR real services
- `web/src/features/tasks/task-table.test.tsx` — Vitest + RTL
- `web/src/api/tasks.test.tsx` — mocked-fetch hook tests

## Search recipes

```bash
# Find handler for an endpoint
grep -rn "POST.*reorder" internal/httpapi/

# Find a sqlc query name
grep -rn "name: ListTasksByScript" internal/db/queries/

# Find a JS-exposed ctx method
grep -rn "ctx.today\|today.*func()" internal/runtime/

# Find the React hook for a server resource
grep -rn "queryKey: \['tasks'" web/src/

# Find tests for a piece of behavior
grep -rln "func Test.*Reorder" internal/task/
```
