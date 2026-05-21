# Phase 06 — HTTP API (chi router + handlers)

> Read `00-index.md` first. Commit after each task.

**Goal:** A `chi`-based HTTP server exposing every endpoint in spec §6 under `/api/v1`, with the uniform error envelope from spec §8, request logging via `slog`, panic recovery, request id, and an SPA fallback handler that serves embedded assets (asset embedding itself lands in phase 09).

**Dependencies:** Phase 03a (task), 03b (tag), 03c (script). Phase 04 (runtime) required for `POST /scripts/:id/run` — the scheduler from phase 05 actually enqueues; the HTTP handler only needs the scheduler's `Enqueue` method, so plan that handler to depend on a narrow interface satisfied by either runtime or scheduler.

**Tech stack:** `github.com/go-chi/chi/v5`, stdlib `net/http`, `encoding/json`, `slog`. UUIDs via `github.com/google/uuid`.

**Parallelizable with:** 04 partial, 05, 07.

## File map

```
internal/httpapi/
├── server.go              # New(...) + Routes()
├── middleware.go          # request id + slog log + panic recovery
├── errors.go              # error envelope helpers
├── tasks.go               # task handlers
├── tags.go                # tag handlers
├── stage.go               # stage-only endpoints (reorder, clear)
├── scripts.go             # script handlers + manual run
├── runs.go                # run/log handlers
├── health.go              # /health, /version
├── spa.go                 # SPA fallback (wired to embed.FS in phase 09)
└── *_test.go              # one test file per handler group
```

## Background

- Endpoint table per spec §6. Re-read that section — this phase implements all of it.
- Error envelope per spec §8 — single shape `{ error: { code, message, details } }` for every 4xx/5xx.
- Filter/sort URL params on `GET /tasks` map to `task.FilterSort` (sort/asc/state[]/tag[]/due/q/limit/offset). **No `staged` server-side filter** — the stage page fetches `GET /tasks` and filters client-side (`staged_order !== null`). Volume is small enough that this is a non-issue for v1.
- All handlers depend on **service interfaces** (not concrete types). Define narrow consumer interfaces in `server.go`.

## Task 1: Add deps

- [ ] Run:
  ```bash
  cd /Users/srliao/code/tt && \
    go get github.com/go-chi/chi/v5@latest && \
    go get github.com/google/uuid@latest && \
    go mod tidy
  ```
- [ ] Commit:
  ```bash
  git add go.mod go.sum && git commit -m "chore(http): add chi + uuid deps"
  ```

## Task 2: Error envelope + helpers — failing test

**Files:** `internal/httpapi/errors_test.go`, `internal/httpapi/errors.go`

- [ ] **Tests cover** `writeError(w, status, code, message, details)`:
  - Response body is JSON shape `{"error":{"code":"...","message":"...","details":{...}}}`.
  - `Content-Type: application/json`.
  - HTTP status matches `status` arg.
  - `writeJSON(w, status, payload)` writes JSON with the right content-type and status.
- [ ] Implement helpers. Define stable error code constants used throughout: `CodeValidation`, `CodeNotFound`, `CodeConflict`, `CodeSchedulerBusy`, `CodeInternal`.
- [ ] Commit:
  ```bash
  git add internal/httpapi/errors.go internal/httpapi/errors_test.go && \
    git commit -m "feat(http): add JSON error envelope helpers"
  ```

## Task 3: Middleware (request id, logging, recovery)

**Files:** `internal/httpapi/middleware.go`, `internal/httpapi/middleware_test.go`

- [ ] **Tests cover:**
  - `requestID` middleware sets a `X-Request-Id` header (uuid v4) and stores it in context for downstream loggers.
  - `slogLog` middleware logs one line per request with method, path, status, duration_ms, request_id.
  - `recoverPanic` middleware catches panics in downstream handlers, logs at error, and returns a 500 with the standard error envelope.
- [ ] Implement each. `slogLog` wraps `http.ResponseWriter` to capture the status code.
- [ ] Commit:
  ```bash
  git add internal/httpapi/middleware.go internal/httpapi/middleware_test.go && \
    git commit -m "feat(http): add request id, logging, panic recovery middleware"
  ```

