# Remote Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Package `tt` as a multi-arch container image published to GHCR by CI, and deploy it to a remote host via `docker compose` with a Cloudflare Tunnel for ingress and Litestream for continuous SQLite backup to Cloudflare R2.

**Architecture:** One `docker compose` stack of two containers with no host ports published. The `tt` image bundles the `litestream` binary and runs it as the entrypoint supervisor (`litestream replicate -exec tt`), restoring the DB from R2 on boot then streaming the WAL back. A `cloudflared` container provides the only ingress via a token-based tunnel; Cloudflare Access gates identity at the edge. CI builds each arch natively on its own runner and merges the two digests into one manifest.

**Tech Stack:** Go 1.26 (pure-Go `modernc.org/sqlite`, `CGO_ENABLED=0`), pnpm 9 + Vite 8 + Node 22 (React SPA embedded via `//go:embed all:dist`), Docker multi-stage build on `alpine`, Litestream 0.3.x, GitHub Actions (`docker/build-push-action`, `docker/metadata-action`), Cloudflare Tunnel + R2.

## Global Constraints

Every task's requirements implicitly include this section. Exact values:

- **Backend build:** `CGO_ENABLED=0`, Go `1.26.1` (go.mod), flags `-trimpath -ldflags="-s -w -X main.Version=<version>"`, package `./cmd/tt`.
- **SPA embed:** built assets MUST land in `internal/web/dist/` before `go build`; the directive is `//go:embed all:dist` in `internal/web/assets.go`. A committed `dist/.gitkeep` must survive.
- **Frontend:** pnpm (lockfile `web/pnpm-lock.yaml` is `lockfileVersion 9.0` → pnpm 9), Node 22 (Vite 8 requires Node ≥20.19/22). Build command is `pnpm run build` (runs `tsr generate && tsc -b && vite build`); it regenerates the git-ignored `web/src/routeTree.gen.ts`.
- **Image name:** `ghcr.io/srliao/tt`. Target platforms: `linux/amd64` and `linux/arm64` (arm64 is the primary deploy target — Oracle free-tier ARM VM).
- **Runtime image:** `alpine` base with `ca-certificates` + `wget`. App listens on port `8080`. Data dir `/data`, DB at `/data/db.sqlite`.
- **Health endpoint (already exists):** `GET /api/v1/health` (`internal/httpapi/server.go:159`).
- **No host ports:** neither compose service publishes a `ports:` mapping. Cloudflare Tunnel is the only ingress.
- **Litestream env contract (used by both `docker/litestream.yml` and `docker-compose.yml`):** `LITESTREAM_ENDPOINT`, `LITESTREAM_BUCKET`, `LITESTREAM_PATH`, `LITESTREAM_REGION`, `LITESTREAM_ACCESS_KEY_ID`, `LITESTREAM_SECRET_ACCESS_KEY`. (Refines spec §6, which flagged the exact names as confirm-at-implementation.)
- **Local dev is unaffected** — `just dev` still runs `go run` directly; Litestream exists only in the container image. Out of scope to change.

---

## Task 1: Container image (Dockerfile, entrypoint, litestream config, .dockerignore)

**Files:**
- Create: `.dockerignore`
- Create: `docker/litestream.yml`
- Create: `docker/entrypoint.sh`
- Create: `Dockerfile`
- Verify: local `docker build` + runtime smoke of the health endpoint

**Interfaces:**
- Produces: a buildable image whose `ENTRYPOINT` is `/entrypoint.sh`; contains `/usr/local/bin/tt`, `/usr/local/bin/litestream`, and `/etc/litestream.yml`. Honors the Litestream env contract (Global Constraints). Runs `tt --data-dir /data --port 8080`.
- Consumes: nothing from other tasks.

- [ ] **Step 1: Create `.dockerignore`**

```
.git
.github
bin
.dev-data
web/node_modules
web/dist
internal/web/dist
docs
*.md
.DS_Store
*.test
*.out
```

Rationale: exclude local build outputs and the host `internal/web/dist` (the image rebuilds the SPA fresh). `.md` exclusion keeps context small; the build needs no docs.

- [ ] **Step 2: Create `docker/litestream.yml`**

