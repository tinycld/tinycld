# Rebuild-from-scratch package management — design

**Date:** 2026-06-10
**Status:** Approved (brainstorm), ready for implementation planning
**Branch:** `feat/self-hosted-app-updates`

## Problem

The in-app package machinery in `/admin` (install / upgrade / downgrade / uninstall
/ core upgrade / rollback) mutates the **live** `/workspace/tinycld/` tree in place:
`npm pack` → extract → copy over the running member dir → `os.RemoveAll` the old
files, plus a separate `git clone`-over-`appDir` path for core, with file backups
under `wsRoot/backups/` to enable rollback. It lives in
`tinycld/core/server/coreserver/pkg_*.go` (~7k lines).

This is buggy precisely because of the hackish in-place edits and selective file
removal: rollback depends on getting a pile of file/migration deltas exactly right,
the live tree is implicit mutable state, and a partial failure can leave the running
workspace half-mutated.

## Solution overview

Replace every mutating operation with the **same** operation: build a complete,
fresh workspace in an isolated build dir, then go live with an atomic symlink swap.
No in-place mutation, no file deletion from the live tree.

Every mutating op (install / upgrade / downgrade / uninstall / core upgrade) is
identical: it produces a **desired package set**, the server builds that set from
scratch, and swaps it in. Rollback reactivates a prior build dir — it never rebuilds.

## Section 1 — Core model

1. **`/admin` sends a desired package set** — `{ slug, version, npm_package spec }`
   for *every* member that should exist, **including `tinycld` itself**. Install =
   add a row; uninstall = drop a row; upgrade/downgrade = change a version; core
   upgrade = change `tinycld`'s spec. The server receives the complete target and
   never diffs "what changed."

2. **The server writes that set to disk** as the build's input — `builds/<id>/manifest.json`.
   This doubles as the rollback record.

3. **Build fresh into `builds/<id>/`** — assemble members, `pnpm install`, generator,
   `go build`, `expo export`. Nothing touches the live tree.

4. **Atomic swap** — flip the `current` symlink the server runs from to the new build
   dir, request restart (exit-75). `pb_data`, `releases`, `builds` are bind-mounts
   *outside* the swapped tree → persist untouched.

The key simplification: **no in-place mutation and no file deletion.** A build dir is
immutable once built. Uninstall = the next build is assembled without that member.
Rollback = reactivate an existing dir.

## Section 2 — Build pipeline

A single `rebuild(manifest) → buildId` function, run as a background job streaming
progress to `/admin` (reusing the existing job/SSE event plumbing).

Resulting layout:

```
builds/<id>/
  manifest.json             ← the input manifest (written first; rollback record)
  package.json              ← workspace root manifest (members list)
  pnpm-workspace.yaml       ← packages: + storeDir: /workspace/.pnpm-store
  tinycld.packages.ts, scripts/, tests/, .npmrc   ← workspace scaffold
  tinycld/                  ← the tinycld member (app shell + core + package-scripts)
  mail/ calc/ ...           ← feature members
  node_modules/             ← hardlinked from shared store (cheap)
```

Pipeline steps:

1. **Fetch every member by spec** — for each manifest row, `npm pack <spec>` into a
   staging dir, untar, move into `builds/<id>/<slug>/`. One uniform fetch path: npm
   names, git URLs, `git+file://` all work through `npm pack`. `tinycld` (app shell +
   core) is fetched exactly like any feature or third-party `@acme/*` package.
2. **Write the workspace scaffold** — root `package.json`, `pnpm-workspace.yaml` (with
   the fixed `storeDir`), `tinycld.packages.ts`; copy `scripts/` + `tests/`. Small
   deterministic templating, lifted/inlined from bootstrap's `assemble-workspace.ts`
   (do **not** subprocess bootstrap — see below).
3. **`pnpm install`** in the build dir — reuses `/workspace/.pnpm-store`, so this is
   hardlink-fast, not a ~2GB re-download. Postinstall runs the generator +
   link-members exactly as in a normal image build.
4. **`go build`** the server binary → `builds/<id>/tinycld/tinycld`.
5. **`expo export --platform web`** → `builds/<id>/tinycld/release-staging/<release-id>/`.

