# Core capability hooks

Core provides Go-implemented **host-side capabilities** that packages drive
through **manifest config** — no feature Go runs in the host. This is the seam
that lets a package contribute protocol servers (CardDAV), full-text search, and
audit logging as *data*, while the trusted Go implementation lives in core
(`core/server/{carddav,fts,audit}/` and `core/server/coreserver/`).

The same config drives both deployment models:

- **Single-tenant app** (`tinycld/server`): `coreserver.Register` reads each
  package's config from the generated `server/bundled-packages.json` and wires the
  capabilities against the one shared app.
- **Multi-org host** (`multi-org` router): the publish path emits a parsed
  `manifest.json` into the package store; `orgmanager` reads it per tenant and
  wires the capability against that org's stock-PocketBase app.

A package author writes each config block **once** in `manifest.ts`.

---

## The `$`-binding seam (`coreserver/jsvm_binds.go`)

The multi-org PocketBase fork calls `jsvm.Config.OnInit` on every JS VM (hook +
callback pools). Core uses it to install native `$`-bindings that package
`.pb.ts` hooks can call — the single, minimal, auditable surface untrusted tenant
TS is allowed to reach.

- `RegisterJSVMBinder(fn)` — a core sub-package registers a binder that installs a
  `$name` namespace onto each VM, without `coreserver` importing it (mirrors the
  `SetShareSessionResolver` decoupling).
- `NewBindNamespace(vm, members)` — helper to build a `$name` object from Go funcs.

**Design rule:** data-plane work triggered by record events (FTS sync, audit,
thumbnails) is a **core Go record hook driven by config, NOT a binding** — it
never crosses the VM boundary and a tenant can't skip it. `$`-bindings are
reserved for logic that must *originate* in package TS (rare). The `$fts` binding
exists for imperative queries but is unused by the contacts pilot.

---

## CardDAV (`carddav/`)

Serves the CardDAV/RFC-6352 protocol (PROPFIND/REPORT, ETags, Basic-Auth, vCard
encode/decode via `go-webdav` + `go-vcard`) for any collection, driven by a
package's `carddav` manifest block. The vCard codec stays Go; only the field map
is data.

### Handlers / entry points

| Symbol | Purpose |
|---|---|
| `Register(app, sources)` | Single-tenant: mounts `/carddav`, `/carddav/{path...}`, `/.well-known/carddav` on the app router with `sharedDBScope`. No-op when `sources` is empty. |
| `HandlerFor(app, sources) http.Handler` | Multi-org: a standalone `http.Handler` for ONE org (tenant), using `singleOrgScope`. Applies the Basic-Auth challenge itself. Returns nil when no sources. |
| `HasPrefix(path)` / `Prefixes()` | Report the URL prefixes the handler owns, for a composing router (`orgmanager.prefixMux`). |

### `OrgScope` — the single-tenant/multi-org difference

The protocol mechanics, codec, and config are identical across models; only org
resolution differs, behind `OrgScope`:

- **`sharedDBScope`** (single-tenant): one process holds every org's data in one
  DB. Books = one per the user's `user_org` membership; the book path carries the
  org slug; owner id = the user's `user_org` row for that org.
- **`singleOrgScope`** (multi-org tenant): the process **is** one org (stock PB
  per tenant, dispatched by subdomain). One book (`/carddav/u/ab/default/`); owner
  id = the user's single `user_org` row in this DB; no slug in paths.

### Manifest block

```ts
carddav: {
    collection: 'contacts',
    listFilter: "owner = {:ownerId} && deleted_at = ''", // {:ownerId} bound per request
    sort: '-updated',
    ownerField: 'owner',        // relation set on new records
    uidField: 'vcard_uid',
    softDeleteField: 'deleted_at', // DELETE stamps this instead of hard-deleting
    vcard: {
        version: '4.0',
        name: { given: 'first_name', family: 'last_name' }, // composes FN + N
        simple: { EMAIL: 'email', TEL: 'phone', ORG: 'company', TITLE: 'job_title', NOTE: 'notes' },
        revField: 'updated',    // emitted as vCard REV
    },
}
```

