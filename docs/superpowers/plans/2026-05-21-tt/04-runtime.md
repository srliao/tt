# Phase 04 — Userscript Runtime (goja + `ctx` API)

> Read `00-index.md` first. Commit after each task.

**Goal:** Execute a userscript inside a fresh `goja.Runtime` per run. Install the `ctx` API per spec §5 (date helpers, script metadata, `lastSpawn`, buffered `state.get/set/delete/all`, logging, deferred `queueTask`). Enforce the 5-second hard timeout. Apply the effect-persistence model: logs are written immediately; queued tasks and state writes flush only on `ok`.

**Dependencies:** Phase 03a (task), 03b (tag), 03c (script).

**Tech stack:** `github.com/dop251/goja`. Stdlib `context`, `time`, `encoding/json`, `sync`.

**Parallelizable with:** 05 partial (scheduler needs `Runner`), 06 (HTTP endpoints can use a `Runner` interface stub until this lands), 07.

## File map

```
internal/runtime/
├── runner.go            # Runner type + main Run() entry point
├── ctx.go               # ctx object construction + bindings
├── ctx_dates.go         # date helpers (ctx.today, ctx.weekday, …)
├── ctx_state.go         # state.get/set/delete/all (buffered)
├── ctx_log.go           # ctx.log + console.* aliases
├── ctx_queue.go         # ctx.queueTask buffer + flush
├── runner_test.go
├── ctx_dates_test.go
├── ctx_state_test.go
└── ctx_queue_test.go
```

## Background (spec invariants)

- **One `goja.Runtime` per Run** — no shared state across scripts (spec §5 "Runtime safety").
- **Effect persistence model** (spec §5):
  - Logs (`ctx.log.*`, `console.*`) write to `script_logs` **immediately** — survives error/timeout.
  - `ctx.queueTask` validates immediately but buffers in memory; flushes only on `ok` outcome.
  - `ctx.state.set/delete` buffer in memory over a snapshot from `scripts.user_state`; flushes only on `ok`.
  - `scripts.last_run_at` updates regardless of outcome.
- **5-second timeout** via `goja.Runtime.Interrupt("timeout")` from a guard goroutine.
- **Sandboxing:** strip `setTimeout`, `setInterval`, `fetch`, `process`. Only `ctx`, `console`, and goja's built-in JS globals are present.
- **`ctx.lastSpawn`** is resolved at `ctx` construction time by querying `task.Service.LatestBySpawningScript(scriptID)`. If nil, the JS value is `null`.

## Task 1: Add goja dep

- [ ] Run:
  ```bash
  cd /Users/srliao/code/tt && go get github.com/dop251/goja@latest && go mod tidy
  ```
- [ ] Verify: `grep goja go.mod` shows the dep.
- [ ] Commit:
  ```bash
  git add go.mod go.sum && git commit -m "chore(runtime): add goja dependency"
  ```

## Task 2: Date helpers — failing tests

**Files:** `internal/runtime/ctx_dates_test.go`

- [ ] Write tests that construct a `goja.Runtime`, attach a `dateBindings(rt, now)` helper, then invoke each function from JS and assert the result. Use a fixed `now = 2026-05-21 14:30 UTC` (Thursday) for determinism.
- [ ] **Tests cover** one assertion per function:
  - `ctx.today()` → `"2026-05-21"`.
  - `ctx.weekday()` → `"thursday"`.
  - `ctx.dayOfMonth()` → `21`.
  - `ctx.month()` → `5`.
  - `ctx.year()` → `2026`.
  - `ctx.isFirstOfMonth()` → `false` (test also with `2026-06-01` → true).
  - `ctx.isLastOfMonth()` → `false` (test also with `2026-05-31` → true).
  - `ctx.isWeekday("thursday")` → `true`; `"monday"` → `false`.
  - `ctx.daysSince("2026-05-19")` → `2`; `daysSince("2026-05-23")` → `-2`.
  - `ctx.daysBetween("2026-05-21","2026-05-24")` → `3`.
  - `ctx.addDays(ctx.parseDate("2026-05-21"), 3)` then `ctx.formatDate(...)` → `"2026-05-24"`.
  - `ctx.parseDate("2026-05-21")` returns a JS `Date` whose `.getDate()` is `21`.
- [ ] Run → fails because `dateBindings` doesn't exist.

