# tt — Task Tracker Design

**Date:** 2026-05-21
**Status:** Draft for review

## 1. Goals & Non-goals

### Goals

- Local-only, single-user task tracker. Tasks are plain text descriptions with state, tags, optional due date, and optional notes.
- Two-tier workflow: a **main list** (all tasks, filter/sort) and a **stage** (a reorderable, focused list of what to work on now).
- Tasks are fully mutable: state freely transitions (including `done → not_done`), all fields editable.
- **Userscripts** are first-class programmable extensions. Each script runs on a Go-evaluated schedule (`every_tick` / `daily` / `weekly` / `monthly`), can be run manually, and interacts with the system via a JavaScript `ctx` API. The only mutating action in v1 is `ctx.queueTask(...)`; the API surface is designed to grow.
- Scripts capture their effect via a bounded **run log** and **per-run log entries**. Spawned tasks remember the script id that created them.
- Ships as **one statically-linked Go binary** with embedded SPA assets, migrations, and any other static content.

### Non-goals

- No multi-user, auth, sync, or cloud features.
- No mobile-specific UI (the browser is the only target).
- No notifications, calendar export, time tracking, sub-tasks, recurring sub-tasks, or task relations beyond the script-spawn link.
- No userscript actions beyond `queueTask` in v1 (reorder/clear-stage/etc. designed-for, not built).
- No real-time push (refetch on user action; no SSE/WebSocket).
- No script sandboxing beyond goja's built-in isolation + a per-run timeout. Local single-user; threat model is "don't accidentally hang the app," not "defend against malicious code."

### Defaults & knobs

- Global scheduler tick: **every 15 minutes** + a sweep on startup. Scripts on `every_tick` run at this cadence; daily/weekly/monthly schedules are short-circuited Go-side on irrelevant ticks.
- Per-script run timeout: **5 seconds** (hard, via `goja.Runtime.Interrupt`).
- Run log retention: **last 500 runs**, FIFO. Logs cascade-delete with their run.
- Timezone: **system local**. Day boundaries are midnight local time.
- Stage soft cap: **7 items**. Beyond this, a non-blocking visual cue appears in the stage view ("focused stages stay small"). No hard limit.

## 2. Architecture Overview

### Process model

One Go process, three concurrent subsystems sharing a single SQLite file:

- HTTP server (`chi`): `/api/v1/*` JSON endpoints + SPA static asset serving from embedded `dist/`.
- Scheduler: a 15-minute ticker that selects due scripts and enqueues runs; a startup sweep handles missed work after the binary was off.
- Script runtime: a single worker goroutine that consumes enqueued runs and executes them in a fresh `goja.Runtime`.

All three share access to the store layer (sqlc-generated queries on top of `*sql.DB`).

### Storage

- SQLite via `modernc.org/sqlite` (pure-Go, no cgo — keeps cross-compile trivial).
- `sqlc` generates type-safe Go from `.sql` query files.
- Migrations via `goose` against an embedded migration set; applied on startup. If the on-disk schema is newer than what the binary knows about, abort startup with a clear error.
- DB path defaults to `$XDG_DATA_HOME/tt/db.sqlite` (Linux/Mac: `~/.local/share/tt/db.sqlite`); overridable with `--db` or `--data-dir`.
- `PRAGMA journal_mode=WAL` for concurrent reads with writes.

### Frontend

- React SPA built with Vite; TypeScript end-to-end.
- shadcn/ui + Tailwind for styling.
- TanStack Router (file-based) for routing; TanStack Query for server state; react-hook-form + zod for forms; dnd-kit for staged reorder; CodeMirror 6 (`@uiw/react-codemirror`) for script editing; lucide-react for icons; date-fns for date math; Biome for lint+format; Vitest + RTL for tests.
- Built to `web/dist/`, then embedded into the Go binary via `go:embed all:dist`.

### Configuration

- CLI flags only in v1: `--port`, `--db`, `--data-dir`.
- No config file. Sensible defaults so `./tt` just works.

## 3. Data Model

All timestamps stored as ISO-8601 strings; all booleans as integers. Schema for v1:

```sql
-- ─────────────────────────────────────────────────────────
-- tasks
-- ─────────────────────────────────────────────────────────
CREATE TABLE tasks (
  id                   INTEGER PRIMARY KEY,
  title                TEXT NOT NULL,
  notes                TEXT NOT NULL DEFAULT '',
  state                TEXT NOT NULL DEFAULT 'not_done'
                          CHECK (state IN ('not_done','done','cancelled')),
  due_date             TEXT,                              -- 'YYYY-MM-DD' or NULL
  priority             REAL NOT NULL DEFAULT 0,           -- main-list ordering key (fractional)
  staged_order         REAL,                              -- NULL = unstaged; any REAL = position in stage
  spawned_by_script_id INTEGER REFERENCES scripts(id) ON DELETE SET NULL,
  created_at           TEXT NOT NULL DEFAULT (datetime('now')),
  completed_at         TEXT,                              -- set on state→done; cleared otherwise
  cancelled_at         TEXT,                              -- set on state→cancelled; cleared otherwise
  updated_at           TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX tasks_state_idx         ON tasks(state);
CREATE INDEX tasks_due_date_idx      ON tasks(due_date);
CREATE INDEX tasks_priority_idx      ON tasks(priority);
CREATE INDEX tasks_staged_order_idx  ON tasks(staged_order) WHERE staged_order IS NOT NULL;
CREATE INDEX tasks_spawned_by_idx    ON tasks(spawned_by_script_id);

-- ─────────────────────────────────────────────────────────
-- tags
-- ─────────────────────────────────────────────────────────
CREATE TABLE tags (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE task_tags (
  task_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  tag_id   INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
  PRIMARY KEY (task_id, tag_id)
);

CREATE INDEX task_tags_tag_idx ON task_tags(tag_id);

-- ─────────────────────────────────────────────────────────
-- scripts (userscripts; replaces the "repetition rule" concept)
-- ─────────────────────────────────────────────────────────
CREATE TABLE scripts (
  id              INTEGER PRIMARY KEY,
  name            TEXT NOT NULL,
  code            TEXT NOT NULL,                          -- JS source
  enabled         INTEGER NOT NULL DEFAULT 1,             -- 0/1
  schedule_kind   TEXT NOT NULL
                     CHECK (schedule_kind IN ('every_tick','daily','weekly','monthly')),
  schedule_config TEXT NOT NULL DEFAULT '{}',             -- JSON; shape depends on kind
  user_state      TEXT NOT NULL DEFAULT '{}',             -- JSON; ctx.state backing store
  last_run_at     TEXT,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX scripts_enabled_idx ON scripts(enabled);

-- schedule_config shapes:
--   every_tick: {}
--   daily:      {}
--   weekly:     { "weekday": "monday" | ... | "sunday" }
--   monthly:    { "day": 1..31 } OR { "day": "last" }

-- ─────────────────────────────────────────────────────────
-- script_runs (bounded global run log)
-- ─────────────────────────────────────────────────────────
CREATE TABLE script_runs (
  id                INTEGER PRIMARY KEY,
  script_id         INTEGER NOT NULL REFERENCES scripts(id) ON DELETE CASCADE,
  started_at        TEXT NOT NULL DEFAULT (datetime('now')),
  finished_at       TEXT,
  status            TEXT NOT NULL DEFAULT 'running'
                       CHECK (status IN ('running','ok','error','timeout')),
  error_message     TEXT,
  spawned_task_ids  TEXT NOT NULL DEFAULT '[]',           -- JSON array
  trigger           TEXT NOT NULL
                       CHECK (trigger IN ('scheduled','manual'))
);

CREATE INDEX script_runs_script_idx     ON script_runs(script_id, started_at DESC);
CREATE INDEX script_runs_started_at_idx ON script_runs(started_at DESC);

-- ─────────────────────────────────────────────────────────
-- script_logs (per-run log entries written by scripts)
-- ─────────────────────────────────────────────────────────
CREATE TABLE script_logs (
  id             INTEGER PRIMARY KEY,
  script_run_id  INTEGER NOT NULL REFERENCES script_runs(id) ON DELETE CASCADE,
  level          TEXT NOT NULL DEFAULT 'info'
                    CHECK (level IN ('debug','info','warn','error')),
  message        TEXT NOT NULL,
  logged_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX script_logs_run_idx ON script_logs(script_run_id, logged_at);
```

### Semantic rules