Result: a complete, self-contained, runnable workspace in `builds/<id>/` —
byte-for-byte equivalent to what a fresh Docker image build would produce for that
member set. If **any** step fails, the build dir is discarded and `current` never
moves — the running server is wholly unaffected.

**Why not bootstrap:** `@tinycld/bootstrap` only `git clone`s from the hardcoded
`github.com/tinycld` org. It can't fetch third-party npm packages, pinned versions
via npm, or `git+file://` specs (which the test harness and operators use). We reuse
its scaffold-templating *idea* but keep the proven `npm pack` member fetch.

## Section 3 — Atomic swap & entrypoint integration

The server runs from a fixed `appDir = resolveServerDir()` (= the binary's own dir).
We make that a **symlink** to the active build dir, and split state out of the
swapped subtree.

```
/workspace/
  current → builds/<id>/tinycld     ← the symlink the server's appDir resolves to
  builds/<id>/                       ← complete build
  .pnpm-store/                       ← shared store (baked + persisted)
  pb_data/    releases/              ← bind-mounts, OUTSIDE any build dir
```

**Mount relocation.** `pb_data` / `releases` / `builds` move from
`/workspace/tinycld/<dir>` to `/workspace/<dir>`. Update Dockerfile mount targets,
entrypoint paths, docker-compose / `deploy.sh`, the path helpers, and the
`run-todo-install.sh` `-v` flags to match. Clean separation: build dirs hold only
**code**; `/workspace` holds only **state**.

**Path resolution split** (the central mechanical change, well-contained):
- `resolveServerDir()` → still the binary's dir (= the active build's `tinycld/`).
  All **code/asset** reads (migrations source, web-bundle source, generator output)
  stay relative to this.
- New `resolveStateDir()` → fixed `/workspace` (env-overridable via e.g.
  `TINYCLD_STATE_DIR` for the test harness). All **stateful** reads resolve here:
  `pb_data/data.db` (`pkg_go_build.go`), `releases/current` (`static.go`,
  `pkg_build.go`), the restart marker (`pkg_restart.go`), `builds/`.

**The swap**, performed by the rebuild job after a successful build + DB sync:
1. Build succeeded → `builds/<id>/` complete; migration diff applied to live
   `/workspace/pb_data` (with a backup taken — see Section 4).
2. Atomic flip:
   `ln -sfn builds/<id>/tinycld /workspace/current.tmp && mv -T /workspace/current.tmp /workspace/current`.
3. `requestRestart()` → exit 75.

**Entrypoint changes** (`config/entrypoint.sh`):
- Binary path becomes `/workspace/current/tinycld` (follow the symlink), not
  `./tinycld` in a fixed dir.
- The exit-75 health-probe stays. **Rollback-on-probe-failure changes** from binary
  swap (`mv tinycld.prev tinycld`) to **flipping `current` back** to the previous
  build dir — the whole tree reverts, not just the binary.
- `promote_release` keeps working: reads `release-staging` from the now-current
  build dir, promotes into the fixed `/workspace/releases`.

This **removes** machinery: no `tinycld.new` / `.prev` / `.failed` juggling, no
`wsRoot/backups/`, no in-place file deletion. The symlink is the single source of
"what's live."

## Section 4 — DB migrations & rollback

`pb_data` carries across builds and can't be rebuilt; its applied schema must be
synced to match the new build's migration set. Done by the rebuild job **after** the
build succeeds, **before** the symlink flip.

1. **Backup first.** Snapshot `/workspace/pb_data/data.db` → `data.db.backup`.
   WAL-safe (checkpoint then copy), as the existing code already does — the test
   harness reproduces the bind-mount WAL-corruption bug specifically.
