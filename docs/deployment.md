# Deployment Runbook

This guide covers deploying `tt` to a remote host using Docker Compose,
Cloudflare Tunnel (for HTTPS ingress), and Cloudflare R2 (for SQLite backup
and restore via Litestream).

> **Security notice:** The app has **no built-in authentication**. The
> Cloudflare Access application described in [Section 4](#4-configure-cloudflare-access)
> is the **only thing** preventing anonymous public access to your data. Do not
> skip it or expose port 8080 directly.

---

## 1. Prerequisites

| Requirement | Notes |
|---|---|
| Remote host | arm64 or amd64 Linux VM with Docker Engine ≥ 24 and Docker Compose v2. The reference target is an Oracle Cloud free-tier Ampere (arm64) instance. |
| Cloudflare account | Free tier is sufficient. Zero Trust must be enabled (free plan available). |
| Cloudflare R2 | Enable R2 in the Cloudflare dashboard; the free tier includes 10 GB storage and 1 M Class-A operations/month. |

Install Docker on the remote host if needed:

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER   # then log out/in
docker compose version           # must show v2.x
```

---

## 2. Create the R2 Bucket and S3 API Token

**Create the bucket:**

1. Cloudflare dashboard → **R2 Object Storage** → **Create bucket**.
2. Name it (e.g. `tt-backup`). Region is always `auto` for R2; you cannot change it.
3. Note your **Account ID** from the R2 overview page — you need it for the endpoint URL.

**Create an S3-compatible API token:**

1. R2 overview → **Manage R2 API tokens** → **Create API token**.
2. Select **Object Read & Write** permissions, scoped to the bucket you just created.
3. Copy the **Access Key ID** and **Secret Access Key** — they are shown only once.

The values you will use:

```
LITESTREAM_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
LITESTREAM_BUCKET=tt-backup          # the bucket name you chose
LITESTREAM_PATH=tt                   # object-key prefix within the bucket
LITESTREAM_REGION=auto
LITESTREAM_ACCESS_KEY_ID=<paste-access-key-id>
LITESTREAM_SECRET_ACCESS_KEY=<paste-secret-access-key>
```

`LITESTREAM_PATH` is a key prefix, not a filename. Litestream will write
objects such as `tt/db.sqlite/generations/…` inside the bucket.

---

## 3. Create the Cloudflare Tunnel

1. Cloudflare dashboard → **Zero Trust** → **Networks** → **Tunnels** → **Create a tunnel**.
2. Choose **Cloudflared** as the connector type. Give the tunnel a name (e.g. `tt`).
3. Copy the **tunnel token** — you will put it in `TUNNEL_TOKEN`.
4. Under **Public Hostnames**, add a route:
   - **Subdomain / domain**: the hostname you want (e.g. `tt.example.com`).
   - **Service type**: `HTTP`
   - **URL**: `tt:8080`

   The URL is the Docker Compose service name (`tt`) and the port the Go
   binary listens on. No port is published on the host; traffic flows
   container-to-container inside the compose network.

5. Save the tunnel. Cloudflare will create a DNS record automatically.

---

## 4. Configure Cloudflare Access

> This step is **mandatory**. Without an Access policy the tunnel hostname
> is publicly reachable by anyone on the internet.

1. Zero Trust → **Access** → **Applications** → **Add an application**.
2. Choose **Self-hosted**.
3. Set the **Application domain** to the same hostname you configured in the
   tunnel (e.g. `tt.example.com`).
4. Under **Policies**, add a policy with an **Allow** action and an identity
   rule such as:
   - **Emails** → enter your own email address, **or**
   - **Email domain** → your domain, **or**
   - **GitHub** / **Google** identity provider (configure the IdP first under
     **Settings → Authentication**).
5. Leave **Session duration** at a sensible value (e.g. 24 hours).
6. Save the application.

From now on, any browser that hits the tunnel hostname will be redirected to
the Cloudflare Access login page before reaching the app.

---

## 5. Pull Access to the Image

The image is `ghcr.io/srliao/tt`. CI publishes two tags on every push to
`main`:
- `:latest` — always the newest build.
- `:sha-<short>` — immutable tag for the exact commit (e.g. `:sha-a1b1026`).

The manifest is multi-arch (linux/amd64 + linux/arm64); Docker picks the
right layer automatically.

**Option A — Make the GHCR package public (simplest):**

On GitHub: your profile → **Packages** → `tt` → **Package settings** →
**Change visibility** → **Public**. No authentication required on the remote
host.

**Option B — Authenticate with a Personal Access Token:**

On the remote host, create a GitHub PAT with the `read:packages` scope, then:

```bash
echo <your-pat> | docker login ghcr.io -u <github-username> --password-stdin
```

Docker stores credentials in `~/.docker/config.json`; subsequent `docker
compose pull` calls use them automatically.

---

## 6. Deploy

**Copy files to the remote host** (one-time setup):

```bash
# From your workstation — copy only the two files needed at runtime
scp docker-compose.yml .env.example user@remote-host:~/tt/
```

Or clone the repo and work from there:

```bash
ssh user@remote-host
git clone https://github.com/srliao/tt.git ~/tt
cd ~/tt
```

**Configure secrets:**

```bash
cd ~/tt
cp .env.example .env
nano .env      # or your preferred editor
```

Fill in every blank variable. The completed `.env` should look like:

```dotenv
TUNNEL_TOKEN=eyJ...                     # from Section 3
LITESTREAM_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
LITESTREAM_BUCKET=tt-backup
LITESTREAM_PATH=tt
LITESTREAM_REGION=auto
LITESTREAM_ACCESS_KEY_ID=abc123
LITESTREAM_SECRET_ACCESS_KEY=supersecret
```

**Start the stack:**

```bash
docker compose up -d
```

**Confirm both services are running and healthy:**

```bash
docker compose ps
```

Expected output (both services should show `healthy` / `running`):

```
NAME          IMAGE                          STATUS
tt-tt-1       ghcr.io/srliao/tt:latest      running (healthy)
tt-cloudflared-1  cloudflare/cloudflared:latest  running
```

Note: `cloudflared` depends on `tt` being healthy, so it starts only after
the health check passes.

**Tail the app logs** to confirm Litestream is replicating:

```bash
docker compose logs -f tt
```

On first start you should see Litestream lines similar to:

```
litestream: no existing database found, skipping restore
litestream: replicating to r2: tt-backup/tt
```

On subsequent starts after a restore you will see restore lines instead (see
Section 8).

---

## 7. Verify

1. Open the tunnel hostname in your browser (e.g. `https://tt.example.com`).
2. You should be redirected to the Cloudflare Access login page.
3. Authenticate with the identity you configured in Section 4.
4. After login you should reach the `tt` UI.

To verify the health endpoint directly from the remote host:

```bash
curl -sf http://localhost:8080/api/v1/health   # only works if you expose the port locally; normally use:
docker compose exec tt wget -qO- http://localhost:8080/api/v1/health
```

Expected response: `{"status":"ok"}` (or similar 200 JSON body).

---

## 8. Disaster-Recovery Drill

This drill verifies that Litestream can restore your database from R2 after
complete local data loss.

```bash
cd ~/tt

# 1. Stop and remove containers
docker compose down

# 2. Delete the local data volume (simulates disk loss)
# Docker Compose prefixes the volume with the project name, which defaults to
# the working directory's name. Confirm the exact name first:
#   docker volume ls
# e.g., if your directory is named "tt" the volume will be "tt_tt-data".
docker volume rm tt_tt-data

# 3. Bring the stack back up — Litestream restores from R2 before the app starts
docker compose up -d

# 4. Watch the restore in the logs
docker compose logs -f tt
```

A successful restore looks like:

```
litestream: restoring snapshot from r2: tt-backup/tt
litestream: restore complete, elapsed=...
```

Once the app is healthy, open the tunnel hostname and confirm your prior tasks
and tags are present.

> If no backup exists yet (e.g. immediately after first deploy before any
> writes), Litestream logs `no snapshot found` and starts with a fresh
> database — this is expected.

---

## 9. Pinning a Version

`:latest` is updated on every push to `main`. To lock the deployment to a
specific commit, use the immutable `sha-<short>` tag.

Find the short SHA for the commit you want:

```bash
# On your workstation
git log --oneline | head -10
```

Edit `docker-compose.yml` on the remote host:

```yaml
services:
  tt:
    image: ghcr.io/srliao/tt:sha-a1b1026   # replace with your target SHA
```

Then apply:

```bash
docker compose up -d
```

Docker will pull the pinned image if not already cached. To revert to
tracking `latest`, change the tag back and run `docker compose pull && docker
compose up -d`.

---

## 10. Updating

To pull the latest image and redeploy with zero downtime:

```bash
cd ~/tt
docker compose pull
docker compose up -d
```

`docker compose pull` fetches the new manifest and layers for both services.
`up -d` then recreates only the containers whose image digest changed.
Cloudflared stays up during the `tt` container restart; the brief gap
(typically < 5 s) will return a 502 from the tunnel, which is acceptable for
a single-user tool.

To update to a specific SHA instead of latest, see [Section 9](#9-pinning-a-version).