- `completed_at` is set to the moment of transition when state becomes `done`, and **cleared to NULL** when state leaves `done`. Re-marking done updates to the new time. Timers restart cleanly.
- `cancelled_at` follows the same pattern with `cancelled`.
- State transitions are enforced in Go service code, not via triggers (clearer error paths and test surface).
- `priority` (main-list order) and `staged_order` (stage order) use fractional keys. Inserting between two siblings sets the new key to the midpoint of their priorities. A rebalance pass runs when two adjacent keys differ by less than `1e-9`, re-spreading the affected list to evenly-spaced integers stored as REAL.
- `staged_order IS NULL` means the task is unstaged. Staging a task sets `staged_order` to the next-largest existing value. Unstaging sets it back to `NULL`. "Clear stage" updates every row with `staged_order IS NOT NULL` to `NULL` in one statement.
- **State transitions do not change `staged_order`.** A task marked `done` or `cancelled` remains in the stage so the user sees their progress through the focused batch. A separate "Clear finished" action removes only `done` + `cancelled` tasks from the stage (sets their `staged_order` to NULL) in one operation.
- Drag-drop reorder is only available when the active sort is "priority" (main list) or in the stage view. Under any other sort, drag handles are not rendered. Filters do not disable drag-drop; reorder semantics match alternative C — the moved item's new key is computed between its new *visible* neighbors, hidden items keep their global keys (see §6).
- `tags.name` is unique. Deleting a tag cascades to remove `task_tags` rows but leaves tasks intact.
- `tasks.spawned_by_script_id` is `ON DELETE SET NULL`: deleting a script does not affect tasks it previously spawned.
- `scripts.user_state` is the JSON blob behind `ctx.state.get/set`. Engine reads it before a run, buffers writes in-memory, and persists on successful completion.
- `script_runs.spawned_task_ids` is the JSON list of task ids created within that run; populated when the run finishes.
- Run log retention: count-based (≤500). Cleanup runs after each insert (delete oldest beyond cap). Cascade removes their logs.

## 4. Module / Package Boundaries

```
tt/
├── cmd/tt/main.go               # entry point: parse flags, wire deps, start
├── internal/
│   ├── config/                  # CLI flags, defaults, data-dir resolution
│   ├── db/
│   │   ├── migrations/          # embedded *.sql migration files (goose)
│   │   ├── queries/             # *.sql for sqlc
│   │   ├── sqlc.yaml
│   │   └── store.go             # Store wraps *sql.DB + sqlc Queries
│   ├── task/                    # Task domain
│   ├── tag/                     # Tag domain (managed list)
│   ├── script/                  # Script persistence + schedule matching
│   ├── scheduler/               # Background ticker, drives runs
│   ├── runtime/                 # JS userscript execution (goja)
│   └── httpapi/                 # chi router, handlers, SPA fallback
├── web/                         # React SPA source (gitignored dist/)
│   ├── src/
│   │   ├── api/                 # fetch wrappers + TanStack Query hooks
│   │   ├── components/          # shared shadcn-derived + app components
│   │   ├── features/{tasks,stage,tags,scripts,runs}/
│   │   ├── routes/              # TanStack Router routes
│   │   ├── types/               # hand-written TS mirrors of Go DTOs
│   │   ├── main.tsx
│   │   └── styles.css
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── biome.json
│   └── package.json
├── justfile
├── go.mod
└── README.md
```

### Dependency rules

- `internal/*` packages talk via narrow interfaces **defined by the consumer**, not the producer. Example: `scheduler` declares `type Runner interface { Run(ctx, scriptID, trigger) error }`, which `runtime` happens to satisfy.
- HTTP handlers depend on service interfaces, not concrete types — keeps handlers thin and testable.
- The runtime's `ctx` API never calls SQL directly. It calls `task.Service` / `tag.Service`, with a per-run `runContext` struct that supplies the current `script_id` (so created tasks get stamped) and the current `script_run_id` (so log writes attach to the right run).
- Only `cmd/tt/main.go` knows concrete types; all other code uses interfaces.

### Sketched interfaces

```go
// task package
type Service interface {
  Create(ctx, CreateInput) (Task, error)               // assigns priority = max(priority)+1.0
  Update(ctx, id, UpdateInput) (Task, error)
  SetState(ctx, id, State) (Task, error)
  Stage(ctx, id) error                                 // assigns staged_order = max(staged_order)+1.0
  Unstage(ctx, id) error                               // sets staged_order = NULL
  ReorderMain(ctx, id, beforeID, afterID *ID) error    // recompute priority as midpoint of neighbors
  ReorderStage(ctx, id, beforeID, afterID *ID) error   // recompute staged_order as midpoint of neighbors
  ClearStage(ctx) error                                // sets staged_order = NULL for all staged
  ClearFinishedFromStage(ctx) error                    // sets staged_order = NULL for staged + (done|cancelled)
  List(ctx, FilterSort) ([]Task, error)
  ByScript(ctx, scriptID) ([]Task, error)
  RebalancePriority(ctx) error                         // re-spread priority to integer-valued REALs
  RebalanceStage(ctx) error                            // re-spread staged_order to integer-valued REALs
}

// script package
type Service interface {
  Create(ctx, CreateInput) (Script, error)
  Update(ctx, id, UpdateInput) (Script, error)
  Delete(ctx, id) error
  Get(ctx, id) (Script, error)
  List(ctx) ([]Script, error)
  DueAt(ctx, t time.Time) ([]Script, error)
  RecordRun(ctx, scriptID, trigger) (RunHandle, error)
  AppendLog(ctx, runID, level, message) error
  FinishRun(ctx, runID, status, errMsg, spawnedIDs) error
  ReadState(ctx, scriptID) (json.RawMessage, error)
  WriteState(ctx, scriptID, json.RawMessage) error
}

// runtime package
type Runner interface {
  Run(ctx context.Context, scriptID int64, trigger string) error
}
```

