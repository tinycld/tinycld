# One-click deploy configs

Configs for deploying TinyCld to managed hosting providers with a **persistent
volume** for the SQLite database. The user-facing page that surfaces these is
[tinycld.org/deploy](https://tinycld.org/deploy); this directory holds the
artifacts behind it.

## The one hard requirement

TinyCld writes its embedded SQLite database, file uploads, and server private
keys to **`/workspace/tinycld/pb_data`** inside the container. That directory
**must** be backed by a persistent volume. Hosts with an ephemeral filesystem
(Heroku, DigitalOcean App Platform, AWS App Runner, Cloudflare Containers, …)
silently destroy the database on every restart and are not supported — use a
host that offers a real block volume, or a plain VPS with the
[root `docker-compose.yml`](../docker-compose.yml).

## Proxy mode (shared by every managed host)

Managed hosts terminate TLS at their edge and forward plain HTTP to the
container. The entrypoint serves plain HTTP on `:7090` when autocert is off, so
every config sets:

| Env var | Value | Why |
| --- | --- | --- |
| `AUTOCERT_ENABLED` | `false` | The host owns TLS; the app must not request its own certs. |
| `PUBLIC_SCHEME` | `https` | The edge serves HTTPS, so the setup URL is `https://`. |
| `HTTP_ADDR` | `0.0.0.0:7090` | Bind the plain-HTTP listener the host routes to. |
| `TINYCLD_PUBLIC_URL` | the host-assigned URL | Makes the printed setup URL correct. Set after first deploy. |

The image is public and multi-arch: **`ghcr.io/tinycld/tinycld:latest`**.

## Providers

### Render — [`render.yaml`](./render.yaml)

Blueprint-driven. Button (point `?repo=` at this repo):

```
https://render.com/deploy?repo=https://github.com/tinycld/tinycld
```

A persistent **Disk** requires a **paid** instance — Render's free web services
can't attach a Disk and sleep after 15 min, which would lose the database.
Floor ≈ $7/mo (Starter) + ~$0.25/GB-mo for the Disk.

### Fly.io — [`fly.toml`](./fly.toml)

No web button — a CLI flow. From a copy of this directory (set `app` first):

```sh
fly launch --image ghcr.io/tinycld/tinycld:latest --no-deploy --copy-config
fly volumes create pb_data --size 1
fly deploy
```

Volumes + LiteFS (distributed SQLite) + Tigris (S3 uploads) make Fly the
strongest *storage* story; the cost is that it's CLI-only.

### Railway — template authored in the dashboard

Railway templates are **created in the Railway dashboard**, not from a repo
file. To (re)create the public template:

1. New project → **Deploy a Docker Image** → `ghcr.io/tinycld/tinycld:latest`.
2. Service → **Settings → Networking**: set the target port to **7090**
   (Railway provisions a public HTTPS domain that proxies to it).
3. Service → **Variables**: add `AUTOCERT_ENABLED=false`, `PUBLIC_SCHEME=https`,
   `HTTP_ADDR=0.0.0.0:7090`, and `TINYCLD_PUBLIC_URL` (the railway.app domain).
4. Service → **Volumes**: add a volume mounted at
   `/workspace/tinycld/pb_data`.
5. Project → **Settings → Create Template** → publish.

Record the resulting button URL here once published:

```
Railway template: https://railway.com/deploy/Aq36I8?referralCode=w9Vg_u&utm_medium=integration&utm_source=template&utm_campaign=generic 
```

Floor ≈ $5/mo (Hobby); the one-time $5 trial credit (and trial volumes) expire.

## Self-host / VPS

No managed config needed — use the canonical
[`docker-compose.yml`](../docker-compose.yml) at the repo root on any Linux box
with Docker (Hetzner CX-class ≈ €4/mo and up). That file is the source of truth
for the self-host path and the
[installation guide](https://tinycld.org/docs/installation).

### Without Docker (bare-metal systemd)

Prefer a single long-lived box with **no container runtime**? See
[`bare-metal/`](./bare-metal/README.md) — a `systemd` service built from source on
the host, with the app terminating TLS itself via autocert (no reverse proxy):

```sh
curl -fsSL https://raw.githubusercontent.com/tinycld/tinycld/main/deploy/bare-metal/install.sh \
  | DOMAIN=tinycld.example.com bash
```

## Optional: offload file uploads to S3

The SQLite database always needs the volume above, but **file uploads** can be
offloaded to any S3-compatible bucket (AWS S3, Cloudflare R2, Backblaze B2,
Hetzner Object Storage) via PocketBase's built-in S3 settings — configured in
the admin dashboard (Settings → Files storage), not via these deploy configs.