## Task 4: Server skeleton + health + version

**Files:** `internal/httpapi/server.go`, `internal/httpapi/health.go`, `internal/httpapi/health_test.go`

- [ ] In `server.go` define narrow consumer interfaces — only the methods this layer calls:
  ```go
  type TaskService interface { /* Create, Update, Get, Delete, List, SetState,
      Stage, Unstage, ClearStage, ClearFinishedFromStage,
      ReorderMain, ReorderStage, ByScript, SetTagsByID */ }
  type TagService interface { /* Create, Rename, Delete, List, Resolve */ }
  type ScriptService interface { /* Create, Update, Get, Delete, List,
      ListRunsByScript, ListAllRuns, GetRun, GetLogs,
      StartRun (used by manual /scripts/:id/run handler) */ }
  // The manual-run handler creates the run row itself (so it can return run_id
  // immediately) and hands the existing runID to the scheduler.
  type ManualRunEnqueuer interface {
      EnqueueManual(scriptID, runID int64) error
  }
  ```
- [ ] `Server` struct holds the four interfaces + a `*slog.Logger` + the `Version` string + an `http.Handler spa` field (the SPA handler; phase 09 wires the embedded FS, phase 06 wires a placeholder that 404s on `/` so tests pass).
- [ ] `New(...)` constructor + `Routes() http.Handler` building a chi router:
  ```go
  r := chi.NewRouter()
  r.Use(requestID, slogLog(logger), recoverPanic(logger))
  r.Route("/api/v1", func(r chi.Router) {
      r.Get("/health", s.health)
      r.Get("/version", s.version)
      r.Route("/tasks", ...) // populated in subsequent tasks
      r.Route("/stage", ...)
      r.Route("/tags",  ...)
      r.Route("/scripts", ...)
      r.Route("/runs", ...)
  })
  r.Handle("/*", s.spa)   // SPA fallback
  return r
  ```
- [ ] Implement `health` returning `{"status":"ok","db":"ok"}` (DB ping via an injected `Pinger interface { Ping(ctx) error }` so it can be mocked). `version` returns `{"version":..., "built_at":...}`.
- [ ] **Tests cover:** `GET /api/v1/health` returns 200 with the expected JSON; `GET /api/v1/version` returns the configured version string.
- [ ] Commit:
  ```bash
  git add internal/httpapi/server.go internal/httpapi/health.go internal/httpapi/health_test.go && \
    git commit -m "feat(http): add server skeleton with health and version"
  ```

## Task 5: Task handlers

**Files:** `internal/httpapi/tasks.go`, `internal/httpapi/tasks_test.go`

Endpoints (spec §6):
- `GET    /tasks` — list with filters.
- `POST   /tasks` — create.
- `GET    /tasks/:id` — get.
- `PATCH  /tasks/:id` — update (title/notes/due_date/tags).
- `DELETE /tasks/:id`.
- `POST   /tasks/:id/state` — `{state: "..."}`.
- `POST   /tasks/:id/stage` / `DELETE /tasks/:id/stage`.
- `POST   /tasks/reorder` — `{task_id, before_id?, after_id?}`.

- [ ] **Tests cover** (use `httptest.NewServer` with a real in-memory store + real services):
  - `POST /tasks` with `{title:"x"}` returns 201 with the created task JSON.
  - `POST /tasks` with empty title returns 400 with `code: validation_failed`.
  - `GET /tasks` returns the list (default sort).
  - `GET /tasks?state=done` returns only done tasks.
  - `GET /tasks?tag=work&tag=urgent` applies AND filtering (requires both tags) — tags resolved by name → id at the handler layer via `TagService.Resolve(names, false)`.
  - `GET /tasks?q=milk` substring search.
  - `GET /tasks?sort=due_date&asc=false` reverses.
  - `PATCH /tasks/:id` updates fields.
  - `POST /tasks/:id/state {state:"done"}` sets state and returns the updated task.
  - `POST /tasks/:id/stage` then `DELETE .../stage` toggles.
  - `POST /tasks/reorder` with `{task_id, before_id, after_id}` reorders and returns the updated task.
  - `DELETE /tasks/:id` returns 204; subsequent GET returns 404 with `code: not_found`.
