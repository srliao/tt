# Phase 09 — Embed, Release Build, Smoke Test

> Read `00-index.md` first. Commit after each task.

**Goal:** Wire the built SPA (`web/dist/`) into the Go binary via `go:embed`, replace the phase 06 placeholder SPA handler with one that serves the embedded assets, finalize `just build` / `just build-release` recipes, and run the smoke test from `00-index.md`.

**Dependencies:** Every previous phase complete and passing tests.

**Tech stack:** Go `embed` package, `io/fs`.

**Parallelizable with:** Nothing — final phase.

## File map

```
internal/web/
├── assets.go               # //go:embed all:dist
└── dist/                   # ephemeral: populated by `just build`, removed after
internal/httpapi/spa.go     # replace placeholder with real embed handler
justfile                    # update build to copy/remove dist
```

## Embed strategy (resolves spec ambiguity)

Go's `//go:embed` cannot reference paths outside the package directory (no `../../`). To keep the source tree clean and avoid symlinks (which are awkward across OSes and CI), the build flow **copies** `web/dist/` into `internal/web/dist/` immediately before `go build`, then **removes** it after the binary is produced.

- `internal/web/dist/` is gitignored — it exists only during a build.
- The `//go:embed all:dist` directive matches `internal/web/dist/*` recursively while the directory is present.
- `go build` fails loudly if `internal/web/dist/` is missing — that's the correct behavior; it signals the build was invoked outside of `just build`.

## Task 1: Add ignore + scaffold the embed package

**Files:** `internal/web/assets.go`, `.gitignore`

- [ ] Append to `.gitignore`:
  ```
  /internal/web/dist/
  ```
- [ ] Write `internal/web/assets.go`:
  ```go
  package web

  import (
      "embed"
      "io/fs"
  )

  //go:embed all:dist
  var distFS embed.FS

  // Dist returns the SPA static files rooted at the dist directory (so paths
  // like "index.html" and "assets/foo.js" work).
  func Dist() (fs.FS, error) {
      return fs.Sub(distFS, "dist")
  }
  ```
- [ ] Create a placeholder `internal/web/dist/.gitkeep` is **not** needed — `//go:embed all:dist` will fail at build time unless `just build` populated the directory first. To make `go build ./...` succeed for downstream phase-09 development (sqlc / tests) without having run a full FE build, add a tiny no-op file the embed directive can grab when the real dist isn't present:
  - Create `internal/web/dist-empty/.placeholder` (a sibling directory, committed) holding the literal byte `0`.
  - In `assets.go`, prefer `dist` if it exists, else fall back to `dist-empty`. Implement via build tags is overkill — simpler: change directive to `//go:embed all:dist all:dist-empty` and have `Dist()` return `fs.Sub(distFS, "dist")` if that subtree has an `index.html`, else fall back to `fs.Sub(distFS, "dist-empty")` with a stub `index.html`.
  - **Simpler alternative (recommended):** require that all `go build` paths go through `just build`, which always populates `dist/` first. Skip the fallback. Document this in the README.

  Pick the simpler alternative. Add a short note in `internal/web/assets.go` doc comment:
  ```go
  // The dist subdirectory is populated by `just build` (which runs `just fe-build`
  // followed by `cp web/dist internal/web/dist`). Running `go build ./...`
  // outside of `just build` will fail at the //go:embed directive — that is
  // intentional and signals the wrong build flow.
  ```
- [ ] Commit:
  ```bash
  git add .gitignore internal/web/assets.go && \
    git commit -m "feat(web): scaffold go:embed of dist"
  ```

## Task 2: Update justfile to copy + remove

**Files:** `justfile`

- [ ] Replace the `build` and `build-release` recipes:
  ```just
  build: fe-build
      #!/usr/bin/env bash
      set -euo pipefail
      rm -rf internal/web/dist
      cp -R web/dist internal/web/dist
      trap 'rm -rf internal/web/dist' EXIT
      go build -trimpath -ldflags='-s -w -X main.Version=dev' -o ./bin/tt ./cmd/tt

  build-release: fe-build
      #!/usr/bin/env bash
      set -euo pipefail
      rm -rf internal/web/dist
      cp -R web/dist internal/web/dist
      trap 'rm -rf internal/web/dist' EXIT
      VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
      CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=$VERSION" -o ./bin/tt ./cmd/tt
  ```
  The `trap` ensures `internal/web/dist/` is removed even if `go build` fails.