## 5. Userscript Runtime

### `ctx` API (v1)

```js
// Calendar / date helpers (read-only)
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

// Script metadata + previous spawn
ctx.script.id                  // number
ctx.script.name                // string
ctx.script.trigger             // "scheduled" | "manual"
ctx.script.lastRunAt           // Date | null

ctx.lastSpawn                  // Task | null
// Auto-resolved as the most recent task with spawned_by_script_id == script.id.
// Shape: { id, title, notes, state, due_date, created_at, completed_at,
//          cancelled_at, tags: ["work","weekly"] }

// Persistent script state (atomic per run)
ctx.state.get("key")           // any
ctx.state.set("key", value)    // void (buffered; flushed on successful run end)
ctx.state.delete("key")        // void
ctx.state.all()                // object snapshot

// Logging
ctx.log("msg")                 // info-level
ctx.log.debug("msg")
ctx.log.info("msg")
ctx.log.warn("msg")
ctx.log.error("msg")
console.log/info/warn/error    // aliases for ctx.log[level]

// Mutating action (only one in v1)
ctx.queueTask({
  title:    "Weekly review",
  notes:    "...",
  tags:     ["weekly"],         // tag names; auto-create if missing (silently)
  due_date: "2026-05-25"        // optional, "YYYY-MM-DD"
}) // -> void
```

**`queueTask` is deferred-write** — the name signals this. Calls are validated immediately (raise a JS error on bad inputs) and queued in memory. Persistence happens only after the script returns successfully — see "Effect persistence model" below. The function returns nothing; there is no id during the run. Scripts that need to inspect what they queued should rely on `ctx.lastSpawn` on a subsequent run, or read the run-detail page.

**Tag auto-creation.** Tag names that don't exist in `tags` are silently inserted into the managed tag list when the queued create is applied. To reduce typos, the script editor surfaces a "Copy tag name" panel listing existing tags (clicking copies the name to clipboard) — see §6. CodeMirror autocompletion of tag names from the managed list is a stretch goal; if the implementation cost is reasonable, include it in v1.

**Designed-for, not built in v1:** `ctx.tasks.byTag(...)`, `ctx.tasks.byState(...)`, `ctx.stage.add/remove/clear/reorder(...)`. Adding these later requires no schema change.

### Effect persistence model

- **Logs** (`ctx.log.*`, `console.*`): written **immediately**, even on timeout or error. Post-mortems remain debuggable.
- **Tasks queued via `ctx.queueTask`**: **deferred**. Validated and queued in memory during the run; applied to the DB only on `ok` outcome. Discarded on `error` / `timeout`. This avoids partial-create surprises (e.g., a script that queues two tasks and then throws on the third leaves *zero* tasks, not two). The runtime layer owns this queue; the underlying `task.Service.Create` does not need to know.
- **`ctx.state.set`**: **buffered in memory**, flushed only on `ok` outcome. Discarded on `error` / `timeout`. State changes are therefore atomic per run.
- **`scripts.last_run_at`**: updated regardless of outcome — prevents tight retry loops on a script that errors every run.

### Schedule matching (Go-side)

`script.Service.DueAt(t time.Time)` returns enabled scripts whose schedule matches `t`:

| `schedule_kind` | Match condition |
|---|---|
| `every_tick` | always due |
| `daily` | `last_run_at` is null OR not on today's local date |
| `weekly` | `t.Weekday()` == `config.weekday` AND not run today |
| `monthly` | (`config.day` is int AND `t.Day() == config.day`) OR (`config.day == "last"` AND `t` is last day of month), AND not run today |

"Today" uses the system local timezone. Day boundaries occur at midnight local time. The Go scheduler short-circuits before invoking goja, so scripts on infrequent schedules incur no JS execution cost on irrelevant ticks.

### Execution lifecycle (one run)