2. **Diff, don't track deltas.** Compare the migration files the *new build* carries
   against the set the *live DB* has applied (PocketBase's `_migrations` table). Clean
   set comparison — no per-build "migrations_applied count" bookkeeping:
   - **UP** (in new build, not yet applied): applied by the *new* binary on its boot
     after the swap (PocketBase auto-migrates on start, and the new binary has the
     closures registered). No separate UP subprocess step — letting boot apply them
     is simplest and is already how a fresh image boot behaves. The post-swap health
     probe gates success: if boot-migration fails, the probe fails and the rollback
     path (restore `data.db.backup`, flip `current` back) triggers.
   - **DOWN** (applied in live DB, absent from new build — a downgrade/uninstall):
     run *before* swapping, via the *outgoing* build's binary, which still has those
     DOWN closures.

   This preserves the existing UP-needs-new-binary / DOWN-needs-old-binary asymmetry
   but drives it from a declarative file-set diff instead of incremental counters.
3. **On any failure** (build, migration, or post-swap health probe): restore
   `data.db.backup`, leave `current` on the old build dir, exit. The running server is
   untouched throughout — nothing mutated the live code tree, only `pb_data`, which is
   backed up.

**Rollback / revert-to-archived-build** (explicit `/admin` action). Every prior build
dir is retained intact, so "revert to build X" is:
1. Backup current `pb_data`.
2. Migration-diff: current live schema → build X's migration set (revert the DOWNs on
   the *current* binary, apply any UPs on X's binary).
3. Flip `current` → `builds/X/tinycld`, restart.

No rebuild, no re-fetch — X is already a complete tree. Rollback is near-instant and
can't fail on a network fetch.

**Retention:** keep the last **N = 5** build dirs (configurable); prune oldest beyond
N. `node_modules` hardlinks into the shared store, so each retained build's real disk
cost is small (manifest + binary + web bundle + hardlinks, not 1.1GB of copies). A
build dir is pruned only when it's neither `current` nor a recent rollback target.

## Section 5 — Registry, job lifecycle, deletions

**`pkg_registry`** stays as the user-facing inventory (what `/admin` lists, status
badges, nav). It's no longer the *build input* — `builds/<id>/manifest.json` is:
- `/admin` computes the desired set from the current registry + the user's action,
  sends it to the server.
- The server builds `manifest.json` from that set, runs the rebuild, and **only on
  success** updates `pkg_registry` rows to match (version bumps, `installed`/`disabled`
  status). A failed build leaves the registry untouched, matching the still-live old
  build.
- **`pkg_build`** keeps tracking build dirs for the rollback UI (build_id, manifest,
  version set, status `current`/`available`/`superseded`), but simplifies: no more
  `migrations_applied` counts / `migration_files` deltas / `pkg_migration_files` — the
  manifest plus the build dir's own migration files are the source of truth.

**Job lifecycle** reuses existing infra: a background job with an SSE progress stream
(`/api/admin/packages/events/<jobId>`), emitting per-step progress (fetch → install →
generate → build → export → migrate → swap). One unified `rebuild(manifest)`
implementation replaces the separate install / version-change / revert / uninstall
implementations; HTTP entry points may stay distinct for the API surface but funnel
into one path.

**Deleted code** (large simplification): `swapPackageFiles`, `swapBaseFiles` /
`base_swap.go`, the `wsRoot/backups/` logic, in-place `copyDir`-over-live +
`os.RemoveAll` cleanup, the `tinycld.new` / `.prev` / `.failed` binary dance, and the
forward-revert migration-restore logic in `pkg_revert.go`. Replaced by:
`assembleBuild`, `runBuildPipeline`, `syncMigrations(oldSet, newSet)`,
`activateBuild(id)` (symlink flip), `pruneBuilds(N)`.

**Preserved logic:** version-compatibility solving (`pkg_compat.go` peerVersions),
the drop-report / schema-loss preview, seed ordering (`pkg_seed.go`), migration
ownership/naming (named UP/DOWN still works), and the health-probe contract.

## Validation

`tinycld/tests/install/run-todo-install.sh` — Docker-based, bind-mounts
`pb_data`/`builds`/`releases` to reproduce the malformed-DB corruption that only
manifests on bind-mounts. Drives a multi-phase Playwright flow: install v1 → upgrade
v2 → downgrade v1 → rollback-to-archived-v2 → delete → core upgrade/downgrade. The
`-v` mount flags update to the new `/workspace/<dir>` layout. Run repeatedly to shake
out bugs.

## Trust model & security

This pipeline runs the installed package's own code. That is **by design**, not a
flaw — but it defines a trust boundary every operator must understand before
installing anything.

**Installing a package runs its author's code as your server.** A "package"
(member) is not inert data the server merely reads. When a build runs:

- `pnpm install` executes the build scripts that the package and its dependency
  graph carry.
- The generator **evaluates** the member's `manifest.ts` to read its metadata.
- `go build` compiles the member's `server/` Go module **into the server binary**.
- `expo export` bundles the member's JavaScript into the web/native app.

The Go consequence is the sharpest one: a member's compiled-in server code runs
**in-process as the server**, on **every boot**, with the server's full privileges
— the PocketBase DB handle, the filesystem, and any secrets/env the process holds.
It is not sandboxed and could not usefully be: it *is* the server. There is no
runtime isolation boundary between "core" and an installed member's server code.

**Therefore: installing a package = trusting its author with full control of your
server and all its data.** Treat the decision exactly like deciding what code to
run on your own host, or what repo to let a CI runner build — it is the same class
of trust as `npm install` on a machine you own. **Only install packages you trust.**

**This is not an injection hole.** Package specs are strictly validated
(`validatePackageSpec` / `validateManifest` in `pkg_validate.go`): leading-dash
flag injection and shell metacharacters are rejected, paths are checked for
traversal, and every spec is handed to `npm pack` / `exec.Command` with **no shell**.
The risk being documented here is not "an attacker smuggles a command through the
spec" — it is the inherent one that the *legitimately installed* package's own
code runs with full privilege. The mitigation for that is trust in the source, not
input sanitization.

**Access is gated to super-admins.** Only a super-admin can reach the Admin console
and trigger an install/upgrade/core-upgrade; this is enforced server-side on the
install endpoints, not merely hidden in the UI. A non-super-admin cannot introduce
code into the build.

**What pnpm's `allowBuilds` covers — and does not.** `pnpm-workspace.yaml` declares
an `allowBuilds` allowlist (currently `esbuild` and `@sentry/cli`). Under pnpm 11,
**transitive dependency** install/postinstall scripts are blocked unless explicitly
allowlisted there — so a malicious *npm dependency pulled in by a member* cannot run
a postinstall script on its own. That allowlist does **not** cover the member's own
Go, JavaScript, or `manifest.ts`: those are compiled/bundled/evaluated by design and
run regardless of `allowBuilds`. `allowBuilds` narrows the dependency-postinstall
surface; it does nothing to constrain the member code we deliberately build and run.

**Why build-pipeline sandboxing isn't attempted.** Sandboxing the *build* (the
generator's `vm.runInNewContext` manifest read, or the `pnpm`/`go`/`expo`
subprocesses) would be security theater here: even a perfectly sandboxed build
still produces a server binary with the member's Go compiled in, which then runs
**unsandboxed as the server** at runtime. The trust boundary is at install
authorization and source selection, not inside the build — so we don't pretend the
build is a security boundary. (The manifest parser's `vm.runInNewContext` is a
robustness/scoping convenience, not a security sandbox; Node's `vm` is escapable
and does not need to be airtight, because member code runs as the server anyway.)

