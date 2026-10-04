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
> `pkg_restart.go`. `app/config/entrypoint.sh` does first-boot environment
> setup only; the relaunch itself is handled by `core/server/supervise`, which
> the entrypoint hands over to.
> It does NOT cover the build-time bundling done by `app/Dockerfile` (that
> assembles the *initial* image; the live installer adds packages to an
> already-running one).

## Contents

- [The big picture](#the-big-picture)
- [Entry points](#entry-points)
- [The install pipeline, stage by stage](#the-install-pipeline-stage-by-stage)
- [Native OTA bundles](#native-ota-bundles)
- [How relaunch works](#how-relaunch-works)
- [Rollback](#rollback)
- [Build history & revert](#build-history--revert)
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
5. The running server **asks the supervisor to replace it** and stays
   read-only. The supervisor starts the new build beside it with the same
   ports; once the new build sends **ready**, the supervisor **promotes** its
   web bundle, **commits** the database backup, and only then tells the OLD
   build to drain and exit. The old build keeps answering requests the whole
   time, so the swap never refuses a connection.

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

`status` is one of `pending` · `running` · `success` · `failed` · `rolled_back`.
`failed` is a pre-swap abort (validation/build/migration-sync) — the live build
is untouched. `rolled_back` is a post-swap health-check failure that the
supervisor reverted (symlink + DB restored), surfaced by the boot reconciler
(see [Rollback](#rollback)). The status survives the relaunch because it's read
from the durable `pkg_install_log` row, not the in-memory job (which the
process replacement discards).

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
| 93–94 | Building native bundles | `npx expo export --platform ios` then `--platform android`, **sequential, after web staging**. Each bundle + its assets are copied into the staged release's `native/<platform>/`. Skipped (`93`, no further work) when the RN toolchain is absent (web-only image) — mobile then stays on its embedded bundle. See [Native OTA bundles](#native-ota-bundles). |
| 95–97 | Updating database | Upsert the `pkg_registry` record (status `installed`) |
| 98–99 | Archiving build | Copy the now-live binary + staged bundle into `builds/<build_id>/`, write `build.json`, and record a `pkg_build` row (status `current`) capturing how many migrations this install applied (see [Build history & revert](#build-history--revert)) |
| 99 | Requesting restart | `requestRestart` — see [How relaunch works](#how-relaunch-works) |

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

## Native OTA bundles

Alongside the web bundle, the install pipeline exports **native JavaScript
bundles** (iOS + Android Hermes bytecode + assets) so mobile apps can update
over-the-air from this server — replacing Expo/EAS Update. The bundle a mobile
app loads is a property of the **server it is connected to**, not the org: every
org on a server shares the server's installed-package set and therefore the same
bundle.

- **Build.** After the web bundle is staged, the pipeline runs `expo export
  --platform ios` then `--platform android` (sequential — parallel Metro
  processes risk OOM) and copies each result into the staged release. Each
  export's `metadata.json` is parsed into per-platform metadata (`bundle_id` =
  `build-<ts>-<platform>`, `bundle_hash` = hex SHA-256 of the `.hbc`,
  `runtime_version` = the app version under the `appVersion` policy, and the
  asset list). This metadata is persisted in the `pkg_build` row's `bundles` JSON
  field and the files are copied into `release/native/<platform>/`. Each staged
  file's existence is re-checked after copy, so the `bundles` row never advertises
  a bundle the archive doesn't actually contain.
- **Runtime version is required.** `runtime_version` comes from `app.json`'s
  `expo.version` (the `appVersion` runtimeVersion policy). If it can't be read,
  native export **fails the install** rather than producing bundles no device can
  match — every client reports a concrete app version, so an empty one is
  permanently undeliverable. (Changing the `runtimeVersion.policy` away from
  `appVersion` would silently break matching — keep it `appVersion`.)
- **Toolchain skip.** A web-only deploy image without the RN toolchain
  (`node_modules/expo`) skips native export entirely; the update endpoint then
  returns `204` for mobile and apps keep their embedded bundle.
- **Serving.** `GET /api/app/update?platform=&runtimeVersion=&currentId=&currentHash=`
  (public, no auth — the app calls it pre-login) reads the current `pkg_build` row
  and returns a JSON manifest (`id`, `bundleUrl`, `bundleHash`, `assets[]`) when a
  newer bundle exists for that platform+runtime, or `204` when up to date / no
  match. "Up to date" matches on EITHER `currentId` (the running bundle id) OR
  `currentHash` (its hex SHA-256) — the hash check spares a fresh App Store install
  (whose id is `embedded-<version>`, never a server `build-<ts>` id) from
  re-downloading a byte-identical bundle on first foreground. `GET
  /api/app/bundle/...` and `/api/app/asset/...` serve the files from the build
  archive. Revert restores an older build's `bundles` pointer along with everything
  else, so reverting a package change reverts the mobile bundle.

## How relaunch works

The installer never restarts the container from the outside. **Under
`tinycld supervise`** (every Docker and bare-metal deployment — see
[Runtime image requirements](#runtime-image-requirements)) it asks the
supervisor to start the new build beside the old one and only tells the old
one to drain once the new one is ready: the old build keeps answering requests,
read-only, for the whole swap, so an upgrade never refuses a connection.
**Without a supervisor** (a hand-run binary, dev, e2e) the server still falls
back to exiting with a sentinel code for an external loop to catch — that path
is covered in [Unsupervised fallback](#unsupervised-fallback) below.

### 1. The server asks to be replaced

`requestRestart` (`pkg_restart.go`) writes a `.restart-requested` marker in the
state dir — beside `pb_data`, not inside it, because a restore's boot swap
renames `pb_data` away as a whole — then, under a supervisor, enters read-only
mode, holds the install-job system for the next process, and sends a `restart`
message over the control socket the supervisor gave this process at startup.
The supervisor answers with `restart-ack` as soon as the message arrives. This
process then keeps serving, read-only, until the supervisor drains it. With no
ack within 10 s it exits 75 instead, and the supervisor handles that as a cold
restart. In dev mode (`go run`) it only logs; you restart manually.

Read-only mode (`core/server/readonly`) refuses unsafe requests on every path
with `503` and `Retry-After`. It also stops most of the writes that do not
come from a request:

- The cron scheduler skips every due job while the mode is on (core's,
  PocketBase's, a package's and a JS hook's alike). A skipped run is not made
  up.
- The auto-upgrade tick (`coreserver/autoupgrade_local.go`) does nothing.
- The automation engine's worker waits for the mode to end before its next
  cycle (`core/server/automation`).
- The audit, comment-mention, and invite tails wait for the mode to end
  before writing (`core/server/audit`, `core/server/notify`,
  `core/server/coreserver/invite.go`).
- A backup or restore job that is already running skips its progress updates
  while the mode is on, catching up on the next tick (`core/server/backup`).
- A package's background workers check `readonly.Active()` or call
  `readonly.WaitInactive(ctx)` before each write cycle.

Collaborative-document (Yjs) journal appends and saves of the realtime save
coordinator (`core/server/realtime`, `core/server/yjsdoc`) are **not** paused
yet: they keep writing through a read-only window. This is a known gap,
tracked for a follow-up.

Writes that do not arrive as an HTTP request are not covered either: a
package's own client protocol, and websocket messages (the
collaborative-document saves above). DAV requests are HTTP requests to the
same server, so read-only mode refuses their unsafe methods like any other
request.

The control messages (`ready`, `restart`, `drain`, `restart-ack`) are permanent:
both sides act on them whatever protocol version the sender states, because
the supervisor is the image's own binary and its children can be newer. A
message type a side does not know is logged and ignored.

### 2. The supervisor starts the new build alongside the old one

`tinycld supervise` (`core/server/supervise`) holds the public ports for the
whole container/unit lifetime and starts each `tinycld serve` child with those
ports handed down as inherited file descriptors, so starting a second child
never contends for a port the first is still using. On a `restart` message it:

1. Starts a new child of the build `current` now points at, with the same
   ports.
2. Waits (up to 60s) for that child's `ready` message, sent from `OnServe`
   once its listeners are set — the same moment `/api/health` would first
   answer.
3. On success: promotes the new child's web bundle, commits the database
   backup (the migrated schema is the keeper), then tells the OLD child to
   drain.
4. On failure (the new child exits before `ready`, or times out): see
   [Rollback](#rollback).

A **cold** restart (the child signals `Cold: true` — currently only a backup
restore, whose boot renames `pb_data` as a whole) drains the old child FIRST,
then starts the new one: the two must never run on the same data directory at
once.

### 3. Draining the old child

Told to drain, a child first stops accepting new HTTP connections (on the main
port and on the `:80` redirect port) and turns off HTTP keep-alives, so each
connection closes after its current request. It then runs the drain-begin
handlers (`tinycld.org/core/drainhooks`). Core's own handler ends every
realtime (SSE) stream, so its client reconnects at once, to the new child,
instead of hearing nothing of the new child's events for the length of the
drain; a realtime connect that still reaches the draining child is answered
`503`. A package serving its own port (such as IMAP and SMTP) stops
accepting there at the same point. The child then waits (up to 30s,
`ChildDrainTimeout`) for in-flight requests on both HTTP servers to finish,
including a connection accepted in the instant before accepting stopped, then
shuts down and exits. A long-lived connection that is not a realtime stream
(IMAP IDLE) is cut at the end of that budget; clients reconnect against the new
child.

**Keep-alive edge case:** an idle keep-alive connection to the draining child
is closed when keep-alives turn off, not after its next request. A client that
sends its next request on that connection as it closes gets nothing back: the
connection reads EOF or is reset, before the request was read. That is at most
one failed request per idle connection, and a retry on a fresh connection
reaches the new child. The swap test in `core/server/supervise/run_test.go`
drives such a client through a swap and checks exactly this. A reverse proxy
or browser retries on its own, so in practice it is at most a single transient
`502` at the moment of a swap, never a sustained outage.

### Unsupervised fallback

A process with no supervisor (`listeners.Supervised()` false — a hand-run
binary, dev, e2e) keeps the old behavior: `requestRestart` calls `os.Exit(75)`
at once, with no read-only window and no in-process health check. Nothing in
a supervised deployment (Docker image or the bare-metal unit) takes this path;
it exists for the standalone binary and tooling that run `tinycld serve`
directly.

## Rollback

A rebuild has two failure regimes split by the atomic `current`-symlink swap
(`activateBuild`):

**Before activation** (assemble, `pnpm install`, `go build`, `expo export`, the
pre-swap DB backup, the DOWN-migration sync) a failure aborts the job in-process:
the half-built `builds/<id>` dir is discarded, the DB is restored from the
pre-swap `VACUUM INTO` snapshot if it had already been taken, and the live
`current` symlink is never touched — the old build keeps serving. The
`pkg_install_log` row ends `failed`. (A broken package that fails its web build —
e.g. an unresolved import in a screen — lands here: `expo export` fails, no swap.)

**After activation** the swap and any DOWN migrations have already hit live
state, and the job has asked to be replaced, so an in-process undo is
impossible. Instead the supervisor renders the verdict — a new child's `ready`
message (or the 60s timeout) in place of the old shell loop's `/api/health`
probe:

- **Healthy** (`ready` arrives) → the backup is committed (the armed snapshot +
  marker are deleted); the new build serves.
- **Unhealthy** (the new child exits before `ready`, or the 60s timeout
  expires — the binary panics at bootstrap, a pending UP migration throws at
  `serve` boot before the HTTP listener binds, or boot hangs) → **cold
  rollback**: stop the new child, stop the old child, restore `data.db` from
  the armed snapshot, flip `current` back to the previous build (the whole
  tree reverts, not just the binary), and start that build again. Unlike a
  healthy swap, this one has a short outage — only on a failed upgrade, never
  on a successful one.

Each rebuild and each revert saves its build id on its `pkg_install_log` row
(`build_id`) before it takes the snapshot. When the supervisor rolls a build back, it writes
a rollback record, `<state>/.rollback-pending` (JSON: `build`, `rolled_to`,
`at`), beside `pb_data` so that a restore swap cannot move it away. On the next
boot, `ReconcileRolledBackInstall` (registered in the `registerStaticServe`
OnServe hook) reads the record and marks every row of that build's latest run
`rolled_back` (a revert re-uses a build id, so rows older than the newest row
of another build are left as they are), whichever database is live:

- the restored snapshot, taken *before* the job wrote its terminal status,
  holds the row at `running`;
- a database that was not restored (a restore swap that was rolled back, or a
  backup the supervisor could not restore) holds it at `success`.

A row written before the `build_id` field has no build id; for it, the
newest `running` row without one is marked. A record left in
`pb_data/.rollback-pending` by an older supervisor (plain-text build id) is
read once in the same way. So a post-restart rollback shows terminal status
**`rolled_back`** at `GET /api/admin/packages/status/{slug}`, distinct from the
pre-swap **`failed`**, and an automatic upgrade that was rolled back is
blocked from being tried again.

The supervisor renders the identical verdict on a fresh start if the whole
process is killed (OOM, `docker kill`, host reboot) between the replace request
and the verdict: an armed backup marker found at boot means a rebuild's health
verdict never completed, so the supervisor checks the current build itself —
commit on healthy, cold-rollback on unhealthy — before settling into its normal
loop.

**A backup the rollback could not restore.** When the restore of the armed
snapshot fails (a full disk, for example), the supervisor still flips `current`
back, but the server then runs on the database the failed build migrated. The
supervisor moves the snapshot out of every automatic path, to
`<state>/unrestored/<build>/data.db`, with a note beside it,
`unrestored.json` (`build`, `rolled_to`, `at`, `restore_error`, `size`), and
logs at Error on every start while it is there. `<build>` is the failed build.
Nothing removes or restores it automatically: a later rebuild, commit or
rollback never touches `unrestored/`.

A backup can also be set aside without a failed restore. When the build that
serves fails to restart (a cold restart with no rebuild) and a backup is still
armed, that backup is the copy from before an update that succeeded: its
commit failed, and the build served writes after it was taken. The supervisor
does not restore it over those writes and does not leave it armed, where the
next start's check of an interrupted rebuild could restore it. It moves it to
`unrestored/<build>/` the same way; `build` and `rolled_to` are then the same
build, and `restore_error` says why the backup was not restored.

- On boot, `reportUnrestored` tells every owner and admin once, in the app
  and by email, and writes an empty marker per channel beside the note
  (`notified-app`, `notified-email`). A channel that fails gets no marker,
  and the next boot sends only that channel again. A dir with `data.db` but
  no note (a crash between the supervisor's two renames) is reported too.
  It runs in a goroutine that `startBootNotices` starts from OnServe, after
  `ReconcileRolledBackInstall`, together with the emails of
  `reconcileAutoUpgradeResults`: a mail server or push service that does not
  answer must not delay `ready` past the supervisor's 60 s. Terminating the
  app cancels the goroutine and waits up to 2 s for it to stop; if it has
  not stopped by then, the app logs a warning and closes the database.
- Automatic upgrades wait until `unrestored/` is empty: the tick does nothing
  and the status line shows "paused: a database backup needs attention".
  Manual version changes are not blocked.
- To inspect it: open a *copy* of `unrestored/<build>/data.db` with `sqlite3`
  and compare it with the live data. `unrestored.json` says when and why the
  restore failed.
- To put it back (only if no data written since the failed update must be
  kept, and only while the server still runs the `rolled_to` build named in
  `unrestored.json`; after a manual change to a newer build the copy no longer
  matches it, so ask for help instead): stop the server (the standard
  container: stop the container and work on its `/workspace` volume; bare
  metal: stop the service), delete `pb_data/data.db-wal` and
  `pb_data/data.db-shm` if present, copy `unrestored/<build>/data.db` over
  `pb_data/data.db` (keep the server user as its owner), delete
  `unrestored/<build>/`, start the server.
- To discard it (the current data is kept): delete `unrestored/<build>/`.

The in-app help topic `core:after-a-failed-update` gives administrators the
same steps.

**Error reporting.** The supervisor sends its warnings and errors (a rollback,
or no build becoming ready) to Sentry when `SENTRY_DSN` is set in its own
environment, and flushes them before it exits. The server takes its DSN from
**Settings → Error Reporting** (the `system_settings` collection), which the
supervisor never reads because it never opens the database. A DSN entered only
in the settings screen therefore does not reach the supervisor: set
`SENTRY_DSN` in the container or unit environment as well. Without it, the
supervisor logs once at start that it reports to stderr only.

### A build older than the supervisor cannot run under it

`tinycld supervise` binds the main port itself and hands each child its
listener as an inherited file descriptor; a build from before the supervisor
existed binds that port itself instead, and fails at once with "address
already in use" when started as a supervised child. Two consequences:

- **Downgrading, or reverting, past the version that introduced the
  supervisor is unsupported.** The version-change or revert pipeline starts
  the older build as a child exactly like any other; that child fails to
  bind, never sends `ready`, and the supervisor treats it as a normal failed
  upgrade — cold rollback to whatever build *was* running. The downgrade
  itself never completes.
- **A rare double fault can strand the service on a build that cannot run.**
  Suppose a rebuild is interrupted on an OLD (pre-supervisor) image — the
  process killed between activation and its health verdict — and the box then
  comes back up on a NEW (supervisor-capable) image before that verdict is ever
  rendered. The entrypoint's `seed_baked_build` runs first: the image's baked
  release id is new, so it points `current` at the new image's baked build and
  records the interrupted build in `.previous-build`. The supervisor's startup
  recovery (see above) then checks the baked build, not the interrupted one.
  Normally the baked build becomes ready and the backup is committed. Only if
  the new image's baked build ALSO fails does the recovery roll back, to
  `.previous-build` — the pre-supervisor build, which cannot bind under the
  supervisor. The service then crash-loops: on each restart the baked id is
  already adopted, so `current` stays on that build.

  **Recovery:** boot with `TINYCLD_RESCUE=1` (Docker: `-e TINYCLD_RESCUE=1`;
  bare metal: `Environment=TINYCLD_RESCUE=1` on the unit, or run
  `/opt/tinycld-entrypoint.sh` by hand with it set) to get a shell as the
  runtime user INSTEAD of handing over to the supervisor — the hatch exits
  before the `supervise` exec, so a build that can't bind under a supervisor
  never gets the chance to try. From that shell, point `current` at a build
  made by the CURRENT (supervisor-capable) image: either re-run the
  entrypoint's own first-boot seed logic by hand (remove
  `$TINYCLD_STATE_DIR/current` and restart normally, which re-seeds from the
  image's baked build), or symlink `$TINYCLD_STATE_DIR/current` directly at a
  known-good `builds/<id>/tinycld` and restart.

## Build history & revert

Beyond the install pipeline's automatic rollback, every **successful** install is
saved as a restorable **build** that a superuser can manually revert to from the
setup dashboard's **Build History** tab (`POST /api/admin/packages/revert`). The
server code lives in `core/server/coreserver/pkg_build.go` (archive + record
helpers) and `pkg_revert.go` (the revert pipeline + endpoints).

### What's saved per build

The install pipeline's "Archiving build" stage writes, under the binary's
directory:

```
builds/<build_id>/
    tinycld          # the server binary that was live after this install (server packages only)
    release/         # a copy of the staged web bundle (app.html + assets + release-id.txt)
        native/      #   per-platform OTA bundles: native/ios/**, native/android/** (when built)
    build.json       # mirror of the pkg_build record, for offline restore
```

and a `pkg_build` collection row (`pbc_pkg_build_01`) with: `build_id`
(= the dir name, `build-<unixMilli>`), `pkg_slug`, `npm_package`, `version`,
`binary_archived`, `release_id`, **`migrations_applied`** + **`migration_files`**
(the exact migrations this install added, captured by diffing the `_migrations`
history table before and after the `migrate` step), and `status` (`current` for
the newest, `available` for older revertible builds, `superseded` for ones a
revert skipped past). The prior `current` build is demoted to `available`. No
per-build DB snapshot is taken — schema rollback is done with `migrate down`.

### How a revert works

`runRevertPipeline` mirrors the install pipeline (same SSE `progress`/`complete`
events, so the same `InstallProgressModal` drives it) and **reuses the same
restart request** ([How relaunch works](#how-relaunch-works)):

1. **Validate** the target build exists, is `available`, and its archive is intact.
2. **Migration safety gate.** Sum `migrations_applied` across every build newer
   than the target — that's the `N` for `migrate down N`. Confirm the live
   `_migrations` tail (newest-first) still equals the recorded chain of those
   newer builds. If it diverged (manual edit, `history-sync`, an out-of-band
   install), **block** — a blind `migrate down N` could reverse the wrong
   migrations. There is no DB-snapshot fallback; the operator resolves the
   mismatch manually.
3. **Backup the DB** (`data.db.backup`) as the revert operation's own safety net.
4. **Swap in the archived binary** (`swapToArchivedBinary`): the live binary
   becomes `tinycld.prev` (so a failed health verdict can roll back to it),
   and the archived binary is *copied* in (the archive stays intact).
5. **`migrate down N`** runs with the **target's** binary, which understands the
   older schema. User data is preserved; only the schema is rolled back.
6. **Re-stage** `builds/<id>/release/` into `release-staging/<release_id>/` so the
   supervisor promotes it once the new child is ready.
7. **Update records (one transaction):** mark the target `current`, mark every
   newer build `superseded`, reconcile `pkg_registry` (a package whose *install*
   was reverted past is set `disabled` — unless an earlier surviving build still
   keeps it installed; bundled packages and the synthetic `(base image)` slug are
   never touched), and point the target's `pkg_registry` row at the reverted
   version. Wrapping these in `app.RunInTransaction` means an interrupted revert
   can't leave the build set with zero or multiple `current` rows. The
   `pkg_install_log` row (action `revert`) is the history trail.
8. **Request a restart** → the supervisor health-checks the reverted build and, on
   failure, cold-rolls-back exactly as it does for an install.

### Revert is one-way

Because step 5 tears down the newer builds' migrations and step 7 marks them
`superseded`, those builds are **permanently unreachable** — their binaries assume
schema that no longer exists, and the migration tail they relied on is gone. The
UI hides **Revert** on `superseded` (and `current`) rows and the confirm dialog
names exactly which builds a revert will invalidate. Moving forward again is a
fresh install of the newer version, which produces a new `available` build.

### The base build (initial deploy)

So that an operator can always return to "fresh image, before any live install",
`SeedBaseBuild` (in `pkg_build.go`, called from the `OnServe` boot hook right after
`SyncBundledPackages`) records a one-time **base build** the first time a deployed
image boots. It is idempotent — it no-ops once any `pkg_build` row exists — and
only runs in the deployed-image layout (it needs the live binary on disk and a
promoted `releases/current/` to archive), so it silently skips in dev / `go run`.

The base build has `build_id = build-base`, `migrations_applied = 0`, and an empty
`migration_files`: the bundled migrations already applied at first boot are the
schema floor and must never be stepped down. Reverting **to** the base build
therefore reverses exactly the migrations of every live-installed build that came
after it, and stops there. Its archived `release/` holds only `app.html` +
`release-id.txt` (matching what `releases/current/` contains); the hashed assets
already live in the append-only `_static/` pool, so promoting the base bundle on a
revert resolves them without re-archiving. The first real install demotes the base
build from `current` to `available`, at which point it becomes a revert target.

### Retention & deletion

Builds are **never auto-pruned** — they accumulate until a superuser deletes one
with the per-row **Delete** action (`POST /api/admin/packages/builds/delete`),
which removes the `pkg_build` record and the `builds/<id>/` archive. The current
build can't be deleted. Archived binaries are large (cgo, ~100 MB+), so operators
should delete builds they no longer need.

## Per-package version changes

Build revert (above) rolls the **whole image** back to an earlier snapshot by
count (`migrate down N`), superseding everything newer. A **version change**
(Setup → Versions) instead moves **one package** to any version its source
publishes — newer (update) or older (downgrade) — leaving every other package's
schema and data untouched.

### Why the named-migration runner exists

All packages' migrations interleave by timestamp in one `_migrations` table, so
the count-based `migrate down N` cannot revert just one package's migrations
without tearing down unrelated ones. `pkg_migrate.go` drives a **named subset**
instead: it looks each migration up by filename in `core.AppMigrations` (the same
global list the jsvm plugin registers every `.js` `migrate(up, down)` into) and
runs its own `Up`/`Down` inside the stock aux+main transaction nesting,
replicating the `_migrations` insert/delete itself. Because it only ever touches
the named files for one package, no other package's history or schema is affected.

A key enabler: the **running process** keeps every migration it registered at its
own startup, regardless of later on-disk file changes — so even after a downgrade
swaps a package's files to an older version (removing the newer migration files
from disk), the running binary can still execute the newer migrations' `Down`
closures.

### Migration → package attribution

The generator (`symlinkServerArtifacts`) emits `server/pb_migrations_owner.json`,
a `{ migration-file → owning-slug }` map (core migrations owned by `core`), while
it flattens each package's `pb-migrations/` into the shared dir. The server reads
it via `migrationsForPackage(slug)` / `packageForMigration(file)`. Each install
also records its package's own migration files on the `pkg_build` record
(`pkg_migration_files`), so a version change knows exactly which files to diff.

### The pipeline (`runVersionChangePipeline`)

`POST /api/admin/packages/versions/apply` with `{ changes: [{ slug, targetVersion }] }`
returns a `jobId` (same SSE progress stream as install). For each change:

1. Re-run the compatibility solver authoritatively against the live registry.
2. Back up the DB (the downgrade safety net).
3. `npm pack` / git-fetch the target version, validate its manifest, swap the
   workspace member files (with a `.bak` restore on rollback).
4. Regenerate wiring — rewrites the owner map so `migrationsForPackage(slug)`
   reflects the **target** version's file set.
5. Diff against the current build's recorded set:
   upgrade → `applyNamedMigrations(target ∖ current)`;
   downgrade → `revertNamedMigrations(current ∖ target)`.
6. `pnpm install`, rebuild the binary (if the package has a server) + web bundle,
   archive a new build, upsert the registry version, and request the restart
   ([How relaunch works](#how-relaunch-works)).

Any failure unwinds the per-package rollback stack and restores the DB backup.
The whole operation holds the same `installMu`/`currentJob` single-flight lock as
install/revert, so version changes can't race them.

Because the deployed image runs with `HooksWatch: true` (JS-hook hot-reload),
re-running the generator mid-pipeline rewrites the watched `pb_hooks` symlinks,
which would otherwise make PocketBase's watcher call `app.Restart()` and tear the
process down between steps. An `OnTerminate` guard
(`shouldSuppressRestart`) vetoes any in-process restart (`IsRestart`) while a
package operation holds the single-flight lock; our own intentional relaunch goes
through `requestRestart` (a different path the guard never sees), so it still
fires once the pipeline finishes.

### Discovery, compatibility, and the drop report

- `GET /api/admin/packages/versions` lists each registry package's available
  versions (npm registry versions or git tags, inferred from the stored spec),
  with a short in-memory TTL cache and per-package failure isolation.
- `POST /api/admin/packages/versions/check` runs the `peerVersions` solver over a
  proposed `{ slug → version }` set and returns violations; the UI disables Apply
  while any exist, and the pipeline re-checks before mutating.
- `POST /api/admin/packages/versions/drop-report` dry-reverts the package's
  current migrations inside a transaction it rolls back, returning the
  collections/fields a downgrade would drop — the UI lists them and gates the
  downgrade behind a typed slug confirmation.

## Runtime image requirements

The live installer shells out to real tools and needs a workspace it can write
to. The runtime image (`app/Dockerfile`) provides:

- **Tools on `PATH`:** `git` (git-spec `npm pack`), `node`/`npm`/`npx`,
  `pnpm` (pre-activated into a shared `COREPACK_HOME=/opt/corepack` so the
  unprivileged user finds it without a network fetch/prompt), the Go toolchain
  (no C compiler: the build is CGo-free — omnidoc decodes HEIF in pure Go,
  retiring the goheif/libde265 dependency — and package builds run with
  `CGO_ENABLED=0`), `sqlite3` (the DB-backup `VACUUM INTO`), and `tar`/`cp`.
- **A writable workspace:** the whole tree lives under `/workspace`, owned by
  the `tinycld` runtime user, so the installer can create `/workspace/<slug>`
  and rewrite the workspace manifests. (The workspace is *not* at the
  filesystem root `/` — that would be root-owned and unwritable, and would also
  break pnpm's same-filesystem linkability probe.)
- **`pnpm` runs with `CI=true`** so `pnpm install` doesn't block on an
  interactive node_modules-purge confirmation.
- **A stop timeout of 45 s.** On a stop, the supervisor drains its child, which
  can take up to 30 s (`ChildDrainTimeout`) plus its shutdown hooks. Docker's
  default stop timeout is 10 s, after which it kills the container and cuts the
  requests still in flight. `docker-compose.yml` sets `stop_grace_period: 45s`.
  With `docker run`, pass `--stop-timeout 45`. On Dokku, run
  `dokku config:set <app> DOKKU_DOCKER_STOP_TIMEOUT=45`. The bare-metal unit
  sets `TimeoutStopSec=45`.
- **TLS on a package's own ports.** The supervisor holds a package's own ports
  (for example mail's IMAP and SMTP ports) and passes them to the server as
  plain TCP listeners. It does not terminate TLS on them. Mail terminates TLS
  itself, for IMAP (`:993`) and for SMTP submission (`:465`). Thus in
  production, mail needs one of these:
  - `IMAP_TLS_CERT` and `IMAP_TLS_KEY`, set to readable certificate and key
    files. IMAP reads only this pair. SMTP submission reads `SMTP_TLS_CERT`
    and `SMTP_TLS_KEY` first, and uses the IMAP pair when they are not set.
  - Autocert: `AUTOCERT_ENABLED=true` and `PRIMARY_DOMAIN`.

  Each of the two servers checks for TLS on its own. In production, if one of
  them has no TLS, the server boot fails. To run mail without TLS, turn off
  both servers: set `IMAP_ENABLED=false` and `SMTP_ENABLED=false`. The inbound
  MX listener (`:25`, on only with `MAIL_INBOUND_SMTP_ENABLED=true`) does not
  need TLS to start. It offers STARTTLS when `SMTP_INBOUND_TLS_CERT` /
  `SMTP_INBOUND_TLS_KEY`, `SMTP_TLS_CERT` / `SMTP_TLS_KEY` or autocert is set.

> **Note.** The runtime image ships no Go module cache, so a server-package
> `go build` downloads its dependencies from the network. Installing a server
> package therefore needs outbound network access and can take several minutes.

## Uninstall

`POST /api/admin/packages/uninstall` with `{ "slug" }` runs the inverse
pipeline (`runUninstallPipeline`): verify the package isn't bundled (bundled
packages can't be uninstalled), remove `/workspace/<slug>`, drop the member from
the workspace manifests, `pnpm install`, regenerate, rebuild + stage the web
bundle, mark the `pkg_registry` record `disabled`, and request the same
restart. Uninstall does **not** rebuild the Go binary — a disabled package's
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

The relaunch is equally legible — the supervisor logs each step under
`pkg=supervise` (`logging.ForPackage("supervise")`); look for:

```
level=INFO pkg=supervise msg="the server asked to be replaced" pid=…
level=INFO pkg=supervise msg="started a server" build=… pid=…
level=INFO pkg=supervise msg="the server is ready" pid=…
```

An install that hangs with an empty `pkg_install_log` means the POST never
fired (e.g. a UI selector targeting the wrong element). A relaunch that never
logs "the server is ready" within ~60s and instead shows "the new build did
not become ready; rolling back" means the new build's boot itself is failing —
check its own log lines just above for the actual panic/migration error.

### Integration test

`tests/install/todo-install.spec.ts` (driven by
`tests/install/run-todo-install.sh`) exercises this whole path end to end: it
builds an image from the working tree, walks `/setup`, installs
`github:tinycld/todo`, and asserts the package is registered, its collection
exists (migration applied), a `pkg_build` row + the base build are listed in
**Build History**, and its route is reachable after the relaunch. It then
**reverts to the base build** through the Build History UI and asserts — after
that second relaunch — that the todo build is now `superseded`, todo's migration
was reversed (`migrate down`), and its nav entry/route are gone.

Run it from the `tinycld` member (needs Docker):

```sh
cd tinycld
bash tests/install/run-todo-install.sh
```

The runner builds the image from the current working tree, boots it, scrapes the
setup code from the container logs, and drives the Playwright spec in a
standalone sandbox. Env knobs:

| Var | Effect |
| --- | --- |
| `IMAGE=<tag>` | Skip the build and test an existing image tag (e.g. one you built earlier). |
| `KEEP=1` | Leave the container running after the run for manual inspection. |
| `PW_BASE_URL` | Override the container URL (default `http://localhost:7090`). |

```sh
# Reuse an already-built image and leave it up to poke at afterwards:
IMAGE=tinycld-todo-test KEEP=1 bash tests/install/run-todo-install.sh
```

The spec is **runner-only** — it hard-skips unless `RUN_TODO_INSTALL_TEST=1` is
set (the runner sets it) and lives outside `tests/e2e/`, so it never runs in the
normal `tinycld-pkg test:e2e` suite or the docker smoke workflow. It builds a
purpose-made image and runs a real, minutes-long install (the server `go build`
downloads its deps from the network), so it's not part of routine CI.

Because the install can outlast Playwright's wait on a cold `go build`, the
authoritative result is the container log, not the Playwright exit code:

```sh
docker logs tinycld-todo-test | grep -E 'COMPLETE status=|asked to be replaced|started a server|the server is ready'
```

A successful run shows `COMPLETE status=success`, then the relaunch
(`the server asked to be replaced` → `started a server` → `the server is
ready`), with the container still up and `Todo` present in `pkg_registry` as
`installed`. The container never restarts: the supervisor swaps the server
inside it.
