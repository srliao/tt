# Remote Deployment — Design Spec

**Date:** 2026-08-12
**Status:** Approved for planning
**Scope:** Package `tt` as a multi-arch container image published by CI, and
deploy it to a remote host via `docker compose` with a Cloudflare Tunnel for
ingress and Litestream for continuous SQLite backup to Cloudflare R2. Primary
deployment target is an arm64 Oracle Cloud free-tier VM; amd64 is equally
supported.

---

## 1. Goals & non-goals

**Goals**
- One `docker compose up -d` brings up the full stack on a fresh remote.
- Multi-arch image (`linux/amd64` + `linux/arm64`) published to GHCR by CI.
- Public access via Cloudflare Tunnel, gated by Cloudflare Access identity.
- Continuous SQLite backup to R2 with automatic restore-on-boot (disaster
  recovery) via Litestream.
- Backend stays statically linked and cgo-free (already true).

**Non-goals**
- No application-level authentication is added; identity is enforced at the
  Cloudflare edge (see §3). App code is unchanged by this project.
- No changes to local dev workflow (`just dev` runs `go run` directly with no
  Litestream; unchanged and out of scope for this spec).
- No horizontal scaling / multi-instance. `tt` remains single-instance,
  single-writer SQLite.

---

## 2. Topology

Two containers on one `docker compose` stack. **No ports are published on the
host** — the VM's public interface stays closed; the Cloudflare Tunnel is the
only ingress.

```
Internet ──HTTPS──▶ Cloudflare edge ──[Access identity gate]──▶ Tunnel
                                                                  │
   Oracle ARM VM (docker compose)                                 │
   ┌──────────────────────────────────────────────────────┐      │
   │  cloudflared  ◀── TUNNEL_TOKEN                         │◀─────┘
   │      │ http://tt:8080 (compose network only)          │
   │      ▼                                                 │
   │  tt container                                          │
   │    litestream replicate -exec "tt --data-dir /data"   │──▶ Cloudflare R2
   │      restore-on-boot ▲   continuous WAL replication ───┘   (backups)
   │      volume: tt-data → /data/db.sqlite                 │
   └──────────────────────────────────────────────────────┘
```

- **tt container** — the CI-built image. `litestream` is bundled and runs as the
  entrypoint process, supervising `tt` via `litestream replicate -exec`. On boot
  it restores `db.sqlite` from R2 if the volume is empty, then runs the app while
  streaming the WAL to R2. Data lives on a named volume `tt-data` at
  `/data/db.sqlite`.
- **cloudflared container** — official `cloudflare/cloudflared` image,
  token-based (remotely-managed) tunnel. Routes the user's public hostname to
  `http://tt:8080` over the internal compose network. Depends on `tt` being
  healthy.

Rationale for bundling Litestream into the tt image (vs. a separate sidecar):
the supervisor pattern (`litestream replicate -exec`) gives correct
restore-before-app-start ordering with no init-container choreography, and is
the pattern Litestream officially recommends. Both `tt` and `litestream` are
static Go binaries, so the image stays small.

---

## 3. Access control

The app has no built-in auth (designed local-only, single-user). Public exposure
is gated by **Cloudflare Access (Zero Trust)**:

- An Access application is configured once in the Cloudflare dashboard over the
  tunnel hostname, with an identity policy (email OTP / Google / GitHub).
- Enforced at the Cloudflare edge — **no application code changes**.
- Free tier covers well beyond a single-user deployment.

This is a documented setup step (§8 runbook), not part of the compose file.

---

## 4. Container image (Dockerfile)

Multi-stage build producing one small image bundling both static binaries.
Because CI builds each arch **natively** (see §5), the Go build inside the
container compiles for its host arch — no `$TARGETARCH` binary-selection logic
is needed in the Dockerfile.

- **Stage 1 — frontend**: node + pnpm. `pnpm install && pnpm run build` →
  `web/dist`.
- **Stage 2 — backend**: golang. Stage the SPA into `internal/web/dist` (mirrors
  the `just build` embed step), then `CGO_ENABLED=0 go build` with
  `-trimpath -ldflags="-s -w -X main.Version=<sha>"` → `/out/tt`. Pure-Go
  `modernc.org/sqlite` means no cgo toolchain is required.
- **Litestream**: `COPY --from=litestream/litestream:<pinned>` the static
  binary into the runtime stage.
- **Runtime stage**: `alpine` (small, multi-arch, ships CA certs for R2 HTTPS,
  `/bin/sh` for the entrypoint, and `wget` for the healthcheck). Chosen over
  `distroless/static` specifically because the restore-then-replicate boot
  sequence needs a shell entrypoint.

**Entrypoint** (`docker/entrypoint.sh`):

```sh
#!/bin/sh
set -e
litestream restore -if-db-not-exists -if-replica-exists /data/db.sqlite
exec litestream replicate -exec "tt --data-dir /data --port 8080"
```

- `-if-db-not-exists` → restore is a no-op on normal restarts (volume already
  has data); it only pulls from R2 on a fresh machine.
- `-if-replica-exists` → does not fail on the very first boot when no backup
  exists yet.

**Litestream config** (`docker/litestream.yml`) is baked into the image but
fully env-driven (Litestream expands `${VAR}`), so the R2 endpoint / bucket /
path / credentials come from compose env. R2 is the default target; any
S3-compatible backend works by changing envs.

Supporting files: `.dockerignore` (exclude `.git`, `node_modules`, `.dev-data`,
`bin`, build caches).

---