```yaml
# Litestream config. All values are injected via environment variables at
# container start (Litestream expands ${VAR}). See docker-compose.yml / .env.
dbs:
  - path: /data/db.sqlite
    replicas:
      - type: s3
        endpoint: ${LITESTREAM_ENDPOINT}
        bucket: ${LITESTREAM_BUCKET}
        path: ${LITESTREAM_PATH}
        region: ${LITESTREAM_REGION}
        access-key-id: ${LITESTREAM_ACCESS_KEY_ID}
        secret-access-key: ${LITESTREAM_SECRET_ACCESS_KEY}
        force-path-style: true
```

`force-path-style: true` and an explicit `region` (set to `auto` for R2) are required for Cloudflare R2's S3 endpoint.

- [ ] **Step 3: Create `docker/entrypoint.sh`**

```sh
#!/bin/sh
set -e

# Restore the database from the replica if the local volume is empty.
# -if-db-not-exists  → no-op on normal restarts (DB already on the volume)
# -if-replica-exists → no-op on the very first boot (no backup yet)
litestream restore -if-db-not-exists -if-replica-exists -config /etc/litestream.yml /data/db.sqlite

# Replicate continuously while supervising the app. Litestream forwards
# signals and exits when tt exits.
exec litestream replicate -config /etc/litestream.yml -exec "tt --data-dir /data --port 8080"
```

- [ ] **Step 4: Create `Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1

# ── frontend: build the React SPA ────────────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /app/web
RUN npm install -g pnpm@9
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

# ── backend: embed SPA and compile the static binary ─────────────
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Stage the freshly built SPA into the //go:embed directory.
COPY --from=frontend /app/web/dist/ ./internal/web/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o /out/tt ./cmd/tt

# ── litestream binary source ─────────────────────────────────────
FROM litestream/litestream:0.3.13 AS litestream

# ── runtime ──────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY --from=backend    /out/tt                    /usr/local/bin/tt
COPY docker/litestream.yml /etc/litestream.yml
COPY docker/entrypoint.sh  /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
```

- [ ] **Step 5: Verify the Litestream base image tag and binary path**

Run: `docker run --rm --entrypoint litestream litestream/litestream:0.3.13 version`
Expected: prints a version like `v0.3.13`. If the tag 404s or the binary path differs, check https://github.com/benbjohnson/litestream releases / the image, pin the latest `0.3.x` tag, and update the `FROM litestream/litestream:` line and the `COPY --from=litestream` path accordingly.

- [ ] **Step 6: Build the image for the host arch**

Run: `docker build --build-arg VERSION=dev -t tt:dev .`
Expected: build completes; final stage produces `tt:dev`. If the `golang:1.26-alpine` tag is unavailable, use `golang:1.26.1-alpine`.

- [ ] **Step 7: Smoke-test the binaries in the image (no R2 needed)**

Run the app directly (bypassing the Litestream entrypoint, since no replica is configured locally) and hit the health endpoint:

```bash
docker run --rm --entrypoint sh tt:dev -c \
  'tt --data-dir /tmp/d --port 8080 & sleep 2; wget -qO- http://localhost:8080/api/v1/health; echo; kill %1'
```

Expected: prints the health JSON (e.g. `{"status":"ok",...}`) — confirms the SPA-embedded binary runs and serves for this arch. Also confirm Litestream is present:

Run: `docker run --rm --entrypoint litestream tt:dev version`
Expected: prints the Litestream version.

- [ ] **Step 8: Commit**

```bash
git add .dockerignore docker/litestream.yml docker/entrypoint.sh Dockerfile
git commit -m "feat(deploy): container image bundling tt + litestream supervisor"
```

---

## Task 2: CI workflow — multi-arch build & push to GHCR

**Files:**
- Create: `.github/workflows/build.yml`
- Verify: `actionlint` static check

**Interfaces:**
- Consumes: the `Dockerfile` from Task 1 (build context `.`).
- Produces: on push to `main`, the manifest `ghcr.io/srliao/tt:latest` and `ghcr.io/srliao/tt:sha-<short>`, each resolving to both `linux/amd64` and `linux/arm64`.

