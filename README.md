# tt

A local-only, single-user task tracker with first-class JavaScript
automation. Ships as one statically-linked Go binary with the React UI
embedded inside — no Node runtime, no database server, no cloud.

```sh
just build && ./bin/tt
# UI:  http://localhost:8080
```

## Why

Most task managers either lock you out of the data (SaaS) or ask you to
glue together a half-dozen processes (database, web server, frontend
build, sync daemon). `tt` is the opposite: a single binary you run on
your own machine, with your data in one SQLite file you can copy, diff,
or back up however you like. Everything is keyboard-first, and the only
extension mechanism is a tiny JS API — no plugin store, no marketplace,
no telemetry.

It is intentionally narrow: one user, one machine, one browser, no auth.
If you want multi-user, sync, or mobile, this isn't the project.

## Features

- **Two-tier workflow.** A main list (filter, sort, tag) and a stage —
  a small drag-orderable list of what you're actually working on now.
  Stage has a soft cap of 7 so it stays focused.
- **Tags with Any / All / Untagged.** Pick any combination from the
  sidebar or command palette. URL-shareable via `?tag_filter=`.
- **Userscripts.** Write small JS scripts that run on a schedule
  (`every_tick` / `daily` / `weekly` / `monthly`) or on demand. Scripts
  use a sandboxed `ctx` API — date helpers, per-script persistent state,
  structured logs, and `ctx.queueTask(...)` to spawn tasks. Each run is
  recorded so you can see what fired when and why.
- **Keyboard-first UI.** Command palette (`⌘K`), j/k navigation,
  multi-select with `x`, `⌘A` for select-all, inline editors. Press
  `?` for the cheat-sheet.
- **One binary.** SPA assets, SQL migrations, and the JS runtime are all
  embedded. Cross-compiles with `CGO_ENABLED=0` because storage uses
  pure-Go SQLite (`modernc.org/sqlite`).
- **Stable JSON API.** Everything the UI does is available at
  `/api/v1/*` with a consistent error envelope. Build your own client if
  you want.

## Install

### Build from source

You need [Go](https://go.dev) 1.22+, [Node](https://nodejs.org) 20+ with
[pnpm](https://pnpm.io), and [just](https://github.com/casey/just).

```sh
git clone https://github.com/srliao/tt.git
cd tt
just build
./bin/tt
```

The `build` target compiles the SPA, embeds it into the binary, and
produces `./bin/tt`. For a tagged release build with `git describe`
version stamping, use `just build-release`.

### Run

```sh
./bin/tt                              # default port 8080, default data dir
./bin/tt --port 9000                  # custom port
./bin/tt --data-dir ~/notes/tt        # custom data directory
./bin/tt --db /path/to/db.sqlite      # custom database path
```

Data defaults to `$XDG_DATA_HOME/tt/db.sqlite` (Linux/macOS:
`~/.local/share/tt/db.sqlite`). The schema is migrated automatically on
startup using embedded migrations. Backups: just copy the `.sqlite`
file.

## Userscript example

A daily 9am stand-up script:

```js
// Spawn a "stand-up notes" task once per weekday morning.
if (ctx.isWeekday(ctx.weekday()) && ctx.weekday() !== "saturday" && ctx.weekday() !== "sunday") {
  const today = ctx.today();
  if (ctx.state.get("lastSpawn") !== today) {
    ctx.queueTask({
      title: `Stand-up — ${today}`,
      tags: ["work", "standup"],
      due_date: today,
    });
    ctx.state.set("lastSpawn", today);
  }
}
```

Set the schedule to `daily`. The script runs at the next tick after
midnight and won't double-spawn because `ctx.state` persists between
runs. Logs from `ctx.log.*` are stored per-run and viewable in the Runs
page.

The full `ctx` API — date helpers, persistent state, logging, and
`queueTask` — is documented in
[docs/userscript-api.md](docs/userscript-api.md).
Notable guarantees: scripts run sequentially in a fresh `goja` runtime
with `setTimeout` / `fetch` / network access removed and a 5-second
hard interrupt. The sandbox is "don't accidentally hang the app," not
"defend against hostile code" — only run scripts you wrote.

## Project layout

```
cmd/tt/             entry point — the only place that wires concrete types
internal/
  config/           CLI flags, defaults
  db/               sqlc-generated queries, goose migrations
  task/             task domain (CRUD, fractional ordering, filter/sort)
  tag/              tag domain
  script/           script domain (CRUD, schedule parsing)
  scheduler/        15-minute ticker + single worker goroutine
  runtime/          goja JS engine, per-run isolated execution
  httpapi/          chi router, JSON error envelope
  web/              embed.FS for the built SPA
web/                React + Vite source (TypeScript, shadcn/ui, TanStack)
docs/agent/         navigation-first development guide
docs/userscript-api.md   public `ctx` API reference for script authors
```

For deeper architecture notes, read
[docs/agent/01-architecture.md](docs/agent/01-architecture.md) — it's
the canonical "what lives where + why" for contributors.

## Development

```sh
just dev          # vite (5173) + go run (8080) in parallel, with hot reload
just be-dev       # backend only
just fe-dev       # frontend only
just test         # backend + frontend tests
just lint         # golangci-lint + biome
just db-gen       # regenerate sqlc code after editing internal/db/queries/*.sql
just clean        # wipe ./bin, web/dist, web/node_modules, ./.dev-data
```

Dev mode writes to `./.dev-data/db.sqlite`. Delete the directory to
reset state.

### Testing

Backend tests use `t.TempDir()` SQLite databases (no fixtures, no
mocks). Frontend tests use Vitest + Testing Library against the real
component tree. Run both with `just test`.

### Conventions

- Backend services follow "consumer declares the interface" — each
  downstream package defines a narrow interface naming only the methods
  it uses; producers satisfy it structurally. Only `cmd/tt/main.go`
  knows concrete types.
- Frontend talks to the backend through TanStack Query hooks in
  `web/src/api/`. Forms use react-hook-form + zod.
- The Go ↔ TS type mirrors in `web/src/types/` are hand-maintained.
  Update both when changing a DTO.
- More detail in `docs/agent/`.

## Status

Built and used as a personal tool; the test suite is comprehensive
(unit + integration on both sides) and the binary has been stable in
day-to-day use. It is not packaged, signed, or distributed in any form
— building from source is the only install path. The HTTP API and DB
schema may change between commits without migration notes; check the
`internal/db/migrations/` directory and recent commits if you upgrade
mid-tree.

## License

[MIT](LICENSE).

## Acknowledgements

Built on top of [chi](https://github.com/go-chi/chi),
[sqlc](https://sqlc.dev), [goose](https://github.com/pressly/goose),
[goja](https://github.com/dop251/goja),
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite),
[React](https://react.dev), [Vite](https://vite.dev),
[TanStack Router](https://tanstack.com/router) +
[Query](https://tanstack.com/query),
[shadcn/ui](https://ui.shadcn.com), and
[CodeMirror 6](https://codemirror.net).