## Task 3: Date helpers — implementation

**Files:** `internal/runtime/ctx_dates.go`

- [ ] Implement `dateBindings(rt *goja.Runtime, now time.Time) map[string]any`. Return a map keyed by each function name above. Each binding is a `func(call goja.FunctionCall) goja.Value` (or a typed Go function — goja auto-wraps).
- [ ] Date math uses `time.Time.AddDate`, `.Day()`, `.Weekday().String()` (lowercased), `time.Parse("2006-01-02", s)`.
- [ ] `now` arg lets tests inject a deterministic instant; production passes `time.Now()`.
- [ ] Run tests → green.
- [ ] Commit:
  ```bash
  git add internal/runtime/ctx_dates.go internal/runtime/ctx_dates_test.go && \
    git commit -m "feat(runtime): add ctx date helpers"
  ```

## Task 4: Logging — implementation (no test yet — exercised via runner test in Task 9)

**Files:** `internal/runtime/ctx_log.go`

- [ ] Implement `logBindings(svc script.Service, runID int64) (logFn func(level, msg) error, ctxLogObj map[string]any, consoleObj map[string]any)`:
  - `logFn(level, msg)` calls `svc.AppendLog(ctx, runID, level, msg)` synchronously. Logs are immediate.
  - `ctxLogObj` is a callable map. In goja that means: install `ctx.log` as a function (callable) that defaults to `info`, but with attached `debug`, `info`, `warn`, `error` properties. Use `rt.ToValue(map[string]any{...})` for the callable form; attach sub-functions via `obj.Set("debug", ...)`. Alternative: install `ctx.log` as a function via `rt.Set("__logFn__", ...)` and then evaluate a JS snippet that wraps it. Use whichever pattern reads more cleanly in tests.
  - `consoleObj` is `{log, info, warn, error}` aliasing the same backing log function (mapping `log → info`).
- [ ] No test for now — verified via Task 9.
- [ ] Commit:
  ```bash
  git add internal/runtime/ctx_log.go && git commit -m "feat(runtime): add ctx.log and console bindings"
  ```

## Task 5: State buffer — failing tests

**Files:** `internal/runtime/ctx_state_test.go`

- [ ] **Tests cover** a `newStateBuffer(initial map[string]any) *stateBuffer`:
  - `Get("k")` returns the value loaded from `initial`.
  - `Set("k", value)` is observable from `Get` within the same buffer (not yet flushed).
  - `Delete("k")` causes `Get` to return undefined/nil.
  - `All()` returns a snapshot of the merged view (initial overlaid with pending writes/deletes).
  - `Flush()` returns a JSON-serializable map and is idempotent.
  - The buffer does not mutate the input `initial` map.
- [ ] Run → undefined.

## Task 6: State buffer — implementation

**Files:** `internal/runtime/ctx_state.go`

- [ ] Implement `stateBuffer`:
  ```go
  type stateBuffer struct {
      initial map[string]any
      writes  map[string]any
      deletes map[string]struct{}
  }
  ```
  - `Get(k)`: if in `deletes`, return nil. If in `writes`, return that. Else `initial[k]`.
  - `Set(k, v)`: writes[k]=v; delete from `deletes`.
  - `Delete(k)`: deletes[k]={}, remove from writes.
  - `All()`: copy initial, apply deletes (remove), apply writes (overwrite).
  - `Flush() map[string]any`: returns `All()`. (Caller marshals to JSON.)
- [ ] Implement `stateBindings(buf *stateBuffer) map[string]any` returning `{get, set, delete, all}` JS functions.
- [ ] Run tests → green.
- [ ] Commit:
  ```bash
  git add internal/runtime/ctx_state.go internal/runtime/ctx_state_test.go && \
    git commit -m "feat(runtime): add buffered ctx.state"
  ```

## Task 7: Queued tasks — failing tests

**Files:** `internal/runtime/ctx_queue_test.go`

- [ ] **Tests cover** a `newTaskQueue()` with `Enqueue(input) error`, `Drain() []queuedTask`:
  - `Enqueue({title: "x"})` succeeds; `Drain()` returns one entry.
  - Empty title returns an error immediately (validation at queue time, per spec).
  - Invalid `due_date` ("not-a-date") returns an error.
  - `tags` is normalized to a deduped, trimmed `[]string`.
  - After `Drain()`, the queue is empty.