- [ ] **Step 1: Create `.github/workflows/build.yml`**

```yaml
name: build

on:
  push:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }} # srliao/tt (already lowercase)

jobs:
  build:
    strategy:
      fail-fast: false
      matrix:
        include:
          - runner: ubuntu-24.04
            platform: linux/amd64
          - runner: ubuntu-24.04-arm
            platform: linux/arm64
    runs-on: ${{ matrix.runner }}
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - name: Compute platform pair label
        id: prep
        run: echo "pair=${PLATFORM//\//-}" >> "$GITHUB_OUTPUT"
        env:
          PLATFORM: ${{ matrix.platform }}

      - uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Docker metadata (labels)
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}

      - name: Build and push by digest
        id: build
        uses: docker/build-push-action@v6
        with:
          context: .
          platforms: ${{ matrix.platform }}
          build-args: |
            VERSION=${{ github.sha }}
          labels: ${{ steps.meta.outputs.labels }}
          outputs: type=image,name=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }},push-by-digest=true,name-canonical=true,push=true

      - name: Export digest
        run: |
          mkdir -p /tmp/digests
          digest="${{ steps.build.outputs.digest }}"
          touch "/tmp/digests/${digest#sha256:}"

      - name: Upload digest
        uses: actions/upload-artifact@v4
        with:
          name: digests-${{ steps.prep.outputs.pair }}
          path: /tmp/digests/*
          if-no-files-found: error
          retention-days: 1

  merge:
    runs-on: ubuntu-24.04
    needs: build
    permissions:
      contents: read
      packages: write
    steps:
      - name: Download digests
        uses: actions/download-artifact@v4
        with:
          path: /tmp/digests
          pattern: digests-*
          merge-multiple: true

      - uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Docker metadata (tags)
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=raw,value=latest
            type=sha,format=short

      - name: Create manifest list and push
        working-directory: /tmp/digests
        run: |
          docker buildx imagetools create \
            $(jq -cr '.tags | map("-t " + .) | join(" ")' <<< "$DOCKER_METADATA_OUTPUT_JSON") \
            $(printf '${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@sha256:%s ' *)

      - name: Inspect manifest
        run: docker buildx imagetools inspect ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:latest
```

- [ ] **Step 2: Lint the workflow**

Run: `actionlint .github/workflows/build.yml`
(If not installed: `brew install actionlint`, or `docker run --rm -v "$PWD":/repo -w /repo rhysd/actionlint:latest -color`.)
Expected: no errors. Fix any reported issues (indentation, unknown keys) before committing.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/build.yml
git commit -m "ci: multi-arch (amd64+arm64) image build and push to GHCR"
```

- [ ] **Step 4: (Post-merge, manual) Confirm the published manifest**

After this lands on `main` and the run finishes, verify from any Docker host:
Run: `docker buildx imagetools inspect ghcr.io/srliao/tt:latest`
Expected: manifest lists both `linux/amd64` and `linux/arm64`. Note: the GHCR package may default to private; make it public or authenticate on the remote when pulling. Record this in the runbook (Task 4).

---

## Task 3: Deployment stack (docker-compose.yml, .env.example)

**Files:**
- Create: `docker-compose.yml`
- Create: `.env.example`
- Modify: `.gitignore` (ignore `.env`)
- Verify: `docker compose config` validates and exposes no host ports

**Interfaces:**
- Consumes: image `ghcr.io/srliao/tt:latest` (Task 2); the Litestream env contract and `/api/v1/health` (Task 1 / Global Constraints).
- Produces: the runnable stack the runbook (Task 4) references.

- [ ] **Step 1: Create `docker-compose.yml`**

```yaml
services:
  tt:
    image: ghcr.io/srliao/tt:latest
    restart: unless-stopped
    volumes:
      - tt-data:/data
    environment:
      LITESTREAM_ENDPOINT: ${LITESTREAM_ENDPOINT}
      LITESTREAM_BUCKET: ${LITESTREAM_BUCKET}
      LITESTREAM_PATH: ${LITESTREAM_PATH}
      LITESTREAM_REGION: ${LITESTREAM_REGION}
      LITESTREAM_ACCESS_KEY_ID: ${LITESTREAM_ACCESS_KEY_ID}
      LITESTREAM_SECRET_ACCESS_KEY: ${LITESTREAM_SECRET_ACCESS_KEY}
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/api/v1/health"]
      interval: 30s
      timeout: 3s
      retries: 3
      start_period: 10s

  cloudflared:
    image: cloudflare/cloudflared:latest
    restart: unless-stopped
    command: tunnel --no-autoupdate run
    environment:
      TUNNEL_TOKEN: ${TUNNEL_TOKEN}
    depends_on:
      tt:
        condition: service_healthy

