# tt

A local-only, single-user task tracker with JavaScript automation. One
statically-linked Go binary with the React UI embedded — no Node runtime,
no database server. Runs on your machine, or self-hosted in a container
(see [Deployment](#deployment)).

## Why

- **Recurring tasks as code.** "Pay rent on the 1st", "follow up 3 days
  after the last one" — instead of a fixed dropdown of recurrence rules,
  you write short JavaScript snippets against a sandboxed `ctx` API, with
  real conditionals, persistent per-script state and date math.
- **A focus list.** The **main list** holds everything you've captured;
  the **stage** is a small, reorderable list (soft cap 7) of what you're
  working on right now.

Your data is one SQLite file. No accounts, no sync, no telemetry.

## Install and run

Needs [Go](https://go.dev) 1.26+, [Node](https://nodejs.org) 22+ with
[pnpm](https://pnpm.io), and [just](https://github.com/casey/just).

```sh
git clone https://github.com/srliao/tt.git && cd tt
just build            # builds the SPA, embeds it, writes ./bin/tt
./bin/tt              # http://localhost:8080
```

| Flag | Default | |
|---|---|---|
| `--port` | `8080` | |
| `--data-dir` | `$XDG_DATA_HOME/tt` (`~/.local/share/tt`) | holds `db.sqlite`; created and migrated on first run |
| `--db` | `<data-dir>/db.sqlite` | override the database path |
| `--timezone` | `$TT_TIMEZONE` → `$TZ` → `UTC` | see [Time zone](#time-zone) |

Back up or move your data by copying `db.sqlite` while `tt` is stopped
(it runs in WAL mode). `just build-release` stamps the version from
`git describe`.

`just build-cli` writes `./bin/tt-cli`, the agent-facing CLI (or
`go install github.com/srliao/tt/cmd/tt-cli@latest`). Point it at a server
with `TT_URL=https://tt.example.com` or `url` in
`~/.config/tt-cli/config.toml`, then run `tt-cli doctor`.

## Development

```sh
just dev          # vite (5173) + go run (8080); TT_TIMEZONE defaults to the host zone
just test         # backend + frontend tests
just lint         # golangci-lint + biome
just db-gen       # regenerate sqlc after editing internal/db/queries/*.sql
```

Dev data lives in `./.dev-data/db.sqlite`; delete it to reset.

## Userscripts

Scripts run on a schedule (or via "Run now") and spawn tasks. The script
editor has the `ctx` reference built in.

```js
// Schedule: daily. Spawn stand-up notes on weekdays.
const wd = ctx.weekday();
if (wd !== "saturday" && wd !== "sunday") {
  ctx.queueTask({
    title:    `Stand-up — ${ctx.today()}`,
    tags:     ["work", "standup"],
    due_date: ctx.today(),
  });
}
```

### Execution model

- One worker runs scripts sequentially, each in a fresh
  [goja](https://github.com/dop251/goja) runtime with a 5-second hard
  timeout. No shared state between runs except `ctx.state`.
- No `setTimeout`, `setInterval`, `fetch`, `require`, filesystem, network
  or top-level `await`. The sandbox stops accidental hangs; it does not
  defend against hostile code — only run scripts you wrote or read.
- A `running` row left behind by a crash is marked `error` on startup.
  The 500 most recent runs (and their logs) are kept.

### Effect persistence

| Effect | Persisted |
|---|---|
| `ctx.log.*`, `console.*` | immediately, even on error/timeout |
| `ctx.queueTask(...)` | only if the run succeeds |
| `ctx.state.set/delete` | only if the run succeeds, atomically |
| script `last_run_at` | always, so failures don't retry in a tight loop |

A script that queues two tasks then throws creates **zero** tasks and
saves no state.

### Schedule kinds

| Kind | Due |
|---|---|
| `every_tick` | every 15 minutes (the scheduler tick) |
| `daily` | once per calendar day |
| `weekly` | `{ "weekday": "monday" }` … `"sunday"` |
| `monthly` | `{ "day": 1..31 }` or `{ "day": "last" }` |

Daily/weekly/monthly scripts run at most once per calendar day — no de-dup
needed in the script. The scheduler also sweeps on startup, so a script
due today that hasn't run yet fires right away; missed earlier days are
not backfilled.

### Time zone

One IANA zone (`--timezone`, else `TT_TIMEZONE`, else `TZ`, else UTC)
decides when a day starts, both for schedule matching and for the `ctx`
date helpers, so a `daily` script's `ctx.today()` always matches the day
it was scheduled for. Timestamps `ctx` hands you (`ctx.script.lastRunAt`,
`created_at`/`completed_at`/`cancelled_at` on spawns) are wall-clock
strings in that zone. Storage stays UTC.

A misspelled zone fails startup (`unknown time zone "..."` in the logs).
The running value is at `GET /api/v1/version`. The web UI uses your
**browser's** zone for "today", so set the server zone to where you live.

### `ctx` API

```js
// Dates (inputs: Date, RFC3339, "YYYY-MM-DD HH:MM:SS", or "YYYY-MM-DD")
ctx.now()                      // Date — current instant
ctx.today()                    // "YYYY-MM-DD" in the app zone
ctx.weekday()                  // "monday".."sunday"
ctx.dayOfMonth() / ctx.month() / ctx.year()
ctx.isFirstOfMonth(date?)      // about today, or a given date
ctx.isLastOfMonth(date?)
ctx.isWeekday("monday")
ctx.daysSince(date)            // int, negative if future
ctx.daysBetween(a, b)
ctx.addDays(date, n)           // Date
ctx.formatDate(date)           // "YYYY-MM-DD"
ctx.parseDate("YYYY-MM-DD")    // Date

// This script
ctx.script.id / .name
ctx.script.trigger             // "scheduled" | "manual"
ctx.script.lastRunAt           // "YYYY-MM-DD HH:MM:SS" | null

// Tasks created by the last successful run, in queueTask order
ctx.lastSpawns                 // Task[]
ctx.lastSpawn                  // last element, or null
// Task: { id, title, notes, state, due_date, created_at,
//         completed_at, cancelled_at, tags: [...] }

// Per-script persistent JSON state (writes buffered until success)
ctx.state.get(key) / .set(key, value) / .delete(key) / .all()

// Logging, shown on the run detail page
ctx.log(msg) / ctx.log.debug|info|warn|error(msg)
console.log|info|warn|error(msg)

// Spawn a task (validated now, created only if the run succeeds)
ctx.queueTask({ title, notes?, tags?, due_date? })
```

Unknown tags are created automatically. New tasks land at the top of the
list; a batch keeps its `queueTask` order top-down.

## Deployment

The container image `ghcr.io/srliao/tt` (linux/amd64 + linux/arm64) runs
`tt` under [Litestream](https://litestream.io), which replicates the
database to Cloudflare R2. `docker-compose.yml` pairs it with a
`cloudflared` sidecar for HTTPS ingress — no host port is published.

> **tt has no authentication.** The Cloudflare Access policy below is the
> only thing keeping your data private. Don't skip it or expose port 8080.

CI pushes `:latest` and an immutable `:sha-<short>` on every push to
`main`.

### Environment

Copy `.env.example` to `.env` and fill it in.

| Variable | |
|---|---|
| `TUNNEL_TOKEN` | Cloudflare Tunnel token |
| `LITESTREAM_ENDPOINT` | `https://<account-id>.r2.cloudflarestorage.com` |
| `LITESTREAM_BUCKET` | R2 bucket name |
| `LITESTREAM_PATH` | key prefix inside the bucket, e.g. `tt` |
| `LITESTREAM_REGION` | `auto` for R2 |
| `LITESTREAM_ACCESS_KEY_ID`, `LITESTREAM_SECRET_ACCESS_KEY` | R2 API token |
| `TT_TIMEZONE` | optional IANA zone, default `UTC` — see [Time zone](#time-zone) |

### Cloudflare setup

On a free Cloudflare account, enable R2 and Zero Trust first.

1. **R2:** create a bucket, then *Manage R2 API tokens* → token with
   **Object Read & Write** on that bucket. Note the account ID for the
   endpoint; the secret is shown only once.
2. **Tunnel:** Zero Trust → Networks → Tunnels → create a Cloudflared
   tunnel and copy its token. Add a public hostname (e.g.
   `tt.example.com`) routing to service `HTTP` / `tt:8080`.
3. **Access:** Zero Trust → Access → Applications → add a **Self-hosted**
   app on the same hostname with an **Allow** policy for your email (or
   domain / IdP).
4. **Image access:** make the GHCR package public, or on the host run
   `docker login ghcr.io` with a PAT that has `read:packages`.

### Run

```sh
# on the host (Docker Engine ≥ 24, Compose v2), in a clone of this repo
# (or a dir holding docker-compose.yml + .env)
docker compose up -d
docker compose ps                    # tt healthy, cloudflared running
docker compose exec tt wget -qO- http://localhost:8080/api/v1/version
```

On start the entrypoint restores `db.sqlite` from R2 if the volume is
empty and a replica exists, then replicates continuously. Browse to the
hostname; you should hit the Access login first.

### Disaster-recovery drill

```sh
docker compose down
docker volume ls                     # find the data volume, e.g. tt_tt-data
docker volume rm tt_tt-data
docker compose up -d
docker compose logs -f tt            # expect "restoring snapshot ... restore complete"
```

Then confirm your tasks are back. With no backup yet, Litestream skips
the restore and starts fresh.

### Updating and pinning

```sh
docker compose pull && docker compose up -d     # move to the newest :latest
```

To pin, set `image: ghcr.io/srliao/tt:sha-<short>` in
`docker-compose.yml` and run `docker compose up -d`. Changing
`TT_TIMEZONE` also takes effect on `docker compose up -d`.

## License

[MIT](LICENSE).
