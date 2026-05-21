# Phase 01 — Project Bootstrap

> Read `00-index.md` first for repo conventions and commit discipline.

**Goal:** Establish the Go module, directory layout, justfile, basic `main.go`, and config package. Produces a binary that boots, parses CLI flags, logs a startup line, and exits.

**Dependencies:** None.

**Tech stack:** Go 1.22+, `flag` (stdlib), `slog` (stdlib), `just`. No external Go deps yet.

**Parallelizable with:** Nothing — runs first.

## File map

```
go.mod
justfile
README.md
.gitignore
cmd/tt/main.go
internal/config/config.go
internal/config/config_test.go
```

## Task 1: Initialize module + .gitignore

**Files:** `go.mod`, `.gitignore`

- [ ] Run `cd /Users/srliao/code/tt && go mod init github.com/srliao/tt`.
- [ ] Create `.gitignore` with: `/bin/`, `/.dev-data/`, `/web/node_modules/`, `/web/dist/`, `.DS_Store`, `.idea/`, `.vscode/`, `*.test`, `*.out`.
- [ ] Verify: `cat go.mod` shows `module github.com/srliao/tt`.
- [ ] Commit:
  ```bash
  git add go.mod .gitignore && git commit -m "chore: bootstrap go module"
  ```

## Task 2: README skeleton

**Files:** `README.md`

- [ ] Write a short README with: project tagline ("Local-only single-user task tracker with userscript automation"), Build section (`just build` then `./bin/tt`), and a Layout section pointing at `cmd/`, `internal/`, `web/`, `docs/superpowers/`.
- [ ] Commit:
  ```bash
  git add README.md && git commit -m "docs: add README skeleton"
  ```

## Task 3: Config package — failing test

**Files:** `internal/config/config_test.go`

- [ ] Create the test file with these test functions (each asserts on `config.Parse(args []string) (Config, error)`):
  - `TestParseDefaults` — `Parse([]string{})` returns no error, `Port==8080`, non-empty `DataDir` and `DBPath`.
  - `TestParseFlags` — `--port 9090 --data-dir /tmp/tt` → `Port==9090`, `DataDir=="/tmp/tt"`.
  - `TestParseDBOverride` — `--db /some/path/db.sqlite` → `DBPath=="/some/path/db.sqlite"`.
  - `TestParseInvalidPort` — `--port abc` → error is non-nil.
- [ ] Run `go test ./internal/config/...`. Expected: build failure (`undefined: config.Parse`).

## Task 4: Config package — implementation

**Files:** `internal/config/config.go`

- [ ] Implement `Config` (fields: `Port int`, `DataDir string`, `DBPath string`) and `Parse(args []string) (Config, error)`:
  - Use `flag.NewFlagSet("tt", flag.ContinueOnError)`.
  - Flags: `--port` (default `8080`), `--data-dir` (default empty → resolved), `--db` (default empty → resolved).
  - `DataDir` resolution: `$XDG_DATA_HOME/tt` if set, else `$HOME/.local/share/tt`, else `.tt-data`.
  - `DBPath` resolution: if empty, `filepath.Join(DataDir, "db.sqlite")`.
- [ ] Run `go test ./internal/config/... -v`. All four tests pass.
- [ ] Commit:
  ```bash
  git add internal/config/ && git commit -m "feat(config): add CLI flag parsing"
  ```

## Task 5: `cmd/tt/main.go` entry point

**Files:** `cmd/tt/main.go`

- [ ] Write a `main()` that:
  - Calls `config.Parse(os.Args[1:])`, exiting `2` on error.
  - Constructs `slog.New(slog.NewTextHandler(os.Stderr, nil))`.
  - Logs one info line `"starting tt"` with attrs `version`, `port`, `data_dir`, `db_path`.
  - Declares `var Version = "dev"` (overridden by `-ldflags` at build time).
  - Logs `"nothing to do yet; exiting"` and returns (HTTP server + scheduler wired in phase 09).
- [ ] Verify build: `go build ./...` → no output, exit 0.
- [ ] Verify run: `go run ./cmd/tt` emits both log lines, exits 0.
- [ ] Verify flag override: `go run ./cmd/tt --port 9999` shows `port=9999`.
- [ ] Commit:
  ```bash
  git add cmd/tt/main.go && git commit -m "feat(cmd): add main entry point with config wiring"
  ```

## Task 6: justfile

**Files:** `justfile`

- [ ] Write the justfile with these recipes — content is the artifact, copy verbatim:

```just
default:
    @just --list

# ── frontend ──────────────────────────────────────────────
fe-install:
    cd web && pnpm install

fe-dev:
    cd web && pnpm run dev

fe-build:
    cd web && pnpm run build

fe-test:
    cd web && pnpm run test

fe-lint:
    cd web && pnpm run lint

# ── backend ──────────────────────────────────────────────
be-dev:
    go run ./cmd/tt --port 8080 --data-dir ./.dev-data

be-test:
    go test ./...

be-lint:
    golangci-lint run ./...

# ── orchestration ────────────────────────────────────────
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'kill 0' EXIT
    just be-dev &
    just fe-dev &
    wait

build: fe-build
    go build -trimpath -ldflags='-s -w -X main.Version=dev' -o ./bin/tt ./cmd/tt

build-release: fe-build
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)' -o ./bin/tt ./cmd/tt

test: be-test fe-test

lint: be-lint fe-lint

clean:
    rm -rf web/dist web/node_modules ./bin ./.dev-data
```

- [ ] Verify: `just` (no args) lists recipes; `just be-test` runs the config tests successfully.
- [ ] Commit:
  ```bash
  git add justfile && git commit -m "chore: add justfile with dev/build recipes"
  ```

## Phase completion checklist

- [ ] `go build ./...` exits clean.
- [ ] `go test ./...` passes (config tests only).
- [ ] `go run ./cmd/tt` boots and logs cleanly.
- [ ] `just` lists recipes; `just be-test` passes.
- [ ] All commits land on `main` with conventional messages.