## 5. CI workflow (`.github/workflows/build.yml`)

**Trigger:** push to `main`.
**Registry:** `ghcr.io/srliao/tt`, using the built-in `GITHUB_TOKEN` with
`packages: write` (no PAT).
**Tags:** short commit SHA + `latest` (computed via `docker/metadata-action`).

**Multi-arch strategy: native runner matrix.**

- **`build` job** — matrix:
  - `ubuntu-24.04` → `linux/amd64`
  - `ubuntu-24.04-arm` → `linux/arm64` (GitHub-hosted arm64 runner)

  Each job: checkout → `docker/setup-buildx-action` → login to GHCR →
  `docker/build-push-action` building **its single native platform**, pushed
  **by digest** (`outputs: type=image,push-by-digest=true`, `provenance: false`
  as needed). Litestream is `COPY --from`'d for the native arch. Each job
  exports its image digest as an artifact/output.

- **`merge` job** (`needs: build`) — combines the two per-arch digests into the
  final multi-arch manifest tags via `docker buildx imagetools create` using the
  tag list from `docker/metadata-action`.

Native builds avoid QEMU emulation entirely and keep the Dockerfile
arch-agnostic; the multi-arch assembly lives in the workflow.

---

## 6. Deployment artifacts

**`docker-compose.yml`** (repo root):

```yaml
services:
  tt:
    image: ghcr.io/srliao/tt:latest
    restart: unless-stopped
    volumes: [ tt-data:/data ]
    environment:
      LITESTREAM_REPLICA_URL:        ${LITESTREAM_REPLICA_URL}      # s3://bucket/path
      LITESTREAM_ENDPOINT:           ${LITESTREAM_ENDPOINT}         # https://<acct>.r2.cloudflarestorage.com
      LITESTREAM_ACCESS_KEY_ID:      ${LITESTREAM_ACCESS_KEY_ID}
      LITESTREAM_SECRET_ACCESS_KEY:  ${LITESTREAM_SECRET_ACCESS_KEY}
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/api/v1/health"]
      interval: 30s
      timeout: 3s
      retries: 3

  cloudflared:
    image: cloudflare/cloudflared:latest
    restart: unless-stopped
    command: tunnel --no-autoupdate run
    environment:
      TUNNEL_TOKEN: ${TUNNEL_TOKEN}
    depends_on:
      tt: { condition: service_healthy }

volumes:
  tt-data:
```

Notes:
- No `ports:` on either service — tunnel-only ingress.
- Health endpoint `GET /api/v1/health` already exists in the app
  (`internal/httpapi/server.go`).
- Exact Litestream env var names to be confirmed against the pinned Litestream
  version during implementation; the config file maps them regardless.

**`.env.example`** (committed) → user copies to `.env` (git-ignored):

| Var | Source |
|---|---|
| `TUNNEL_TOKEN` | Cloudflare Zero Trust → Tunnels → create tunnel → copy token; public hostname routes to `http://tt:8080` |
| `LITESTREAM_REPLICA_URL` | `s3://<r2-bucket>/tt` |
| `LITESTREAM_ENDPOINT` | `https://<account>.r2.cloudflarestorage.com` |
| `LITESTREAM_ACCESS_KEY_ID` / `LITESTREAM_SECRET_ACCESS_KEY` | R2 API token (S3 credentials) |

---

## 7. Repo layout (new files)

```
Dockerfile
.dockerignore
docker/entrypoint.sh
docker/litestream.yml
.github/workflows/build.yml
docker-compose.yml
.env.example
docs/deployment.md
```

Optional: a `just docker-build` recipe for a local image smoke test.

---

## 8. Documentation (`docs/deployment.md`)

End-to-end runbook:
1. Create R2 bucket + S3 API token.
2. Create Cloudflare Tunnel (token) + public hostname → `http://tt:8080`.
3. Configure a Cloudflare Access application + identity policy over the hostname.
4. Copy `.env.example` → `.env`, fill secrets.
5. `docker compose up -d`.
6. **Disaster-recovery drill**: destroy the `tt-data` volume, `up` again, confirm
   Litestream restores `db.sqlite` from R2 on boot.
7. Pinning: how to deploy a specific SHA tag instead of `latest`.

Per the repo's mandatory rule, a doc-sync pass updates `docs/agent/` and
`CLAUDE.md` (new commands, files, deployment topology, gotchas).

---

## 9. Load-bearing decisions (summary)

| Decision | Choice |
|---|---|
| Access control | Cloudflare Access (Zero Trust) at edge; no app auth |
| Backup target | Cloudflare R2 (env-parameterized S3; any provider works) |
| Litestream topology | Bundled in tt image, `replicate -exec` supervisor (restore-on-boot) |
| CI trigger | Push to `main` → GHCR, tags: short SHA + `latest` |
| Multi-arch build | Native runner matrix (amd64 + arm64) + manifest merge job |
| Runtime base image | `alpine` (shell + CA certs + wget) |
| Host exposure | None; Cloudflare Tunnel is the only ingress |

---

## 10. Success criteria

- Push to `main` produces a `ghcr.io/srliao/tt:latest` manifest resolving to both
  `linux/amd64` and `linux/arm64`.
- On a fresh arm64 Oracle VM: `docker compose up -d` with a filled `.env` brings
  the app up, reachable only through the Access-gated tunnel hostname.
- Writes to the app appear as Litestream snapshots/WAL in R2.
- Destroying the volume and re-`up`ing restores the prior DB state from R2.
- Local dev (`just dev`) is unaffected.