- [ ] Run → undefined.

## Task 8: Queued tasks — implementation

**Files:** `internal/runtime/ctx_queue.go`

- [ ] Implement:
  ```go
  type queuedTask struct {
      Title   string
      Notes   string
      Tags    []string
      DueDate *string // YYYY-MM-DD
  }
  type taskQueue struct { items []queuedTask }
  ```
  - `Enqueue(raw map[string]any) error`: extract fields, validate (title non-empty after trim; due_date parses if present; tags is array-of-strings); append.
  - `Drain()`: returns and clears.
- [ ] Implement `queueBinding(queue *taskQueue) func(input goja.Value) goja.Value` that:
  - Converts `input` to `map[string]any` via `goja.Value.Export()`.
  - Calls `queue.Enqueue`. If error, `panic(rt.NewGoError(err))` so the JS side sees a try/catchable Error.
  - Returns `goja.Undefined()` — the JS function returns nothing.
- [ ] Run tests → green.
- [ ] Commit:
  ```bash
  git add internal/runtime/ctx_queue.go internal/runtime/ctx_queue_test.go && \
    git commit -m "feat(runtime): add ctx.queueTask buffer"
  ```

## Run ownership note (resolves spec ambiguity)

**Run-row ownership lives with the caller, not the runtime.** The caller (HTTP handler for manual triggers; scheduler for scheduled ones) calls `script.Service.StartRun(scriptID, trigger)` to obtain a `Run` (in `running` state), then passes the resulting `runID` to `Runner.Run`. This lets the HTTP `POST /scripts/:id/run` endpoint return `{run_id}` immediately so the UI can navigate to `/runs/:id` while the run is still in flight.

`Runner.Run` therefore takes a pre-created `runID`. It is responsible for: loading the script, executing it, writing logs to that run, calling `FinishRun` with the terminal outcome, calling `SetLastRunAt`, and pruning old runs. It does NOT call `StartRun` itself.

## Task 9: Runner — failing test

**Files:** `internal/runtime/runner_test.go`

- [ ] **Tests cover** `Runner.Run(ctx, scriptID, runID, trigger) error` end-to-end against real `task.Service`, `tag.Service`, `script.Service` in an in-memory store. Tests must first call `script.StartRun(scriptID, trigger)` and pass the returned `runID` into `Runner.Run`:
  - **Happy path:** A script with code `ctx.queueTask({title: "hi", tags: ["weekly"]})` invoked manually creates one task (after Run returns). The run row has status `ok`, `spawned_task_ids` contains that task's id, the `weekly` tag exists.
  - **Logs survive error:** `ctx.log("before"); throw new Error("boom");` — `script_logs` has the "before" entry; run status is `error`; `error_message` mentions "boom"; no task created.
  - **State buffering atomicity:** `ctx.state.set("k", 1); throw new Error();` — `user_state` remains `{}`. On success: state persists.
  - **`ctx.lastSpawn`:** after a first run creates a task, a second run that reads `ctx.lastSpawn.title` and writes it via `ctx.log(JSON.stringify(ctx.lastSpawn))` should log a non-null JSON object containing the prior task's title.
  - **Timeout:** `while(true){}` — the run finishes within ~5s + a small margin, status is `timeout`, error_message mentions "timeout"; no task or state persisted; logs emitted before the loop are present.
  - **`last_run_at` updates regardless:** verify after an error run that `scripts.last_run_at` is non-null.
- [ ] Allow overriding the 5-second timeout to a smaller value (e.g. 200ms) in tests via a `WithTimeout(d)` option on `Runner` for fast feedback.
- [ ] Run → undefined `runtime.Runner`.

## Task 10: Runner — implementation

**Files:** `internal/runtime/runner.go`, `internal/runtime/ctx.go`

- [ ] Define `Runner` struct with dependencies:
  ```go
  type Runner struct {
      tasks   task.Service
      tags    tag.Service
      scripts script.Service
      logger  *slog.Logger
      now     func() time.Time
      timeout time.Duration // default 5*time.Second
  }
  func New(tasks task.Service, tags tag.Service, scripts script.Service, logger *slog.Logger, opts ...Option) *Runner
  ```
  Options: `WithTimeout(d)`, `WithClock(func() time.Time)`.