```
1. Scheduler picks a script (schedule matches now & not already run today)
   OR user clicks "Run now"
2. INSERT script_runs row { script_id, trigger, status='running' }
3. Load script.code, script.user_state JSON
4. Create new goja.Runtime; install ctx and console.* shims; strip Node globals
5. Bind run_id into the runtime so logging writes attach to the right row
6. Launch a 5-second guard goroutine that calls runtime.Interrupt("timeout")
7. Execute script.code (top-level statements; no top-level await)
8. On completion (any of):
     ok        -> apply queued ctx.queueTask calls (stamp spawned_by_script_id,
                  auto-create missing tags); collect new ids into spawned_task_ids
                  flush state buffer to scripts.user_state
                  UPDATE script_runs SET status='ok', finished_at=now,
                                       spawned_task_ids='[...]'
     error     -> discard task queue and state buffer
                  UPDATE script_runs SET status='error', error_message=...
     timeout   -> discard task queue and state buffer
                  UPDATE script_runs SET status='timeout', error_message=...
9. UPDATE scripts SET last_run_at=now (regardless of status)
10. Prune script_runs FIFO if count > 500 (logs CASCADE)
```

### Startup recovery

On startup, before the scheduler begins ticking, mark any `script_runs` rows still in `status='running'` as `status='error'` with `error_message='interrupted (binary restart)'` and `finished_at=now`. This handles binary crashes / kills mid-run and keeps the "last run status" view accurate.

### Concurrency model

- One worker goroutine processes runs sequentially. Local single-user; no parallelism needed; scripts can't observe race conditions.
- Scheduled and manual triggers enqueue into the same buffered channel (size 100). If full, scheduled triggers are dropped (logged); manual triggers return an HTTP `503` with a "scheduler busy" message.
- Worst-case latency from "due" to "ran" is `tick_interval + queue_drain_time` — fine for daily/weekly cadences.

### Manual "Run now"

- `POST /api/v1/scripts/:id/run` enqueues an immediate execution with `trigger="manual"`, bypassing schedule check (but still respecting `enabled`).
- Response returns `{ "run_id": N }` so the UI can navigate to the run detail page and watch logs as they arrive.

### Runtime safety

- `goja.New()` per run — no shared state between scripts; no leak risk.
- No `setTimeout`, `setInterval`, `fetch`, `process`, filesystem, or network. Only `ctx` and `console`.
- 5-second hard timeout via `Interrupt`.
- Panics inside Go-backed `ctx` methods are recovered and translated to JS `Error`s (so scripts can `try/catch`).
- Panics inside the worker goroutine itself are recovered, logged, and marked as run `error`; the worker continues.

## 6. Web UI (React SPA)

### Routes

| Path | Page | Purpose |
|---|---|---|
| `/` → `/tasks` | — | Landing redirect |
| `/tasks` | Main list | All tasks with filter/sort. Add/edit/stage/delete actions. |
| `/stage` | Stage | Focused, reorderable list of staged tasks. Mark done, clear stage. |
| `/tags` | Tags | Managed tag list: add, rename, delete (with cascade confirm). |
| `/scripts` | Scripts list | All userscripts: name, schedule, enabled, last run status. |
| `/scripts/$id` | Script editor | Name + schedule form + CodeMirror + Run-now + recent runs table. |
| `/scripts/new` | Script editor (empty) | Same as above but unsaved. |
| `/runs` | Run log | Global, paginated. Filterable by script, status, date range. |
| `/runs/$id` | Run detail | Status, timing, logs (chronological with level badges), spawned task links. |

### Cross-cutting layout

- Top nav: `Tasks · Stage · Scripts · Tags · Runs` + a "Stage (N)" badge counter.
- Theme: shadcn defaults + system dark/light preference, toggleable.
- Page chrome: sticky header, max-width container.
- Filter and sort state live in the URL via TanStack Router search params — shareable, browser-back works, refresh-stable. Server endpoints accept the same shape.

### Keyboard shortcuts (global, registered once at the app root)

| Key | Action |
|---|---|
| `n` | New task (opens add-task modal from anywhere) |
| `/` | Focus the search input on `/tasks` (navigates there first if elsewhere) |
| `g t` | Go to Tasks |
| `g s` | Go to Stage |
| `g c` | Go to Scripts |
| `g g` | Go to Tags |
| `g r` | Go to Runs |
| `?` | Show shortcut cheatsheet (modal) |

Per-page (only active while the page is focused):

| Page | Key | Action |
|---|---|---|
| `/tasks`, `/stage` | `j` / `k` | Move row focus down / up |
| `/tasks`, `/stage` | `enter` | Open the focused row's edit modal |
| `/tasks`, `/stage` | `e` | Edit focused row |
| `/tasks` | `s` | Stage the focused row |
| `/stage` | `u` | Unstage the focused row |
| `/tasks`, `/stage` | `space` | Toggle focused row's checkbox (bulk select) |
| `/tasks`, `/stage` | `d` | Set focused row state to done |
| modal/edit forms | `cmd/ctrl + enter` | Submit the form |
| modal/edit forms | `esc` | Close the modal |

