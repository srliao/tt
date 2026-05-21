# tt — Task Tracker Implementation Plan (Index)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement these plans phase-by-phase. Each phase is in its own file. Tasks use checkbox (`- [ ]`) syntax for tracking.

**Source spec:** `docs/superpowers/specs/2026-05-21-task-tracker-design.md` — read this first if you have no context for the project.

**Goal:** Build `tt` — a local-only, single-user task tracker in Go + React with userscript automation. Ships as one static binary. See spec §1.

**Architecture:** Go backend (chi + sqlite + goja for JS userscripts) serves a React SPA (Vite + shadcn + TanStack). Frontend builds to `web/dist/`, embedded into the Go binary via `go:embed`. See spec §2.

**Tech Stack** (versions verified 2026-05-21):
- **Backend:** Go 1.22+, chi, modernc.org/sqlite, sqlc, goose, goja, slog.
- **Frontend:** Vite **8.x**, React **19.x**, TypeScript **6.x**, Tailwind **v4.x** (CSS-first config via `@tailwindcss/vite`), shadcn **4.x** (CLI; note package renamed from `shadcn-ui`), TanStack Router v1 (file-based) + Query v5, dnd-kit (sortable v10), CodeMirror 6, react-hook-form + **zod v4** with `@hookform/resolvers v5`, date-fns v4, lucide-react v1, Biome **v2**, Vitest **v4** + Testing Library 16, pnpm.
- **Build:** Justfile orchestration.

If any frontend dependency seems out of date during execution, use the **find-docs** / **context7** skill to fetch current versions and APIs before installing — this ecosystem moves fast.

---

## Repository convention

- Git **is** initialized at `/Users/srliao/code/tt/` on the `main` branch. Commit after each task.
- All file paths in plan files are absolute or relative to `/Users/srliao/code/tt/`.
- Phase files live in `docs/superpowers/plans/2026-05-21-tt/`.

## Commit discipline

- **One commit per completed task.** When a task's verification step passes, run the commit step before moving on.
- Use **Conventional Commits**: `feat(scope): …`, `fix(scope): …`, `chore(scope): …`, `test(scope): …`, `refactor(scope): …`, `docs(scope): …`.
- Scope is the package or area (e.g. `task`, `tag`, `script`, `db`, `runtime`, `http`, `web`, `runs`).
- Stage narrowly: prefer `git add <specific paths>` over `git add -A`. Each phase file shows the exact `git add` invocation per task.
- No `--no-verify`, no `--amend` (always create a new commit), no force-push.
- If a verification step fails, fix the issue and commit normally; do not amend.

## Phases

| File | Phase | Depends on | Parallelizable with |
|---|---|---|---|
| `01-project-bootstrap.md` | Project bootstrap (Go module, dirs, justfile, main, config) | — | — |
| `02-db-layer.md` | DB layer (sqlite, goose migrations, sqlc, store) | 01 | — |
| `03a-task-service.md` | Task domain service | 02 | 03b, 03c |
| `03b-tag-service.md` | Tag domain service | 02 | 03a, 03c |
| `03c-script-service.md` | Script domain + schedule matching + runs/logs | 02 | 03a, 03b |
| `04-runtime.md` | goja userscript runtime + `ctx` API | 03a, 03b, 03c | 05 (partial), 06, 07 |
| `05-scheduler.md` | Background scheduler + worker goroutine | 03c, 04 | 06, 07 |
| `06-http-api.md` | chi router + all JSON handlers + SPA fallback wiring | 03a, 03b, 03c, 04 (for "run now") | 04 partial, 05, 07 |
| `07-frontend-bootstrap.md` | Vite + shadcn + routing + base layout + shortcuts | — | 01–06 (fully independent) |
| `08a-tasks-page.md` | `/tasks` page + add-task modal + drag-drop reorder | 06 (tasks), 07 | 08b, 08c, 08d, 08e |
| `08b-stage-page.md` | `/stage` page + dnd-kit reorder | 06 (stage), 07 | 08a, 08c, 08d, 08e |
| `08c-tags-page.md` | `/tags` page | 06 (tags), 07 | 08a, 08b, 08d, 08e |
| `08d-scripts-page.md` | `/scripts` list + editor + spawned-tasks panel | 06 (scripts), 07 | 08a, 08b, 08c, 08e |
| `08e-runs-page.md` | `/runs` global log + `/runs/$id` detail | 06 (runs), 07 | 08a, 08b, 08c, 08d |
| `09-embed-release.md` | `go:embed dist`, wire static handler, `just build`, smoke test | All previous | — |

## Dependency graph

```
01 ──→ 02 ──┬──→ 03a ──┐
            ├──→ 03b ──┼──→ 06 ──┐
            └──→ 03c ──┤          ├──────────────────────┐
                       │          │                      │
                       └──→ 04 ──→ 05                    │
                                                          ▼
                                                         09
                                                          ▲
07 ──┬──→ 08a ────────────────────────────────────────────┤
     ├──→ 08b ────────────────────────────────────────────┤
     ├──→ 08c ────────────────────────────────────────────┤
     ├──→ 08d ────────────────────────────────────────────┤
     └──→ 08e ────────────────────────────────────────────┘
```

