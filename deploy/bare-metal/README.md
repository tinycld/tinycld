# Bare-metal install (Debian/Ubuntu + systemd, no Docker)

Run TinyCld on a plain VPS as a native **systemd** service, built from source on
the box — no Docker, no Dokku, no reverse proxy. The app terminates TLS itself
via autocert (Let's Encrypt) and binds `:80`/`:443` directly.

> **These scripts are an example, not a turnkey installer.** `install.sh` and
> `build.sh` assume a Debian/Ubuntu host on x86-64 and that TinyCld owns
> `:80`/`:443`. Most real deployments need edits — a different distro or package
> manager, an ARM box, or running behind an existing reverse proxy / load
> balancer. Read them before you run them and adapt them to your setup; they're
> meant as a clear starting point you own.

This is an alternative to the Docker paths ([`docker-compose.yml`](../../docker-compose.yml)
for a VPS, or the managed-host configs in [`../`](../README.md)). Use it when you
want a single long-lived box with no container runtime.

## Quick start

On a fresh Debian 12/13 or Ubuntu 22.04+ host, as root:

```sh
curl -fsSL https://raw.githubusercontent.com/tinycld/tinycld/main/deploy/bare-metal/install.sh \
  | DOMAIN=tinycld.example.com bash
```

Or clone the repo and run it locally:

```sh
git clone https://github.com/tinycld/tinycld
DOMAIN=tinycld.example.com ./tinycld/deploy/bare-metal/install.sh
```

That installs the toolchain, creates a `tinycld` service user, builds the app
from `main`, writes a systemd unit, and starts it. Once autocert finishes,
`https://tinycld.example.com` is live. Follow along with
`journalctl -u tinycld -f`.

## Requirements

- **A public domain** whose A record points at this host, with **:80 and :443
  reachable** (autocert uses them to issue + serve the cert). Add **:465/:993**
  if you enable the mail listeners. Behind NAT, forward those ports.
- ~2 vCPU / 2 GB RAM and ~20 GB disk. The build (and the in-app package installer)
  run `expo export` + `go build` on the box, which is memory-hungry — a small box
  may need swap.
- Debian/Ubuntu with `apt` and `systemd`. (Other distros: adapt the apt + Go/Node
  install in `install.sh`.)

## The two scripts

| Script | Role |
| --- | --- |
| [`install.sh`](./install.sh) | One-shot installer **and** upgrade path: toolchain, service user, state dirs, low-port sysctl, env file, build, systemd unit, start. Idempotent. |
| [`build.sh`](./build.sh) | The from-source build only: `bootstrap --assemble-only` → `pnpm install` → `packages:generate` → `expo export` → `go build`, baked to `/opt/tinycld-baked`, plus installing the entrypoint and clearing the stale seeded build. Called by `install.sh`; also runnable on its own to rebuild. |

## Configuration

All via environment variables passed to `install.sh`:

| Var | Default | Meaning |
| --- | --- | --- |
| `DOMAIN` | **required** | Canonical domain; autocert certs + binds it. |
| `ADDITIONAL_DOMAINS` | — | Comma-separated extra cert domains. |
| `TINYCLD_VERSION` | `main` | Git ref/tag to build (the shell + every feature). |
| `TINYCLD_FEATURES` | full set | Space-separated feature members to include. |
| `SENTRY_DSN` | — | Enables Sentry on **both** the Go server (runtime) and the web bundle (inlined at build time). |
| `ENV_EXTRA` | — | Newline-separated `KEY=VALUE` lines appended to the service env file — e.g. `MAIL_PROVIDER`, `POSTMARK_SERVER_TOKEN`. |

Example with mail + Sentry:

```sh
DOMAIN=tinycld.example.com \
SENTRY_DSN="https://…@…/…" \
ENV_EXTRA=$'MAIL_PROVIDER=postmark\nPOSTMARK_SERVER_TOKEN=…' \
  ./deploy/bare-metal/install.sh
```

## How it runs

```
/opt/tinycld-baked/        pristine workspace baked by build.sh (the binary +
                           web bundle + node_modules + feature siblings)
/opt/tinycld-entrypoint.sh the app's own config/entrypoint.sh (the supervisor)
/workspace/                state root: pb_data/ (the SQLite DB — back this up!),
                           releases/, builds/, current -> builds/<id>/tinycld
/etc/tinycld/tinycld.env   root-only secrets/config, read by the unit
/etc/systemd/system/tinycld.service
```

The systemd unit runs **`/opt/tinycld-entrypoint.sh`**, not the binary directly —
the entrypoint is the supervisor (first-boot seed, web-release promotion, and the
in-app package installer's exit-75 → health-probe → rollback loop). systemd just
keeps it alive. It starts as root to fix state-dir ownership, then drops to the
unprivileged `tinycld` user via `gosu`.

## Updating

Re-run the installer (or `build.sh`) — it rebuilds from `TINYCLD_VERSION` and
restarts:

```sh
DOMAIN=tinycld.example.com TINYCLD_VERSION=v0.0.5 ./deploy/bare-metal/install.sh
```

`build.sh` clears the previous seeded build so the new bake is what gets served
(the entrypoint otherwise reuses the existing `/workspace/current`). `pb_data` is
never touched by a rebuild.

> The in-app package installer (the setup dashboard's "install package" flow)
> rebuilds in place independently of this — it `go build`s a fresh tree under
> `/workspace/builds/<id>/` and flips `current`. That path is unaffected by these
> scripts.

## Notes / gotchas

- **Privileged ports without root:** the app drops to an unprivileged user but
  binds `:80/:443/:465/:993`. `install.sh` sets
  `net.ipv4.ip_unprivileged_port_start=80` (what Docker effectively does with its
  default of `0`). A `CAP_NET_BIND_SERVICE` approach does **not** work here —
  `gosu` clears the ambient set and the entrypoint's per-boot `chown` strips file
  caps — so the sysctl is the reliable mechanism.
- **Host toolchain is required, not optional:** the in-app installer runs
  `pnpm install` + `go build` on the host, so Node, pnpm, Go, and a C toolchain
  must stay installed even after the initial build.
- **Back up `/workspace/pb_data`** — it holds the SQLite DB, uploads, and the
  server's private keys.