volumes:
  tt-data:
```

Note: no `ports:` on either service — the tunnel is the only ingress.

- [ ] **Step 2: Create `.env.example`**

```bash
# ── Cloudflare Tunnel ────────────────────────────────────────────
# Zero Trust dashboard → Networks → Tunnels → create a tunnel → copy the
# token. Configure its public hostname to route to http://tt:8080, and add
# a Cloudflare Access application + identity policy over that hostname.
TUNNEL_TOKEN=

# ── Litestream → Cloudflare R2 (S3-compatible) ───────────────────
# Any S3-compatible backend works; values below are for Cloudflare R2.
LITESTREAM_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
LITESTREAM_BUCKET=<r2-bucket-name>
LITESTREAM_PATH=tt
LITESTREAM_REGION=auto
LITESTREAM_ACCESS_KEY_ID=<r2-access-key-id>
LITESTREAM_SECRET_ACCESS_KEY=<r2-secret-access-key>
```

- [ ] **Step 3: Ignore the real `.env`**

Add to `.gitignore` (append under the existing entries):

```
/.env
```

- [ ] **Step 4: Verify compose config validates and publishes no ports**

```bash
docker compose --env-file .env.example config > /tmp/tt-compose.rendered.yml
grep -q 'condition: service_healthy' /tmp/tt-compose.rendered.yml && echo "depends_on OK"
grep -q 'ports:' /tmp/tt-compose.rendered.yml && echo "FAIL: ports published" || echo "no host ports OK"
```

Expected: the `config` command succeeds (valid YAML/schema), prints `depends_on OK` and `no host ports OK`. No `published:`/`ports:` mapping should appear.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml .env.example .gitignore
git commit -m "feat(deploy): docker-compose stack with cloudflared tunnel + litestream"
```

---

## Task 4: Deployment runbook (docs/deployment.md)

**Files:**
- Create: `docs/deployment.md`

**Interfaces:**
- Consumes: everything above (image name, env contract, compose file names).
- Produces: the operator-facing runbook.

- [ ] **Step 1: Write `docs/deployment.md`**

Include these sections, each with concrete commands (no placeholders beyond user-supplied secret values):

1. **Prerequisites** — a remote host with Docker + Docker Compose (arm64 Oracle free-tier VM as the reference target); a Cloudflare account; an R2 bucket.
2. **Create the R2 bucket + S3 API token** — dashboard steps; note the endpoint form `https://<account-id>.r2.cloudflarestorage.com`, bucket name, region `auto`, and the access-key/secret pair.
3. **Create the Cloudflare Tunnel** — Zero Trust → Networks → Tunnels → create → copy the token; add a public hostname routing to `http://tt:8080` (service is the compose service name).
4. **Configure Cloudflare Access** — create an Access application over the tunnel hostname with an identity policy (email OTP / Google / GitHub). Explain this is the only thing preventing anonymous public access, since the app has no built-in auth.
5. **Pull access to the image** — either make the GHCR package public, or `echo <PAT> | docker login ghcr.io -u <user> --password-stdin` on the remote. (Reference Task 2 Step 4.)
6. **Deploy** — `git clone`/copy `docker-compose.yml` + `.env.example`, `cp .env.example .env`, fill secrets, `docker compose up -d`, then `docker compose ps` (both healthy) and `docker compose logs -f tt` (expect Litestream restore/replicate lines).
7. **Verify** — browse the tunnel hostname → hit the Access gate → reach the app.
8. **Disaster-recovery drill** — `docker compose down`, `docker volume rm <project>_tt-data`, `docker compose up -d`; confirm from logs that Litestream restores `db.sqlite` from R2 and prior data is present.
9. **Pinning a version** — deploy a specific immutable tag by setting `image: ghcr.io/srliao/tt:sha-<short>` instead of `:latest`.
10. **Updating** — `docker compose pull && docker compose up -d`.

