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

    routes: { directory: 'screens' },          // org-scoped routes
    publicRoutes: { directory: 'public-screens' }, // public top-level routes

    nav: {                                     // nav-rail entry
        label: 'Contacts',
        icon: 'users',                         // lucide-react-native name
        order: 10,
        shortcut: 'o',                         // single char; unique across packages
    },

    migrations: { directory: 'pb-migrations' },
    hooks: { directory: 'pb-hooks' },          // PocketBase JS hooks

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
| `routes.directory` | Each file becomes an org-scoped route under `app/a/[orgSlug]/<slug>/`. |
| `publicRoutes.directory` | Each file becomes a public top-level route under `app/<path>` (outside the org-scoped `app/a/[orgSlug]/` tree — e.g. drive's share routes). |
| `nav` | Adds a nav-rail entry. `shortcut` registers a `t <letter>` jump and must be unique (validated at generate time). |
| `migrations.directory` | `*.js` migrations symlinked into `server/pb_migrations/`. |
| `hooks.directory` | PocketBase JS hooks symlinked into `server/pb_hooks/`. |
| `collections` | `register` + `types` export subpaths; wires pbtsdb collections and the schema type. |
| `sidebar` / `provider` | A package may contribute a sidebar component **or** a context provider that wraps app children. |
| `settings[]` | Personal Settings panel contributions (`slug`, `label`, `component`). See [Extension points](#extension-points-settings-panels-and-sidebar-slots) below. |
| `slots[]` | Names of sidebar slots this package exposes for *other* packages to render into. Free-form strings; duplicates within one manifest are a generator error. Render with `<SidebarSlot target="<this-slug>" slot="<name>" />` from `@tinycld/core/components/sidebar-primitives`. |
| `sidebarContributions[]` | Inverse of `slots`: this package's contributions into *another* package's slot. Each `{ target, slot, component, order? }` is generator-validated. |
| `help.directory` | `<id>.md` topics surfaced in the in-app help hub. |
| `seed.script` | Dev sample-data function. |
| `server` | Go server extension: `package` is the subdir, `module` is its Go module path. |
| `build.script` | A build script run before bundling (e.g. an embedded webview bundle). |
| `dependencies[]` | A **slug-only** list of other packages — **advisory + seed-ordering only.** Used to topologically sort seed execution and as a soft hint; it imposes **no** version constraint and is never a compile-time import. |
| `peerVersions` | `{ '<slug or @tinycld/core>': '<semver range>' }` (e.g. `{ '@tinycld/core': '>=2.1 <3' }`) — **enforced** semver ranges keyed by slug. See [`dependencies` vs `peerVersions`](#dependencies-vs-peerversions) below. |

### `dependencies` vs `peerVersions`

These two fields look similar but do different jobs — keep them distinct:

- **`dependencies`** is a **slug-only** array (`['drive']`). It is **advisory + seed-ordering only**: the generator uses it to topologically sort seeds (a package's declared deps seed first) and as a soft presence hint. It carries **no** version information, is never enforced, and is **not** a compile-time import (a hard `@tinycld/<slug>` import would break the lean-shell guarantee — see [Cross-package coupling](#cross-package-coupling)).
- **`peerVersions`** declares **enforced semver ranges** keyed by slug, e.g. `{ '@tinycld/core': '>=2.1 <3' }`. It is checked by the **version-compatibility solver** (Setup → Versions UI): a proposed version change is **blocked** if it would leave any declared range unsatisfied. The check runs in the UI before **Apply**, and again **authoritatively on the server** before the change is actually applied.

A package can contribute **only** a settings panel (no nav, no routes — like
`@tinycld/google-takeout-import`), or purely `publicRoutes`, or any
combination. The manifest is read by the generator with a regex +
`new Function` (not a real `import`), so it must be a plain object literal with
no runtime imports.

### Manifest variation across shipped packages

- **contacts / mail** use `sidebar`; **calc** uses `provider`.
- Only **mail** declares `settings`.
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

export function registerCollections(
    newCollection: ReturnType<typeof createCollection<MergedSchema>>,
    coreStores: CoreStores,            // access core collections for expand relations
) {
    const contacts = newCollection('contacts', {
        omitOnInsert: ['created', 'updated', 'deleted_at'] as const,
        expand: { owner: coreStores.user_org },
        collectionOptions: { autoIndex: 'eager' as const, defaultIndexType: BasicIndex },
    })
    return { contacts }
}
```

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
that always provides `org` and `userOrg`, and optionally `user`:

```ts
import type PocketBase from 'pocketbase'

interface SeedContext {
    user: { id: string; email: string; name: string }
    org: { id: string }
    userOrg: { id: string }
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

### A. Route re-exports → `app/a/[orgSlug]/<slug>/**`

For each file under `routes.directory`, a one-line re-export shim that plugs a
sibling screen into Expo Router's filesystem routing — Expo Router needs real
files on disk, so these can't be derived at runtime:

```ts
export { default } from '@tinycld/contacts/screens/index'
```

`publicRoutes` work the same way but land at the public top level — `app/<path>`
(e.g. drive's share routes) — rather than under the org-scoped
`app/a/[orgSlug]/` tree. The generated public-route shims are gitignored; any
hand-written layout/index files in the public tree stay tracked and are
force-added despite the gitignore.

### B. `tinycld.config.ts` (via `scripts/generate-config.ts`)

The **single source of truth** for what's installed — a typed array, one entry
per linked package, each built by
`definePackageEntry<PkgSchema>()({ manifest, registerCollections, sidebar, provider, settings, sidebarContributions })`.
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

### G. Go server wiring → `server/package_extensions.go` + `server/go.work`

See [the Go server section](#the-go-pocketbase-server-side).

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
| `derive-components.ts` | `deriveSidebars` / `deriveProviders` / `deriveSettings` / `deriveSidebarContributions` + the `packageSidebars` / `packageProviders` / `packageSettings` / `packageSidebarContributions` consts | `package-sidebars.ts`, `package-providers.ts`, `package-settings.ts`, (new) `package-sidebar-contributions.ts` |
| `derive-seeds.ts` | `deriveSeeds` (ports the `dependencies` topo-sort) | `package-seeds.ts` |

`usePackages()` still merges the static set (`packageRegistry`) with
runtime-installed DB packages, exactly as before.

### Validation

The generator fails fast on:

- **Duplicate `nav.shortcut`** letters across packages.
- **Duplicate slot names** within one package's `manifest.slots`.
- **`sidebarContributions` targeting an unknown slot** on a present host package — the error lists the host's actual `slots` array so you can spot the typo immediately.

It warns (but does not fail) when:

- A `sidebarContributions` entry's `target` is not a present member. The contribution silently goes inactive — normal for a partial checkout — and wakes up automatically when the target is installed.

### Extension points: settings panels and sidebar slots

The generator threads two manifest-declared extension points through the same lazy-import pipeline. Both share the same lifecycle: **manifest field → `gen-config.ts` emits a `lazy(() => import(...))` entry → runtime derivation in `core/lib/packages/derive-components.ts` → host UI calls a React helper that consumes the registry → component loads under `<Suspense>` on first render.**

**Settings panels** (`manifest.settings: [{ slug, label, component }]`):

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

// app/app/a/[orgSlug]/settings/index.tsx
packageSettings.map(group => group.panels.map(panel => /* render link */))
// app/app/a/[orgSlug]/settings/[...section].tsx
// looks up the matching panel by [pkgSlug, panelSlug] and renders panel.Component
```

The `component` subpath must resolve through the package's `package.json` `exports` wildcard (e.g. `"./settings/*": "./tinycld/mail/settings/*.tsx"`). The component must default-export — the generator imports by default name. The `slug` must be unique across **all** installed packages, not just within one manifest.

**Sidebar slots** (`manifest.slots: ['sidebar.<name>']` on the host + `manifest.sidebarContributions: [{ target, slot, component, order? }]` on the contributor):

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

### Test build tag

`pnpm run test:go` builds with the `no_ui` tag so PocketBase's admin UI route
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

---

## Development loop & where to edit

```sh
# Clone the feature siblings you want next to the app shell, then:
cd ~/code/tinycld                   # workspace root
pnpm install                         # links members + runs the generator

cd ~/code/tinycld/tinycld           # other commands run from the app shell
pnpm run dev                         # packages:generate, then Expo + PocketBase
pnpm run checks                      # biome + tsc (lints app + all linked siblings)
pnpm run test:unit                   # vitest (includes sibling tests)
pnpm run test:e2e                    # playwright (all linked siblings)
pnpm run test:e2e --project=@tinycld/mail   # a single package's e2e specs
pnpm run test:go                     # Go tests (no_ui build tag)
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
- generated org-scoped routes under `tinycld/app/a/[orgSlug]/<slug>/**` and the
  generated public routes under `tinycld/app/<path>`
- `tinycld/server/pb_migrations/` + `tinycld/server/pb_hooks/` (symlinks),
  `server/package_extensions.go`, `server/go.work`
- `tinycld/core/types/pbSchema.ts` and `pbZodSchema.ts` (regenerated from the
  on-disk PocketBase migrations every install — the source of truth)

(The `node_modules/@tinycld/*` symlinks are also local-only state, created by
`link-members.ts` — never `git add` them. The app-owned files
`app/a/[orgSlug]/_layout.tsx` and `app/a/[orgSlug]/settings/*` are force-added
to git despite living under a gitignored tree.)

---

## See also

- Root `CLAUDE.md` — condensed repo-layout overview Claude reads during dev.
- `CONTRIBUTING.md` — code style, data-query conventions, Zustand/form rules.
- https://tinycld.org/docs — task-framed docs for human contributors.
