# Userscript API reference

Userscripts are JavaScript snippets that run on a schedule (or on demand)
and can spawn tasks. They execute in a sandboxed
[goja](https://github.com/dop251/goja) runtime — no `setTimeout`, no
`fetch`, no filesystem, no network. The only mutating action in v1 is
`ctx.queueTask(...)`.

This document is the canonical reference for the `ctx` API surface.

## Execution model

- One worker goroutine processes runs sequentially. A script never sees
  concurrent execution of itself or another script.
- Each run gets a fresh `goja.Runtime`. No shared state between runs
  besides what you persist via `ctx.state.*`.
- 5-second hard timeout via `goja.Runtime.Interrupt`. A script that
  loops forever gets killed; queued effects are discarded.
- Top-level statements only — no top-level `await`.
- `console.{log,info,warn,error}` are aliased to `ctx.log[level]`.

## Effect persistence

Not all side-effects are equal:

| Effect                 | When persisted                                         |
|------------------------|--------------------------------------------------------|
| `ctx.log.*`, `console.*` | **Immediate** — written even on error/timeout (so post-mortems are debuggable). |
| `ctx.queueTask(...)`    | **Deferred** — applied only on successful completion. Discarded on error/timeout. |
| `ctx.state.set/delete`  | **Buffered** — flushed only on successful completion. Atomic per run. |
| `scripts.last_run_at`   | **Always** — updated regardless of outcome, to prevent tight retry loops. |

If a script queues two tasks then throws on the third, **zero** tasks
are created. State changes from a failed run are likewise discarded.

## Schedule kinds

A script's `schedule_kind` is one of:

| Kind         | Match condition                                                                  |
|--------------|----------------------------------------------------------------------------------|
| `every_tick` | Always due. Runs every 15 minutes (the scheduler tick interval).                 |
| `daily`      | Due once per local-time day.                                                     |
| `weekly`     | Due on a specific weekday (`schedule_config = { "weekday": "monday" \| ... }`). |
| `monthly`    | Due on `{ "day": 1..31 }` or `{ "day": "last" }` for last-day-of-month.          |

"Today" uses the system local timezone. Day boundaries are midnight
local time. Daily/weekly/monthly scripts that have already run today
are short-circuited Go-side, so they incur no JS execution cost on
irrelevant ticks.

A 15-minute global ticker drives the scheduler. On startup, a sweep
catches up scripts that were due while the binary was off.

## `ctx` API (v1)

### Date helpers (read-only)

```js
ctx.now()                      // JS Date — current instant (local TZ)
ctx.today()                    // "YYYY-MM-DD"
ctx.weekday()                  // "monday".."sunday"
ctx.dayOfMonth()               // 1..31
ctx.month()                    // 1..12
ctx.year()                     // int
ctx.isFirstOfMonth()           // bool
ctx.isLastOfMonth()            // bool
ctx.isWeekday("monday")        // bool
ctx.daysSince(dateOrString)    // int (negative if future)
ctx.daysBetween(a, b)          // int
ctx.addDays(date, n)           // Date
ctx.formatDate(date)           // "YYYY-MM-DD"
ctx.parseDate("YYYY-MM-DD")    // Date
```

Date inputs accept JS `Date`, RFC3339, `"YYYY-MM-DD HH:MM:SS"`, or
`"YYYY-MM-DD"`. Outputs use the JS `Date` constructor so userscripts
get full `Date.prototype` (`toISOString`, etc).

### Script metadata

```js
ctx.script.id                  // number
ctx.script.name                // string
ctx.script.trigger             // "scheduled" | "manual"
ctx.script.lastRunAt           // string ("YYYY-MM-DD HH:MM:SS" UTC) | null
```

### Previous-spawn lookup

```js
ctx.lastSpawns                 // Task[] — every task created by the most
                               // recent successful run, ordered by id ASC
                               // (insertion order). [] if no successful run.
ctx.lastSpawn                  // last element of lastSpawns, or null
                               // (back-compat with the prior single-task API)
```

Task shape:

```js
{
  id, title, notes, state, due_date,
  created_at, completed_at, cancelled_at,
  tags: ["work", "weekly"]
}
```

Failed and timed-out runs are skipped — `lastSpawns` always reflects
the last `ok` invocation's batch.

### Persistent state

```js
ctx.state.get("key")           // any (undefined if absent)
ctx.state.set("key", value)    // void  — buffered; flushed on ok
ctx.state.delete("key")        // void  — buffered; flushed on ok
ctx.state.all()                // object — merged snapshot (initial + pending)
```

State is per-script JSON stored in `scripts.user_state`. Corrupted
state falls back to an empty object — historical bad data can't
brick a script.

### Logging

```js
ctx.log("msg")                 // info-level
ctx.log.debug("msg")
ctx.log.info("msg")
ctx.log.warn("msg")
ctx.log.error("msg")

console.log("msg")             // aliases for ctx.log[level]
console.info("msg")
console.warn("msg")
console.error("msg")
```

Logs are written immediately, attached to the current `script_runs` row,
and visible on the run detail page.

### Mutation

```js
ctx.queueTask({
  title:    "Weekly review",   // required, trimmed
  notes:    "...",             // optional, defaults to ""
  tags:     ["weekly"],        // optional, tag names; auto-created if missing
  due_date: "2026-05-25"       // optional, "YYYY-MM-DD"
})
// returns nothing
```

Validation is synchronous (bad inputs raise a JS error you can
`try/catch`). Persistence is deferred until the run finishes
successfully — there is no id available during the run. Inspect what
you queued via `ctx.lastSpawn` on the next run, or the run detail page.

Per-item persistence is best-effort: if one item fails to insert, the
remaining items still apply. Tags that don't exist are silently created
in the managed tag list.

## Examples

### Daily stand-up notes (weekdays only)

```js
const weekday = ctx.weekday();
if (weekday === "saturday" || weekday === "sunday") {
  return;
}

const today = ctx.today();
if (ctx.state.get("lastSpawn") === today) {
  return; // already spawned today
}

ctx.queueTask({
  title:    `Stand-up — ${today}`,
  tags:     ["work", "standup"],
  due_date: today,
});
ctx.state.set("lastSpawn", today);
```

Schedule: `daily`. Persists `lastSpawn` so re-runs in the same day are
no-ops.

### Monthly bills, day 1

```js
const bills = ["Rent", "Internet", "Phone"];
for (const name of bills) {
  ctx.queueTask({
    title:    `Pay ${name}`,
    tags:     ["bills"],
    due_date: ctx.today(),
  });
}
ctx.log.info(`Queued ${bills.length} bill reminders`);
```

Schedule: `monthly`, `{ "day": 1 }`. The loop creates all three in a
single run; either all persist or (if one validation fails) none do.

### Follow-up N days after spawning

```js
const FOLLOWUP_DAYS = 3;
const prev = ctx.lastSpawn;
if (prev && ctx.daysSince(prev.created_at) >= FOLLOWUP_DAYS) {
  ctx.queueTask({
    title:    `Follow up on: ${prev.title}`,
    notes:    `Original task #${prev.id} created ${prev.created_at}`,
    tags:     ["followup"],
    due_date: ctx.today(),
  });
}
```

Schedule: `daily`. Uses `ctx.lastSpawn` to chain a follow-up off the
previous spawn.

## What is NOT available

- No `setTimeout`, `setInterval`, `fetch`, `process`, `require`, or
  Node-style modules. These globals are deleted from the runtime after
  `ctx` is installed.
- No filesystem access.
- No network access.
- No top-level `await`.
- No way to read or modify other scripts' state.
- No `ctx.dryRun`, `ctx.tasks.byTag`, `ctx.tasks.byState`,
  `ctx.stage.*`, or `ctx.task.update` in v1 — designed-for, not built.

The sandbox philosophy is "don't accidentally hang the app," not
"defend against hostile code." Only run scripts you've written or read.

## Run lifecycle (for debugging)

When something looks wrong, this is what happened under the hood:

```
1. Scheduler picks a due script (or user clicks "Run now")
2. INSERT script_runs { script_id, trigger, status='running' }
3. Load code + user_state JSON
4. New goja.Runtime; install ctx; delete Node globals
5. Bind run_id so logs attach to this row
6. Start 5s interrupt timer
7. Execute script.code
8. On outcome:
     ok      → apply queued tasks, persist state, status='ok'
     error   → discard queue + state buffer, status='error'
     timeout → discard queue + state buffer, status='timeout'
9. UPDATE scripts SET last_run_at = now  (always)
10. Prune script_runs to 500 most recent (logs CASCADE)
```

If a `running` row is left behind by a crash, startup recovery marks
it as `error` with `"interrupted (binary restart)"`.

## Adding new ctx methods

If you're extending the runtime itself (not just writing scripts), see
[docs/agent/05-runtime.md](agent/05-runtime.md) for the internal
binding pattern and the consumer-side `Runner` interface.
