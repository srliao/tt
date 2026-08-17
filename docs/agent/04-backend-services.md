# Backend Services

Three domain packages — `task`, `tag`, `script`. All follow the same skeleton.

## Common shape

Each `internal/<domain>/` contains:

```
types.go      ── domain structs, enums, Input types
service.go    ── interface Service + struct Impl + New(store) + var _ Service = (*Impl)(nil)
service_test.go
```

Pattern: **producer defines the interface as documentation of the full surface**, consumers (HTTP, scheduler, runtime) declare their own narrower interfaces. The compile-time assertion `var _ Service = (*Impl)(nil)` catches breaks.

## task

File: `internal/task/service.go`.

| Method | Notes |
|---|---|
| `Create` | New row gets `priority = MIN(priority) - 1.0` → sorts **above** every existing task (lists are ascending). Tag attach is caller's responsibility via `SetTagsByID`. |
| `Get` / `Update` / `Delete` | CRUD; `Update` accepts full new state (PATCH semantics live at HTTP). |
| `SetState` | Stamps `completed_at` / `cancelled_at`; clears them on transition to `not_done`. Does NOT change `staged_order`. |
| `Stage` / `Unstage` | Assigns `MIN(staged_order) - 1.0` (top of the focused batch) or NULL. |
| `ClearStage` / `ClearFinishedFromStage` | One-statement bulk ops. |
| `ReorderMain` / `ReorderStage` | Compute midpoint; trigger rebalance if neighbors within 1e-9. |
| `RebalancePriority` / `RebalanceStage` | Wraps a tx, reassigns to `float64(i)` in current ascending order. |
| `List(FilterSort)` | Dynamic SQL — see [03-data-layer.md](./03-data-layer.md). |
| `ByScript(scriptID, limit, offset)` | Used by `/scripts/:id/tasks` and the spawned-tasks panel. |
| `LatestBySpawningScripts` | Returns the full batch of tasks created by the most recent successful (`status='ok'`) run for the script, in **spawn order** — the query sorts by position in `spawned_task_ids`, not `tasks.id`. Empty slice (not an error) when no such run exists. Powers runtime `ctx.lastSpawns` / `ctx.lastSpawn`. |
| `SetTagsByID` | Replace-all in a tx. Caller resolves names via `tag.Service.Resolve`. |

**Always set tags via `SetTagsByID` after creating/updating** — `Create`/`Update` themselves do not touch tags.

## tag

File: `internal/tag/service.go`.

| Method | Notes |
|---|---|
| `Create` | Idempotent — normalizes (trim + lowercase) and looks up by name first to dodge UNIQUE constraint. Empty name → error. Assigns `color_hue` via private `pickLeastUsedHue` (least-used hue from `HuePalette`, ties → lower hue). |
| `Rename` / `Delete` / `List` / `GetByName` | Straightforward; `Rename` and `GetByName` also normalize their name input. |
| `ListWithCounts` | Same ordering as `List` plus `count` (distinct task ids via `task_tags` LEFT JOIN). Tags with no tasks come back with `count=0`. Backs `GET /tags?counts=1`. |
| `Resolve(names, autoCreate)` | Normalize (trim + lowercase), dedupe-preserve-order, lookup each. With `autoCreate=true`, insert missing. With `false`, return `"tag: unknown tags: a, b"` error containing every missing name. |

All tag-name entry points funnel through a single `normalize` helper, so stored names are always lowercase regardless of user input. Lookups (`GetByName`, `Resolve`) lowercase before querying, keeping case-insensitive matches in sync.

The runtime uses `Resolve(..., autoCreate: true)` when flushing `ctx.queueTask` so userscripts can introduce new tags without ceremony. The HTTP layer uses `false` when filtering — a typo should fail loud.

The `color_hue` column (12-hue palette in `HuePalette`, `internal/tag/types.go`) is the persisted source of truth for tag chip color. Both `Create` and `Resolve(autoCreate=true)` route inserts through `pickLeastUsedHue`, backed by `CountTagsByHue` — so the first 12 tags on a fresh DB get unique hues and additions thereafter stay balanced. The palette is **duplicated** on the frontend (`web/src/lib/tag-color.ts` `HUES`); change both together.

## script

File: `internal/script/service.go` + `internal/script/schedule.go`.

| Method | Notes |
|---|---|
| `Create` / `Update` / `Get` / `Delete` / `List` | CRUD. Schedule is JSON-encoded via `Schedule.MarshalConfig()` before store. |
| `DueAt(now)` | Lists enabled scripts, filters in Go using `Schedule.Matches`. |
| `SetLastRunAt` | Stamps regardless of outcome — prevents tight retry loops on broken scripts. |
| `ReadUserState` / `WriteUserState` | Raw `[]byte` to/from `scripts.user_state`. Empty → "{}". |
| `StartRun` / `FinishRun` / `AppendLog` | The run lifecycle. `FinishRun` JSON-encodes `spawned_task_ids`. |
| `GetRun` / `GetLogs` / `ListRunsByScript` / `ListAllRuns` | Read paths for the UI. |
| `CountRuns` / `PruneRuns(keep)` | Retention; pruning oldest cascades to logs. |
| `RecoverOrphanedRuns` | Called once at startup by the scheduler. |

### Schedule matching

`Schedule.Matches(now, lastRunAt)` — read `internal/script/schedule.go`. Truth table:

| Kind | Match when |
|---|---|
| `every_tick` | always |
| `daily` | last run not on today's local date |
| `weekly` | local weekday matches AND not run today |
| `monthly` | day-of-month matches (int 1-31 OR "last") AND not run today |

"Today" = `time.Time.Local()` year+yearday. Day boundaries at midnight local time.

`schedule_config` JSON shapes:
- `every_tick` / `daily`: `{}`
- `weekly`: `{"weekday": "monday"|...|"sunday"}`
- `monthly`: `{"day": 1..31}` OR `{"day": "last"}`

`MonthlyDay` is a tagged-union with custom `MarshalJSON`/`UnmarshalJSON` — read `internal/script/types.go` to understand the shape before changing wire-format.

## Error idioms

Service methods return `fmt.Errorf("<pkg>: <action>: %w", err)`. The HTTP error mapper (`writeServiceError` in `internal/httpapi/tasks.go`) uses substring matching on messages — keep these prefixes consistent:

- `"is required"`, `"invalid"`, `"must be"`, `"unknown tags"`, `"out of range"`, `"not staged"`, `"monthly schedule missing day"` → 400
- wrapped `sql.ErrNoRows` (via `%w`) → 404
- `"UNIQUE constraint failed"` / `"constraint failed"` → 409
- anything else → 500

**If you add a new validation error, use one of the existing substrings** or extend the list in `isValidationMessage`.

## Test scaffolding

Use `dbtest.New(t)` for an in-memory `*db.Store` with migrations applied. See `internal/db/dbtest/dbtest.go`. Service tests typically:

```go
store := dbtest.New(t)
svc := task.New(store)
ctx := context.Background()
// ...exercise svc...
```
