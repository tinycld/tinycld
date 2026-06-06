# Live Package Install & Relaunch

How the in-app package installer fetches, builds, and activates a feature
package **inside a running production container** — and how the container
relaunches itself onto the rebuilt server + web bundle without external
orchestration.

This is the operator/agent-facing reference for the mechanics. For the package
*format* (manifests, the generator, route re-exports) see
[`packages.md`](./packages.md).

> **Scope.** This describes the runtime install pipeline driven from the setup
> dashboard (`POST /api/admin/packages/install`), implemented in
> `core/server/coreserver/pkg_install.go` + `pkg_go_build.go` +
> `pkg_restart.go`, and the relaunch handled by `app/config/entrypoint.sh`.
> It does NOT cover the build-time bundling done by `app/Dockerfile` (that
> assembles the *initial* image; the live installer adds packages to an
> already-running one).

## Contents

- [The big picture](#the-big-picture)
- [Entry points](#entry-points)
- [The install pipeline, stage by stage](#the-install-pipeline-stage-by-stage)
- [How relaunch works](#how-relaunch-works)
- [Rollback](#rollback)
- [Runtime image requirements](#runtime-image-requirements)
- [Uninstall](#uninstall)
- [Observability & troubleshooting](#observability--troubleshooting)

## The big picture

A TinyCld image ships with a set of **bundled** packages baked in at build time.
The live installer lets a superuser add *more* packages to a running container —
including third-party packages with their own Go server code — entirely
in-place:

1. The package source is fetched (npm registry **or** a git spec) and copied
   into the workspace as a new member.
2. The workspace is re-linked (`pnpm install`) and the generator re-runs,
   materializing the new package's routes, config, migrations, and Go wiring.
3. If the package ships a Go server, a **new server binary is compiled** from
   the now-larger workspace and swapped in (with a DB backup first).
4. The web bundle is rebuilt (`expo export`) and staged.
5. The running server **exits with code 75**, and `entrypoint.sh` catches that,
   health-checks the new binary, and **restarts the server in place** onto the
   new binary + promoted web bundle.

The whole thing runs as the unprivileged `tinycld` user inside the container.
The workspace root is `/workspace` (the binary lives at
`/workspace/app/tinycld`, so the installer derives `wsRoot = /workspace`); new
members land at `/workspace/<slug>`.

## Entry points

The installer is registered in `pkg_install.go::RegisterPackageInstallEndpoints`
under `/api/admin/packages` (all require superuser auth):

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/install` | Body `{ "npmPackage": "<spec>" }` — start an install job. Returns `202 { "jobId" }`. |
| `POST` | `/uninstall` | Body `{ "slug": "<slug>" }` — start an uninstall job. |
| `GET` | `/events/{jobId}` | Server-Sent Events stream of progress for a job (`progress` + `complete` events). Auth via header or `?token=` (EventSource can't send headers). |
| `GET` | `/status/{slug}` | Last recorded install-log status for a slug. |

Only **one** install/uninstall job runs at a time; a second request while one is
in flight returns `409` with the current job's info.

The setup dashboard's **Packages → Install** form (`PackageManager.tsx`) POSTs
the spec and then opens an SSE connection to render `InstallProgressModal`.

### Accepted specs

`<spec>` is whatever `npm pack` understands, validated by
`validatePackageSpec`:

- a bare npm name — `@tinycld/mail`, `mail`, `mail@1.2.3`
- a git spec — `github:owner/repo`, `gitlab:owner/repo`,
  `bitbucket:owner/repo`, the `owner/repo` shorthand, `git+https://…`,
  `git+ssh://…`, `https://….git`

Specs are rejected if they start with `-` (flag injection) or contain shell
metacharacters/whitespace. Anything outside the `@tinycld/` npm scope (every
git spec included) is flagged **untrusted** and surfaces a "proceed with
caution" warning — install only packages you trust, since installing one
compiles and runs its server code.

## The install pipeline, stage by stage

A background goroutine (`runInstallPipeline`) drives the install and emits an
SSE `progress` event at each step. The percentages double as a legible failure
map — a hang or error reports the last stage reached. Each step also logs to
stdout (visible in `docker logs`).

| % | Stage | What happens |
| --- | --- | --- |
| 5 | Validating package name | `validatePackageSpec(spec)` |
| 8 | Security warning | Emitted only for non-`@tinycld/` specs |
| 15 | Downloading package | `npm pack <spec>` into a temp dir (clones via `git` for git specs) |
| 20–30 | Parsing manifest | Untar the `package/`-prefixed tarball, parse `manifest.ts` |
| 33–35 | Validating manifest | Required fields, slug shape, server prereqs (see below) |
| 38–40 | Installing files | `cp -a` the extracted source to `/workspace/<slug>` |
| 43–45 | Updating workspace | Add the member to `package.json` + `pnpm-workspace.yaml` |
| 50–55 | Installing dependencies | `CI=true pnpm install --no-frozen-lockfile` at `/workspace` |
| 60–65 | Generating wiring | `npx tsx scripts/generate.ts` — routes, config, migration symlinks, Go `go.work` |
| 67 | Updating Go modules | `go work sync` (server packages only) |
| 70 | Building server | `CGO_ENABLED=1 go build -o tinycld.new .` (server packages only) |
| 73 | Validating binary | Run `tinycld.new --help` to confirm the build produced a working executable (the real boot health-check happens later, at relaunch) |
| 75 | Backing up database | `sqlite3 <db> "VACUUM INTO 'data.db.backup'"` |
| 77 | Swapping binary | `tinycld → tinycld.prev`, `tinycld.new → tinycld` (atomic renames) |
| 80–83 | Running migrations | `<binary> migrate` — applies the package's `pb-migrations` |
| 85–88 | Building web app | `npx expo export --platform web` |
| 90–92 | Staging release | Move `dist/` → `release-staging/<id>/`, rename `index.html` → `app.html` |
| 95–97 | Updating database | Upsert the `pkg_registry` record (status `installed`) |
| 99 | Requesting restart | `os.Exit(75)` — hands off to the entrypoint (see next section) |

### The server-package prerequisite gate

A package that declares a `server` in its manifest can only be installed if the
runtime has a Go toolchain: `validateManifest` calls `checkGoBuildPrereqs`,
which requires both `go` **and** a C compiler on `PATH`. If either is missing,
the install fails at the **Validating manifest** step with *"package … has
server components which require Phase 3 support"*. Pure-frontend packages skip
the Go build / binary-swap steps entirely.

### Migrations apply through a shared directory

The generator (re-run at the **Generating wiring** step) symlinks the new
package's `pb-migrations` into `/workspace/app/server/pb_migrations`. The
runtime jsvm plugin reads `/workspace/app/pb_migrations`, which the image makes
a **symlink** to `server/pb_migrations` — so build-time (bundled), runtime, and
installer-added migrations all share one directory and the **Running
migrations** step actually applies the new package's migrations.

## How relaunch works

The installer never restarts the container from the outside. It signals the
running server to exit with a sentinel code, and the entrypoint's supervisor
loop does the relaunch.

### 1. The server signals (exit 75)

`requestRestart` (`pkg_restart.go`) writes a `pb_data/.restart-requested`
marker and calls `os.Exit(75)`. In dev mode it logs and returns instead (you
restart manually).

### 2. The entrypoint catches it

`entrypoint.sh` runs the server in a supervisor loop:

```sh
while true; do
    EXIT_CODE=0
    run_tinycld serve "$@" || EXIT_CODE=$?      # gosu → unprivileged tinycld
    if [ $EXIT_CODE -eq 75 ]; then
        # ... health-check + restart (below) ...
        continue
    fi
    exit $EXIT_CODE                              # any other code = real exit
done
```

The `|| EXIT_CODE=$?` is load-bearing: the script runs under `set -e`, so a
bare `run_tinycld serve` would let the shell abort on the non-zero 75 *before*
the restart logic — the container would just exit 75 instead of relaunching.

### 3. Health-check the new binary

Before committing to the swap, the entrypoint boots the **new** binary on a
throwaway port (`127.0.0.1:19876`) and polls `/api/health` for up to 10s:

```sh
(
    export IMAP_ENABLED=false SMTP_ENABLED=false
    run_tinycld serve --http=127.0.0.1:19876
) &
HEALTH_PID=$!
```

`IMAP_ENABLED=false SMTP_ENABLED=false` are essential: a full `serve` also binds
the mail package's fixed IMAP `:993` / SMTP `:465` ports. Without disabling
them, the probe holds those ports, and after it's killed they aren't released
before the real server restarts — the relaunch then dies with *"listen tcp
:993: bind: address already in use."* The probe only needs the HTTP listener to
answer the health check.

### 4. Promote the web bundle and restart

- **Health passes** → kill the probe and `continue` the loop. The next
  iteration re-runs `serve` with mail enabled as normal, now executing the
  swapped-in binary.
- On that next boot the entrypoint's `promote_release` finds the staged
  `release-staging/<id>/` (with its `app.html` + `release-id.txt`), merges its
  hashed assets into the cross-release `_static/` pool, and atomically points
  `releases/current` at the new release. The SPA fallback then serves the new
  `app.html`.

The container's uptime does **not** reset — it's the same container, the same
PID 1 entrypoint, looping onto a new server process.

## Rollback

Failures before the binary swap (validation, download, build) just abort the
job and run a rollback stack that undoes each completed step (remove the copied
member dir, restore `package.json`/`pnpm-workspace.yaml`, re-run the generator).

Failures *after* the swap are caught at relaunch:

- If the **health-check fails**, the entrypoint restores the previous binary
  (`mv tinycld tinycld.failed; mv tinycld.prev tinycld`) and continues onto the
  old binary.
- The pre-swap **SQLite `VACUUM INTO` backup** (`data.db.backup`) lets the
  install pipeline's rollback restore the database if a later step fails.

## Runtime image requirements

The live installer shells out to real tools and needs a workspace it can write
to. The runtime image (`app/Dockerfile`) provides:

- **Tools on `PATH`:** `git` (git-spec `npm pack`), `node`/`npm`/`npx`,
  `pnpm` (pre-activated into a shared `COREPACK_HOME=/opt/corepack` so the
  unprivileged user finds it without a network fetch/prompt), the Go toolchain
  plus `gcc` **and** `g++` (the server's cgo set links libmupdf *and*
  goheif/libde265), `sqlite3` (the DB-backup `VACUUM INTO`), and `tar`/`cp`.
- **A writable workspace:** the whole tree lives under `/workspace`, owned by
  the `tinycld` runtime user, so the installer can create `/workspace/<slug>`
  and rewrite the workspace manifests. (The workspace is *not* at the
  filesystem root `/` — that would be root-owned and unwritable, and would also
  break pnpm's same-filesystem linkability probe.)
- **`pnpm` runs with `CI=true`** so `pnpm install` doesn't block on an
  interactive node_modules-purge confirmation.

> **Note.** The runtime image ships no Go module cache, so a server-package
> `go build` downloads its dependencies from the network. Installing a server
> package therefore needs outbound network access and can take several minutes.

## Uninstall

`POST /api/admin/packages/uninstall` with `{ "slug" }` runs the inverse
pipeline (`runUninstallPipeline`): verify the package isn't bundled (bundled
packages can't be uninstalled), remove `/workspace/<slug>`, drop the member from
the workspace manifests, `pnpm install`, regenerate, rebuild + stage the web
bundle, mark the `pkg_registry` record `disabled`, and request the same exit-75
relaunch. Uninstall does **not** rebuild the Go binary — a disabled package's
server code simply stops being registered after the regenerate + restart.

## Observability & troubleshooting

Every stage and every shelled-out command is echoed to the server's stdout, so
`docker logs <container>` is a full install trace:

```
[pkg_install] [job_…] [50%] Installing dependencies: Running pnpm install
[pkg_install] $ (cd /workspace && CI=true pnpm install --no-frozen-lockfile)
[pkg_install] output of pnpm:
…
[pkg_install] [job_…] COMPLETE status=success
```

The same per-stage detail streams over the SSE endpoint to the progress modal,
and a permanent record is written to the `pkg_install_log` collection
(`action`, `status`, `error`, `log`, timestamps).

The relaunch is equally legible — look for:

```
[entrypoint] Restart requested (exit code 75)
[entrypoint] Health check passed, restarting server
[entrypoint] promoting release <id> …
… Server started at http://0.0.0.0:7090
```

A relaunch that crashes on `:993` means the health-check probe ran without the
mail listeners disabled; an install that hangs with an empty `pkg_install_log`
means the POST never fired (e.g. a UI selector targeting the wrong element).

### Integration test

`tests/install/todo-install.spec.ts` (driven by
`tests/install/run-todo-install.sh`) exercises this whole path end to end: it
builds an image from the working tree, walks `/setup`, installs
`github:tinycld/todo`, and asserts the package is registered, its collection
exists (migration applied), and its route is reachable after the relaunch. It is
**runner-only** — gated behind `RUN_TODO_INSTALL_TEST=1` and excluded from
normal CI — because it builds a purpose-made image and runs a real
minutes-long install.
