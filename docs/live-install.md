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
| 93–94 | Building native bundles | `npx expo export --platform ios` then `--platform android`, **sequential, after web staging**. Each bundle + its assets are copied into the staged release's `native/<platform>/`. Skipped (`93`, no further work) when the RN toolchain is absent (web-only image) — mobile then stays on its embedded bundle. See [Native OTA bundles](#native-ota-bundles). |
| 95–97 | Updating database | Upsert the `pkg_registry` record (status `installed`) |
| 98–99 | Archiving build | Copy the now-live binary + staged bundle into `builds/<build_id>/`, write `build.json`, and record a `pkg_build` row (status `current`) capturing how many migrations this install applied (see [Build history & revert](#build-history--revert)) |
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
events, so the same `InstallProgressModal` drives it) and **reuses the exit-75
relaunch**:

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
   becomes `tinycld.prev` (so the entrypoint's health-check can roll back to it),
   and the archived binary is *copied* in (the archive stays intact).
5. **`migrate down N`** runs with the **target's** binary, which understands the
   older schema. User data is preserved; only the schema is rolled back.
6. **Re-stage** `builds/<id>/release/` into `release-staging/<release_id>/` so the
   entrypoint's `promote_release` serves it after relaunch.
7. **Update records (one transaction):** mark the target `current`, mark every
   newer build `superseded`, reconcile `pkg_registry` (a package whose *install*
   was reverted past is set `disabled` — unless an earlier surviving build still
   keeps it installed; bundled packages and the synthetic `(base image)` slug are
   never touched), and point the target's `pkg_registry` row at the reverted
   version. Wrapping these in `app.RunInTransaction` means an interrupted revert
   can't leave the build set with zero or multiple `current` rows. The
   `pkg_install_log` row (action `revert`) is the history trail.
8. **`os.Exit(75)`** → the entrypoint health-checks the reverted binary and, on
   failure, auto-restores `tinycld.prev` exactly as it does for an install.

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
   archive a new build, upsert the registry version, and request the exit-75
   relaunch.

Any failure unwinds the per-package rollback stack and restores the DB backup.
The whole operation holds the same `installMu`/`currentJob` single-flight lock as
install/revert, so version changes can't race them.

Because the deployed image runs with `HooksWatch: true` (JS-hook hot-reload),
re-running the generator mid-pipeline rewrites the watched `pb_hooks` symlinks,
which would otherwise make PocketBase's watcher call `app.Restart()` and tear the
process down between steps. An `OnTerminate` guard
(`shouldSuppressRestart`) vetoes any in-process restart (`IsRestart`) while a
package operation holds the single-flight lock; our own intentional relaunch uses
`os.Exit(75)` (a different path the guard never sees), so it still fires once the
pipeline finishes.

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
exists (migration applied), a `pkg_build` row + the base build are listed in
**Build History**, and its route is reachable after the relaunch. It then
**reverts to the base build** through the Build History UI and asserts — after
that second relaunch — that the todo build is now `superseded`, todo's migration
was reversed (`migrate down`), and its nav entry/route are gone.

Run it from the app member (needs Docker):

```sh
cd app
bash tests/install/run-todo-install.sh
```

The runner builds the image from the current working tree, boots it, scrapes the
setup token from the container logs, and drives the Playwright spec in a
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
docker logs tinycld-todo-test | grep -E 'COMPLETE status=|Restart requested|Server started'
```

A successful run shows `COMPLETE status=success`, then the relaunch
(`Restart requested` → `Health check passed` → a fresh `Server started`), with
the container still up and `Todo` present in `pkg_registry` as `installed`.
