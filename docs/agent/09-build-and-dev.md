# Build, Dev Loop, Embed

`just` is the canonical dev runner. Read `justfile` first — anything important is there.

## Common commands

```bash
just                # list all
just dev            # vite (5173) + go run (8080) in parallel; Ctrl-C kills both
just be-dev         # go run ./cmd/tt --port 8080 --data-dir ./.dev-data
just fe-dev         # cd web && pnpm run dev
just fe-install     # pnpm install (run once)
just test           # be-test + fe-test
just lint           # be-lint + fe-lint (golangci-lint + biome check)
just build          # fe-build → copy dist → go build → ./bin/tt
just build-release  # CGO_ENABLED=0 + -ldflags version baked from git
just db-gen         # cd internal/db && sqlc generate
just clean          # nuke web/dist, web/node_modules, ./bin, ./.dev-data, internal/web/dist/*
```

## Dev workflow

```
┌──────────────┐         /api/v1/*          ┌──────────────┐
│ vite 5173    ├─── proxy ─────────────────►│  go run :8080│
│ HMR-enabled  │                            │  serves /api │
│ SPA          │                            │  + SPA stub  │
└──────────────┘                            └──────────────┘
       browser at http://localhost:5173
```

- Frontend HMR is fast — edit `web/src/**` and the browser updates.
- Backend has no hot-reload — Ctrl-C `just be-dev` and re-run after Go edits.
- Data path during dev: `./.dev-data/db.sqlite` (created on first run). Delete this file to wipe state.

## Embed pipeline

`just build` does this in order:

1. `pnpm run build` → `web/dist/` (index.html + hashed JS/CSS).
2. **Wipe `internal/web/dist/`** (except `.gitkeep`).
3. `cp -R web/dist/. internal/web/dist/` so the `//go:embed all:dist` directive in `internal/web/assets.go` can reach it.
4. `go build -trimpath -ldflags='-s -w -X main.Version=dev' -o ./bin/tt ./cmd/tt`.
5. **Trap on exit cleans up `internal/web/dist/`** so the working tree stays untouched even on build failure.

If you build with `go build` directly (skipping `just build`), the binary will respond 404 to SPA routes — the `dist/.gitkeep` sentinel makes the directive resolvable but the bundle isn't there. Always go through `just build`.

## Migrations

Embedded via `internal/db/migrations/embed.go`. Applied on startup with goose. Adding a migration:

1. Create `internal/db/migrations/000N_<name>.sql` with `-- +goose Up` / `-- +goose Down` markers.
2. Update `internal/db/queries/*.sql` if needed.
3. `just db-gen` regenerates `internal/db/sqlc/*.sql.go`.
4. Commit both.

## Versioning

`-ldflags "-X main.Version=$(git describe --tags --always --dirty)"` set in `just build-release`. Exposed at `GET /api/v1/version` and surfaceable from the SPA footer.

## Cross-compile

Pure-Go SQLite (`modernc.org/sqlite`) means no cgo. `CGO_ENABLED=0` build (release mode) is portable across darwin/linux/windows × amd64/arm64.

## Data path

Default: `$XDG_DATA_HOME/tt/db.sqlite` or `~/.local/share/tt/db.sqlite`. Override with `--data-dir` or `--db`. See `internal/config/config.go:resolveDataDir`.

## Common dev gotchas

- After changing a sqlc query, you must run `just db-gen` or builds fail with a missing method on `*sqlcgen.Queries`.
- After changing a route file, `pnpm tsr generate` (auto-run by build) regenerates `routeTree.gen.ts`. If you forget, the dev server will still hot-reload via the plugin but a fresh `tsc -b` will fail.
- `internal/web/dist/.gitkeep` MUST exist or `//go:embed all:dist` fails the build on a clean checkout.
- `just clean` wipes `.dev-data/` — your local DB. Use deliberately.
