# Package System

Full technical reference for how TinyCld feature packages are developed,
linked into the app shell, and wired together by the generator. This is the
developer/agent-facing companion to the task-framed website docs at
https://tinycld.org/docs.

> **Scope.** This document describes the *mechanics* — file layout, the
> generator's exact outputs, bundler/test resolution, and the Go server
> wiring. For code-style and data-access conventions see `CONTRIBUTING.md`
> and the root `CLAUDE.md`.

## Contents

- [The big picture](#the-big-picture)
- [Anatomy of a feature package](#anatomy-of-a-feature-package)
- [The manifest](#the-manifest)
- [TypeScript schema integration](#typescript-schema-integration)
- [Linking a package](#linking-a-package)
- [The generator](#the-generator)
- [Bundler & test resolution](#bundler--test-resolution)
- [The Go (PocketBase) server side](#the-go-pocketbase-server-side)
- [Cross-package coupling](#cross-package-coupling)
- [Development loop & where to edit](#development-loop--where-to-edit)

---

## The big picture

TinyCld is an **Expo Router (React Native) + PocketBase** application.
Features are not built into the app — they are **independent sibling git
repos** that get *linked* into a single **app shell** at build time. The app
shell (`tinycld/`) is the only runnable artifact; it ties everything together.

The monorepo root (`~/code/tinycld/`) is a **pnpm workspace**. `pnpm-workspace.yaml`
lists the workspace members and pnpm settings — most importantly `nodeLinker: hoisted`
(an npm-like flat `node_modules`) and `strictPeerDependencies: false`. Linking is
**`pnpm install` at the workspace root**; the postinstall's `link-members` step
materializes the `node_modules/@tinycld/*` symlinks that make each member resolvable
by its package name (pnpm only links depended-on members itself).

Three kinds of code live in the ecosystem:

| Kind | Where it lives | Git | Has `manifest.ts`? | Discovered how |
|---|---|---|---|---|
| **App shell** | `tinycld/` (the repo root) | own repo | n/a | it *is* the runner |
| **`@tinycld/core`** | nested inside the shell repo at `tinycld/core/` | **no separate repo** | **no** | wired in explicitly |
| **Feature packages** | sibling repos (`mail/`, `contacts/`, `calc/`, `calendar/`, `drive/`, `text/`, `google-takeout-import/`) | each its own repo + remote | **yes** | auto-discovered |

The defining structural rule: **a directory is a feature package iff it
contains `manifest.ts`.** Core has no manifest, which is exactly why it is
treated as a library, not a feature.

```
~/code/tinycld/                       # pnpm workspace root (package.json + pnpm-workspace.yaml)
    node_modules/
        @tinycld/
            core     -> ../../tinycld/core  # pnpm workspace symlink
            mail     -> ../../mail          # pnpm workspace symlink
            contacts -> ../../contacts
            ...
    tinycld/                          # app shell = repo root (the only runnable thing)
        core/                         # @tinycld/core — nested member (own package.json, no manifest.ts)
        node_modules/                 # heavy deps (React/RN/Expo/…) live HERE
        scripts/generate.ts           # the generator (+ gen-*.ts helpers)
        tinycld.packages.ts           # getPackages() — reads workspace members
        tinycld.config.ts             # generated source of truth (gitignored)
        lib/generated/                # generated runtime artifacts (gitignored)
        server/                       # Go (PocketBase) — module tinycld.org/tinycld
    mail/   contacts/   calc/   ...   # sibling feature repos (workspace members)
```

There is **no hand-curated package list.** `tinycld.packages.ts::getPackages()`
enumerates the workspace member siblings that contain a `manifest.ts`, plus the
nested `@tinycld/core`. **The set of workspace members present = the set of
linked packages.** A fresh clone has only the nested `@tinycld/core` subtree;
developers clone the siblings they want and re-run `pnpm install`.

> **pnpm-hoisting reality.** With `nodeLinker: hoisted`, pnpm flattens the heavy
> framework deps into the **workspace-root `node_modules`** (an npm-like flat
> tree). Feature siblings declare React / React Native / Expo / pbtsdb /
> `@tanstack/*` as `peerDependencies` and carry no `dependencies` of their own,
> so they resolve those libraries by walking up to the root install. `@tinycld/core`
> declares the full framework peer set so pnpm auto-installs those into
> `core/node_modules/` and it typechecks standalone. Metro watches the workspace
> root and Vitest aliases resolve the same way (see
> [Bundler & test resolution](#bundler--test-resolution)).

---

## Anatomy of a feature package

A minimal package is three files at its repo root: `manifest.ts`,
`package.json`, `.gitignore`. Everything else is optional and declared in the
manifest. A real package (e.g. `contacts/`) looks like:

```
contacts/
    manifest.ts            # metadata + feature declarations (exported default)
    package.json           # name, exports map, peerDependencies (NO dependencies)
    tsconfig.json          # extends @tinycld/core/tsconfig.package-base.json
    .gitignore             # node_modules/, *.tsbuildinfo, lockfiles
    pb-migrations/         # PocketBase migrations (referenced by literal dir name)
    help/                  # in-app help markdown topics
    server/                # Go server extension (own go.mod)
    tests/                 # vitest unit + playwright e2e specs
    tinycld/contacts/      # ← the actual TS SOURCE lives nested here
        collections.ts     # registerCollections(...)
        types.ts           # ContactsSchema type
        seed.ts            # default async seed fn
        sidebar.tsx
        screens/  components/  hooks/  stores/
```

### Source nesting

The TypeScript source does **not** sit at the repo root — it lives at
`<repo>/tinycld/<slug>/`. The `package.json` `exports` map plus the tsconfig
`paths` are what make the public specifier `@tinycld/contacts/screens/index`
resolve to `./tinycld/contacts/screens/index.tsx`.

### `package.json`

```jsonc
{
    "name": "@tinycld/contacts",       // the canonical identity (pnpm links by this)
    "private": true,
    "type": "module",
    "exports": {
        "./package.json": "./package.json",
        "./manifest": "./manifest.ts",
        "./types": "./tinycld/contacts/types.ts",
        "./collections": "./tinycld/contacts/collections.ts",
        "./screens/*": "./tinycld/contacts/screens/*.tsx",   // wildcard — required
        "./seed": "./tinycld/contacts/seed.ts",
        "./sidebar": "./tinycld/contacts/sidebar.tsx",
        "./hooks/*": "./tinycld/contacts/hooks/*.ts",
        "./components/*": "./tinycld/contacts/components/*.tsx"
    },
    "scripts": { "typecheck": "tsc --noEmit --skipLibCheck" },  // ONLY this
    "peerDependencies": {                // everything is a PEER, never a dep
        "react": ">=19", "react-native": ">=0.83",
        "expo-router": ">=55.0.0", "pbtsdb": ">=0.5",
        "@tanstack/db": ">=0.6", "@tanstack/react-db": ">=0.1"
    }
}
```

Three rules enforced here:

- **Wildcard exports** (`./screens/*`) — Metro cannot resolve literal bracket
  entries like `./screens/[id]`. A single `"./screens/*": "./screens/*.tsx"`
  matches both `index` and `[id]`.
- **`peerDependencies` only, no `dependencies`** — siblings carry no runtime
  deps of their own (even heavy ones like `hyperformula`/`yjs` in calc are
  peers). pnpm's `nodeLinker: hoisted` flattens those peers into the
  **workspace-root** `node_modules` (a flat npm-like tree), so siblings resolve
  a single copy by walking up. Always run `pnpm install` at
  the **workspace root** (`~/code/tinycld/`), never inside an individual
  sibling — a per-sibling install would materialize a duplicate `react` /
  `react-native` / `pbtsdb` / `yjs`, and TypeScript would then see two copies
  of every type and emit hundreds of "Type X is not assignable to type X"
  errors.
- **No lint/test scripts** — the canonical Biome config is `tinycld/biome.json`
  (`root: false`); a minimal `root: true` config at the workspace root extends it
  (written by bootstrap/the generator, gitignored) so biome resolves from inside
  any member. A sibling ships **no** `biome.json` by default and inherits those
  rules; it adds one only to override a rule
  (`{ "root": false, "extends": ["../tinycld/biome.json"], … }`). The only script
  a sibling carries is `typecheck`.

### `tsconfig.json`

Siblings extend `@tinycld/core/tsconfig.package-base.json` (resolved by package
name) and declare only their own `~/tinycld/<slug>/*` self-alias. Every
`@tinycld/*` dependency — `@tinycld/core`, `@tinycld/app-generated`, and any
cross-sibling dep — resolves **by package name** through the
`node_modules/@tinycld/*` symlinks + each package's `exports` map under
`moduleResolution: bundler`, so **no sibling tsconfig needs a `paths` entry for
any `@tinycld/*` dep**:

```jsonc
{
    "extends": "@tinycld/core/tsconfig.package-base.json",
    "compilerOptions": {
        "baseUrl": ".",
        "paths": {
            "~/tinycld/contacts/*": ["./tinycld/contacts/*"] // own source only
        }
    },
    "include": ["tinycld/**/*.ts", "tinycld/**/*.tsx"],
    "exclude": ["node_modules", "server", "pb-migrations", "tests/**/*.spec.ts"]
}
```

### `.gitignore`

Every sibling must ignore `node_modules/`, `*.tsbuildinfo`, lockfiles, and
`.DS_Store`. If a `node_modules/` or lockfile slips in, delete both:

```sh
rm -rf ../<sibling>/node_modules ../<sibling>/package-lock.json
```

### Import conventions inside a sibling

```tsx
import { EmptyState } from '@tinycld/core/components/EmptyState'   // core
import { useOrgHref } from '@tinycld/core/lib/org-routes'          // core
import { ContactRow } from '../components/ContactRow'              // same pkg (relative)
import { Pressable } from 'react-native'                           // app shell node_modules
```

- `~/*` and `@tinycld/core/*` both resolve to core.
- Same-package modules use **relative** imports (`../components/...`).
- Siblings must **not** import each other directly — see
  [Cross-package coupling](#cross-package-coupling).

---

## The manifest

The default export of `manifest.ts` drives the entire generator. Only the four
base identifiers (`name`, `slug`, `version`, `description`) are required; every
other field opts the package into one generator capability.

```ts
const manifest = {
    name: 'Contacts',                          // human-readable
    slug: 'contacts',                          // URL segment + collection prefix
    version: '0.1.0',
    description: 'Shared contacts for your organization',

    routes: { directory: 'screens' },          // signed-in app routes
    publicRoutes: { directory: 'public-screens' }, // public top-level routes

    nav: {                                     // nav-rail entry
        label: 'Contacts',
        icon: 'users',                         // lucide-react-native name
        order: 10,
        shortcut: 'o',                         // single char; unique across packages
    },

    migrations: { directory: 'pb-migrations' },
    hooks: { directory: 'pb-hooks' },          // PocketBase JS hooks (and the
                                               // caldavHook/webdavHook seams)

    collections: { register: 'collections', types: 'types' },

    sidebar: { component: 'sidebar' },         // OR provider: { component: 'provider' }
    settings: [{ slug: 'provider', label: 'Provider', component: 'settings/provider' }],

    help: { directory: 'help' },
    seed: { script: 'seed' },
    tests: { directory: 'tests' },

    server: { package: 'server', module: 'tinycld.org/packages/contacts' },
    build: { script: 'build' },                // pre-bundle build artifact (e.g. webview)

    dependencies: ['drive'],                   // other package slugs (seed ordering)
}

export default manifest
```

### Field reference

| Field | Effect when present |
|---|---|
| `name` / `slug` / `version` / `description` | Required identifiers. `slug` is the URL segment and collection-name prefix. |
| `routes.directory` | Each file becomes an authenticated route under `app/a/(app)/<slug>/`. |
| `publicRoutes.directory` | Each file becomes a public top-level route under `app/<path>` (outside the authenticated `app/a/(app)/` group — e.g. drive's share routes). |
| `nav` | Adds a nav-rail entry. `shortcut` registers a `t <letter>` jump and must be unique (validated at generate time). |
| `migrations.directory` | `*.js` migrations symlinked into `server/pb_migrations/`. |
| `hooks.directory` | PocketBase JS hooks symlinked into `server/pb_hooks/`. |
| `collections` | `register` + `types` export subpaths; wires pbtsdb collections and the schema type. |
| `sidebar` / `provider` | A package may contribute a sidebar component **or** a context provider that wraps app children. The provider is emitted as a `{ load: () => import(...) }` thunk — not `lazy()` — and core resolves every provider before the route tree mounts (`core/lib/packages/provider-loader.ts`), so the package area never suspends on it. |
| `settings[]` | Org-administration panels (`slug`, `label`, `component`), admin-only, at `/settings/<pkgSlug>/<slug>`. See [Extension points](#extension-points-settings-panels-and-sidebar-slots) below. |
| `accountSettings[]` | Per-user panels, same shape as `settings`, listed in Settings' **Account** group and open to every role, at `/settings/account/<pkgSlug>/<slug>`. |
| `systemSettings[]` | Deployment-wide panels (`slug`, `label`, `component`, optional `keyPrefix`), owner-only, at `/settings/system/<pkgSlug>/<slug>`. |
| `slots[]` | Names of sidebar slots this package exposes for *other* packages to render into. Free-form strings; duplicates within one manifest are a generator error. Render with `<SidebarSlot target="<this-slug>" slot="<name>" />` from `@tinycld/core/components/sidebar-primitives`. |
| `sidebarContributions[]` | Inverse of `slots`: this package's contributions into *another* package's slot. Each `{ target, slot, component, order?, label? }` is generator-validated. `label` names the contribution where the host lays the slot out as tabs. `target: 'core'` renders into a slot core owns (see [Setup wizard steps](#setup-wizard-steps-setupsteps)). |
| `setupSteps[]` | Steps this package adds to the first-run setup wizard (`id`, `label`, `module`, `order?`). See [Setup wizard steps](#setup-wizard-steps-setupsteps) below. |
| `help.directory` | `<id>.md` topics and any images they reference (relative to the directory) surfaced in the in-app help hub. |
| `seed.script` | Dev sample-data function. |
| `server` | Go server extension: `package` is the subdir, `module` is its Go module path. The module exports ONE entry point, `Register(app *pocketbase.PocketBase)`, called by the generated registrar — the same call in every composition. |
| `ports[]` | Public TCP ports this package's server code listens on: `{ name, port, addrEnv?, enabled? }`. `name` is the stable name the server code asks for (`[a-z0-9-]+`, and not one of the reserved names `http` / `https` / `http-redirect`, which the supervisor uses for its own listeners). `addrEnv` names an env var that overrides the listen address; `enabled` (`{ env, default }`) names an env var that turns the port on or off. Generator-validated for duplicate names/ports across packages and materialized to `server/ports.json`; the supervisor that holds the public ports binds each one once and passes it to the server child. |
| `carddav` / `caldav` / `webdav` | Declarative protocol config, served by the matching core library. Each block **mirrors the `Source` literal the package's own Go registers**, and exists so a deployment that links no feature package can still serve the protocol: the supervisor materializes the block into the runtime config dir and core reads it there. Keep the two in sync. **Mount protocol handlers under the reserved `/dav` prefix** (e.g. `/dav/drive`): a DAV `prefix` becomes a literal server route, which beats the SPA catch-all, so mounting at a bare package slug makes the in-app route of the same name unreachable on a hard load — WebDAV at `/drive` shadowed the `/drive` screen exactly this way. `/dav` is reserved and must never be a package slug. Authorization is deliberately absent from all three: core evaluates the collections' own PocketBase rules, which travel in the schema (a Go closure cannot cross a process boundary). See [hooks.md](hooks.md). |
| `payloads` | `{ package }` — dir (relative to the member root, e.g. `'server/api'`) of a **self-contained** Go package holding one exported struct per HTTP request/response. The generator parses it with go/ast (never imports it) and emits `lib/generated/<slug>-api.ts`, importable as `@tinycld/app-generated/<slug>-api` — the Go structs are the single source of truth for API payload shapes, consumed by both the web client and the CLI. Constraints (hard generation errors otherwise): no imports beyond `time`, structs + basic-literal consts only, every exported field json-tagged, no embedded fields, no `any`. |
| `quota` | `[{ collection, sizeField, ownerField? }]` — storage-bearing collections `core/quota` enforces ceilings on, as record hooks. A source with no `ownerField` counts toward the org ceiling only. |
| `automation` | `{ definitions }` — exports subpath to a pure-data module default-exporting `AutomationDefinitions<PkgSchema>`: the package's workflow-rules **triggers** (record events users build rules on) and **actions** (record-ops core executes anywhere, or native Go handlers registered via `automation.RegisterAction`). Generator-validated and materialized to `server/automation_defs.json`; the engine resolves and serves the catalog to the builder UI. Full authoring guide: [automation.md](automation.md). |
| `notifications` | `{ events }` — exports subpath to a pure-data module default-exporting the package's notification events (`PackageNotifyEvents`: name → channels + variant) and augmenting core's `NotificationEvents` with their payload types. Core composes the dispatcher's registry from every installed package at boot; core's own list names no package. |
| `recordLabels` | `{ <collection>: { format, relations? } }` — how core's relation pickers (the rule builder's record picker, a form's person field) and a rule's relation `{{placeholder}}` name a record, and how the pickers search a record of one of this package's **own** collections (`<slug>_*`). `format` is literal text with `{field}` and `{relation.field}` placeholders; `relations` maps each relation the format reads to its target collection, e.g. `{ mail_addresses: { format: '{local_part}@{domain.domain}', relations: { domain: 'mail_domains' } } }`. A format reads at most one relation, and only a single (not multi) relation. The picker fetches the related row through `materialize()`, and its search matches what it shows: each word of the term (split on spaces and on the format's literal text) must appear in an own field, or in a related field, which the picker resolves to related ids with one bounded query of the related collection. Without an entry the picker falls back to the server's display-field heuristic (`name`, `title`, `label`, `subject`, `display_name`, `email`, `username`, else the id). Generator-validated; parsed at runtime by `core/lib/record-labels/`, and copied into `server/automation_defs.json` for the rules engine (`core/server/automation/record_labels.go`). |
| `forms` | `{ hosts, ui }` — two exports subpaths. `hosts` is a pure-data module declaring which of the package's records carry a form and their built-in fields; the generator materializes it to `server/form_hosts.json`. A host may declare `layout` (pages, a page and width per built-in, default rules) for its default form: the first seeded form and every new form an admin starts from Settings → Forms → New form. A width is `full`, `half`, `third`, `quarter` or `auto` (the field's content width, capped at the row). A layout field may give a `narrowWidth` (one of the same widths): the seeded field's width when the form is narrower than 640 px; without it a quarter is half there, `auto` stays `auto` and the others are full. A layout field may also give a `description` (at most 500 characters): the built-in's help text, copied into the seeded field and into a stored form that gains the built-in later. A `text` built-in may declare `maxLength` (1 to 10000; refused on any other kind): the form refuses a longer value before the save, and the server refuses a write that changes the column to a longer value, keyed by the column (`<Label> is too long`). `ui` is the host's form UI module. Design: `docs/superpowers/specs/2026-10-14-forms-engine-design.md` in the workspace-root repo. |
| `build.script` | A build script run before bundling (e.g. an embedded webview bundle). |
| `dependencies[]` | A **slug-only** list of other packages — **advisory + seed-ordering only.** Used to topologically sort seed execution and as a soft hint; it imposes **no** version constraint and is never a compile-time import. |
| `peerVersions` | `{ '<slug or @tinycld/core>': '<semver range>' }` (e.g. `{ '@tinycld/core': '>=2.1 <3' }`) — **enforced** semver ranges keyed by slug. See [`dependencies` vs `peerVersions`](#dependencies-vs-peerversions) below. |

#### Help screenshots

A package regenerates its help images with one command, run inside the member:

```sh
pnpm exec tinycld-pkg help:screenshots [--topic <topic-id>]
```

- **Lives in:** a new verb `help:screenshots` in `tinycld/package-scripts/src/cli.ts` (`Verb` in `targets.ts`, runner in `runners.ts`), so it works for any member and uses the same discovery as `test:e2e`.
- **Input:** `<member>/help/screenshots.ts`. Its default export is `{ [topicId: string]: Shot[] }`. A `Shot` is `{ name: string; setup: (page: Page) => Promise<void>; target?: string }`. `setup` drives the real UI with the helpers from `@tinycld/core/e2e-helpers` (`login`, `navigateToPackage`, `clickSidebarItem`) and must not write data through raw PocketBase calls. The runner logs in before `setup`. `target` is an optional CSS selector string to clip to, otherwise the viewport. Types: `import type { ScreenshotPlan } from '@tinycld/package-scripts/help-screenshots'` and use `satisfies ScreenshotPlan`.
- **Output:** for each shot, `help/images/<name>.png` (light) and `help/images/<name>.dark.png` (dark), where `<name>` follows `<topic-id>-<n>`. The same `setup` runs twice, once per `colorScheme`.
- **Data:** it runs against the seeded e2e server; the script never seeds by itself.
- **Behaviour:** the command uses the member's own `playwright.config.ts` (web server, ports) with one worker and no retries. A file is written only after both `setup` and the screenshot succeed. An unknown `--topic` and a bad plan (missing or empty topic, invalid or duplicate shot name, name without the `<topic-id>-` prefix) fail the run before the servers start. The viewport is 1280x800. Light and dark come from the browser colour scheme. `--topic` needs no `--` separator. Light files are written before dark files, so a failed dark shot leaves a mismatched pair: rerun the topic. A removed shot leaves its PNGs behind: delete them by hand.
- **Not in scope of the core change:** the generator and renderer only consume the files this command writes.

### `dependencies` vs `peerVersions`

These two fields look similar but do different jobs — keep them distinct:

- **`dependencies`** is a **slug-only** array (`['drive']`). It is **advisory + seed-ordering only**: the generator uses it to topologically sort seeds (a package's declared deps seed first) and as a soft presence hint. It carries **no** version information, is never enforced, and is **not** a compile-time import (a hard `@tinycld/<slug>` import would break the lean-shell guarantee — see [Cross-package coupling](#cross-package-coupling)).
- **`peerVersions`** declares **enforced semver ranges** keyed by slug, e.g. `{ '@tinycld/core': '>=2.1 <3' }`. It is checked by the **version-compatibility solver** (Setup → Versions UI): a proposed version change is **blocked** if it would leave any declared range unsatisfied. The check runs in the UI before **Apply**, and again **authoritatively on the server** before the change is actually applied. Before the new files are fetched, only the ranges of packages the change leaves alone are known, so those are checked first; the ranges declared by each target version are checked after the rebuild fetches it, before anything goes live. A package being updated is therefore never held back by its own old range, so core and its features can move past a bounded range (`<0.7.0`) together.

A package can contribute **only** a settings panel (no nav, no routes — like
`@tinycld/google-takeout-import`), or purely `publicRoutes`, or any
combination. The manifest is read by the generator with a regex +
`new Function` (not a real `import`), so it must be a plain object literal with
no runtime imports.

### Collection naming and package access enforcement

**Name every collection your package creates `<slug>_*` (or exactly the
slug).** That is more than a style rule: `core/server/pkgaccess` resolves
which installed package (pkg_registry) owns a collection by this convention —
dashes in a slug match underscores in the name — and enforces the per-user
`org_pkg_access` level on it server-side. A `readonly` or `none` user's
writes to owned collections are refused on the REST record endpoints, the
DAV protocol servers, and mail's IMAP/SMTP sessions; reads stay governed by
the collection's own rules. The client mirrors the same resolution in
`usePkgAccess` for UI gating.

Because ownership is name-derived, enforcement covers TS-only packages with
no Go of their own. A collection named outside the convention is treated as
core/shared data and gets **no** package-access enforcement — deliberate for
genuinely shared infrastructure (drive's polymorphic `comment_mentions`),
a silent hole for anything else.

### Manifest variation across shipped packages

- **contacts / mail** use `sidebar`; **calc** uses `provider`.
- **mail** and **hosting-ui** declare `settings`; **mail** also declares `systemSettings`.
- **boards** (GitHub) and **google-takeout-import** declare `accountSettings`.
- Only **calc** declares cross-package `dependencies` (`['drive']`).
- `nav.shortcut` is optional.
- A package with a `tests/` dir need not declare `tests` in the manifest
  (mail omits it — test discovery is glob-based, see below).

---

## TypeScript schema integration

The type system is fully integrated end-to-end so `useStore('contacts')` is
strongly typed.

`types.ts` exports a `{PascalSlug}Schema` type keyed by collection name:

```ts
import type { UserOrg } from '@tinycld/core/types/pbSchema'

export interface Contacts { id: string; first_name: string; /* … */ owner: string }

export type ContactsSchema = {
    contacts: { type: Contacts; relations: { owner: UserOrg } }
}
```

`collections.ts` exports a **named** `registerCollections` that intersects
core's `Schema` with the package schema and returns a name→collection map:

```ts
import type { CoreStores } from '@tinycld/core/lib/pocketbase'
import type { Schema } from '@tinycld/core/types/pbSchema'
import type { createCollection } from 'pbtsdb/core'
import { BasicIndex } from 'pbtsdb/core'
import type { ContactsSchema } from './types'

type MergedSchema = Schema & ContactsSchema

// Hoisted so every collection in this package shares one object literal —
// `indexing` for the TanStack DB passthrough, `onDemand` for the sync posture
// core uses for all of its own collections.
const indexing = {
    collectionOptions: { autoIndex: 'eager' as const, defaultIndexType: BasicIndex },
}
const onDemand = { syncMode: 'on-demand', realtime: 'query' } as const

export function registerCollections(
    newCollection: ReturnType<typeof createCollection<MergedSchema>>,
    coreStores: CoreStores,            // access core collections as relation targets
) {
    const contacts = newCollection('contacts', {
        omitOnInsert: ['created', 'updated', 'deleted_at'] as const,
        relations: { owner: coreStores.users },   // where expanded records are FILED
        ...onDemand,
        ...indexing,
    })
    return { contacts }
}
```

`relations` (not `expand`) names the collection each expanded record is filed
into; rows never carry an `expand` property. Read a relation through
`materialize()`, a join, or `target.get(id)`.

`...onDemand` is the default posture for a new collection: with
`syncMode: 'on-demand'` only the rows a live query asks for enter the store,
and `realtime: 'query'` subscribes with that query's own filter, so the
subscription covers exactly the rows the query holds. Declare
`alwaysFetchRelations` only when this collection is the sole path by which rows
enter a target — a target with a query of its own already loads itself, so the
expand just enlarges every request here.

The generator emits each package's `{Pkg}Schema` into a single literal
intersection — `MergedPackageSchema` in `tinycld.config.ts` (e.g.
`CalcSchema & ContactsSchema & …`) — and `pocketbase.ts` forms
`type MergedSchema = Schema & MergedPackageSchema`. The intersection is written
out **literally** rather than derived from `typeof tinycldConfig`: deriving it
would create a circular type reference (the config's `coreStores` field flows
through `createCollection<MergedSchema>`). At runtime, `buildPackageStores`
(in `core/lib/packages/derive-stores.ts`) spreads each entry's
`registerCollections(...)` call to assemble the package store map. See
[The generator](#the-generator) for the full config-and-derive picture.

### `seed.ts`

A package's seed exports a **default async function** receiving a `SeedContext`
carrying the seeded user (single-org: seeds own data by the user id directly):

```ts
import type PocketBase from 'pocketbase'

interface SeedContext {
    // `username` is what mail derives mailbox addresses from — derive from it,
    // never from the email local-part, or seeded users get a different address
    // than the server would provision.
    user: { id: string; username: string; email: string; name: string }
}

export default async function seed(pb: PocketBase, ctx: SeedContext): Promise<void> {
    // …create sample records via pb
}
```

Seeds run in dependency order: if a package declares `dependencies: ['drive']`,
drive's seed runs first. The generator wires every seed into `tinycld.seeds.ts`;
the `deriveSeeds` helper (`core/lib/packages/derive-seeds.ts`) topologically
sorts them at run time.

---

## Linking a package

Linking is plain **pnpm workspace resolution** — there are no bespoke
`packages:link` / `packages:install` / `packages:unlink` scripts anymore (they,
and `scripts/link-package.ts` / `scripts/install-package.ts`, were deleted). To
add a feature package:

```sh
# 1. Clone the package repo as a sibling of the app shell.
cd ~/code/tinycld
git clone <git-url> contacts            # → ~/code/tinycld/contacts

# 2. Run pnpm install at the WORKSPACE ROOT (not inside the sibling).
cd ~/code/tinycld
pnpm install
```

What `pnpm install` does:

1. Reads the `pnpm-workspace.yaml` `packages:` list and the sibling's own
   `package.json` for its **canonical name** (`@tinycld/contacts`).
2. The generator runs first (materializing `lib/generated/`), then
   `link-members.ts` creates the `node_modules/@tinycld/<name>` symlink (scoped
   names nest under the scope dir) pointing at the sibling — plus
   `@tinycld/app-generated` — so each package name resolves everywhere.
3. The generator runs on `postinstall` and materializes the on-disk artifacts
   it still owns (route shims, `tinycld.config.ts`, migration symlinks, Go
   wiring — see [The generator](#the-generator)).

To **remove** a feature package, delete its sibling clone (or its entry from
the workspace list) and re-run `pnpm install` at the root.

`getPackages()` now enumerates the workspace members that contain a
`manifest.ts` (plus nested core). The set of cloned-and-installed members is
the source of truth — there is no hand-curated list and nothing to `git add`
under `node_modules/@tinycld/` (those symlinks are local-only, created by
`link-members.ts`).

---

## The generator

`tinycld/scripts/generate.ts` (with its `gen-*.ts` helpers) is now **thin**.
pnpm owns package linking, and most of what the old generator hand-wrote into
`lib/generated/` is now **derived at runtime** from a single typed config (see
[Config & runtime derivations](#config--runtime-derivations) below). The
generator only emits the artifacts that genuinely have to exist on disk before
the bundler runs.

It runs on `postinstall` (before `link-members.ts`, so its symlink target
exists), via `pnpm run packages:generate`, and automatically before `dev` and
`build:web`. It walks `getPackages()` and, **for every output below, the result
is gitignored — never commit any of it.**

For each linked package it reads `manifest.ts` and emits **only**:

### A. Route re-exports → `app/a/(app)/<slug>/**`

For each file under `routes.directory`, a one-line re-export shim that plugs a
sibling screen into Expo Router's filesystem routing — Expo Router needs real
files on disk, so these can't be derived at runtime:

```ts
export { default } from '@tinycld/contacts/screens/index'
```

`publicRoutes` work the same way but land under the public tree — `app/p/<path>`
(e.g. drive's share routes) — rather than the signed-in `app/a/(app)/` tree. The
generated public-route shims are gitignored; any hand-written layout/index files
in the public tree stay tracked and are force-added despite the gitignore.

### B. `tinycld.config.ts` (via `scripts/generate-config.ts`)

The **single source of truth** for what's installed — a typed array, one entry
per linked package, each built by
`definePackageEntry<PkgSchema>()({ manifest, registerCollections, sidebar, provider, settings, accountSettings, systemSettings, sidebarContributions })`.
It also emits `MergedPackageSchema` as a **literal** intersection of every
package's `{Pkg}Schema` (`CalcSchema & ContactsSchema & …`); `pocketbase.ts`
forms `type MergedSchema = Schema & MergedPackageSchema` from it. (It must be a
literal intersection, not `typeof tinycldConfig`-derived, to avoid a circular
type reference through `coreStores` → `createCollection<MergedSchema>`.) This is
the bundled (Hermes-safe) config — it carries no Node-only seed code.

### C. `tinycld.seeds.ts`

Seed wiring lives in a **separate** Node-only file, *not* in
`tinycld.config.ts`. Seed modules use Node `import.meta.dirname` / `fs`, which
Hermes (iOS) can't bundle — keeping them out of the bundled config is what lets
the app build for native. The `deriveSeeds` helper consumes this at run time.

### D. `lib/generated/*`

The generated runtime artifacts that must exist on disk live under
`tinycld/lib/generated/`. The whole directory is gitignored. It contains:

- **`package.json`** — makes `lib/generated/` itself the `@tinycld/app-generated`
  package, so member code can import generated output by the stable name
  `@tinycld/app-generated/*`. (`@tinycld/app-generated` is **not** a workspace
  member, so pnpm doesn't link it — `link-members.ts` creates that symlink
  explicitly, which is why the generator must run *before* it.)
- **`tinycld-config.ts`** — the generated re-export/entry the runtime helpers
  consume (paired with the top-level `tinycld.config.ts` source of truth).
- **`package-help.ts`** — parsed help-topic frontmatter + bodies (core included
  explicitly). Help topics are parsed markdown content, so they're pre-extracted
  to a generated file rather than derived at runtime.
- **`package-icons.ts`** — the nav/lucide icon map for installed packages.
- **`<slug>-api.ts`** — one per package declaring `payloads`: TypeScript
  interfaces/consts emitted from the package's Go payload structs (see the
  `payloads` field reference). Orphans from removed members are pruned.
- **`uniwind-sources.css`** — Tailwind v4 `@source` roots (see below).

### E. CSS source roots → `lib/generated/uniwind-sources.css`

Tailwind v4's scanner respects `.gitignore`, and the workspace symlinks live in
gitignored paths — so a utility class used **only** inside a linked package
would silently produce no CSS rule. The generator writes one absolute
`@source "<real-path>";` line per package; `global.css` does
`@import './lib/generated/uniwind-sources.css';`. Diagnose missing styles by
checking `document.styleSheets` in DevTools for a `.your-class { ... }` rule;
if absent, re-run `pnpm run packages:generate`.

### F. PocketBase migrations & hooks → `server/pb_migrations/`, `server/pb_hooks/`

Each package's `pb-migrations/*.js` is **symlinked** into the shared
`server/pb_migrations/`; `pb-hooks/*` are symlinked into `server/pb_hooks/` the
same way. Core's migrations are symlinked in via a separate explicit pass (core
has no manifest, so it doesn't flow through the per-package loop). Note the
source-dir naming difference: siblings use `pb-migrations` (hyphen); core uses
`pb_migrations` (underscore).

The on-disk PocketBase migrations are the **source of truth** for the TypeScript
schema types: `tinycld/core/types/pbSchema.ts` and `pbZodSchema.ts` are
**generated** (gitignored) from them on every install. Never hand-edit those two
files — regenerate by re-running the install/generate. (Likewise the per-package
`core/types/pb*Schema.ts` are generated, not authored.)

#### Rules for a migration file's top level

A migration file's top level runs at every boot, and again for each `up` or
`down` call. The server does not keep a migration's JS runtime after the file
registers its migrations: each call builds a new runtime and runs the top level
again to get its function (see `third_party/pocketbase/FORK.md`). So:

- The top level may only declare values and functions and call `migrate()`.
- Put every side effect (a save, a delete, a query) inside `up` or `down`.
- `up` and `down` share no runtime state. A value that `up` sets is gone when
  `down` runs.
- The number and order of the `migrate()` calls must not depend on time, the
  environment or data. A call whose re-run makes a different number of
  `migrate()` calls fails with an error.

### G. Go server wiring → `server/package_extensions.go` + `server/go.work`

See [the Go server section](#the-go-pocketbase-server-side).

The ONE generated registrar serves both modes of the dual-mode binary
(DESIGN-org-package-agency D5): `server/main.go` dispatches to per-org mode
when invoked with `--org-dir` (the flag contract a composing server uses,
making a per-org build artifact a drop-in per-org binary) and passes the same
`registerPackageExtensions` there. A package that must behave differently
in that mode reads the context the composing server stamps on the app
before the registrar runs. There is no separate per-org registrar or entry
point.

What the generator **no longer** emits: `package-collections.ts`,
`package-registry.ts`, `package-sidebars.ts`, `package-providers.ts`,
`package-settings.ts`, and `package-seeds.ts`. Those are all derived at runtime
from `tinycld.config.ts` / `tinycld.seeds.ts` — see below. It also no longer
maintains a `node_modules/@tinycld/*` shim: `link-members.ts` owns those
symlinks (created right after the generator runs).

### Config & runtime derivations

`tinycld.config.ts` is consumed by helpers under `core/lib/packages/`, each of
which replaces a file the old generator used to write:

| Runtime helper (`core/lib/packages/…`) | Exports | Replaces (old generated file) |
|---|---|---|
| `derive-stores.ts` | `buildPackageStores` | `package-collections.ts`'s `packageStores` |
| `static-registry.ts` | `packageRegistry`, `toStaticRegistry` | `package-registry.ts` |
| `derive-components.ts` | `deriveSidebars` / `deriveProviders` / `deriveSettings` / `deriveAccountSettings` / `deriveSystemSettings` / `deriveSidebarContributions` + the `packageSidebars` / `packageProviders` / `packageSettings` / `packageAccountSettings` / `packageSystemSettings` / `packageSidebarContributions` consts | `package-sidebars.ts`, `package-providers.ts`, `package-settings.ts`, (new) `package-sidebar-contributions.ts` |
| `derive-seeds.ts` | `deriveSeeds` (ports the `dependencies` topo-sort) | `package-seeds.ts` |

`usePackages()` still merges the static set (`packageRegistry`) with
runtime-installed DB packages, exactly as before.

### Validation

The generator fails fast on:

- **Duplicate `nav.shortcut`** letters across packages.
- **Duplicate slot names** within one package's `manifest.slots`.
- **`sidebarContributions` targeting an unknown slot** on a present host package — the error lists the host's actual `slots` array so you can spot the typo immediately.
- **`ports`**: a duplicate name or port number across packages, a port outside 1-65535, a name that does not match `[a-z0-9-]+`, or a name that collides with a supervisor-reserved name (`http`, `https`, `http-redirect`).
- **`recordLabels`**: a collection the package does not own (not `<slug>` or `<slug>_*`, dashes as underscores), the same collection labelled by two packages, or a `format` that does not parse — no placeholder, a placeholder that is not `{field}` / `{relation.field}`, an unmatched brace, a relation read but not named in `relations` (or named but not read), or more than one relation.
- **`notifications.events`**: a subpath with no plain-string `./<subpath>` entry in the package's `exports` map, an entry whose file does not exist, or a module with a runtime import from `@tinycld/core` or `@tinycld/app-generated` (use `import type`). The module must be pure data: the generated config imports it, and a runtime import that reaches the dispatcher would make `tinycldConfig` read itself while it initialises.

It warns (but does not fail) when:

- A `sidebarContributions` entry's `target` is not a present member. The contribution silently goes inactive — normal for a partial checkout — and wakes up automatically when the target is installed.

### Extension points: settings panels and sidebar slots

The generator threads two manifest-declared extension points through the same lazy-import pipeline. (Package `provider`s are the exception: they are needed on every signed-in boot, so they are loaded up front by `core/lib/packages/provider-loader.ts` rather than `lazy()`-suspended in place.) Both share the same lifecycle: **manifest field → `gen-config.ts` emits a `lazy(() => import(...))` entry → runtime derivation in `core/lib/packages/derive-components.ts` → host UI calls a React helper that consumes the registry → component loads under `<Suspense>` on first render.**

**Settings panels** come in three registries. Pick by who the panel acts for:

| Field | Acts on | Shown to | Route tree |
|---|---|---|---|
| `accountSettings` | the viewer's own data | every role, in the **Account** group | `settings/account/[...section]` |
| `settings` | the organization | owners and admins, in a group named after the package | `settings/[...section]` |
| `systemSettings` | the whole deployment | owners, in the **System** group | `settings/system/[...section]` |

Each tree resolves against its own registry, so one package can use the same panel slug in more than one. The example below uses `settings`; the other two follow the same pipeline (`packageAccountSettings`, `packageSystemSettings`).

```ts
// app/tinycld.config.ts (emitted)
definePackageEntry<MailSchema>()({
    manifest: { /* ... */ },
    settings: [
        { slug: 'provider', label: 'Provider',
          Component: lazy(() => import('@tinycld/mail/settings/provider')) },
    ],
})

// derive-components.ts
export const packageSettings = deriveSettings(tinycldConfig)
// → PackageSettingsGroup[] grouped by package

// app/a/(app)/settings/index.tsx
packageSettings.map(group => group.panels.map(panel => /* render link */))
// app/a/(app)/settings/[...section].tsx
// looks up the matching panel by [pkgSlug, panelSlug] and renders panel.Component
```

The `component` subpath must resolve through the package's `package.json` `exports` wildcard (e.g. `"./settings/*": "./tinycld/mail/settings/*.tsx"`). The component must default-export — the generator imports by default name. A panel is addressed by `[pkgSlug, slug]`, so its `slug` must be unique within one registry of one manifest.

**Sidebar slots** (`manifest.slots: ['sidebar.<name>']` on the host + `manifest.sidebarContributions: [{ target, slot, component, order?, label? }]` on the contributor):

```ts
// Host: calendar/manifest.ts
slots: ['sidebar.after-calendars']

// Host: calendar/tinycld/calendar/sidebar.tsx
import { SidebarSlot } from '@tinycld/core/components/sidebar-primitives'
<SidebarSlot target="calendar" slot="sidebar.after-calendars" />

// Contributor: calendar-slots/manifest.ts
sidebarContributions: [
    { target: 'calendar', slot: 'sidebar.after-calendars',
      component: 'sidebar-contributions/booking-pages' },
]

// app/tinycld.config.ts (emitted for the contributor)
sidebarContributions: [
    { target: 'calendar', slot: 'sidebar.after-calendars', order: 0,
      Component: lazy(() => import('@tinycld/calendar-slots/sidebar-contributions/booking-pages')) },
]

// derive-components.ts
export const packageSidebarContributions = deriveSidebarContributions(tinycldConfig)
// → Record<targetSlug, Record<slotName, SidebarContributionEntry[]>> (sorted)

// core/components/sidebar-primitives/SidebarSlot.tsx
const entries = packageSidebarContributions[target]?.[slot] ?? []
// renders each entry's Component under <Suspense fallback={null}>
```

The contributor must add an `exports` wildcard matching the `component` subpath (e.g. `"./sidebar-contributions/*": "./tinycld/calendar-slots/sidebar-contributions/*.tsx"`). The component is a regular React component — `<SidebarSlot>` renders contributions back-to-back with no wrapper, so the contributor owns its own heading, items, and structure. Ordering: ascending `order` (default 0), ties broken by contributor slug.

The host can also declare slots and never render them: the generator will allow that, but no contribution targeting that slot will appear — typically a sign the host's sidebar JSX is out of sync with its manifest.

For author-facing prose, link readers to the website pages: [Settings](https://tinycld.org/docs/anatomy/settings) and [Sidebar slots](https://tinycld.org/docs/anatomy/sidebar-slots).

### Setup wizard steps (`setupSteps`)

The first-run setup wizard opens for an owner or admin after the server is claimed. Core supplies four steps. A package can add its own steps with `manifest.setupSteps`:

```ts
setupSteps: [
    { id: 'address', label: 'Mail address', module: 'setup/address', order: 'a1V' },
]
```

| Field | Rule |
|---|---|
| `id` | Must match `[a-z0-9-]+`, because it appears in the URL (`/a/setup/<slug>.<id>`). The wizard stores the step as `<slug>:<id>`. |
| `label` | The name in the progress bar. Keep it short. |
| `module` | A `package.json` `exports` subpath, like `component` in `settings`. |
| `order` | Optional. A fractional-indexing key (the `fractional-indexing` library). The generator refuses a key that is not valid. Steps without `order` come last. |

**Order keys.** Core's steps use `a0` (Workspace), `a1` (Apps), `a2` (Email sending) and `a3` (Your team). Keys sort by plain character order, so `a0V` comes after `a0` and before `a1`, and `Zz` comes before `a0`. To put a step between two steps, use a key between their keys. You do not have to change other keys. If two steps have the same key, the step id decides the order.

**Module contract.** The module must default-export the step component. The component gets one prop, `next`. Call `next()` after the step saves its data. The wizard then marks the step as done and opens the next step. The module can also export these hooks. The wizard calls them on every render of every step, so keep them cheap and free of side effects:

- `useIsStepDone(): boolean | undefined` — Tells the wizard that the step is done because of real data (for example, a record exists). Return `undefined` while the data loads. If the module does not export it, the step is done only after the person clicks Continue (`next()`). Do not derive "done" from a value that a new server already has.
- `useIsStepVisible(): boolean | undefined` — Tells the wizard to show or hide the step (for example, owner-only steps). Return `undefined` while the answer loads: the wizard then does not show the step and waits before it opens the next step, so it does not skip past it. If the module does not export it, the step is always visible.

When the person skips a step, the progress bar shows it as skipped, and the wizard does not open it again when it resumes. Continue (`next()`) always moves on: if `useIsStepDone` returns `false` (or `undefined`), Continue records the step as skipped, the same as Skip. When the step becomes done later, the wizard shows it as done.

**Continue button and e2e tests.** Use `SetupContinueButton` from `@tinycld/core/components/setup/wizard/SetupContinueButton` for the step's Continue. The wizard gives the current step's container the test ID `setup-step-<slug>:<id>`, and the button has the test ID `setup-continue`. The helpers in `@tinycld/core/e2e-setup-helpers` (also in `@tinycld/core/e2e-helpers`) use these IDs: `expectSetupStep(page, stepId)`, `continueSetupStep(page, stepId)` and `continueSetupSteps(page, stepIds)`. Core's step IDs are in `CORE_STEP_IDS`. Use the helpers, not the step text, so that your tests do not fail when core changes its text.

**The `setup-team` slot.** Core renders a sidebar slot on the Team step, above the invite form. A package can add content there (for example, a link to import people from another service) with a `sidebarContributions` entry that targets core:

```ts
sidebarContributions: [
    { target: 'core', slot: 'setup-team', component: 'setup/team-import' },
]
```

Core's slot names are listed in `core/lib/setup/core-slots.ts`. The generator uses that list to validate contributions with `target: 'core'`, so a wrong slot name stops the build.

**The `group-member-removal` slot.** Core renders this slot in the confirm dialog that removes a person from a group, and in the confirm dialog that deletes a group. Use it to show what the person (or the members) lose when the group access ends. Core passes `GroupMemberRemovalNoticeProps` (from `@tinycld/core/lib/groups/removal-notice`) to every contribution: `groupId`, and `userId`. `userId` is `null` when the whole group is deleted. A contribution must render nothing when there is nothing to say.

**The `member-drawer` slot.** Core renders this slot in the Settings → Members drawer, below the person's groups. Use it to add a section about that person. Core passes `MemberDrawerPanelProps` (from `@tinycld/core/lib/members/member-panel`) to every contribution: `userId`. The contribution reads its own collections and renders its own heading. Only admins see the drawer, because its screen is admin-only.

**The `group-drawer` slot.** Core renders this slot in the Settings → Groups drawer, below the members list. Use it to add a section about that group. Core passes `GroupDrawerPanelProps` (from `@tinycld/core/lib/groups/group-panel`) to every contribution: `groupId`. The contribution reads its own collections and renders its own heading. Only admins see the drawer, because its screen is admin-only.

---

## Bundler & test resolution

The pnpm workspace does the heavy lifting now — a single copy of each shared
library is hoisted (singletons like `zustand`/`yjs` resolve to one copy through
the workspace), and the `node_modules/@tinycld/*` symlinks make every member
resolvable by name. The bundler and test configs are correspondingly much
smaller.

### Metro (`metro.config.cjs`)

Down to **~25 lines** (from 388). It is essentially:

- `getDefaultConfig` + `withUniwindConfig`,
- a single `watchFolders: [workspaceRoot]` entry so Metro bundles sibling source
  living outside the app repo,
- the `@tinycld/app-generated/*` alias.

There is **no** custom `@tinycld/*` `resolveRequest`, **no**
`unstable_enableSymlinks`, and **no** singleton pins — those are unnecessary now
that the workspace hoists one copy of each dep. Metro's **default** resolver
follows the workspace symlinks and resolves each package's `.ts` / `.tsx` /
directory-index mix natively.

`~/*` is **not** handled in Metro — siblings' tsconfig maps `~/*` onto
`@tinycld/core/*`, and Metro resolves the resulting `@tinycld/core/*`
specifier.

### Vitest (`vitest.config.ts`)

Vitest still needs explicit `@tinycld/core/*` path aliases: unlike Metro,
Vite's `exports` resolution lacks the directory-index fallback that core's
subpaths rely on. It also keeps the dedup pins (`react`, `yjs`, `y-protocols`,
`hyperformula`). Sibling unit tests are discovered via `../<sibling>/tests/**`
globs. Environment is `node`; setup is `tests/unit-setup.ts`.

### Playwright (`playwright.config.ts`)

Playwright generates **one project per workspace member**, derived from the
`node_modules/@tinycld/*` workspace symlinks (project name = `@tinycld/mail`,
etc.), with `testDir` pointing at that member's `tests/`. Run one package's
specs with:

```sh
pnpm run test:e2e --project=@tinycld/mail
```

Playwright **owns the server lifecycle** — it resets the DB and starts
PocketBase + Expo itself (`webServer.reuseExistingServer: false`). Never start
or kill servers manually around a Playwright run.

---

## The Go (PocketBase) server side

Two Go module boundaries, one binary:

- **App** = `tinycld.org/tinycld` (`server/go.mod`), which pulls core via
  `replace tinycld.org/core => ../core/server`.
- **Core** = `tinycld.org/core`
  (`core/server/go.mod`), exporting
  `coreserver.Register(app, Options)` plus subsystems (`notify`, `push`,
  `mailer`, `audit`, `textextract`, `thumbnails`, `realtime`, `render`).
- **Each server-bearing sibling** = `tinycld.org/packages/<slug>`, exporting
  `func Register(app *pocketbase.PocketBase)`. Siblings require
  `tinycld.org/core` but carry **no `replace`** of their own.

### The two generated files

`server/package_extensions.go` (generated, `package main`):

```go
// Code generated by scripts/generate.ts. DO NOT EDIT.
package main

import (
    "github.com/pocketbase/pocketbase"
    contacts "tinycld.org/packages/contacts"
    mail "tinycld.org/packages/mail"
    // …one import per server-bearing sibling
)

func registerPackageExtensions(app *pocketbase.PocketBase) {
    contacts.Register(app)
    mail.Register(app)
    // …one call per server-bearing sibling
}
```

`server/go.work` (generated, gitignored) `use`s the app module, every
server-bearing sibling's `server/`, and core's `server/`. This lets the
`tinycld.org/packages/<slug>` imports resolve to sibling sources at build time
while the tracked `go.mod` stays lean.

### The decoupling seam

`server/main.go` passes `registerPackageExtensions` into
`coreserver.Register` via `Options.RegisterExtras`:

```go
coreserver.Register(app, coreserver.Options{
    // …
    RegisterExtras: registerPackageExtensions,
})
```

Core never imports any sibling, so **core typechecks and builds with zero
siblings linked.** A sibling is wired into Go only when its manifest declares a
`server: { package, module }` field — packages with no `server` field (e.g.
`google-takeout-import`) are excluded from both `go.work` and
`package_extensions.go`.

Go server hooks use SDK methods that **bypass PocketBase API rules** — they
implement authorization manually. When changing API rules on a collection,
check whether a Go hook also accesses that collection and update its filters to
match.

### Hooks inside a transaction

The batch API (`/api/batch`, which the client's `atomic()` uses) is on, and any
signed-in client can call it. It runs several record writes in one database
transaction, so every record hook of every package must be correct there.
Three facts set the rules:

- In a batch, `e.App` is the transaction. An `app` captured by the closure is
  not: it sees committed rows only, and a write through it waits for the single
  write connection that the batch holds, until the batch times out.
- A batch nests its operations. `e.Next()` of one operation returns after all
  later operations ran.
- A batch can roll back after every hook returned success (timeout, client
  disconnect, commit error).

The rules, for every hook that runs before the commit (`OnRecord*Request`,
`OnRecordCreate/Update/Delete`, their `Execute` forms):

1. **Read and write through `e.App`**, never through a captured `app`. Pass
   `e.App` to helpers.
2. **A side effect that leaves the database waits for the commit** — mail,
   push, a webhook, a background job, a file write, in-memory state. Use an
   `OnRecordAfter*Success` hook (PocketBase runs it at commit), or
   `e.App.TxInfo().OnComplete(func(txErr error) error { ... })` and act only
   when `txErr == nil`. Outside a transaction `e.App.TxInfo()` is nil and the
   effect can run at once.
3. **State kept for an after-success hook stays until the commit.** That hook
   runs after `e.Next()` returned when the request is in a transaction.

A hook that holds a process lock across `e.Next()` must not take it when
`e.App.IsTransactional()`: the transaction holds the write connection, and a
request outside it can hold the lock while it waits for that connection.

An OAuth token cannot call the batch API, and read-only mode refuses it.

Test a hook in a batch with `tests.ApiScenario` against `/api/batch` after
setting `app.Settings().Batch.Enabled = true` on the test app.

### Registries: how core learns what a package provides

**Core must not name a package.** Not a slug, not a scope, not a collection,
not a route. The shell, `@tinycld/core` and the CLI shell build and run with
zero features linked, and a package is installable without a core change —
both hold only while core has no idea which packages exist. A line in core
that says `boards` is a core PR for something the package should own, and it
is silently wrong on every deployment without boards.

When core needs to know something about a package, the package **registers**
it from its own `Register(app)`, and core reads the accumulated set at
runtime. The registries that exist:

| Core needs to know… | The package calls |
|---|---|
| its OAuth scopes, and which collections and routes they govern | `oauth.RegisterPackage` |
| its contribution to the federated search | `search.RegisterSources` |
| its storage-bearing collections | `quota.RegisterSources` |
| which authorship FKs to reassign on offboarding | `offboard.RegisterReassignable` |
| how to move ownership a flat FK rewrite cannot settle on offboarding | `offboard.RegisterHandler` |
| which collections to audit-log | `audit.RegisterCollection` |
| which view collections stay live, and the collections their SQL reads | `changesignal.RegisterView` |
| which collections a stored virtual table holds (a full-text index), so a report or a view may read it | `changesignal.RegisterVirtualTable` (`fts.RegisterSync` calls it for each index) |
| which of its collections reports may read, with their fields and links (see "Report sources" below) | `reports.RegisterSource` |
| the reports it ships, published to every user (see "Shipped and saved reports" below) | `reports.RegisterShipped` |
| a package membership table that carries group grants | `groups.RegisterGrantTable` |
| join and leave events for group membership, sent after the change commits. Every listener runs, in registration order. Errors are logged, not retried; one listener's error does not stop the others. Boot sends no events, so a package reconciles its own state at boot. The event carries the app that saved the change; the registry is process global, so a listener works through `e.App`, not an app it captured | `groups.OnMembershipChange` |
| its full-text index | `fts.Register` / `fts.RegisterSync` |
| a CalDAV / CardDAV / WebDAV source | `caldav.Register` / `carddav.Register` / `webdav.Register` |
| a native automation action | `automation.RegisterAction` |
| when core's rule actions that send mail (`core:send-email`) must not send for a trigger record of its collection (e.g. mail that was sent automatically). A guard that says skip makes the action return `automation.Skip(reason)`, which run history shows as "Skipped" with the reason; a skip is not a failure. Guards run in registration order, and the first skip wins | `automation.RegisterSendGuard` |
| which of its records carry a form, and that record's built-in fields | `forms: { hosts, ui }` in the manifest; the generator emits `server/form_hosts.json`, which core's `forms` package reads |
| that an anonymous visitor of one of its screens (a share link) may read form definitions, and the rule clause that vouches for that visitor | `forms.RegisterPublicReader` |
| version snapshot/restore hooks for its drive item type | `versionhooks.Register` |
| a sink for provider delivery notifications about its own sent messages | `deliveryevents.Register` |
| what to stop at once when a supervised server begins to drain (e.g. stop accepting on its own ports) | `drainhooks.OnBegin` |
| its user-facing notification events and their channels/variant | `notifications: { events }` in the manifest; core composes `notifyRegistry` from every entry at boot |

`offboard.RegisterReassignable` rewrites every matching row to the successor in
one `UPDATE`. That is wrong for a membership table: the successor inherits every
role the leaver held, and a unique `(resource, user)` index fails the `UPDATE`
when the successor is already a member. For those tables, register a handler:

```go
offboard.RegisterHandler("calendar", func(txApp core.App, leaver *core.Record, plan offboard.Plan, actorUserID string) error {
    // Move only what the leaver solely owns; merge into an existing membership.
    return nil
})
```

`OffboardUser` calls every handler inside its transaction, in every mode
(`reassign`, `delete_my_data` and `keep`), after the reassignable FKs are settled
and before the user is anonymized. Use `txApp` for every read and write.
`actorUserID` is the admin on `/api/admin/users/offboard`, the leaver on
`/api/account/delete`, or `""` for system use. Any error rolls back the whole
offboard. Wrap `offboard.ErrInvalidPlan` to refuse the plan; both endpoints
return that as a 400 with your message. A second registration under the same
name is a no-op, and handlers run in name order. A self-delete with no plan
(`/api/account/delete` without `plan`) runs in `keep` mode: reassignable
records stay attributed to the anonymized account, but the handlers still run.

A handler must not assume a heir. In `delete_my_data` or `keep` mode with the
leaver as the actor, there is no successor and no admin. If the leaver is the
only owner of a resource other people use, refuse with a wrapped
`ErrInvalidPlan` that tells the user to transfer ownership or delete the
resource first. This is how a no-plan delete is refused while the user solely
owns shared resources; core itself names no package. A handler can also delete
what must not outlive the account (mail deletes the leaver's personal mailbox).

A package hook that grants rows from a users update
(`app.OnRecordAfterUpdateSuccess("users")`) must call `offboard.IsOffboarded(e.Record)`
and, when it is true, `return e.Next()` without granting. PocketBase runs
after-update hooks at commit, after the offboard handlers removed the leaver's
rows, so a grant there gives them back. `IsOffboarded` stays true on every later
save of the record.

The OAuth registry is the reference shape. A package declares its scopes with
consent copy, the collections each scope reads and writes, its bespoke
endpoints, and per-record route families — and nothing else in the system
carries a copy:

```go
oauth.RegisterPackage(oauth.Package{
    Slug: "boards",
    Scopes: []oauth.Scope{
        {ID: "boards:read", Label: "Read your boards and cards"},
        {ID: "boards:write", Label: "Create and modify your boards and cards"},
    },
    Collections: map[string]oauth.Access{
        "boards_cards":    {Read: []string{"boards:read"}, Write: []string{"boards:write"}},
        "boards_activity": {Read: []string{"boards:read"}}, // read-only for tokens
    },
    Endpoints: map[string][]string{
        "GET /api/boards/export":  {"boards:read"},
        "POST /api/boards/import": {"boards:write"},
    },
    EndpointPrefixes: []oauth.EndpointPrefix{
        {Method: "POST", Prefix: "/api/boards/cards/", Scopes: []string{"boards:write"}},
    },
})
```

The catalog (`scopes_supported` in the discovery document), the consent
screen's labels, and the CLI's login request are all derived from what is
registered. A scope must be in the package's own namespace (`<slug>:…`),
endpoints must live under `/api/<slug>/`, and a malformed registration panics
at boot rather than silently default-denying. A collection two packages both
claim (core's `labels`, used by mail and contacts) unions their rules. A
package pins its own CLI-reachable routes in its own tests — an unclassified
route 403s for OAuth callers only, which nothing else notices.

The rule is enforced by `oauth.TestCoreDeclaresNoPackageScopes` (the
registry is empty with no features present) and by
`pnpm run check:core-isolation`, which derives the installed packages' slugs
and collections from the workspace and refuses any reference to them in the
shell's tracked runtime code. The script carries an allowlist of pre-existing
debt, each entry naming the registry that should replace it; it fails when an
entry goes stale, so the list only shrinks. Do not add to it to make a change
pass.

### Change signals and live views

After a record of any collection is created, updated or deleted and the write
commits, core sends a message on the realtime topic `collection/<name>`. The
payload is `{}`. A client of the fork's resumable realtime receives
`{"seq":N}`. The message carries no record data. It says only that the
collection changed, and when.

Messages are coalesced per topic over 200 ms, so a batch of writes sends one.
Only signed-in realtime clients subscribed to the topic receive it. System
collections (`_otps`, `_mfas`, `_authOrigins`, `_externalAuths` and the rest)
send no signal, from a hook or from `Touch`. Any signed-in client may subscribe
to any topic, and the timing of sign-in or one-time-password writes is
information.

A live report subscribes to the topic of every collection it reads. It runs
again on a message. On the client, pbtsdb (2.1.0 and later) provides
`subscribeTopic(pb, topic, onMessage)` and the collection option `liveTopics`.

**Raw SQL sends no signal.** A write through `app.DB()` (a raw `UPDATE`, a
bulk `INSERT … SELECT`, a migration) runs no model hook. After such a write to
a collection that a report can read, call `Touch` with the app that ran the
write:

```go
if _, err := txApp.DB().NewQuery("UPDATE zoo_feedings SET grams = 0").Execute(); err != nil {
    return err
}
changesignal.Touch(txApp, "zoo_feedings")
```

Inside a transaction the signal waits for the commit and is dropped on a
rollback. `Touch` for an unknown or system collection logs a warning and sends
nothing.

**View collections.** PocketBase sends no record events for a view, so a view
is live only when it declares the collections its SQL reads. Do it from the
package's `Register(app)`:

```go
changesignal.RegisterView("zoo_fed", "zoo_animals", "zoo_feedings", "zoo_keepers")
```

`RegisterView` takes collection names, not ids, for the view and for each
source: topics and writes are matched by name. A write to any source then also
signals `collection/zoo_fed`. The client keeps
the view live with `liveTopics: ['collection/zoo_fed']`
(`...liveViewOptions('zoo_fed')` from `@tinycld/core/lib/change-signal`). List
every collection the SQL reads, including one read only in a subquery, an
`EXISTS` or a CTE: PocketBase's view parser reads only the outer FROM and
JOIN. A source may be another registered view. Pin the list in the package's
tests, after its migrations apply:

```go
if err := changesignal.CheckViews(app); err != nil {
    t.Fatal(err)
}
```

`CheckViews` compiles each registered view with SQLite `EXPLAIN` and fails
when its bytecode opens a table, or an index of a table, that the
registration does not cover. It also fails when the view or a source is
registered by an id or by a spelling other than the collection's name.

**Virtual tables.** A report or a view may read a table-valued function
such as `json_each` or `json_tree`. It holds no stored rows, so it adds no
topic. A stored virtual table, such as an FTS5 index, is different: its rows
change when its source collection changes. Map it to that collection from
the package's `Register(app)`:

```go
changesignal.RegisterVirtualTable("fts_zoo_animals", "zoo_animals")
```

`RegisterVirtualTable(table, collections...)` follows the same rules as
`RegisterView`: it takes collection names, not ids, and the list you give is
authoritative. A statement that reads the table reads those collections, so a
live report gets their topics, and a view over it must list them in
`RegisterView`. `fts` registers each index automatically, so a package with an
`fts` config has nothing to add.

Report and view SQL may read table-valued functions such as `json_each`. A
stored virtual table must be mapped, or the query is refused with an error
that names the table. Temp and attached virtual tables are covered only when
the check and the statement run in one transaction.

SQLite's bytecode does not name the virtual table it opens, so the check
reads names. When a statement opens a virtual table, every identifier in
its SQL, and in the SQL of each view it names, is compared with the stored
virtual tables in the schema of every database the connection holds (main,
temp and each attached one; temp views count too). A name inside a dbx
`{{table}}` or `[[alias.column]]` marker is read whole, as dbx quotes it.
The scan is conservative: a column or an
alias spelled like a stored virtual table counts as a read of it. A false
hit makes the statement need the mapping (or be refused); it never misses a
topic.

### Report sources

Core's report engine (`tinycld.org/core/reports`) runs reports over every
package's data. A report starts from a source and follows the source's
links. It reads only the collections, fields and links that a source
registers. Register sources from the package's `Register(app)`:

```go
reports.RegisterSource(reports.Source{
    Key:        "zoo.feedings", // "<slug>.<name>", stable
    Collection: "zoo_feedings",
    Label:      "Feedings",
    Fields: []reports.Field{
        {Key: "cost", Label: "Cost", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
        {Key: "currency", Label: "Currency", Type: reports.Text},
        {Key: "fed", Label: "Fed", Type: reports.Date, Aggregates: reports.MinMax},
    },
    Links: []reports.Link{
        {Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: reports.ToOne, Field: "animal"},
    },
    FiscalStartMonth: func(app core.App, reader *core.RequestInfo) int { return 1 },
})
```

- Every link declares its cardinality. A to-one link names its single
  relation `Field` on this source. A to-many link names its `BackField`: the
  single relation on the target that points back at this source.
- Field types are `Text`, `Number`, `Money` (integer cents), `Date`,
  `DateTime`, `Bool`, `Select` (with `Options` for the labels) and
  `Relation`. A money field names a `Currency` text field of the same source,
  or the source sets `Source.Currency`, a function that gives one fixed
  currency for the reader (see below). A money field with neither panics
  in `RegisterSource`: the engine never adds amounts it cannot tell apart.
  A source whose money fields name a `Currency` field can also set
  `Source.Currency`. The engine then uses it only when the report reads no
  rows at all: the total, the fill records, the computed rows and the empty
  layout sections are in that currency (for example `$0.00`, not `0.00`). When the report
  reads rows, each amount is in its row's currency, and a row with a blank
  currency field has no currency.
  A field offers the aggregates in `Aggregates` (`SumAll`, `MinMax`,
  `CountOnly`); with none it can be counted. Every source also offers `id`.
- To rename or remove a field or a link, keep the old key in `Aliases`
  (`"old": "new"`). Saved reports then keep working.
- `FiscalStartMonth` (1 to 12) sets the fiscal year for the source's reports:
  the fiscal presets, `fiscalYear` and `fiscalPeriod`. A value outside 1 to
  12 counts as 1.
- `Heading` returns the name a report on this base source prints above its
  title. The result carries it as `heading` (trimmed); nil leaves it empty,
  and the shell prints the organization's name.
- `Heading`, `FiscalStartMonth` and `Currency` get the reader: the signed-in
  user of the read, with the request's headers. The reader is never nil and
  never a superuser. Each read of a report calls `Heading` and
  `FiscalStartMonth` one time. It calls `Currency` at most one time for each
  source, and only when the read shows or compares an amount in that
  source's fixed currency, or reads an amount of that source by its
  currency field, so that a read that finds no rows still has a currency.
  The read uses that one value everywhere. A param's options do not call `Heading`. `Validate` calls none of the three.
  The functions run with the app, not under the reader's rules. They must
  return only what that reader may see.
- The reader is read-only. The run's list rules use the same request, so a
  change to the reader changes what the whole run may read.
- To read a record for the reader, use `reports.FindViewableRecord`. It
  applies the collection's view rule. It returns `sql.ErrNoRows` when the
  record does not exist or the reader may not view it. Any other error is a
  fault, for example an unknown collection or a rule that cannot be built.

  ```go
  log := logging.ForPackage("zoo") // tinycld.org/core/logging

  Heading: func(app core.App, reader *core.RequestInfo) string {
      rec, err := reports.FindViewableRecord(app, reader, "zoo_office", officeID)
      if errors.Is(err, sql.ErrNoRows) {
          return "" // the reader may not view the office row
      }
      if err != nil {
          // A fault, but the report still runs with the organization's name.
          log.Warn("zoo office heading", "error", err)
          return ""
      }
      return rec.GetString("name")
  },
  ```
- `Watch` names the collections that `Heading`, `FiscalStartMonth` and
  `Currency` read. A run that reads this source also subscribes to them, so a
  change there runs an open report again. The start check refuses a watched
  collection that does not exist, is named by id, or is a system collection.
- `Field.TextDay` makes a `Date` field read a text column that holds a day as
  `YYYY-MM-DD`. Filters, buckets, presets and pivots treat it as a date.
  `CheckSources` refuses `TextDay` on any other type, and on a column that is
  not text.
- A path that ends at a to-one link, or at the `id` field, is a relation
  value: it carries the target source. A formula, filter or column may use it
  as a value (the linked record's id; blank when the reader cannot see it).
- A group whose column is a relation value is a record group. Its row shows
  the record's label: by default the target's first text field, else its
  first other field that is not a relation or `id`, else the source's own
  name. Set the column's `labelField` or a `groups[].label` formula to show
  another field.

When the server starts, core checks every source against the database:

- The collection and each column exist, with matching types.
- No column is hidden from the API.
- Each link follows its relation.
- The collection's list rule can be built for a reader.

A source that fails is refused: core logs it, and reports cannot use it. A
source that links to a refused source is refused too. An auth collection's
`email` column is never a field. Each record's `emailVisibility` decides who
sees it, and a report cannot apply that. Pin your sources in the package's tests, after its migrations apply:

```go
if err := reports.CheckSources(app); err != nil {
    t.Fatal(err)
}
```

A report runs as its reader, through `POST /api/reports/run`:

- Every collection it reads, in every join and subquery, is filtered by the
  reader's own list rule.
- A linked row the reader cannot see leaves empty cells. Totals across a
  to-many link leave it out.
- A superuser-only rule (null) selects nothing. A report never runs as a
  superuser.
- A result lists the change-signal topics of every collection it read. A
  raw-SQL write to a source's collection must call `changesignal.Touch` (see
  "Change signals and live views" above).

**What a report refuses.** The engine answers 400 with the problem list when
a spec breaks one of these limits. It answers 422 when the spec names a field
or link that no source has, and when an amount is too large to total (code
`amount_too_large`). It answers 408 when a run passes 10 s: filter the report
or index the column. It answers 499, and logs no error, when the client goes
away before the run ends.

- A path follows at most 4 links, then the field. A longer path is refused.
- A to-many link counts once. A formula reads a to-many path only through
  `exists` or an aggregate column, and `with` does not apply to an `exists`
  link.
- A page cursor is tied to the report's sort keys, its viewer sort
  (`options.sort`), its group levels, its fold state (`options.expand`,
  `options.expandAll`, `options.collapse`) and the value types. A cursor from a
  report that sorts, groups or folds differently is refused. A detail or
  grouped report pages: a page holds 200 rows by default, and at most 1,000
  (`options.limit`). A grouped report (not a pivot, a layout report or a nest)
  counts its group, opening, detail and subtotal rows in document order: a
  closed group counts as its one row, and a group row and its opening row stay
  on one page. Its
  group rows page too, however many groups it has. Every page ends with the
  Total. Pivot, layout and nest reports read their group rows whole, at most
  5,000, and count their detail rows.
- A formula holds at most 32 levels and 1,000 nodes. Each formula column it
  reads counts in full, as the compiler copies it in. The filter has the same
  limit.
- A formula that combines amounts of two currency sources is refused
  ("amounts in different currencies cannot be combined"). This applies to
  `add`, `sub`, `min`, `max`, `if`, `case` and `coalesce`. Each currency
  field is a source, and so is each source's fixed `Source.Currency`, even
  when two sources give the same code. An amount combines with amounts of
  its own source.
- `options.expand` and `options.collapse` each list at most 5,000 group keys.
  `options.collapse` closes groups under a level that is open by default or
  under `options.expandAll`. When `collapse` is sent (also empty), the levels
  open by default stay open and `expand` opens groups of closed levels; when
  only `expand` is sent, only its groups open.
- A layout holds at most 200 items.
- A computed row's formula holds at most 2,000 nodes after the rows it reads
  are inlined. All computed rows together hold at most 20,000 (counted for
  every cell column). If a statement hits a cap, reference fewer earlier rows
  or split the statement.
- The package slug `reports` is refused by the OAuth registry: its
  `/api/reports/` routes belong to core.

**What a report returns.**

- Pivot, nest and layout results stop at 5,000 rows and add a notice, once per
  run. A `rowCap` or `nestCap` notice gives the rows it kept in `count`. A
  `pivotCap` notice gives the pivot values it left out in `leftOut`. A grouped
  report pages its group rows instead, so it has no `rowCap` notice. A layout
  report never drops its computed rows for the cap, and it ends in those
  computed rows with no Total row.
- Money columns group by currency. A column with more than one currency is
  marked (`mixedCurrency`, `currencyCell`) and the result adds a notice. An
  amount read through a formula keeps its currency, so its totals group by
  currency too. A nested (child row) amount names its currency in
  `currencyCell`, and each child row holds it.
- A money value mixed with a non-money operand in a formula rounds to whole
  cents.
- A formula column that reads a row value outside an aggregate is blank on
  group, subtotal and total rows, where there is no single row. Wrap the value
  in an aggregate to total it.
- A posted `null` or an empty multiple-choice list for a parameter means
  unset. A filter condition on an unset parameter matches every row. `and`
  and `or` leave that condition out. A `not`, or an `exists` whose `where` is
  only such conditions, also matches every row. A parameter with a default
  takes the default when nothing is posted.
- Each entry of `params` is a parameter as the run resolved it. A date range
  gives its days in `from` and `to`, an unset one `unset: true`. A range on
  the `allDates` preset is set but has no bounds, so its entry has no
  `unset`, no `from` and no `to` (`{ "id": "period", "type": "dateRange" }`):
  read that shape as "All dates". A `between` on it matches every row, a row
  with no day too, and a `bound` of it has no value.
- A row's `kind` is `detail`, `group`, `opening`, `subtotal`, `computed` or
  `total`. An `opening` row (id `opening:<group key JSON>`) is the first row
  under an opened group whose level has `opening`; it carries only the cells
  its `cells` name, and its `section` is the group row.
- `options.sort` (`[{ col, dir }]`, at most 10) orders detail rows inside their
  groups before the spec's own sort keys; group order and running values keep
  the spec's order. A `ResultColumn` with `sortable: true` accepts it. A sort
  on a running value or percent, a child row column, an unknown column or a
  pivot report is refused under `options.sort`.
- A result column may carry `heading: { value, type }` (see `columns[].heading`
  in "Fields added in version 4"); the view shows it in place of `label`. A group
  or subtotal row carries `level`: the group level, 1 for the first. Other rows
  have none. With a skipped level (`groups[].blank`), `depth + 1` is not the
  level.
- A grouped result pages (`page.next`), its group rows too; running values
  continue on the next page. `page.size` is the page's detail rows. A client
  merges pages by row id and drops each earlier page's trailing total and
  computed rows (a Total inside a page's slice, one per currency
  block, stays). Core's view also holds the newest page's trailing total and
  computed rows back while it has `page.next` (`holdWholeReportRows`), so the
  Total shows only with the last page; its column measure still reads them.
- A `lookup` with `pick: "sum"` over amounts in more than one currency gives no
  value and the notice `mixedCurrency` naming its columns (and, in
  `conditions`, each condition or sort key that read such a sum, by its spec
  path; a lookup's own `where` that read one is named by its path, for
  example `columns.2.formula.where`). A cell with no value for that reason
  names no currency. A notice of more than one
  sentence gives each sentence in `parts`; a client that joins the notices of
  several pages joins their parts, so a sentence two pages share shows once.

**Fields added in version 2.**

| Field | What it does |
|---|---|
| `columns[].total` | `"recompute"` (default) totals every row the filter keeps. `"rows"`: the innermost group level computes the column from its own base rows, as `"recompute"` does; the levels above it and the Total add the cells of the rows the result shows, after `having`, per currency. A column has an aggregate when it sets `aggregate`, or its formula holds an aggregate operator, or its formula reads (`{col}`) a column that has an aggregate or is a `"rows"` column, at any depth (a lookup's `value` and `where` do not count: they read another source). A column with no aggregate adds its detail cells on the innermost level. With no groups, the Total adds the detail cells. Pivot cells follow the same rule. Layout sections add their shown rows; computed rows are unchanged. Only for number and amount columns, and not on a running value or percent. |
| `columns[].labelField` | The field of a relation column's record that its cells show. |
| `columns[].detail` | `"blank"`: the column has no cell (and no `@currency` or `@label` cell) on any detail row, nested child rows included. Group, subtotal and total rows are unchanged. This is a column's string member, not the spec's top-level `detail` object (`detail.label`). Refused (`columns.N.detail`) for any other value, in a report with no groups, on a pivot report, on the column that labels detail rows (the first column when `detail.label` is not set, or the column a `detail.label` of exactly `{ col }` names), and on the first column of each nest level, which labels that level's child rows. Use it on a column that reads the group's record (a `record` formula): a plain path column that is not the group key has no value on group rows, so with `"blank"` it shows nowhere. |
| `groups[].having` | A condition on the level's totals, columns and record fields. A hidden group hides its children. Refused on the first group of a layout report. |
| `groups[].label` | A formula that labels the group's row. |
| `groups[].fill` | `{ where? }` lists the target's records that have no rows, on any level, under each parent group. |
| `groups[].open` | The level is open when the report opens and no expansion state is sent. |
| `sort[]` | `{ col, dir }` or `{ formula, dir }`. A formula sort key that reads a row's fields sorts detail rows only; group levels skip it. |
| `detail.label` | A formula that labels each detail row. Default: the first column. |
| `params[].optionsFrom` | `{ source, value, label, group?, groupOrder?, groupLabel?, where? }`: a choice param reads its options from a source, under the reader's rules. `groupOrder` (with `group`) orders the groups: `{ "by": "label" }` (default) by the group's label, a number by its value; `{ "by": "declared" }` in the order the group's select field declares its options (refused unless `group` ends at a select field; a stored value no option names comes after the named ones, by label); `{ "by": "path", "path": [...] }` by the smallest value of `path` (a to-one path on the options source, bound as `group` is; it reads the field's stored value, so a select orders by its stored value, not its label or declared place; a blank text is no value) over each group's options, with groups that have no value after the others. Ties and every order end by label; the blank group is always first. The options come group by group in that order, then by label, and a page's `next` carries the group, so each page only appends. `groupLabel` (with `group`) is the text of a whole-group pick, with `{group}` where the group's label goes (`"All {group} accounts"`); absent: `All {group}`. |
| formula leaf `{ record, path }` | Reads a field of a record group's record. |
| operator `lookup` | `{ op: "lookup", source, where?, value, pick? }`: from the rows of another source that the reader may see, one value (`min` by default, or `max`), their sum (`sum`: numbers or amounts; no match is empty), or the count of rows with a value (`count`: no match is 0). |
| `pivot.labelField` | The field of a link's record that a pivot over that link names its columns by. |
| `display.layouts[]` | `{ width, minWidth?, tracks, cells: [{ col, line, start?, span?, lines?, justify? }] }`: an arrangement of the columns on lines. The view uses one line when the rows are at least `display.oneLine.minWidth` wide and its columns fit (each column sized by its heading and values), else the first arrangement the rows reach (its `minWidth`) whose lines fit, widest `width` first, else automatic lines. Inside a line each column gets its content's width; `span` is its share of the spare room. `minWidth` (px, for example `"600px"`): the arrangement is used only where the rows are at least that wide. |

**Fields added in version 3.**

| Field | What it does |
|---|---|
| formula leaf `{ outer: [path] }` | Inside a lookup's `where` or `value`: the report row's value of a to-one path of the base source (a nested lookup still reads the report's row). Blank on group, subtotal and total rows. Refused outside a lookup and on a to-many path. |
| formula leaf `{ param, bound: "from" \| "to" }` | The first or last day of a date range param, as a `Date`. Blank (not unset) on an unset range or on All dates, so a comparison with it matches no row. |
| range preset `allDates` | A date range with no bounds: `between` on it matches every row, a row with a blank day included. The resolved param has no `from` or `to`. |
| `groups[].opening` | `{ label, cells? }`: the opening row of each opened group of the level. Each cell is a formula on the level (totals, records of this and outer groups, lookups, params) whose type and currency are its column's; not on the first column; not on a pivot's last level. A level with `opening` lets a group with no base rows open (a fill group included). |
| `columns[].rollup` | A formula shown on group, subtotal and total rows (and layout sections) in place of the column's total; detail rows keep the column's value. Blank where it reads a value the row does not have (a group's record on the Total). Refused with `total: "rows"`, on a child row column, on a pivot's cell column and on a column a group level groups by with no `bucket`. |
| run `options.sort` | See "What a report returns". |

**Spec version 4.** `reports.CurrentVersion` is 4, and it is the only version a
server reads: a spec of any other version is refused (`version must be 4`), and a
saved report of another version opens Broken. Version 4 adds the column fit, read
as the CSS properties of the same names. A setting that could cut a figure (a
number, amount, percent, date, datetime or yes/no column, or a first column whose
label is one) is refused: a figure takes only `flexGrow` and `minWidth` `auto` or
`max-content`.

**Fields added in version 4.**

| Field | What it does |
|---|---|
| `display.oneLine.minWidth` | px. The rows are drawn on one line only where they are at least this wide (and the columns fit); below it the arrangements and automatic lines are used. |
| `display.columns.<col>.width` | A length (`12ch`, `120px`, `25%`): the column is exactly this wide. `ch` and `px` are the text's width (the cell's padding is added); `%` is the cell's whole width as a part of the line. |
| `display.columns.<col>.minWidth` | `auto` (default: eight letters for text, the whole value for a figure), `min-content` (the longest word), `max-content` (the whole value), or a length. |
| `display.columns.<col>.maxWidth` | `none` (default) or a length; never under `minWidth`. |
| `display.columns.<col>.whiteSpace` | `nowrap` (default: one line, ends in "…") or `normal` (the text wraps at spaces). A wrapping column is never narrower than its longest measured word (plus 2 px for the width estimate; the measure reads the first 500 rows): a smaller `width` or `minWidth`, or a `%` that gives less, is raised to it. A `minWidth` wider than the content is kept. On web a word wider than its line is clipped, not split. |
| `display.columns.<col>.lineClamp` | 1 to 4, with `whiteSpace: "normal"`: the most lines the text wraps to (default 2). |
| `display.columns.<col>.textOverflow` | `ellipsis` only. |
| `display.columns.<col>.flexGrow` | 0 to 10: the column's share of the spare room on one line, the room past every column's least width. While both are below their content's width, a column with 2 gets twice the room of a column with 1, whatever their content. A growing column stops at its content's width while another growing column is still below its own, and the others share what it leaves by grow; once every growing column has its content's width, the rest is shared by grow again. A column with 0 gets at most its content's width (default 1 for text and the label column, 0 for other figures). |
| `groups[].blank` | `"skip"`: a base row whose key at this level is blank skips the level: no group row and no subtotal for it; its rows sit directly under the parent group (or layout section), one depth less per skipped level, keep their full group key (with the blank), and count in the parent's totals. Absent: a blank key forms a `(none)` group. A fill below a skipped level fills under the parent the rows reach. Refused on the first and the last group level, on a bucketed level and for any other value. |
| `groups[].hideEmpty` | `true`: a group whose child groups are all hidden, with no shown row passed up from a skipped level, is hidden with its subtotal, as a having would hide it (a `"rows"` total above it adds nothing it did not show; a `"recompute"` total counts every row the filter keeps, the rows of hidden groups included, as with any having). Allowed on a bucketed level. Refused on the last level, and on the first group of a layout report (its sections always show). |
| `columns[].heading` | A formula over params and values, computed once per run; the result column carries `heading: { value, type }` (text, date, datetime or number), which the view shows in place of `label` (a day in the reader's short form). Absent when the heading is blank: an unset param, a null, or an empty or whitespace-only text. `label` stays the column's name, and Customize and the builder name the column by it. Refused when it reads anything but params and values (a path is refused), or is of another type: money, yes/no, a date range and a list are refused. |
| `columns[].hidden` | `true`: the column is computed (a group, sort or section key, a `{col}` read) but the result leaves it and its cells out: no heading, no cell, no Customize entry, no arrangement, no export column, not a viewer sort key. Refused on the first column, on the first column of each nest level (it labels the child rows), on a column `display.columns` or `display.layouts` names, on a column a group's `opening` names, and on a pivot's cell column. |
| `layout[].within` | On a computed row: the id of a section earlier in the layout. The row is emitted after that section's rows and before its subtotal, at depth 1, only while the section is open, with `section` naming the section's group row; it never opens. A section with a within row can always open, also when a deeper level's having hides every one of its own rows. The section's group row and subtotal add it, and a later row's `{ subtotal: <section> }` reads the total with it (and with every within row of that section before the reader). Its formulas read lookups, params, values, earlier computed rows and other sections; reading its own section is refused. |
| `layout[].cells` | On a computed row: `{ <col id>: formula }`, the row's cell in that column (same leaves as `formula`). A number column it does not name takes `formula`; with no `formula` that cell is blank. A row with neither is refused, and so is a cell on the first column, a non-number column or a pivot's cell column. A **derived column** (a formula column with no aggregate or rollup that reads only `{col}`, `{param}`, values and scalar operators) is computed on every computed row, and on a section total that holds a within row, from that row's own cells. Any computed row, top-level or within, may read `{param}`. |
| `columns[].formula` `percentOfSection` | `{ op: "percentOfSection", args: [{ col }], subtotal, scale }`, in a layout report: the row's value of a numeric column (the engine accepts a number, amount or percent column, not a yes/no one; the builder offers number and amount) ÷ the total of that column in the layout section `subtotal` names × 100, at `scale` places, half away from zero. Type percent. Every detail, group, subtotal, section and computed row divides its own cell; a section's total is read with its `within` rows, also when the section is folded. On a pivot report the argument is the row's total across the pivot. Each currency divides by its own section's total; a total of 0 or blank gives no figure; an opening row stays blank. It is a column's whole formula and no other formula reads that column. Refused outside a layout report, when `subtotal` names no section (422 `unknown_subtotal`), when the argument is not a `{ col }` of a numeric column (a yes/no column is refused), inside any other operator, on a column with `aggregate`, and on a pivot's cell column; `subtotal` on any other operator is refused. |
| `pivot.range` | The id of a `dateRange` param. When the param resolves to both bounds, the pivot has one column for every bucket its `from`..`to` touches, in order, whether or not a row falls in it (a sum reads 0 there; min, max and average stay blank), at most `maxColumns`, then the `pivotCap` notice with the count left out in `leftOut`. A row outside the range counts only in the cell column. On All dates, or with no `range`, the columns are the buckets that have rows. Refused when the param is unknown (422 `unknown_param`) or not a `dateRange`, on a pivot with no `bucket`, and on a pivot not over a date. |
| `pivot.order` | `"spec"`: the result keeps the spec's column order and puts the pivot columns just before their cell column, which follows them as the row's total. Absent: the pivot columns come after every column. Refused for any other value and when the cell column is the spec's first column. |
| `columns[].pivot.labelType` | On a bucketed pivot column (and as `labelType` on a bucketed group row and its subtotal): `month`, `quarter`, `year`, `fiscalYear` or `day`, with the raw key in `labelValue` (`"2026-01"`, `"2026-Q1"`, `"2026"`, the fiscal year's end year as a number, `"2026-01-05"`). `label`, `pivot.key` and the group key stay the raw key; the view writes it in words (English): "Jan 2026", "Q1 2026", "2026", "FY 2026", the short day. These types are never a field's type. |

`<col>` is a spec column id. A pivot's cell columns (one per pivot value) all take
the fit of their spec column.

A param of type `amount` holds cents and a currency. A multiple choice param
may hold a whole group as `{"group": "<value>"}`; the server expands it under
the reader's rules at run time. Report option lists and group keys bind as one
JSON parameter, so there is no SQL variable limit on them.

For example, a chart of accounts source (`zoo.accounts`) whose `type` select
declares its options as asset, liability, equity, income, expense lists an
Accounts filter's groups in that order, not A–Z:

```json
{
  "id": "accounts",
  "type": "choice",
  "multiple": true,
  "optionsFrom": {
    "source": "zoo.accounts",
    "value": [{ "field": "id" }],
    "label": [{ "field": "name" }],
    "group": [{ "field": "type" }],
    "groupOrder": { "by": "declared" },
    "groupLabel": "All {group} accounts"
  }
}
```

To order the types by a number the source keeps instead, use
`"groupOrder": { "by": "path", "path": [{ "field": "sort" }] }`.

For example, a ledger of lines (`zoo.lines`) grouped by account shows each
account's number on the account's row and leaves it out of the opened lines,
which the memo labels:

```json
{
  "version": 4,
  "source": "zoo.lines",
  "columns": [
    { "id": "name", "path": [{ "link": "account" }] },
    {
      "id": "no",
      "label": "No.",
      "formula": { "record": "byAccount", "path": [{ "field": "number" }] },
      "detail": "blank"
    },
    { "id": "debit", "path": [{ "field": "amount" }], "aggregate": "sum" }
  ],
  "groups": [{ "id": "byAccount", "col": "name" }],
  "detail": { "label": { "path": [{ "field": "memo" }] } }
}
```

**Param options.** `POST /api/reports/param-options` reads one page of a
choice param's options for a draft `spec` or a saved `reportId` the caller can
view. It sends `param`, and optionally `search`, `group`, `after`, `limit`
(default 50), `params` and `today`; `values` instead asks for the labels of
those values alone. It answers `options`, `groups` (with the first page only),
`groupsCut` and `next`. It reads under the reader's list rules and writes
nothing.

### Shipped and saved reports

People save reports in core's `reports` collection and share them through
`report_shares`, with users or groups, as viewer or editor. A share gives the
design only: each reader runs the report with their own rights. An admin can
publish a report to every user except guests.

A package ships its own reports as published reports. Register them from the
package's `Register(app)`:

```go
reports.RegisterShipped(reports.Shipped{
    Key:         "zoo.feed_costs", // "<slug>.<name>", stable
    Title:       "Feed costs",
    Description: "Every feeding's cost",
    Spec:        json.RawMessage(feedCostsSpec),
    Revision:    1,
})
```

- When the server starts, and when an admin is saved, core creates each
  shipped report that is missing. The first admin (the oldest owner, else
  the oldest admin) owns and publishes it. A fresh install gets its shipped
  reports when the setup wizard creates the first owner.
- Raise `Revision` whenever `Spec`, `Title` or `Description` changes. At the
  next start, core replaces the stored report's spec, title and description.
  It keeps the owner, and keeps the report unpublished if its owner
  unpublished it.
- A shipped spec that does not validate against the sources is logged and
  skipped. Pin the spec in the package's tests: call `reports.CheckSources`,
  then `reports.Load` and `reports.Validate`.
- Set `Unpublished: true` for a report only some people should see. Core
  creates it unpublished and never publishes it, also on a new revision. The
  package shares it itself: bind `app.OnRecordAfterCreateSuccess("reports")`
  and add `report_shares` rows for each new row with the package's
  `shipped_key` (the `app.Save` in `ReconcileShipped` fires this hook), and
  reconcile again when the server starts, after `ReconcileShipped`, to cover
  reports that already exist. Both run for one report, so the share code must
  be idempotent: add only the shares that are missing. Core has no share API
  for this.
- A shipped report cannot be deleted while the package ships it. The
  server answers "This report ships with a package. An admin who owns or
  edits it can unpublish it instead." Only an admin with update access (the
  report's owner or an editor) can change `published`.

A source, link or field can disappear when a package is upgraded or removed.
Saved reports then cannot run: a run answers 422 and names the missing part.
At every start, core marks each such report (`reports.broken`); in dev it
logs each one with its owner. To list them with their owners:

```sh
tinycld reports validate
```

It exits non-zero when a saved report is broken or a report source is
refused. Keep an alias for every renamed field or link (see "Report sources"
above), so saved reports keep working.

When a user is offboarded, each offboard mode settles their reports:

- Reassign: every report goes to the successor.
- Delete: each unpublished report is deleted with its shares.
- Keep: each unpublished report stays with the anonymized account.

In every mode, a published or shipped report goes to the heir and is never
deleted. The heir is the successor, else the admin who offboards the user,
else the first admin. When no heir exists, the offboard is refused and
nothing changes.

#### Report e2e helpers

A package's e2e opens core's report view with `openReportsHub`, `hubRow`,
`rowOf`, `cellOf`, `pressToolbar` and `chooseParamOption`, all from
`@tinycld/core/e2e-helpers`.

#### Linking to a package's reports

Core's Reports hub is at `/a/reports`. To show only your package's reports and
report sources (for a sidebar "Reports" entry), link to it with your slug:

```ts
router.push(orgHref('reports', { package: '<slug>' }))
```

The hub then lists, in one list with no tabs, every saved report the reader can
open whose source key starts with `<slug>.` (their own, shared with them, and
published), and offers only those sources — so give every source a key that starts
with your slug. The builder reads
the sources from `GET /api/reports/sources` (signed-in users, not API tokens): each
source's fields with their types and aggregates, and its links. It shows no record
data.

### Test build tag

`pnpm run test:server` builds with the `no_ui` tag so PocketBase's admin UI route
registration is skipped during tests (PB v0.37+ panics on duplicate route
registration across test scenarios that share an app). The shipped binary is
built **without** `no_ui`, so the admin UI ships in production.

---

## Cross-package coupling

Siblings must not depend on each other. When a feature in one package depends
on another being present (e.g. the takeout importer wants to import mail data
only if mail is linked), use the runtime package registry:

```ts
import { usePackages } from '@tinycld/core/lib/packages/use-packages'

const packages = usePackages()
const mailAvailable = new Set(packages.map((p) => p.slug)).has('mail')
```

Do **not** add a hard `@tinycld/mail` import to the dependent package — that
makes the dependency load-bearing at compile time and breaks core's lean-shell
guarantee (a fresh clone must typecheck with zero feature packages linked). If
a package genuinely needs a type from another, declare a minimal local
interface and tolerate the schema's absence at runtime.

The `dependencies` manifest field is **not** a compile-time import — it only
orders seed execution.

The same rule applies with more force to core itself: **core must not name a
package.** See [Registries: how core learns what a package
provides](#registries-how-core-learns-what-a-package-provides).

A package may set a sign-in notice (`useSignInNoticeStore` from
`@tinycld/core/lib/stores/sign-in-notice-store`) before it routes an
unauthenticated user to sign-in; the login form shows it and clears it after a
successful sign-in.

---

## Development loop & where to edit

```sh
# Clone the feature siblings you want next to the app shell, then:
cd ~/code/tinycld                   # workspace root
pnpm install                         # links members + runs the generator

cd ~/code/tinycld/tinycld           # other commands run from the app shell
pnpm run dev                         # packages:generate, then Expo + PocketBase
pnpm run checks                      # biome + tsc (lints app + all linked siblings)
pnpm run test                        # vitest for the app shell
pnpm run pkg:test:unit               # vitest across every present member
pnpm run test:e2e                    # playwright (all linked siblings)
pnpm run test:e2e --project=@tinycld/mail   # a single package's e2e specs
pnpm run test:server                 # Go tests (no_ui build tag)
```

Where changes go:

| Change | Repo to edit |
|---|---|
| A feature's behavior | that sibling repo (`mail/`, `calc/`, …) — commit & push there |
| Shared lib / UI / providers | `tinycld/core/` (`@tinycld/core`, nested in the shell repo) |
| Bundler config, scripts, generator, `app/` tree, provider wiring | `tinycld/` (app shell) |

**Never commit generator output.** These paths are gitignored and regenerate
on every `packages:generate`:

- `tinycld/lib/generated/` (incl. its `package.json` for `@tinycld/app-generated`,
  `tinycld-config.ts`, `package-help.ts`, `package-icons.ts`,
  `uniwind-sources.css`)
- `tinycld/tinycld.config.ts` and `tinycld/tinycld.seeds.ts`
- generated app routes under `tinycld/app/a/(app)/<slug>/**` and the
  generated public routes under `tinycld/app/p/<path>`
- `tinycld/server/pb_migrations/` + `tinycld/server/pb_hooks/` (symlinks),
  `server/package_extensions.go`, `server/go.work`
- `tinycld/core/types/pbSchema.ts` and `pbZodSchema.ts` (regenerated from the
  on-disk PocketBase migrations every install — the source of truth)

(The `node_modules/@tinycld/*` symlinks are also local-only state, created by
`link-members.ts` — never `git add` them. The app-owned route dirs
`app/a/(app)/settings/`, `app/a/(app)/help/`, and `app/a/(app)/admin/`, plus
`app/a/(app)/_layout.tsx` and `app/p/_layout.tsx`, are tracked via `.gitignore`
carve-outs despite living under a gitignored tree.)

---

## See also

- Root `CLAUDE.md` — condensed repo-layout overview Claude reads during dev.
- `CONTRIBUTING.md` — code style, data-query conventions, Zustand/form rules.
- https://tinycld.org/docs — task-framed docs for human contributors.
