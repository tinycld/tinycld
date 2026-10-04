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
| The UI extensions route is bound once per app, so an app can build more than one router | — | `apis/extensions.go` (handler `Id`) |
| The collection's view rule gates every file download, not only a `protected` one | `apis/file_tinycld.go` | `apis/file.go` (one call); `apis/file_test.go` relaxes the fixture's users view rule |
| Realtime: send a `delete` to a subscription that a record leaves on update | `apis/realtime_leave.go` | `apis/realtime.go` |
| After-success hooks run with the app that started the write, not with a finished transaction that a hook swapped into `e.App` | — | `core/db.go` (`event.App = app`, three places) |
| `NoAttachDBConnect`: connections that cannot `ATTACH` another database file | `core/db_noattach*.go` | `core/base.go` (`ReapplyNoAttachLimits` after each pool is configured) |
| Cross-instance notify events are not lost on kqueue (macOS/BSD) | `core/notify_watcher_tinycld.go` | `core/notify_watcher.go`; `core/notify_watcher_test.go` fixes a race in the test |
| `core.OnFilesystemDelete`: the storage delete hook, exported so a backup can hold deletes | `core/filesystem_hooks_tinycld.go` | — |
| `apis.SetRedirectListener`: Serve's HTTP->HTTPS redirect server uses an injected listener instead of binding `config.HttpAddr` itself. `apis.SetRedirectServerHook`: a hook gets the redirect server and its listener before it serves, and returns the listener to serve on | `apis/serve_redirect_tinycld.go` | `apis/serve.go` (one call) |

### Why `core/db.go` sets `event.App = app`

A hook may swap `e.App` for a transaction that it opened around `e.Next()`
(`e.App = txApp`). That transaction has ended when the after-success hooks
run. Upstream's error path already resets `event.App`. The fork does the same
on the success path, because otherwise the after hooks (the realtime access
checks among them) query through a finished transaction and silently fail.
