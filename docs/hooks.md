# Server-side extensions: package Go + TS hooks

A feature package extends the PocketBase server in two complementary ways:

- **Go** — the package ships its own Go module (`server/`, declared by the
  manifest's `server: { package, module }` field). Its `Register(app)` is linked
  into the app-shell binary by the generator and called once at boot. This is
  where protocol servers (CardDAV/CalDAV), raw-SQL work (FTS5), record hooks, and
  custom HTTP routes live.
- **TypeScript** — `.pb.ts` hooks in the package's `pb-hooks/` directory (declared
  by `hooks: { directory: 'pb-hooks' }`) run on the sobek jsvm alongside the Go.
  This is the **customization seam**: authors and downstream integrators add or
  layer behavior without forking the package's Go.

Core provides **reusable Go helpers** a package's `Register(app)` calls (it links
no feature package itself), plus the `$`-binding seam that lets a package's Go
expose native functions to TS hooks.

> The `contacts` package is the end-to-end reference: `contacts/server/` ships
> CardDAV, FTS + `/api/contacts/search`, audit logging, and a `vcard_uid` autogen
> hook, and exposes `$contacts.search(...)` to TS.

---

## Package Go server

Declare it in `manifest.ts`:

```ts
server: { package: 'server', module: 'tinycld.org/packages/<slug>' },
```

`package` is the directory (relative to the package root) holding the Go module
(`go.mod` lives there); `module` is that module's path (always
`tinycld.org/packages/<slug>`). The module exports one entry point:

```go
package <slug>

func Register(app *pocketbase.PocketBase) {
    // bind record hooks, mount routes on OnServe, register capabilities…
}
```

The generator (`scripts/generate.ts` → `gen-server.ts`) emits
`server/package_extensions.go` with a `<slug>.Register(app)` call and wires the
module into `server/go.work`, so no manual wiring is needed — the manifest field
plus an on-disk `server/` dir is enough. `main.go` passes
`registerPackageExtensions` to `coreserver.Register` via `Options.RegisterExtras`,
so **core never imports a feature** and still builds with zero packages linked.

A member's `server/go.work` (gitignored, generated) replaces `tinycld.org/core`
with the local copy and — when the PocketBase fork is checked out at
`<workspace>/pocketbase` — the fork too, so a standalone `go build`/`go test` from
`<member>/server` resolves the same sobek-forked core the assembled build uses.

### Reusable core helpers a package's Go calls

Core keeps these as **generic, config-driven libraries** any package server can
import and drive from its `Register(app)` — core does NOT auto-wire them at boot,
the package does, passing a config for its own collection:

| Package | What |
|---|---|
| `tinycld.org/core/carddav` | CardDAV/RFC-6352 protocol server + vCard codec for any collection. `Register(app, []Source)` mounts `/carddav` on serve (single-tenant); `HandlerFor(app, []Source)` returns a standalone `http.Handler` for one org (the multi-org host). A `Source` is the record↔vCard field map. |
| `tinycld.org/core/fts` | SQLite FTS5 index sync + search for any collection. `Register(app, []Config)` binds index-sync record hooks + a `GET /api/{slug}/search` route; `Search(app, cfg, userID, opts)` runs an owner-scoped query. The FTS5 virtual table is created by the package's pb-migration. |
| `tinycld.org/core/audit` | `RegisterCollection(app, name, *CollectionConfig)` — binds create/update/delete audit hooks writing to `audit_logs`, with field diffs, delete snapshots, redaction, and a customizable label extractor. |
| `tinycld.org/core/coreserver` | The `$`-binding seam — `RegisterJSVMBinder` / `NewBindNamespace` (below). Plus subsystems: `notify`, `push`, `mailer`, `render`, `thumbnails`, `textextract`, `sharelink`, … |

These are **libraries, not boot-time wiring**: a package contributes the config
(a `carddav.Source`, an `fts.Config`, an audit label extractor) from its own Go so
there is exactly one copy of the heavy protocol/index code, shared by every
consumer — the single-tenant app (via each package's `Register`) and the multi-org
host (which imports `carddav.HandlerFor` directly to serve stock-PB tenants that
have no feature Go of their own). See `contacts/server/register.go` for the
reference: it builds a `carddav.Source` + `fts.Config` and calls these.

Go server hooks use SDK methods that **bypass PocketBase API rules** — they
authorize manually. When changing a collection's API rules, check whether a Go
hook also accesses it and update its filters to match.

---

## The `$`-binding seam (`coreserver/jsvm_binds.go`)

The PocketBase fork calls `jsvm.Config.OnInit` on every JS VM (hook + callback
pools). A package's Go can install native `$`-bindings that its `.pb.ts` hooks
call — the auditable surface TS is allowed to reach into Go.

- `RegisterJSVMBinder(fn)` — register a binder that installs a `$name` namespace
  onto each VM. Call it from the package's `Register(app)` before serve.
- `NewBindNamespace(vm, members)` — build a `$name` object from Go funcs.

Example (contacts exposes a Go-backed search to TS):

```go
coreserver.RegisterJSVMBinder(func(vm *sobek.Runtime, app *pocketbase.PocketBase) error {
    obj, _ := coreserver.NewBindNamespace(vm, map[string]any{
        "search": func(userID string, opts map[string]any) (map[string]any, error) { /* … */ },
    })
    return vm.Set("$contacts", obj)
})
```

```ts
// in a pb-hooks/*.pb.ts hook:
const { items, total } = $contacts.search(userId, { q: 'ada', limit: 10 })
```

**Design rule:** reserve `$`-bindings for logic that must *originate* in package
TS or must stay in Go but be *called* from TS (raw SQL, protocol codecs). Ordinary
data-plane work (FTS index sync, audit) is a plain Go record hook the package
binds directly in `Register` — it never crosses the VM boundary.

---

## TypeScript hooks (`pb-hooks/`)

Drop a `.pb.ts` into the package's `pb-hooks/` dir and bind PocketBase events:

```ts
/// <reference path="../../tinycld/server/pb_data/types.d.ts" />

onRecordCreate((e) => {
    if (!e.record.getString('source')) e.record.set('source', 'web')
    e.next()
}, 'contacts')
```

- TS hooks run **alongside** the package's Go on the same record events — a TS
  author can react to `onRecordCreate/Update/Delete('<collection>')` that the Go
  server also handles, without touching the Go.
- Because the fork's TS→JS transpile wraps each callback, **top-level module
  bindings (a `const` or `function` declared outside the callback) are not in
  scope at request time** — keep everything a hook needs inside the callback body.
- Call `$`-bindings the package's Go exposes for Go-backed logic.

---

## Adding server behavior to a package

1. Create `<pkg>/server/` (a Go module `tinycld.org/packages/<slug>`) with a
   `Register(app)` that binds your hooks/routes and calls any core helpers
   (`audit.RegisterCollection`, `RegisterJSVMBinder`, …).
2. Add `server: { package: 'server', module: 'tinycld.org/packages/<slug>' }` to
   `manifest.ts`.
3. For TS extension points, add `hooks: { directory: 'pb-hooks' }` and ship a
   `pb-hooks/<name>.pb.ts` (even an example/placeholder documents the seam).
4. Ensure any collections + FTS5 virtual tables your Go reads exist in the
   package's `pb-migrations`.
5. Run `pnpm run packages:generate` (from `tinycld/`) to wire the Go server.

See the `contacts` package for the full reference.
