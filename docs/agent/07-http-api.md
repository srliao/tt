# HTTP API

`chi` router under `/api/v1`. SPA served from `/*` fallback.

Files: `internal/httpapi/{server,middleware,errors,spa,health,tasks,stage,tags,scripts,runs}.go`.

## Endpoint map

```
GET    /api/v1/health
GET    /api/v1/version

GET    /api/v1/tasks?state=&tag_filter=&tags_exclude=&due=&q=&sort=&asc=&limit=&offset=
POST   /api/v1/tasks
GET    /api/v1/tasks/{id}
PATCH  /api/v1/tasks/{id}
DELETE /api/v1/tasks/{id}
POST   /api/v1/tasks/{id}/state    { state }
POST   /api/v1/tasks/{id}/stage
DELETE /api/v1/tasks/{id}/stage
POST   /api/v1/tasks/reorder       { task_id, before_id?, after_id? }

POST   /api/v1/stage/reorder       { task_id, before_id?, after_id? }
DELETE /api/v1/stage               (clear all staged)
DELETE /api/v1/stage/finished      (clear only done+cancelled)

GET    /api/v1/tags?counts=1       (counts=1 → [{id,name,count,…}] via tag.Service.ListWithCounts)
POST   /api/v1/tags                { name }
PATCH  /api/v1/tags/{id}           { name }
DELETE /api/v1/tags/{id}

GET    /api/v1/scripts
POST   /api/v1/scripts
GET    /api/v1/scripts/{id}
PATCH  /api/v1/scripts/{id}
DELETE /api/v1/scripts/{id}
POST   /api/v1/scripts/{id}/run    → { run_id }
GET    /api/v1/scripts/{id}/runs?limit=&before=
GET    /api/v1/scripts/{id}/tasks?limit=&cursor=

GET    /api/v1/runs?script_id=&status=&from=&to=&limit=&cursor=
GET    /api/v1/runs/{id}            ── flattened run + logs + spawned_tasks

GET    /*                           ── SPA index.html (or asset under /assets/)
```

## Error envelope (uniform)

```json
{
  "error": {
    "code":    "validation_failed",
    "message": "title is required",
    "details": { "field": "title" }
  }
}
```

Codes (`internal/httpapi/errors.go`):
- `validation_failed` — 400
- `not_found` — 404
- `conflict` — 409
- `scheduler_busy` — 503
- `internal` — 500

The HTTP error mapper is `writeServiceError` in `internal/httpapi/tasks.go`. It matches errors **by substring** to avoid importing scheduler/sqlite from every handler — see [04-backend-services.md](./04-backend-services.md) for the canonical substrings.

## Server construction

`httpapi.New(tasks, tags, scripts, enqueuer, pinger, Options{...})` returns a `*Server`. Each service param is one of the consumer interfaces declared in `server.go`:

- `TaskService` — narrow slice of `task.Service`.
- `TagService` — narrow slice of `tag.Service`.
- `ScriptService` — narrow slice of `script.Service`.
- `ManualRunEnqueuer` — single-method `EnqueueManual(scriptID, runID int64) error`.
- `Pinger` — `Ping(ctx) error`. Use `PingerFunc(db.Open's *sql.DB.PingContext)`.

`Options` carries the non-service knobs: `Logger`, `Version`, `BuiltAt`, `Timezone` (resolved app zone name, surfaced by `/version`), `SPA`.

`Routes()` returns the `http.Handler` with all middleware mounted. Middleware chain (in order): `requestID`, `slogLog`, `recoverPanic`.

## Adding a new endpoint