- [ ] **Step 2: Verify referenced files exist**

```bash
for f in docker-compose.yml .env.example Dockerfile .github/workflows/build.yml; do
  test -e "$f" && echo "ok: $f" || echo "MISSING: $f"; done
```

Expected: all `ok:`.

- [ ] **Step 3: Commit**

```bash
git add docs/deployment.md
git commit -m "docs: remote deployment runbook"
```

---

## Task 5: Documentation sync (CLAUDE.md + docs/agent/) — MANDATORY per repo rule

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/agent/` (whichever files genuinely need it; e.g. architecture / commands / a new deployment topic)

**Interfaces:**
- Consumes: the full set of changes from Tasks 1–4.
- Produces: docs consistent with the new deployment surface.

- [ ] **Step 1: Dispatch the documentation subagent**

Use the Agent tool (`subagent_type: general-purpose`) with this prompt:

> Review and update documentation for the new remote-deployment surface (Dockerfile + docker/ entrypoint & litestream config, .github/workflows/build.yml, docker-compose.yml, .env.example, docs/deployment.md). Scope:
>   1. `docs/agent/` — the agent-development guide.
>   2. `/CLAUDE.md` — the project root memory file.
>
> Keep docs optimized for agent use: concise, navigation-first, focused on design patterns and where to find code rather than exact implementation. Each file stays self-contained and short.
>
> For `docs/agent/`: update only files that genuinely need it (e.g. architecture/process model gains a container deployment mode; a navigation/commands file gains the new files). Update the README index if you add or rename a file. Do NOT paste large code excerpts — link by file path + symbol.
>
> For `CLAUDE.md`: add a terse note on the deployment artifacts and the image build path (`Dockerfile` is separate from `just build`; CI builds multi-arch to `ghcr.io/srliao/tt`). Verify every command, file path, and rule still matches reality. Do NOT contradict the existing rule "Don't bypass `just build` when producing a binary" — clarify that the container image is the deployment path and the CI/Dockerfile performs the equivalent SPA-embed step. Keep it terse.
>
> Verify all file paths mentioned in either location still exist. Report what you changed and what you intentionally left alone.

- [ ] **Step 2: Verify no broken paths were introduced**

```bash
grep -rEoh '(docs/[A-Za-z0-9._/-]+\.md|docker/[A-Za-z0-9._/-]+|\.github/workflows/[A-Za-z0-9._/-]+)' CLAUDE.md docs/agent 2>/dev/null | sort -u | while read -r p; do test -e "$p" && echo "ok: $p" || echo "MISSING: $p"; done
```

Expected: no `MISSING:` lines.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md docs/agent
git commit -m "docs: sync agent guide + CLAUDE.md for remote deployment"
```

---

## Self-review notes

- **Spec coverage:** §2 topology → Tasks 1+3; §3 access control → Task 4 (Access setup) + no-auth callouts; §4 image → Task 1; §5 CI → Task 2; §6 compose/secrets → Task 3; §7 repo layout → Tasks 1–4; §8 docs → Task 4; §9 decisions → all; §10 success criteria → verification steps in Tasks 1–3 + Task 4 DR drill. Covered.
- **Env-name refinement:** the spec's illustrative `LITESTREAM_REPLICA_URL` is replaced by the discrete `LITESTREAM_ENDPOINT/BUCKET/PATH/REGION/*_KEY_*` set, used identically in `docker/litestream.yml` (Task 1) and `docker-compose.yml`/`.env.example` (Task 3). Consistent across tasks. Spec §6 explicitly permitted this.
- **Version pins to confirm at build time** (each has an explicit verify step): `litestream/litestream:0.3.13`, `golang:1.26-alpine` (fallback `1.26.1-alpine`), `node:22-alpine`, `alpine:3.20`, `pnpm@9`.
