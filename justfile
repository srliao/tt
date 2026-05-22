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
    #!/usr/bin/env bash
    set -euo pipefail
    # Stage the freshly built SPA into a directory the //go:embed directive
    # in internal/web/assets.go can reach. Restore the .gitkeep-only state
    # on exit so the working tree stays clean even if the build fails.
    find internal/web/dist -mindepth 1 -not -name .gitkeep -delete
    cp -R web/dist/. internal/web/dist/
    trap 'find internal/web/dist -mindepth 1 -not -name .gitkeep -delete' EXIT
    go build -trimpath -ldflags='-s -w -X main.Version=dev' -o ./bin/tt ./cmd/tt

build-release: fe-build
    #!/usr/bin/env bash
    set -euo pipefail
    find internal/web/dist -mindepth 1 -not -name .gitkeep -delete
    cp -R web/dist/. internal/web/dist/
    trap 'find internal/web/dist -mindepth 1 -not -name .gitkeep -delete' EXIT
    VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o ./bin/tt ./cmd/tt

test: be-test fe-test

lint: be-lint fe-lint

# ── db codegen ───────────────────────────────────────────
db-gen:
    cd internal/db && sqlc generate

clean:
    rm -rf web/dist web/node_modules ./bin ./.dev-data
    find internal/web/dist -mindepth 1 -not -name .gitkeep -delete 2>/dev/null || true
