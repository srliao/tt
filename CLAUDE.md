# CLAUDE.md

Local-only single-user task tracker with userscript automation. Ships as one statically-linked Go binary with an embedded React SPA.

## Agent Documentation — READ FIRST

**Before touching code, consult `docs/agent/`.** It is the navigation-first
guide written specifically for AI agents:

- `docs/agent/README.md` — index; routes you to the right file by task
- `docs/agent/01-architecture.md` — process model, layer map, load-bearing rules
- `docs/agent/02-navigation.md` — file/symbol locator tables
- `docs/agent/03-data-layer.md` through `11-common-changes.md` — focused topics
- `docs/userscript-api.md` — public-facing `ctx` API reference for userscripts

## Keeping docs + CLAUDE.md current — MANDATORY

Whenever you make a non-trivial change (new endpoint, schema migration, new
ctx method, new service method, changed design pattern, moved files, new
top-level package, new command, new gotcha), **delegate a documentation pass
to a subagent before finishing the task**. The subagent's scope is BOTH
`docs/agent/` AND this file (`CLAUDE.md`) — keeping them in sync is part
of the job.

Use the Agent tool (subagent_type: `general-purpose`) with a prompt of the
form:

> "Review and update documentation for <SPECIFIC CHANGE>. Scope:
>   1. `docs/agent/` — the agent-development guide.
>   2. `/CLAUDE.md` — the project root memory file.
>
> Keep docs optimized for agent use: concise, navigation-first, focused on
> design patterns and where to find code rather than exact implementation.
> Each file stays self-contained and short.
>
> For `docs/agent/`: update only the files that genuinely need it; update
> the README index if you add or rename a file. Do NOT paste large code
> excerpts — link by file path + symbol instead.
>
> For `CLAUDE.md`: verify every command, file path, and rule still
> matches reality. Update load-bearing rules if patterns changed. Update
> 'After … changes' steps if new generated files or required regen
> commands were introduced. Keep it terse — CLAUDE.md is loaded into
> every session.
>
> Verify all file paths mentioned in either location still exist. Report
> what you changed and what you intentionally left alone."

Skip the subagent only for trivial changes (typo fixes, single-line bug
fixes, comment edits) that don't change any pattern, location, command,
or contract.

## Commands

```bash
just dev          # vite (5173) + go run (8080) in parallel
just be-dev       # backend only
just fe-dev       # frontend only
just test         # be-test + fe-test
just lint         # golangci-lint + biome check
just build        # full build with embedded SPA → ./bin/tt
just db-gen       # regen sqlc after editing internal/db/queries/*.sql
just clean        # nukes web/dist, node_modules, ./bin, ./.dev-data
```

Dev data lives in `./.dev-data/db.sqlite`. Delete to wipe state.

## Load-bearing rules (full list in docs/agent/01-architecture.md)

- **Consumer declares the interface.** Downstream packages define narrow
  interfaces; producer's `Impl` satisfies them structurally. Only
  `cmd/tt/main.go` knows concrete types.
- **`ctx` API never calls SQL directly** — it goes through task/tag/script
  services. Mutating effects are deferred-buffered, flushed only on
  `RunStatusOK`.
- **Fractional ordering keys** (priority, staged_order). Reorder = midpoint;
  rebalance when neighbors are within `1e-9`. See
  `internal/task/reorder.go`.
- **One scheduler worker.** All script execution is sequential.
- **JSON error envelope** for all non-2xx. Codes in
  `internal/httpapi/errors.go`. Service errors map via substring match in
  `writeServiceError` — keep prefixes like `"is required"`, `"invalid"`,
  `"must be"` consistent.
- **Timestamps as TEXT**: SQLite `datetime('now')` layout OR RFC3339.
  Parsers accept both.
- **Type mirrors are hand-maintained** in `web/src/types/`. No codegen
  between Go and TS — update both when changing DTOs.
- **Tag hue palette is duplicated** between `internal/tag/types.go`
  (`HuePalette`) and `web/src/lib/tag-color.ts` (`HUES`). Both must
  change together if expanded. Tags get a hue server-side via
  `pickLeastUsedHue` on `Service.Create` / `Resolve(autoCreate=true)`.
- **Task selection lives in `sessionStorage` (`tt:selection`), not the
  URL.** Single source of truth via `useSelection` in
  `web/src/features/tasks/use-selection.ts` (module-level store +
  `useSyncExternalStore` so every consumer shares one snapshot).

## After backend changes

- Edited `internal/db/queries/*.sql` → run `just db-gen`.
- Added a migration → numbered sequentially; embed.go picks it up
  automatically.
- Changed a service interface → update the consumer interface in
  `internal/httpapi/server.go` and any other declaring package.

## After frontend changes

- Added a route file → `pnpm tsr generate` (or just run `pnpm run build`).
- Added a UI shortcut → update `web/src/components/shortcut-cheatsheet.tsx`
  so users can discover it.

## Don't

- Don't edit `internal/db/sqlc/*.sql.go` or `web/src/routeTree.gen.ts` —
  both are generated.
- Don't add `setTimeout`/`fetch`/network access to the userscript runtime —
  sandbox is intentional.
- Don't bypass `just build` when producing a binary — the SPA embed step
  must run.