### Empty states

- **`/tasks` empty**: large "Create your first task" CTA, a short paragraph explaining the main-list / stage / scripts loop, and a "Show me how scripts work" link pointing to `/scripts`.
- **`/stage` empty**: "Nothing staged. Pick a few tasks from your list to focus on now." with a button linking back to `/tasks`.
- **`/scripts` empty**: "Userscripts let you auto-create tasks on a schedule (daily, weekly, etc.). Examples: weekly review, monthly bills, after-N-days follow-ups." + "Create your first script" button. Optionally ship one disabled example script as a starting template.
- **`/runs` empty**: "No script runs yet. Manually trigger a script or wait for its schedule."
- **`/tags` empty**: "No tags yet. Add one to start organizing tasks." + add field.

### Per-page detail

**`/tasks`**
- Layout: left sidebar (filters + quick filters, collapsible) + table.
- **Quick filters** (sidebar shortcuts that set the filter state to a preset):
  - "All open" (state=not_done; default)
  - "Overdue"
  - "Due today"
  - "Recently completed" (state=done, sorted by completed_at desc, last 7 days)
  - "Cancelled"
- Filters: state (multi-checkbox; defaults to `not_done` only), tags (multi-select combobox), due (none / overdue / today / this week / no due date), free-text search across title+notes.
- **Search behavior**: case-insensitive substring match against `title` and `notes`. Debounced 300ms. Search respects the active state filter (so the default "All open" hides done/cancelled even when searching) — to search done/cancelled, switch to the "Recently completed" or "Cancelled" quick filter first. A subtle "Searching open tasks only" hint sits below the search input when state filters are restricted.
- Sort axes: **priority** (default — supports drag-drop reorder), due date, created_at, title — single axis at a time, asc/desc.
- Row content: drag handle (rendered only when sort=priority), title (click → edit modal), state pill, tag chips, due date (red when overdue), staged indicator, kebab menu (edit, state→, stage/unstage, delete).
- **Drag-drop reorder semantics** (Alternative C): allowed only when sort=priority. Filter does not disable drag. When a filter is active, the moved task's new `priority` key is computed between its *visible* neighbors (midpoint); hidden tasks retain their global keys and may end up bracketing the moved item in global order. UI cue: a small note "filtered view — hidden tasks unchanged" appears in the sort/filter bar while filters are active.
- Add task: keyboard shortcut + button → modal with title, notes, tags, due date. New tasks get a `priority` key equal to current max + 1.0 (i.e., land at the bottom of the priority list).
- Bulk select with checkboxes + bulk actions (mark done, stage all, delete) in a floating action bar.

**`/stage`**
- Top bar: "N staged" + buttons "Clear finished" (removes done+cancelled from stage), "Clear stage" (single click with confirm dialog; removes everything), "Add from list →" (links to `/tasks`).
- **Soft cap cue**: when staged count > 7, a subtle inline hint appears above the list: "Focused stages stay small — consider clearing finished items or unstaging anything that can wait." Hint is dismissible per session.
- List rows: drag handle (dnd-kit), title, due date, tag chips, state toggle, unstage button. **Done and cancelled tasks remain in the stage** — they render with strikethrough title and a desaturated background so the user can see progress through the batch. Their `staged_order` is preserved.
- Reorder commits to `POST /api/v1/stage/reorder` with `{ task_id, before_id?, after_id? }`; server picks the midpoint `staged_order` key. Optimistic client update.

**`/scripts/$id`**
- Header: name (editable), enabled toggle, "Run now" button.
- Schedule form: dropdown `every_tick | daily | weekly | monthly` with inline sub-fields (`weekly` → weekday; `monthly` → day number or "last").
- **Selecting `every_tick` raises an inline confirmation banner**: "Every-tick scripts run on every global tick (currently every 15 min). Buggy scripts can flood your task list. Confirm to use this schedule." User must check a box before saving.
- Editor: CodeMirror 6 in JS mode, ~50% page height.
- **Cheatsheet sidebar** (collapsible): two sections.
  - **API**: list of `ctx.*` entries with one-line descriptions; rendered from a static markdown file bundled with the SPA.
  - **Tags**: list of every name in the managed tag list; click-to-copy to clipboard so scripts can paste exact names and avoid typos (`queueTask({ tags: ["..."], ... })`).
- **Spawned tasks tab/section**: a separate panel ("Tasks created by this script") listing all tasks ever spawned by the script — paginated, with state, created_at, and link to the task. Backed by `GET /api/v1/scripts/:id/tasks`.
- Recent runs table: last 20 runs with status pill, started_at, duration, spawned task count, link to `/runs/$id`.
- Unsaved-changes guard on navigation.