## Parallelism guide

### Wave 1 (sequential)
1. **01** — Project bootstrap.
2. **02** — DB layer.

### Wave 2 (3 agents in parallel)
- **03a** task service · **03b** tag service · **03c** script service.

These touch independent tables, share only the store; no cross-imports.

### Wave 3 (parallel)
- **04** runtime — depends on 03a/b/c.
- **06** HTTP API — depends on 03a/b/c. Endpoints that call `runtime.Run` (POST `/scripts/:id/run`) require 04 finished; the rest don't.
- **07** frontend bootstrap — fully independent; can start in Wave 1 if a frontend agent is available.

### Wave 4
- **05** scheduler — depends on 04 + 03c. Can run alongside any unfinished 06 work and any 08x work.

### Wave 5 (5 agents in parallel)
After 06 + 07 land:
- **08a** tasks page · **08b** stage page · **08c** tags page · **08d** scripts page · **08e** runs page.

### Wave 6 (sequential)
- **09** embed + release build + smoke test.

## Conventions for all phases

### TDD discipline
Every behavior-bearing task follows: **failing test → minimal implementation → green test → commit.** Pure scaffolding tasks (writing config files, regenerating sqlc, etc.) skip the failing-test step but still include a verification command before committing.

### Go conventions
- Module path: `github.com/srliao/tt`.
- Package names: short, lowercase, no underscores.
- Errors: wrap with `fmt.Errorf("...: %w", err)`; sentinel errors for known cases.
- Loggers: pass `*slog.Logger` explicitly; no globals.
- Time-dependent logic accepts a `clock` interface so tests can use a fake.

### Test database
Services that touch the DB use a real in-memory SQLite with migrations applied. The `internal/db/dbtest.New(t)` helper from phase 02 is the single entry point.

### Frontend conventions
- TypeScript `strict: true`.
- Components: `PascalCase.tsx`. Hooks/utils: `camelCase.ts`.
- One component per file unless tightly coupled.
- All server data flows through TanStack Query hooks in `web/src/api/*` — components never call `fetch` directly.
- Types: hand-written TS in `web/src/types/` that mirror Go DTOs.

### When a phase blocks on another
If an agent discovers a missing dependency from an earlier phase, **stop**, record it in "Cross-phase findings" below, and either complete the missing piece in its proper phase or escalate.

## Resolved cross-phase contracts

These were ambiguous in the spec; the plans now spell them out. Implementation should follow these without revisiting:

- **Run row ownership.**
  - `runtime.Runner.Run(ctx, scriptID, runID, trigger)` — runID is passed in by the caller.
  - **Scheduled** triggers: the scheduler's worker goroutine calls `script.StartRun(scriptID, "scheduled")` to obtain `runID` *just before* invoking the runtime.
  - **Manual** triggers: the HTTP handler calls `script.StartRun(scriptID, "manual")` first (so it can return `{run_id}` immediately), then enqueues `EnqueueManual(scriptID, runID)`. If the queue is full, the handler must `FinishRun(runID, RunStatusError, "scheduler busy", nil)` so the row doesn't sit in `running` forever, then return 503.
- **No `staged=true` server filter on `GET /tasks`.** The stage page fetches the full list and filters client-side. Volume is small enough that this is a non-issue for v1.
- **Embed strategy.** Go's `//go:embed` can't reference `../../web/dist`. `just build` *copies* `web/dist/` into `internal/web/dist/` immediately before `go build` and *removes it* via a shell trap once the build completes. `internal/web/dist/` is gitignored. No symlinks.

## Cross-phase findings

_(Append entries here as agents discover gaps during execution. Format: `[YYYY-MM-DD] [phase-id] description.`)_

- [2026-05-21] [01] `build-release` recipe in the verbatim justfile block had `$(git describe ...)` inside single quotes, which `sh` does not expand. Reworked the recipe into a bash shebang block that captures `VERSION` first, then passes it into `-ldflags` via double quotes.
- [2026-05-21] [01] Added `/internal/web/dist/` to `.gitignore` ahead of phase 09 to align with the embed strategy in "Resolved cross-phase contracts" — protects against a half-completed `just build` leaving copied assets on disk.

## Verification before claiming "done"

Each phase file ends with a **Phase completion checklist**. Do not move to the next phase until every box is green.

## After all phases complete

Phase 09 produces a single `bin/tt`. Smoke-test:
1. `./bin/tt --data-dir /tmp/tt-smoke` starts.
2. Open `http://localhost:8080` → SPA loads.
3. Create a task → see it in `/tasks`.
4. Stage it → appears in `/stage`.
5. Create an `every_tick` script with `ctx.queueTask({title: "hi", tags: []})` → click "Run now" → confirm a task appears and the run shows `ok`.

If all five steps work, v1 is done.