- [ ] Implement handlers. Use `chi.URLParam(r, "id")` + `strconv.ParseInt` for path params. Translate service errors:
  - `errors.Is(err, sql.ErrNoRows)` → 404 / `not_found`.
  - Validation error (title empty, invalid date, invalid state) → 400 / `validation_failed` with `details: {field: msg}`.
  - Otherwise 500 / `internal`.
- [ ] On the `PATCH` flow with tags: after `Update`, call `TagService.Resolve(names, autoCreate=true)` then `TaskService.SetTagsByID(id, ids)`. Reload via `Get` to get the freshest tag list and return it.
- [ ] Commit:
  ```bash
  git add internal/httpapi/tasks.go internal/httpapi/tasks_test.go && \
    git commit -m "feat(http): add task handlers"
  ```

## Task 6: Stage handlers

**Files:** `internal/httpapi/stage.go`, `internal/httpapi/stage_test.go`

Endpoints:
- `POST   /stage/reorder` — `{task_id, before_id?, after_id?}` → `ReorderStage`.
- `DELETE /stage` — clear all.
- `DELETE /stage/finished` — clear only done/cancelled from stage.

- [ ] **Tests cover:** each endpoint round-trips against a real store with two staged tasks, asserting expected `staged_order` outcome.
- [ ] Commit:
  ```bash
  git add internal/httpapi/stage.go internal/httpapi/stage_test.go && \
    git commit -m "feat(http): add stage reorder/clear endpoints"
  ```

## Task 7: Tag handlers

**Files:** `internal/httpapi/tags.go`, `internal/httpapi/tags_test.go`

Endpoints:
- `GET    /tags`, `POST /tags`, `PATCH /tags/:id`, `DELETE /tags/:id`.

- [ ] **Tests cover:**
  - Create + list + rename + delete round-trip.
  - Empty name → 400 validation.
  - Duplicate name on rename: SQLite unique constraint → return 409 with `code: conflict`.
  - Delete cascades (verify by creating a task with the tag, deleting the tag, then asserting the task no longer has it).
- [ ] Commit:
  ```bash
  git add internal/httpapi/tags.go internal/httpapi/tags_test.go && \
    git commit -m "feat(http): add tag handlers"
  ```

## Task 8: Script handlers + manual run

**Files:** `internal/httpapi/scripts.go`, `internal/httpapi/scripts_test.go`

Endpoints:
- `GET    /scripts`, `POST /scripts`, `GET /scripts/:id`, `PATCH /scripts/:id`, `DELETE /scripts/:id`.
- `POST   /scripts/:id/run` — manual trigger; enqueue and respond `{run_id}`.
- `GET    /scripts/:id/runs?limit=&before=` — paginated recent runs.
- `GET    /scripts/:id/tasks?limit=&cursor=` — tasks spawned by this script.

- [ ] **Tests cover:**
  - Create + list + get + patch + delete round-trip.
  - Schedule validation: posting `{schedule: {kind: "weekly", weekday: "fundayday"}}` returns 400.
  - `POST /scripts/:id/run`:
    - On a disabled script: returns 409 with `code: validation_failed` and a message about being disabled.
    - On enabled script: handler calls `scripts.StartRun(ctx, id, TriggerManual)` to obtain a `runID` immediately, then `enqueuer.EnqueueManual(scriptID, runID)`. Response is 200 with `{run_id: N}` even though the run is still in flight; UI uses this to navigate to `/runs/:run_id` and watch logs stream in. The runtime later calls `FinishRun(runID, ...)` to terminalize it (it does NOT create the row).
    - When `EnqueueManual` returns `scheduler.ErrSchedulerBusy`: handler must mark the just-created run as failed (`scripts.FinishRun(runID, RunStatusError, "scheduler busy", nil)`) so the run row doesn't sit in `running` forever, then return 503 with `code: scheduler_busy`.
  - `GET /scripts/:id/runs` paginates by `started_at DESC`; `limit=10` default.
  - `GET /scripts/:id/tasks` returns tasks created by the script in newest-first order.