- [ ] Implement `Runner.Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error`:

  Note: `runID` is **already created** by the caller (HTTP handler or scheduler) via `script.Service.StartRun`. The runtime does not create the run row.

  Sketch of the execution kernel (per spec §5 lifecycle):
  ```
  1. svc.Get(scriptID); if !enabled return error
  2. defer: svc.SetLastRunAt(scriptID, now) — runs regardless
  3. raw := svc.ReadUserState(scriptID); decode JSON to map[string]any
  4. stateBuf := newStateBuffer(initial)
  5. queue := newTaskQueue()
  6. rt := goja.New(); install ctx; install console; install nothing else
  7. lastSpawn := tasks.LatestBySpawningScript(scriptID); set ctx.lastSpawn
  8. guard := time.AfterFunc(timeout, func() { rt.Interrupt("timeout") })
  9. _, err := rt.RunString(script.Code)
 10. guard.Stop()
 11. switch {
     case errors.Is(err, *goja.InterruptedError) && err.Value() == "timeout":
         svc.FinishRun(runID, RunStatusTimeout, "timeout after 5s", nil)
     case err != nil:
         svc.FinishRun(runID, RunStatusError, err.Error(), nil)
     case ok:
         spawnedIDs := flushQueue(ctx, queue, scriptID)
         persistState(ctx, scriptID, stateBuf)
         svc.FinishRun(runID, RunStatusOK, "", spawnedIDs)
     }
 12. svc.PruneRuns(ctx, 500)
  ```

- [ ] `flushQueue` for each queued task: call `tags.Resolve(tags, autoCreate=true)` then `tasks.Create(input)` with `SpawnedByScriptID = &scriptID`, then `tasks.SetTagsByID(taskID, ids)`. Collect ids; return.

- [ ] `persistState`: `json.Marshal(stateBuf.Flush())` → `svc.WriteUserState(scriptID, blob)`.

- [ ] `ctx.go` exports `installCtx(rt *goja.Runtime, deps ...)` that:
  - Builds the date bindings (Task 3).
  - Builds the log bindings (Task 4); installs both `ctx.log` (callable + sub-functions) and `console.{log,info,warn,error}`.
  - Builds the state bindings (Task 6) → `ctx.state.{get,set,delete,all}`.
  - Builds the queue binding (Task 8) → `ctx.queueTask`.
  - Sets `ctx.script = { id, name, trigger, lastRunAt }`.
  - Sets `ctx.lastSpawn` to the JSON object form of the spawned task (id, title, notes, state, due_date, created_at, completed_at, cancelled_at, tags) or `null`.

- [ ] Panic recovery: wrap each `ctx.*` Go-backed method so a Go `panic` becomes `panic(rt.NewGoError(err))` — JS-catchable. Also wrap the top-level `Run` in `defer recover()` that turns a runtime-internal panic into a logged error and a `RunStatusError` finish.

- [ ] Run all runtime tests → green.

- [ ] Commit:
  ```bash
  git add internal/runtime/runner.go internal/runtime/ctx.go internal/runtime/runner_test.go && \
    git commit -m "feat(runtime): add Runner with goja sandbox and deferred effects"
  ```

## Task 11: Define `Runner` consumer interface

**Files:** `internal/runtime/runner.go` (already created above)

The scheduler (phase 05) and HTTP API (phase 06) consume a narrow `Runner` interface. Per spec §4 dependency rules, the consumer declares the interface.

- [ ] At the top of `runner.go`, leave a doc comment noting that callers declare their own narrow `Runner` interface and `*Runner` satisfies them structurally. The shape consumers will use:
  ```go
  // Implemented by *runtime.Runner:
  //   Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error
  ```

- [ ] Compile-time check at the bottom of the file — declare a private interface mirroring the consumer signature and assert `var _ exampleRunner = (*Runner)(nil)`.

- [ ] Commit:
  ```bash
  git add internal/runtime/runner.go && git commit -m "docs(runtime): document consumer interface contract"
  ```

## Phase completion checklist

- [ ] `go test ./internal/runtime/... -v` all pass (including the timeout test).
- [ ] `go build ./...` clean.
- [ ] All six effect-persistence behaviors verified: logs immediate, queue deferred, state buffered, last_run_at always updated, panics recovered, timeout enforced.