`vcard.simple` keys are vCard property names (`EMAIL`, `TEL`, …); values are the
record field. Empty simple fields are omitted from the card.

---

## Full-text search (`fts/`)

Owns SQLite FTS5 index sync + query for any collection, driven by an `fts`
manifest block. The FTS5 virtual table is created by the package's JS migration;
this capability only reads/writes it.

### Handlers / entry points

| Symbol | Purpose |
|---|---|
| `Register(app, configs)` | Binds index-sync record hooks (create/update/delete) and, on serve, a `GET /api/{slug}/search` route per config. |
| `RegisterSync(app, cfg)` | The index-sync hooks alone (idempotent delete-then-insert on `NonconcurrentDB()`). Core-registered — never triggered by TS. |
| `Search(app, cfg, userID, opts)` | Owner-scoped FTS5 `MATCH`, ordered by rank; returns typed columns + total. |
| `SanitizeQuery(input)` | FTS5-safe AND-ed prefix terms (handles email punctuation). |
| `JSVMBinder(configs)` | Installs the `$fts.search` binding (for future imperative TS use; unused by the pilot). |

### Manifest block

```ts
fts: {
    collection: 'contacts',
    table: 'fts_contacts',          // created by the package's pb-migration
    columns: [
        { fts: 'first_name', field: 'first_name' },
        { fts: 'notes', field: 'notes', strip: true }, // strip HTML before indexing
    ],
    owner: { field: 'owner', via: 'user_org', userField: 'user' }, // scopes search to the user's orgs
    output: [                        // columns the search route returns per hit
        { column: 'first_name' },
        { column: 'favorite', type: 'bool' }, // coerce so JSON shape matches the schema
    ],
    softDeleteField: 'deleted_at',   // ?deleted=true searches within soft-deleted rows
}
```

`output[].type` (`bool` | `number`; omit for string) coerces the value so a bool
column doesn't come back as the truthy string `"0"`.

---

## Audit (`audit/`)

Registers audit-log hooks for a collection from config, replacing the former
per-package `audit.RegisterCollection` Go call.

### Handlers / entry points

| Symbol | Purpose |
|---|---|
| `RegisterFromDescriptors(app, descriptors)` | Wires create/update/delete audit hooks for each descriptor. |
| `Descriptor` / `OrgVia` | The declarative form (collection, org-resolution-via-relation, label fields/join). |

### Manifest block

```ts
audit: [
    {
        collection: 'contacts',
        resolveOrg: { field: 'owner', collection: 'user_org', orgField: 'org' }, // resolve-via-relation
        labelFields: ['first_name', 'last_name'], // joined with labelJoin (default " ")
    },
]
```

`audit` is a list — a package may audit several collections. Omit `resolveOrg` /
`labelFields` to fall back to core's default resolver / extractor.

---

## How config reaches core

| | Single-tenant | Multi-org tenant |
|---|---|---|
| Source | `server/bundled-packages.json` (generator emits full manifest) | `manifest.json` in the package store (publish emits it via esbuild→sobek) |
| Reader | `coreserver.loadFTSConfigs` / `loadCardDAVConfigs` / `loadAuditDescriptors` | `controlplane.CardDAVSources` (router), fed to `orgmanager.Config.CardDAVSources` |
| Wiring | `coreserver.Register` | `orgmanager.load` → `composeMux` (prefix-routes `/carddav` to the handler, else the stock mux) |

Malformed config **fails loud at boot** (single-tenant `log.Fatalf`), mirroring
`assertSafeImportField` — a silently-unwired capability is worse than a clear
boot failure.

---

## Adding a capability to a new package

1. Add the `carddav` / `fts` / `audit` block(s) to the package's `manifest.ts`.
2. Ensure the referenced collections + any FTS5 virtual table exist in the
   package's `pb-migrations`.
3. Move any remaining imperative logic to a `.pb.ts` hook in `pb-hooks/`.
4. Drop the package's `server` field (no feature Go); regenerate.

See the `contacts` package for the reference end-to-end example.
