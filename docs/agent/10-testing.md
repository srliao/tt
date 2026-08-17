# Testing

## Layout

Test files live next to the code they exercise.

- Go: `foo.go` → `foo_test.go`. Same package; package-private tests are normal.
- Frontend: `Component.tsx` → `Component.test.tsx`. Hooks: `useFoo.ts` → `useFoo.test.ts`.

## Backend (Go)

### Test DB

`internal/db/dbtest/dbtest.go` provides `dbtest.New(t) *db.Store` — fresh in-memory SQLite (`:memory:`) with migrations applied, auto-closed via `t.Cleanup`.

```go
func TestThing(t *testing.T) {
    store := dbtest.New(t)
    svc := task.New(store)
    // ...
}
```

In-memory DB uses `SetMaxOpenConns(1)` to pin all calls to the same connection (otherwise each `sql.Conn` sees an empty `:memory:` instance).

### Service tests

Real DB, real service. Hit every state-transition path, every filter axis, every reorder edge case. Examples:

- `internal/task/service_test.go` — CRUD, state transitions, filter+sort matrix, reorder midpoint correctness, newest-first key minting (`Create` / `Stage` land at the top of the ascending list).
- `internal/task/reorder_test.go` — pure-math tests on `Midpoint`, `NeedsRebalance`, `EvenSpread`.
- `internal/tag/service_test.go` — Resolve (autoCreate=true / false), dedupe, unknown-tag error message.
- `internal/script/service_test.go` — CRUD, run lifecycle, retention pruning, orphan recovery.
- `internal/script/schedule_test.go` — schedule matching truth table, day-boundary edge cases.

### Runtime tests

`internal/runtime/runner_test.go` — real `goja`, real services on `dbtest.New`. Each ctx.* method has a test that builds a tiny script string, runs it, and asserts on the DB state / log rows. Also exercises the 5s timeout path, error handling, state buffer atomicity (queue/state discarded on error/timeout), and the reverse flush (a multi-item `ctx.queueTask` batch must read top-down in spawn order while `ctx.lastSpawns` stays in spawn order).

Use `runtime.WithTimeout(d)`, `runtime.WithClock(fn)`, and `runtime.WithLocation(loc)` to make tests fast and deterministic. Zone-sensitive assertions pair a fixed clock with a fixed location — pick an instant where UTC and `loc` fall on different calendar days, otherwise the test passes for the wrong reason.

### Scheduler tests

`internal/scheduler/scheduler_test.go` — fake clock, fake `Runner` (struct with a slice of received jobs). Cover each schedule_kind, day-boundary, and missed-run sweep on startup. `scheduler.WithLocation(loc)` fixes the zone the sweep hands to `DueAt`.

### HTTP handler tests

`internal/httpapi/*_test.go` — use `httptest.NewRecorder` + `Routes()`. Construct `Server` with real services (via `dbtest.New`) OR with hand-rolled stubs implementing the consumer interface.

Pattern:

```go
srv := httpapi.New(taskSvc, tagSvc, scriptSvc, enqueuer, pinger, httpapi.Options{...})
req := httptest.NewRequest("GET", "/api/v1/tasks", nil)
rec := httptest.NewRecorder()
srv.Routes().ServeHTTP(rec, req)
// rec.Code, rec.Body, decode JSON...
```

### Running

```bash
just be-test                        # go test ./...
go test ./internal/task -run Reorder -v
go test ./internal/runtime -race
```

`-race` is healthy because the scheduler + worker model has actual goroutine interactions.

## Frontend (Vitest + RTL)

### Setup

- jsdom environment via `web/vitest.config.ts`.
- Custom setup in `web/vitest.setup.ts` (jest-dom matchers, etc.).
- React 19 + RTL.

### Patterns

- **Hook tests** mock `fetch` globally and assert against the result of `useXxx`. See `web/src/api/tasks.test.tsx`.
- **Component tests** wrap in a fresh `QueryClientProvider` + `RouterProvider` if the component uses navigation. There's not a shared helper — each test wires what it needs.
- **Reorder math** is exported from the table component (`computeDragEnd`, `computeReorderPayload`, `moveTask`) so tests can verify neighbor calc without dnd-kit.
- **Keyboard shortcuts** are tested by dispatching `KeyboardEvent` and asserting side effects (focus, mutation called, etc.).

### Running

```bash
just fe-test               # cd web && pnpm run test (single run)
cd web && pnpm run test:watch
cd web && pnpm vitest --run path/to/file.test.tsx
```

## What to focus on

From the spec §9, prioritize coverage in:
- State-transition logic (`task.SetState`, completed/cancelled timestamp clearing).
- Schedule matching (every weekday/monthly variant + day-boundary edge cases).
- Run lifecycle (ok/error/timeout effect persistence rules).
- Optimistic update reconciliation (drag-drop reorder under invalidation).

These are subtle and expensive when wrong. There's no coverage threshold; use judgement.
