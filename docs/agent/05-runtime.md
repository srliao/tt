# Userscript Runtime

JS execution engine for userscripts. One run = one fresh `goja.Runtime` + 5s timeout + deferred-effect persistence.

## Files

| File | Concern |
|---|---|
| `runner.go` | Run orchestration: load script, install ctx, execute, flush, prune. |
| `ctx.go` | Installs the `ctx` object tree on goja; sandbox sweep. |
| `ctx_dates.go` | `ctx.today`, `weekday`, `daysSince`, `addDays`, `parseDate`, `formatDate`, etc. |
| `ctx_state.go` | `ctx.state.{get,set,delete,all}` + the `stateBuffer` overlay. |
| `ctx_queue.go` | `ctx.queueTask` + the `taskQueue` validating buffer. |
| `ctx_log.go` | `ctx.log` (callable + `.debug/.info/.warn/.error`) and `console.*` aliases. |

## Run lifecycle (Runner.Run)

```
1. (Caller) script.Service.StartRun → row in 'running' status; runID returned.
2. (Runner) Load script; if disabled, FinishRun(error, "script is disabled").
3. defer SetLastRunAt(now) + PruneRuns(500) — both run regardless of outcome.
4. Load user_state into stateBuffer (overlay; never mutates persisted blob).
5. Create taskQueue (in-memory buffer for ctx.queueTask).
6. Load lastTasks = LatestBySpawningScripts(scriptID) — the entire batch
   spawned by the most recent successful run, ordered by tasks.id ASC.
7. goja.New(); installCtx(...) — see ctx.go.
   - Date helpers, ctx.log + console, ctx.state, ctx.queueTask,
     ctx.script metadata, ctx.lastSpawns (array) + ctx.lastSpawn
     (last element of the array, or null when empty — pre-batch contract).
   - delete this.setTimeout/setInterval/fetch/process/require.
8. execute(rt, code) under time.AfterFunc(timeout, rt.Interrupt("timeout")).
9. Switch on status:
     ok       → flush queue → tasks.Create + tags.Resolve(autoCreate=true) +
                SetTagsByID; persistState; FinishRun(ok, spawnedIDs).
     timeout  → FinishRun(timeout, errMsg). Discard queue + state buffer.
     error    → FinishRun(error, errMsg). Discard queue + state buffer.
```

Important: **logs are immediate**, not deferred. `ctx.log` writes through `script.Service.AppendLog` synchronously, so timeouts and errors still leave a post-mortem trail.

`ctx.lastSpawns` derives from `script_runs.spawned_task_ids` (JSON array, written by `FinishRun` on `ok`). The query `ListLatestSpawnedTasksByScript` in `internal/db/queries/tasks.sql` JOINs `tasks` with `json_each(spawned_task_ids)` of the latest `status='ok'` row for the script, so failed/timeout runs are skipped and the batch reflects exactly what the previous successful invocation produced.

## Effect persistence model

From the spec §5:

| Effect | When persisted |
|---|---|
| `ctx.log.*`, `console.*` | Immediate (per-call `INSERT` into `script_logs`). |
| `ctx.queueTask` | Deferred — applied only on `RunStatusOK`. Discarded on error/timeout. |
| `ctx.state.set/delete` | Buffered — flushed only on `RunStatusOK`. Atomic per run. |
| `scripts.last_run_at` | Always (defer in `Runner.Run`). Prevents tight retry loops. |

This is intentional. If a script errors after queueing two tasks, **zero** tasks are persisted, not two.

## stateBuffer semantics

`internal/runtime/ctx_state.go`. Overlay over `initial` (the loaded `user_state` JSON):

- `Get` returns deletes-first, then writes, then initial.
- `Set(k, v)` records a pending write; rescinds any pending Delete on k.
- `Delete(k)` records pending delete; drops any pending Set on k.
- `All` / `Flush` return the merged snapshot. `Flush` is idempotent.

A corrupt or unparseable `user_state` blob falls back to an empty map — historical bad data can't brick a script.

## taskQueue semantics

`internal/runtime/ctx_queue.go`. `Enqueue(map[string]any)` validates immediately so the script gets synchronous feedback on bad input:

- `title` required, trimmed.
- `notes` defaults to empty string.
- `due_date` (if present) must be `YYYY-MM-DD`.
- `tags` must be an array of strings; duplicates removed; empty entries dropped.

Persistence: per item, resolve tags via `tag.Service.Resolve(..., autoCreate: true)` then `task.Service.Create(...)` then `task.Service.SetTagsByID(...)`. **One failure does NOT abort the rest** — partial-spawn is preferred to nothing.

## Sandbox

- `goja.New()` per run — no shared state.
- After ctx install: `delete this.setTimeout/setInterval/fetch/process/require`.
- 5s hard timeout via `goja.Runtime.Interrupt`. Timeout messages contain the budget value.
- Two panic recovers in `Runner.Run`: one outside `execute` (failRun → error status), one inside `execute` (translate to error status). Worker goroutine in scheduler also recovers per-job so a bad script can't take down the worker.

## ctx API (callable from JS)

See spec §5 for the full list. Key categories:

| Category | Methods |
|---|---|
| Date helpers | `ctx.today/weekday/dayOfMonth/month/year/isFirstOfMonth/isLastOfMonth/isWeekday/daysSince/daysBetween/addDays/formatDate/parseDate` |
| Script metadata | `ctx.script.{id,name,trigger,lastRunAt}` — `lastRunAt` is a string in `"YYYY-MM-DD HH:MM:SS"` UTC layout |
| Spawn lookup | `ctx.lastSpawns` — array of task objects from the most recent successful run (ordered by `tasks.id ASC` = insertion order; `[]` when no such run). `ctx.lastSpawn` — last element of that array, or `null` when empty (back-compat with the prior single-task surface). |
| State | `ctx.state.{get,set,delete,all}` |
| Logging | `ctx.log(msg)`, `ctx.log.{debug,info,warn,error}`, `console.{log,info,warn,error}` |
| Mutation | `ctx.queueTask({title, notes?, tags?, due_date?})` |

## JS↔Go bridging

- Date inputs to ctx.* accept JS `Date`, RFC3339, `"YYYY-MM-DD HH:MM:SS"`, or `"YYYY-MM-DD"` (`acceptedDateLayouts` in `ctx_dates.go`).
- Date outputs use the JS `Date` constructor invoked from Go so userscripts see full `Date.prototype` (toISOString, etc.).
- Goja errors raised from Go bindings use `rt.NewGoError(err)` so user scripts can `try/catch`.

## Consumer-side interface

```go
// runtime.Runner is satisfied by *Runner.
type Runner interface {
    Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error
}
```

The scheduler and HTTP layer declare their own copies of this interface — never import `runtime` from elsewhere to reach for the concrete type.

## Adding a new ctx method

1. Decide which file holds the binding (dates, state, queue, log, or a new file).
2. Add the Go function. Validation should panic via `rt.NewGoError(...)` for JS-visible errors.
3. Wire it into `ctxObj.Set("name", fn)` in `installCtx` (or in a sub-installer like `installState`).
4. Document the surface in the spec / cheatsheet markdown if user-visible.
5. Test it in `internal/runtime/runner_test.go` — there's a pattern of building a tiny script + asserting the resulting `ctx.log` lines / queued tasks / DB state.

If the new method is a **mutation**, decide if it's deferred (like `queueTask`) or immediate. Deferred is preferred for anything that produces side-effects on the task store, so the per-run atomicity rule holds.