**Deferred mitigation (not yet implemented).** A server-enforced install
allowlist / scope policy — e.g. refusing any spec outside an operator-configured
set such as `@tinycld/*` only, enforced authoritatively on the install endpoint —
is the planned next step to give operators a hard guardrail. It is **not** built in
this round: today the only signal is the install-time warning for non-`@tinycld/`
specs (advisory, not enforced). This section documents the accepted trust model in
the meantime; it is not a bug to be "fixed" by a future reader.

## Affected files (initial map)

- `core/server/coreserver/pkg_install.go` — `resolveServerDir`, install handler →
  `rebuild`; add `resolveStateDir`.
- `core/server/coreserver/pkg_version_change.go`, `base_swap.go`, `pkg_revert.go` —
  replaced by the rebuild/activate/sync/prune functions; in-place swap logic deleted.
- `core/server/coreserver/pkg_build.go`, `pkg_go_build.go`, `pkg_migrate.go`,
  `pkg_restart.go`, `static.go` — repoint state paths at `resolveStateDir()`.
- `core/server/coreserver/pkg_seed.go`, `pkg_compat.go` — preserved, possibly
  re-wired into the pipeline.
- `config/entrypoint.sh` — symlink-aware binary path, symlink-flip rollback,
  relocated mounts.
- `Dockerfile`, docker-compose / `deploy.sh`, `tests/install/run-todo-install.sh` —
  mount targets move to `/workspace/<dir>`.
- New scaffold-templating module (inlined from bootstrap's `assemble-workspace.ts`).
