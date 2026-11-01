# PocketBase fork

This directory is a vendored copy of [PocketBase](https://github.com/pocketbase/pocketbase)
with tinycld changes on top. The upstream release it is based on is the top
entry of `CHANGELOG.md`.

## Upgrading to a new upstream release

The branch `vendor/pocketbase` holds only pristine upstream releases, at this
same path, with the admin UI sources stripped (the fork embeds `ui/dist`). The
fork's history merges that branch, so an upgrade is an ordinary 3-way merge:
git applies upstream's changes and stops only where they touch our lines.

1. Run `scripts/pocketbase-vendor.sh <tag>` (for example `v0.40.4`). It
   commits the release onto `vendor/pocketbase`.
2. Run `git merge vendor/pocketbase` on your working branch and resolve the
   conflicts. The rules below tell you where our changes are.
3. Update the `go` directive and the dependencies if upstream bumped them,
   then run `go mod tidy` in this directory, `tinycld/core/server` and
   `tinycld/server`.
4. Run `go test ./...` in this directory.
5. Push `vendor/pocketbase` together with your branch.

Never edit `vendor/pocketbase` by hand. It must stay identical to upstream, or
the next merge will silently revert or duplicate changes.

## Rules for fork changes

The goal is that an upstream merge touches as few of our lines as possible.

- **Put new code in new files.** Name them `*_tinycld.go` (or give them a
  name upstream does not use, as `realtime_leave.go` and `db_noattach.go` do).
  Put new tests in `*_tinycld_test.go`.
- **Change an upstream file with a single call into a fork file**, not with
  inline logic. Mark the line with `// fork:`.
- **Do not reformat or reword upstream code or comments.**
- **The JS engine is `github.com/grafana/sobek`, not `dop251/goja`.** Upstream
  files import it as `goja "github.com/grafana/sobek"` so their code stays
  identical to upstream. Fork files import it as `sobek`. The node.js
  modules (`buffer`, `console`, `process`, `require`, `util`) are a sobek port
  of `goja_nodejs` in `plugins/jsvm/internal/nodejs`.
- **Fork-only `jsvm.Config` fields go in the block at the end of the struct.**

## What the fork changes

| Change | Fork files | Touch points in upstream files |
|---|---|---|
| sobek in place of goja | `plugins/jsvm/internal/nodejs/` | import lines in `plugins/jsvm/*.go`, `go.mod` |
| jsvm: sandbox mode, execution budget, embedded hooks/migrations (`HooksFS`/`MigrationsFS`), private migrations list, shared compiled programs (`ProgramSource`), loader-only init (`OnLoaderInit`), rejection of module syntax | `plugins/jsvm/jsvm_tinycld.go`, `callable.go`, `program_source.go`, `transform.go` | `jsvm.go` (Config block and one-line calls), `binds.go` (`executors.mustCompile`), `pool.go` (`tinycld.run`) |
| `apis.BuildServeMux`: build the HTTP handler without starting a server | `apis/serve_tinycld.go` | `apis/serve.go` (the router setup moved to `buildBaseRouter`) |
| Each app keeps its own list of JS migrations: jsvm registers into `core.OwnAppMigrations(app)`, and `RunAppMigrations`, `RunAllMigrations` and `migrate up/down` read `core.AppMigrationsOf(app)` (`core.AppMigrations` plus the app's list) | `core/migrations_app_tinycld.go`, `plugins/jsvm/jsvm_tinycld.go` (`migrationsTarget`), `plugins/jsvm/migrations_runtime_tinycld.go` | `core/base.go` (one call each in `RunAppMigrations` and `RunAllMigrations`), `plugins/migratecmd/migratecmd.go` (one call) |
| jsvm: a migration file's runtime does not outlive the call that needs it. Registration runs the file's top level once and drops the runtime; each registered `Up`/`Down` keeps the file's source (interned with `unique.Make`) and its `migrate()` call index, and builds a new runtime, runs the top level again and calls its function within the budget. Upstream kept one runtime per file for the app's life (about 0.2 MiB each) | `plugins/jsvm/migrations_runtime_tinycld.go` | `plugins/jsvm/jsvm.go` (the `registerMigrations` loop body is one `runtimes.register` call; the runtime set-up moved to `migrationRuntimes.newVM`, so an upstream change to those bindings must be copied there) |
| jsvm: a relative `require` from a hook or migration file resolves from the app's hooks dir, and a bare module name is looked up in `<hooksDir>/node_modules` first, so apps in one process do not share the working directory's modules | `plugins/jsvm/jsvm_tinycld.go` (`newRequireRegistry`, `hooksDirResolver`) | `plugins/jsvm/jsvm.go` (the two `newRequireRegistry` calls pass the hooks dir) |
| An app's notify guard is dropped after its terminate, so a process that runs many apps over time does not keep one per app | `core/notify_watcher_tinycld.go` (`forgetNotifyGuard`) | `core/notify_watcher.go` (one deferred call at the top of the `OnTerminate` handler) |
| The UI extensions route is bound once per app, so an app can build more than one router | — | `apis/extensions.go` (handler `Id`) |
| The collection's view rule gates every file download, not only a `protected` one | `apis/file_tinycld.go` | `apis/file.go` (one call); `apis/file_test.go` relaxes the fixture's users view rule |
| Realtime: send a `delete` to a subscription that a record leaves on update | `apis/realtime_leave.go` | `apis/realtime.go` |
| Realtime resume: a client outlives its SSE connection for up to 10 minutes, its events carry a per-client `seq`, and `GET /api/realtime?resume=<clientId>&after=<seq>` from the same auth record and IP resumes it (`"resumed":true` in `PB_CONNECT`). While detached, a create or update is not rule-checked for it, only noted; the resume sends each noted record as it is then. A delete is still rule-checked and queued when it happens. `POST /api/realtime/release` lets the subscriber drop a client it will not resume | `apis/realtime_resume.go`, `tools/subscriptions/resumable_tinycld.go` | `apis/realtime.go` (client creation, register, release and the `PB_CONNECT` payload in `realtimeConnect`; one bind call in `bindRealtimeEvents`; the `/release` route in `bindRealtimeApi`; one `realtimeDeferChange` call in `realtimeBroadcastRecord`) |
| After-success hooks run with the app that started the write, not with a finished transaction that a hook swapped into `e.App` | — | `core/db.go` (`event.App = app`, three places) |
| `NoAttachDBConnect`: connections that cannot `ATTACH` another database file. Each pool keeps its own cap (at most 24), so the single-writer pools stay at 1 connection. The record of a restricted pool is weak, so a closed pool is not kept for the life of the process | `core/db_noattach*.go` | `core/base.go` (`ReapplyNoAttachLimits` after each pool is configured) |
| Cross-instance notify events are not lost on kqueue (macOS/BSD) | `core/notify_watcher_tinycld.go` | `core/notify_watcher.go`; `core/notify_watcher_test.go` fixes a race in the test |
| A notify reload does not run after `ClearBootstrap` (its DB handles are nil, and the query panics in the timer goroutine), and `ClearBootstrap` waits for a reload in progress. The watcher goroutine reads a `live` flag, not `IsBootstrapped`, which races the DB handles that `Bootstrap`/`ClearBootstrap` write | `core/notify_watcher_tinycld.go` (`enterNotifyReload`, `notifyLive`, `bindNotifyClearGuard`) | `core/notify_watcher.go` (one bind call in `registerNotifyWatcherHooks`; the guard at the top of the debounce callback; `notifyLive` in the event filter; `trackNotifyWatcher` as the WaitGroup of the watcher loop's `FireAndForget`, so `ClearBootstrap` stops the watcher and its loop instead of leaking them); `core/base.go` (one `markNotifyLive` call at the end of `Bootstrap`'s body, not a hook handler) |
| `ClearBootstrap` waits for the background storage deletes that follow a delete of a model with files, so none recreates `<dataDir>/storage` after it | `core/files_delete_tinycld.go` | `core/base.go` (a `fileDeletes` field at the end of `BaseApp`; its init and the WaitGroup argument of `FireAndForget` in `registerBaseHooks`; one call at the top of `ClearBootstrap`). Plain calls, not hook handlers, so no app's hook chain grows |
| A failed `Bootstrap` leaves no pool open: `ClearBootstrap` on an app that is not bootstrapped closes the pools a failed `Bootstrap` stored (no `OnBootstrapClear` hook runs), and a pool init that fails closes the pools it opened before the failure | `core/bootstrap_partial_tinycld.go` | `core/base.go` (one call in `ClearBootstrap`'s not-bootstrapped branch; three `return closeOpened(...)` lines each in `initDataDB` and `initAuxDB`) |
| `core.OnFilesystemDelete`: the storage delete hook, exported so a backup can hold deletes | `core/filesystem_hooks_tinycld.go` | — |
| `apis.SetRedirectListener`: Serve's HTTP->HTTPS redirect server uses an injected listener instead of binding `config.HttpAddr` itself. `apis.SetRedirectServerHook`: a hook gets the redirect server and its listener before it serves, and returns the listener to serve on | `apis/serve_redirect_tinycld.go` | `apis/serve.go` (one call) |
| `cron.Cron.SetSkip`: a function the scheduler asks before it starts each due job; it can hold the job back for that tick (core uses it to pause every cron job while the server is read-only) | `tools/cron/cron_tinycld.go` | `tools/cron/cron.go` (a `skip` field at the end of the struct; one call in `runDue`) |

### Why `core/db.go` sets `event.App = app`

A hook may swap `e.App` for a transaction that it opened around `e.Next()`
(`e.App = txApp`). That transaction has ended when the after-success hooks
run. Upstream's error path already resets `event.App`. The fork does the same
on the success path, because otherwise the after hooks (the realtime access
checks among them) query through a finished transaction and silently fail.