**`/runs/$id`**
- Header: script name + link, started/finished, duration, status pill, trigger.
- Error block (if status != `ok`): error_message in monospace.
- Logs: tabular — relative + absolute time, level badge, message. Search/filter. Scrollable.
- Spawned tasks: chips linking to each task.

### API endpoints (JSON, under `/api/v1`)

```
GET    /tasks?state=&tag=&due=&q=&sort=
POST   /tasks
GET    /tasks/:id
PATCH  /tasks/:id
DELETE /tasks/:id
POST   /tasks/:id/state          { state }
POST   /tasks/:id/stage
DELETE /tasks/:id/stage          (unstage)
POST   /tasks/reorder            { task_id, before_id?, after_id? }  (main-list drag-drop; server picks midpoint priority key)
POST   /stage/reorder            { task_id, before_id?, after_id? }  (stage drag-drop; server picks midpoint staged_order key)
DELETE /stage                    (clear all staged)
DELETE /stage/finished           (clear only done+cancelled from stage)

GET    /tags
POST   /tags                     { name }
PATCH  /tags/:id                 { name }
DELETE /tags/:id

GET    /scripts
POST   /scripts
GET    /scripts/:id
PATCH  /scripts/:id
DELETE /scripts/:id
POST   /scripts/:id/run          (manual trigger; returns { run_id })
GET    /scripts/:id/runs?limit=&before=
GET    /scripts/:id/tasks?limit=&cursor=   (all tasks spawned by this script)

GET    /runs?script_id=&status=&from=&to=&limit=&cursor=
GET    /runs/:id                 (run detail + logs + spawned task summaries)

GET    /health
GET    /version
```

### Client state strategy

- Server state: TanStack Query. List endpoint cache keys include the URL search params so filter/sort changes cache cleanly. Mutations invalidate the relevant queries.
- Local UI state: `useState` per page. Form state via `react-hook-form`.
- Optimistic updates for stage/unstage, reorder, and state changes. Add-task and script edits use plain mutate + invalidate.

### Out of scope for v1 UI

- No undo for deletes — confirm dialogs only.
- No toast system beyond surfacing errors.
- No CodeMirror lint plugin or `ctx` autocomplete (designed-for future).
- No "dry-run" mode for scripts (could add later as a `ctx.dryRun` flag).

## 7. Build, Embed, Dev Loop

### Build artifacts

- Frontend: `pnpm run build` → `web/dist/` (index.html + hashed JS/CSS chunks + assets).
- Backend: `go build` produces a `tt` binary with:
  - `web/dist/**` embedded via `go:embed all:dist`
  - `internal/db/migrations/**` embedded via `go:embed`
  - Cheatsheet markdown and any other static content embedded similarly.
- Final output: one statically-linked binary (`CGO_ENABLED=0`), ~15–25 MB.

### Embed layout

```go
// internal/web/assets.go
package web

import "embed"

//go:embed all:dist
var Dist embed.FS

// internal/db/migrations.go
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
```

### Cache headers

- Hashed assets (`/assets/*-<hash>.{js,css}`): `Cache-Control: public, max-age=31536000, immutable`.
- `index.html`: `Cache-Control: no-cache` (revalidate; references current asset hashes).

### `justfile`

```just
default:
    @just --list

# ── frontend ────────────────────────────────────────────────
fe-install:
    cd web && pnpm install

fe-dev:
    cd web && pnpm run dev      # vite dev on :5173, proxies /api → :8080

fe-build:
    cd web && pnpm run build

fe-test:
    cd web && pnpm run test

fe-lint:
    cd web && pnpm run lint

# ── backend ────────────────────────────────────────────────
be-dev:
    go run ./cmd/tt --port 8080 --data-dir ./.dev-data

be-test:
    go test ./...

be-lint:
    golangci-lint run ./...

# ── orchestration ──────────────────────────────────────────
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'kill 0' EXIT
    just be-dev &
    just fe-dev &
    wait

build: fe-build
    go build -trimpath -ldflags='-s -w' -o ./bin/tt ./cmd/tt

build-release: fe-build
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ./bin/tt ./cmd/tt

test: be-test fe-test

lint: be-lint fe-lint

clean:
    rm -rf web/dist web/node_modules ./bin ./.dev-data
```

### Dev loop

- Two processes: `vite` on `:5173`, `go run` on `:8080`.
- `web/vite.config.ts` sets `server.proxy['/api'] = 'http://localhost:8080'`.
- Frontend hot-reload via Vite HMR.
- No backend hot-reload; manual restart of `just be-dev` when Go changes.
- `just dev` brings everything up; Ctrl-C kills both.