- [ ] Commit:
  ```bash
  git add internal/httpapi/scripts.go internal/httpapi/scripts_test.go && \
    git commit -m "feat(http): add script handlers and manual run"
  ```

## Task 9: Run / log detail handlers

**Files:** `internal/httpapi/runs.go`, `internal/httpapi/runs_test.go`

Endpoints:
- `GET /runs?script_id=&status=&from=&to=&limit=&cursor=`.
- `GET /runs/:id` — run + logs + spawned task summaries.

- [ ] **Tests cover:**
  - List filter combinations: `?script_id=`, `?status=ok|error|timeout|running`, date range.
  - Detail: returns the run + a `logs` array + a `spawned_tasks` array (each a short summary `{id,title,state}` looked up by id).
  - Missing id → 404.
- [ ] Implement. For list filters, build SQL dynamically (similar pattern to `task.List`) — add a new sqlc query later if filters get gnarly; for v1 a Go-side `strings.Builder` is fine.
- [ ] Commit:
  ```bash
  git add internal/httpapi/runs.go internal/httpapi/runs_test.go && \
    git commit -m "feat(http): add runs list and detail handlers"
  ```

## Task 10: SPA fallback handler (placeholder)

**Files:** `internal/httpapi/spa.go`

- [ ] Implement `NewSPAHandler(fs fs.FS) http.Handler` that:
  - For requests to `/assets/*` returns the matching file (or 404 if missing).
  - For anything else returns `index.html` (SPA fallback so client-side routes work on refresh).
  - Sets `Cache-Control` per spec §7: hashed `/assets/*` → `public, max-age=31536000, immutable`; `index.html` → `no-cache`.
- [ ] For phase 06, `fs` may be empty/nil and the handler returns 404 with a placeholder message. Phase 09 swaps in the embed.FS.
- [ ] **Tests cover** (against a `fstest.MapFS` fixture): `/index.html` served with `no-cache`; `/assets/foo-abc.js` served with the long-cache header; missing `/assets/missing.js` → 404; `/some/spa/route` → `index.html`.
- [ ] Commit:
  ```bash
  git add internal/httpapi/spa.go internal/httpapi/spa_test.go && \
    git commit -m "feat(http): add SPA fallback handler"
  ```

## Task 11: Wire HTTP server into `cmd/tt/main.go`

**Files:** `cmd/tt/main.go`

- [ ] Update `main` to:
  - Open the store (`db.Open(ctx, cfg.DBPath)`).
  - Instantiate `task`, `tag`, `script` services.
  - Instantiate the runtime `Runner`. Pass it to the scheduler. Start the scheduler (which performs `RecoverOrphanedRuns` + initial sweep).
  - Instantiate `httpapi.Server` with: `task`, `tag`, `script` services + the scheduler as the `ManualRunEnqueuer`. Mount routes onto `http.Server{Addr: fmt.Sprintf(":%d", cfg.Port)}`.
  - Listen for `SIGINT`/`SIGTERM` and gracefully shut down: HTTP server first (`server.Shutdown(ctx)`), then `scheduler.Stop()`, then `store.Close()`.
  - Replace the previous "nothing to do yet" log with the actual lifecycle.
- [ ] Verify: `go run ./cmd/tt --data-dir /tmp/tt-dev --port 8080` starts; `curl http://localhost:8080/api/v1/health` returns `{"status":"ok","db":"ok"}`; `Ctrl-C` shuts down cleanly.
- [ ] Commit:
  ```bash
  git add cmd/tt/main.go && git commit -m "feat(cmd): wire HTTP server and scheduler in main"
  ```

## Phase completion checklist

- [ ] `go test ./internal/httpapi/... -v` all pass.
- [ ] `go build ./...` clean.
- [ ] `curl http://localhost:8080/api/v1/health` returns 200 and the right JSON when the binary is running.
- [ ] Every endpoint in spec §6 has a handler and a test.
- [ ] Error envelope is uniform across every endpoint.