1. **Pick the right file** (tasks.go, stage.go, scripts.go, tags.go, runs.go) and the right `mount*Routes` method.
2. **Add the service method** in `internal/<domain>/service.go` (interface AND impl). Update the consumer interface in `server.go` to expose it.
3. **Decode body / parse query**. Use the package's existing helpers (`parsePathID`, `parseIntDefault`, `parseRFC3339Optional`).
4. **Validation errors** → `writeError(w, http.StatusBadRequest, CodeValidation, msg, details)`.
5. **Service errors** → `writeServiceError(w, err)`.
6. **Success** → `writeJSON(w, statusCode, payload)` or `w.WriteHeader(http.StatusNoContent)` for empty success.
7. **Test it** in `<file>_test.go` using `httptest` + an in-memory `dbtest.New(t)` or a stub service.

## Search/filter conventions

- Query-string filters use **multi-value params** when multi-select (e.g., `?state=not_done&state=done`).
- Booleans use `strconv.ParseBool` (accepts `1/0`, `true/false`).
- Tag filters accept **names**, not ids. The handler resolves them via `tag.Service.Resolve(..., autoCreate: false)` — unknown tags currently 400. (This was a deliberate design choice; see `handleListTasks` comments.)
- Tag inclusion uses a single `tag_filter=` param of the form `<mode>:<csv>` where mode is `any` or `all` and the CSV may include the `@untagged` sentinel (e.g. `tag_filter=any:work,errand`, `tag_filter=all:work,urgent`, `tag_filter=any:@untagged`, `tag_filter=any:@untagged,work`). The `all:@untagged,…real` combination is an impossible set and returns an empty list (never 500). Malformed strings (no colon / unknown mode / empty list) degrade to "no filter" silently; unknown tag NAMES still surface as 400. The legacy `tag=` (repeated) + `tag_mode=` reader was removed in Phase 6 and is now silently ignored.
- `tags_exclude=` is **CSV** (`?tags_exclude=a,b`) and drops any task carrying at least one excluded tag. Inclusion and exclusion compose with AND.
- Tag names starting with `@` are reserved for sentinel tokens (currently only `@untagged`) and rejected by `POST /tags` and the `tag.Service.Resolve` path with a 400.
- `sort` defaults to `priority`; priority always sorts ASC regardless of `asc`. Keys are minted newest-first (`MIN - 1`), so a task created via `POST /tasks` comes back at the **head** of the default list — no reorder call needed.
- Date filters (`from`, `to` on `/runs`) are RFC3339.

## Manual script run

POST `/scripts/{id}/run` flow:

```
1. Load script. 404 if missing.
2. If !enabled → 409 with code=validation_failed, message="script is disabled".
3. StartRun(scriptID, manual) → returns run_id immediately.
4. enqueuer.EnqueueManual(id, run_id).
   - ErrSchedulerBusy → FinishRun(error, "scheduler busy"); 503 scheduler_busy.
   - other error → FinishRun(error, "scheduler busy"); 500.
5. 200 { "run_id": N }.
```

The UI navigates to `/runs/$id` on success and polls.

## SPA handler

`internal/httpapi/spa.go`:
- `/assets/<file>` → 31536000s cache (immutable hashed bundles).
- `/index.html` or any non-asset path → `Cache-Control: no-cache`, serves `index.html`.
- Missing asset → 404 envelope (do NOT fall back to index.html for `.js`/`.css` — corrupts the bundle).

## Health & version

- `/health` pings the DB with a 2s timeout. Returns `{ "status": "ok", "db": "ok"|"down" }`.
- `/version` returns `{ "version", "built_at", "timezone" }` (`versionResponse` in `health.go`; empty fields are omitted). `version` / `built_at` come from `-ldflags` at build time; `timezone` is the resolved app time zone (`Options.Timezone` ← `config.Config.Location.String()`), reported so "why did my daily script fire at 8pm" is answerable with one HTTP call.

## Middleware notes

- **Request ID**: incoming `X-Request-Id` is trusted; otherwise a fresh UUID is minted. Available via `RequestIDFromContext(ctx)`.
- **Logging**: one line per request with `method/path/status/duration_ms/request_id`. Uses `slog` text handler in dev, JSON-capable handler in prod (configurable in `main.go`).
- **Recover**: any handler panic → 500 envelope, logged at error severity with request_id.