### Prod loop

- `just build` produces `bin/tt`.
- `./bin/tt` serves both API and SPA on a single port.

### Migrations & sqlc

- Files: `internal/db/migrations/0001_init.sql`, etc. (goose naming with `-- +goose Up` / `-- +goose Down`).
- Applied on startup with goose. On failure, abort startup with a clear error.
- `internal/db/sqlc.yaml` points at `queries/*.sql` and `migrations/*.sql` for schema. Queries grouped by table.
- `just db-gen` regenerates Go from queries; CI check ensures the generated code is current.

### Versioning

- `git describe`-based version baked in via `-ldflags`. Exposed at `GET /api/v1/version` and shown in the SPA footer.

## 8. Error Handling, Logging, Observability

### API error responses

Uniform JSON shape across all endpoints:

```json
{
  "error": {
    "code":    "task_not_found",
    "message": "no task with id 42",
    "details": { }
  }
}
```

- HTTP status: 4xx for client errors (`400` validation, `404` not found, `409` conflict), `5xx` for server.
- `code` is a stable string the SPA can branch on (e.g., `tag_in_use`, `validation_failed`, `scheduler_busy`).
- Validation errors return `400` with `details: { field: "message" }`.

### Server logging

- `slog` standard library. JSON handler in prod, text handler in dev.
- Levels: `debug | info | warn | error`.
- Request middleware logs one line per request: method, path, status, duration_ms, request_id.
- Each script run logs at info: `script.run.start` and `script.run.finish` with script_id, run_id, status, duration. Script-internal `ctx.log` calls go to `script_logs`, not server stdout.
- Panics in handlers are recovered, logged at error, and returned as `500` with a generic message.
- No log file by default — stderr only. User can redirect (`./tt 2>> tt.log`).

### Health / version

- `GET /api/v1/health` → `{ "status": "ok", "db": "ok" }` (DB ping included).
- `GET /api/v1/version` → `{ "version": "...", "commit": "...", "built_at": "..." }`.

### Data safety

- No built-in backups in v1; document the data path so users can copy the SQLite file (Time Machine, cron, manual).
- SQLite WAL mode handles single-process concurrent reads with writes; we're single-process anyway.
- Migrations run on startup; failures abort startup with a clear message rather than running on a half-migrated schema.
- Schema-ahead detection: if the DB's migration version exceeds what the binary knows about, refuse to start with a "DB was migrated by a newer binary" error.

### Script runtime safety

- 5-second goja `Interrupt` timeout per run.
- Recovered panics inside `ctx` Go methods → JS-visible `Error` (script can `try/catch`).
- Recovered panics inside the worker goroutine → log + mark run as `error`; worker continues.
- Single in-flight run; a hung script can't fan out into N concurrent runs. Worst case: a 5-second tax per stuck script per matching tick.
- No filesystem, network, or `process` access from inside scripts.

### Explicitly out of scope

- No metrics (Prometheus etc.).
- No tracing.
- No external error reporting.
- No UI-action audit log — task history is implicit via `updated_at` + `completed_at` / `cancelled_at`.

## 9. Testing Approach

- **Service tests** (`task`, `tag`, `script` packages): real in-memory SQLite, full SQL exercise. Migrations applied at the start of each test (or per-package suite).
- **Scheduler tests**: fake clock + fake `Runner` interface. Cover each `schedule_kind`, day-boundary edge cases, missed-run sweep on startup.
- **Runtime tests**: real `goja` against an in-memory store. Exercise every `ctx.*` method, the 5s timeout path, error handling, state buffering atomicity, log retention.
- **HTTP handler tests**: `httptest` + service interfaces (real or mock depending on what's exercising).
- **Frontend unit tests**: Vitest + React Testing Library for components and hooks.
- **End-to-end**: Playwright optional; not part of v1.

### Coverage targets

- No hard threshold for v1. Focus areas: state-transition logic, schedule matching, run lifecycle, optimistic update reconciliation. These are where bugs are subtle and expensive.

## 10. Open Items / Future Work (designed-for)

- Additional `ctx` actions: `tasks.byTag`, `tasks.byState`, `stage.add/remove/clear/reorder`, `task.update`.
- `ctx.dryRun` flag to preview script effects without persisting.
- CodeMirror lint plugin + autocomplete from a generated `ctx` type definitions file.
- Backup/export commands (CLI sub-command `tt export` to dump JSON).
- Mobile-friendly responsive pass on stage and task list.
- Optional second worker for higher script throughput (only if needed).
