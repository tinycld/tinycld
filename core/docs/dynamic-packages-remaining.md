# Dynamic Package Registry — Remaining Work

> **⚠️ This plan predates the package extraction (2026-04-18).** Packages now live in sibling git repositories (linked via `pnpm run packages:link`) instead of a `packages/*` workspace directory. The Phase 2/3 install pipeline described below assumes the old layout and needs rework: `pnpm pack` unpacking into `packages/<slug>/` should become `git clone ../<slug>` + `packages:link`, and the generation/build steps stay largely the same. See `docs/packages.md` for the current architecture. This doc is kept as historical planning context.

## Phase 1 Remaining Items

### Not yet implemented

1. **`pkg_install_log` collection** — Audit trail for install/uninstall actions. Fields: `pkg` (relation→pkg_registry), `action` (install/uninstall/enable/disable), `performed_by` (relation→users), `details` (json), `created` (autodate).

2. **`lib/packages/use-pkg-registry.ts`** — Client-side hook to read `pkg_registry` from the database and merge with the generated registry. Currently `usePackages()` still returns only the static generated registry; the DB is only checked in `useAccessiblePackages()`.

3. **Merge DB + generated registry in `usePackages()`** — `lib/packages/use-packages.ts` should merge the generated `packageRegistry` with DB records so that future dynamically-installed packages appear everywhere (rail, sidebar, settings) without needing a rebuild.

4. **Read package list from DB in generation script** — `scripts/generate-packages.ts` currently reads only from `tinycld.packages.ts`. Phase 1 planned for it to also read a DB-exported JSON so dynamically-registered packages get included in the next build.

5. **Workspace rail/sidebar badges** — Show status indicators (e.g. "installing...", "update available") on packages in the navigation rail.

6. **`org_pkg_enabled` API rules** — Currently create/update/delete rules are `null` (superuser only). Org admins need write access so the toggle UI works without superuser auth. Rules should be something like: `createRule: 'org.user_org_via_org.user ?= @request.auth.id'` with an admin role check, or the mutations should go through a server endpoint.

---

## Phase 2 — JS-Only Package Pipeline

Automated installation for packages that don't include Go server code.

### Server-side pipeline

1. **`server/pkg_installer.go`** — Core installation pipeline:
   - Validate permissions and bun package name
   - `pnpm pack` to temp directory, unpack, read manifest, validate structure
   - Copy to `packages/<slug>/`, run `pnpm install`
   - Execute `pnpm exec tsx scripts/generate-packages.ts`
   - Run `pnpm run build:web` to rebuild Expo routes
   - Rollback on failure: delete temp dir, restore previous files, re-run generation

2. **`server/pkg_endpoints.go`** — HTTP endpoints:
   - `POST /api/admin/packages/install` — trigger installation
   - `POST /api/admin/packages/uninstall` — trigger removal
   - `GET /api/admin/packages/:slug/status` — check install progress
   - `GET /api/admin/packages/:slug/logs` — stream install logs

3. **Install progress UI**:
   - `components/setup/PackageInstaller.tsx` — Form for bun package name/URL input
   - `components/setup/PackageInstallLog.tsx` — Real-time progress viewer (SSE or polling)
   - Status badges on package list during installation

4. **Package allowlist** — Only `^@tinycld/` packages by default; superadmin override for third-party packages.

5. **Manifest validation** — Strict structure validation; reject path traversal attempts. Parse manifests as JSON, not with `new Function()`.

---

## Phase 3 — Go Package Support (Full Pipeline)

Automated installation for packages that include Go server extensions. Hardest phase.

### Additional pipeline stages

1. **Go compilation** — `go build -o server/tinycld.new ./server/` to new binary path, health check validation.

2. **Migration execution** — Take SQLite backup, run migrations via new binary, validate success.

3. **Binary swap & graceful restart**:
   - Rename `server/tinycld` → `server/tinycld.prev`
   - Rename `server/tinycld.new` → `server/tinycld`
   - Signal running process to gracefully restart
   - Options: process manager restart (systemd/Docker) or `cloudflare/tableflip` for zero-downtime

4. **`server/pkg_rollback.go`** — Rollback stack and recovery logic:
   - Each stage maintains a rollback stack with description and undo function
   - On failure: execute rollback steps in reverse, update status to `failed`, write error
   - SQLite backup before binary swap as safety net
   - Binary fallback to `tinycld.prev` if new binary fails to start

### Security

- Go code requires explicit superadmin approval before installation
- Integrity checking — verify bun package tarball integrity hash
- Sandboxed JS hooks — PocketBase jsvm provides isolation for third-party hooks
- Permission scoping — only org owners can install (not admins)