- [ ] Verify:
  - `just fe-build` succeeds and produces `web/dist/`.
  - `just build` succeeds and produces `bin/tt`. **After** the build, `internal/web/dist/` no longer exists (trap fired).
  - The binary runs (next task).
- [ ] Commit:
  ```bash
  git add justfile && \
    git commit -m "chore(build): copy web/dist into internal/web/dist for go:embed"
  ```

## Task 3: Wire the real SPA handler

**Files:** `internal/httpapi/spa.go`

- [ ] Replace the placeholder implementation from phase 06 Task 10 with a real one that:
  - Accepts an `fs.FS` (the result of `web.Dist()`).
  - Serves `/` and any unmatched path → `index.html` (SPA fallback).
  - Serves `/assets/*` from the embedded FS with `Cache-Control: public, max-age=31536000, immutable`.
  - Serves `index.html` with `Cache-Control: no-cache`.
  - Returns 404 for nested asset paths that don't exist (do not fall back to `index.html` for `/assets/*`).
- [ ] Update `internal/httpapi/spa_test.go` to exercise this against a synthetic `fstest.MapFS` with `index.html` and `assets/app-abc.js`.
- [ ] Commit:
  ```bash
  git add internal/httpapi/spa.go internal/httpapi/spa_test.go && \
    git commit -m "feat(http): serve SPA from embedded dist with cache headers"
  ```

## Task 4: Wire the real handler into `cmd/tt/main.go`

**Files:** `cmd/tt/main.go`

- [ ] Replace the placeholder SPA handler injection (from phase 06 Task 11) with:
  ```go
  distFS, err := web.Dist()
  if err != nil { return fmt.Errorf("load embedded dist: %w", err) }
  spaHandler := httpapi.NewSPAHandler(distFS)
  server := httpapi.New(taskSvc, tagSvc, scriptSvc, enq, logger, version, spaHandler)
  ```
- [ ] Commit:
  ```bash
  git add cmd/tt/main.go && git commit -m "feat(cmd): serve embedded SPA in main"
  ```

## Task 5: Verify build artifacts

The justfile updates landed in Task 2. Sanity-check the full flow once more:

- [ ] `just clean` removes `web/dist`, `web/node_modules`, `bin/`, `.dev-data`. (Add `internal/web/dist` to the clean recipe if it isn't already covered.)
- [ ] `just fe-install && just build` from scratch produces `bin/tt`. After the build, `internal/web/dist/` is absent (trap removed it).
- [ ] `just build-release` succeeds with `CGO_ENABLED=0`. The resulting binary is statically linked. Confirm with `file bin/tt` on macOS or `ldd bin/tt` on Linux (should report "not a dynamic executable" on Linux).
- [ ] If you updated `just clean` to include the embed dir, commit:
  ```bash
  git add justfile && git commit -m "chore: include internal/web/dist in just clean"
  ```

## Task 6: Smoke test

This is the gate for v1 done — mirrors the `00-index.md` "After all phases complete" section.

- [ ] Run `just build` from a clean checkout. Confirm `bin/tt` exists.
- [ ] Run `./bin/tt --data-dir /tmp/tt-smoke --port 8080`. Server logs `starting tt`.
- [ ] Open `http://localhost:8080`:
  - SPA loads.
  - Navigate to `/tasks`. Page renders.
  - Click "Create your first task". Add `title: "smoke test"`. Confirm it appears in the table.
  - Stage it. Visit `/stage` — it appears.
  - Visit `/scripts/new`. Create a script with name `smoke`, schedule `every_tick` (check the confirm), code `ctx.queueTask({title: "from script", tags: []});`. Save.
  - Click "Run now". Confirm navigation to `/runs/$id`, status becomes `ok`, logs (if any) appear, "from script" task is linked under spawned tasks.
  - Navigate to `/tasks` — confirm "from script" task is present with the script's id under spawned_by_script_id (via API response).
- [ ] `Ctrl-C` shuts down the binary cleanly.
- [ ] No commit required (this is a manual verification step).

## Phase completion checklist (= project completion)

- [ ] `just build` produces `bin/tt`.
- [ ] `./bin/tt` serves both API and SPA on a single port.
- [ ] All five smoke-test steps from `00-index.md` pass.
- [ ] `go test ./...` clean.
- [ ] `cd web && pnpm run test && pnpm run lint && pnpm run build` clean.

v1 is done.
