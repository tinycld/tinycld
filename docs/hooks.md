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
no feature package itself), plus two seams between Go and TS:

| Seam | Direction | For |
|---|---|---|
| `$`-bindings | TS → Go | logic that stays in Go but must be *callable* from TS (raw SQL, protocol codecs) |
| Hook points | Go → TS | letting a deployment intercept a decision Go owns (veto a write, hide a listing entry) |

Hook points are opt-in and cost one atomic load when unused — see the section
below for why that matters.

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
| `tinycld.org/core/caldav` | CalDAV/RFC-4791 protocol server + iCalendar codec over a calendars+events collection pair. Same `Register` / `HandlerFor` split. A `Source` carries the two collection names, the field maps, `Defaults` for schema-required fields iCalendar may omit, and an optional `OnError` reporter. Authorization is NOT a config field: core evaluates the collections' own PocketBase rules via `app.CanAccessRecord`. Exposes four TS hook points (below). |
| `tinycld.org/core/fts` | SQLite FTS5 index sync + search for any collection. `Register(app, []Config)` binds index-sync record hooks + a `GET /api/{slug}/search` route; `Search(app, cfg, userID, opts)` runs an owner-scoped query. The FTS5 virtual table is created by the package's pb-migration. |
| `tinycld.org/core/audit` | `RegisterCollection(app, name, *CollectionConfig)` — binds create/update/delete audit hooks writing to `audit_logs`, with field diffs, delete snapshots, redaction, and a customizable label extractor. |
| `tinycld.org/core/coreserver` | The `$`-binding seam — `RegisterJSVMBinder` / `NewBindNamespace` (below). Plus subsystems: `notify`, `push`, `mailer`, `render`, `thumbnails`, `textextract`, `sharelink`, … |

These are **libraries, not boot-time wiring**: a package contributes the config
(a `carddav.Source`, an `fts.Config`, an audit label extractor) from its own Go so
there is exactly one copy of the heavy protocol/index code, shared by every
consumer — the single-tenant app (via each package's `Register`) and the multi-org
host (which imports `carddav.HandlerFor` directly to serve tenants, which link
no feature package of their own). See `contacts/server/register.go` for the
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

## Hook points: calling TS *from* Go (`coreserver/ts_hooks.go`)

`$`-bindings run TS→Go. A **hook point** runs the other direction: Go invokes
registered package TS at a place the host owns — a protocol handler, a request
path — and uses what it returns. This is for behaviour a declarative config
can't express ("block this filename", "hide these entries") without turning the
feature's whole data model into configuration.

### The performance contract

**The fast path never touches a JS VM.** A hook point holds an `atomic.Bool`; a
point nobody registered costs one atomic load. This is not an optimization
detail — it is why the seam is safe to put on a hot path at all. jsvm's runtime
pool falls back to constructing an entire new `sobek.Runtime` (the full bind
chain, tens of milliseconds) once every pooled executor is busy, which is
exactly what a fan-out like a WebDAV `PROPFIND` would trigger. Deployments that
customize nothing pay nothing; only those that register a handler take the cost.

Always gate on `Enabled()` before building a payload:

```go
if hp.Enabled() {
    res, err := hp.Call(map[string]any{"name": name, "userId": userID})
    // …
}
```

### Go side

```go
// Declare the point (idempotent — same name returns the same point).
hp := coreserver.RegisterHookPoint("webdav.drive.beforeWrite")

// Install the JS-facing registration binding. This must be a LOADER binder:
// OnInit fires on every pooled VM, so a binding whose job is to REGISTER a
// handler would otherwise register it once per runtime.
coreserver.RegisterLoaderBinder(func(loader *sobek.Runtime, compile jsvm.Compiler, app *pocketbase.PocketBase) error {
    return loader.Set("myHook", func(handlerSource string) error {
        fn, err := compile(handlerSource)
        if err != nil { return err }
        hp.Add(fn)
        return nil
    })
})
```

`Call` returns `HookResult{Value, Handled}`. `Handled` distinguishes "no handler
registered" from "a handler ran and returned a zero value". A handler error
aborts the chain and is returned — for a veto point, that *is* the rejection.

### Ordering (this bites)

`jsvm.Register` executes the hook files **synchronously**, so every binding a
package contributes must exist before it runs. `coreserver.Register` therefore
calls `RegisterExtras` (which registers feature packages) *before*
`jsvm.MustRegister`. Register bindings from your package's `Register(app)` and
you are in time; register them later and the package's `.pb.ts` dies at boot
with `<yourHook> is not defined`.

### Handler forms

A handler reaches Go as a **source string**, and the three ways of writing one
stringify differently:

```ts
myHook({
    beforeWrite(e) {…},              // → "beforeWrite(e) {…}"   (method shorthand)
    canRead: function (e) {…},       // → "function (e) {…}"
    filterList: (e) => {…},          // → "(e) => {…}"
})
```

Method shorthand is a *method definition*, not an expression, so it must be
normalized (prefix `function `) before compiling — see
`core/server/webdav/tshooks_register.go`. Handlers are also recompiled
standalone, so **a handler closes over nothing**: everything it needs must be
inside its own body.

### Security rule

**A hook point may restrict a decision, never widen one.** Apply the
authoritative check in Go first and treat the hook's answer as an additional
constraint. Where a hook returns a collection (a filtered listing), intersect it
with what Go authorized rather than trusting it — otherwise a handler could name
records the caller was never granted. `core/server/webdav/hooks.go` is the
reference implementation.

### In use: WebDAV and CalDAV

`core/webdav` is the first consumer, exposing five points via a single
`webdavHook({...})` binding — `beforeWrite`, `beforeDelete`, `beforeMove`,
`canRead`, `filterList`. Points are namespaced per source slug
(`webdav.<slug>.<hook>`) so two packages serving trees never share a handler.
`filterList` receives a whole directory batch, so a listing costs one VM borrow
rather than one per entry. See `drive/help/webdav-hooks.md` for the
administrator-facing description.

`core/caldav` is the second, with `caldavHook({...})` and four points —
`beforeWrite`, `beforeDelete`, `canRead`, `filterList` — namespaced
`caldav.<slug>.<hook>`. There is no `beforeMove`: CalDAV has no cross-calendar
move (a client relocating an event PUTs it to the new calendar and DELETEs the
old copy, which the write and delete points already see). `filterList` batches a
whole calendar's UIDs. See `calendar/help/caldav-hooks.md`.

> **Both are single-tenant only for now.** `serve-org` sets neither `OnInit` nor
> `OnLoaderInit`, so a tenant's VMs carry no bindings and package TS cannot
> register a handler there. Everything else — including every access check —
> works identically in a tenant, because authorization is a PB rule that travels
> in the schema, not a Go closure.

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
   Register any `$`-binding or hook-point binding from `Register(app)` — it runs
   before `jsvm`, which executes the hook files synchronously.
4. Ensure any collections + FTS5 virtual tables your Go reads exist in the
   package's `pb-migrations`.
5. Run `pnpm run packages:generate` (from `tinycld/`) to wire the Go server.

See the `contacts` package for the full reference.
