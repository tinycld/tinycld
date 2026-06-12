# @tinycld/core

The shared runtime + UI library for the TinyCld ecosystem. A nested member
at `tinycld/core/` inside the `tinycld` app shell repo — its own package,
imported as `@tinycld/core`.

It exposes the app-facing surface of core via `@tinycld/core/*` subpaths:
`~/lib/*`, `~/ui/*`, `~/components/*`, `~/types/*`, the top-level `Providers`,
and the runtime package-derivation modules under `lib/packages/`
(`config-types`, `derive-stores`, `derive-components`, `derive-seeds`,
`static-registry`) which the app consumes from the generated
`tinycld.config.ts`. The Go side (`server/`, module `tinycld.org/core`) provides
`coreserver` plus subsystems (notify, push, mailer, audit, textextract,
thumbnails, render, realtime, sharelink) and core's PocketBase migrations.

## Layout

Core is nested inside the `tinycld` shell repo; feature packages are sibling
repos under the same workspace root:

```sh
git clone git@github.com:tinycld/tinycld.git    ~/code/tinycld/tinycld    # app shell — core lives at tinycld/core/ (this repo)
git clone git@github.com:tinycld/contacts.git   ~/code/tinycld/contacts   # any feature package
```

## Public surface

Consumers import core through the `@tinycld/core/*` subpaths declared in
`package.json` `exports`:

| Subpath | What it provides |
| --- | --- |
| `@tinycld/core` | top-level `index.ts` re-exports |
| `@tinycld/core/lib/*` | runtime helpers (pocketbase, mutations, errors, store, org-routes, the `packages/` derivation modules, …) |
| `@tinycld/core/ui/*` | Gluestack + Uniwind UI primitives (forms, menu, modal, …) |
| `@tinycld/core/components/*` | shared React components |
| `@tinycld/core/types/*` | schema types (`pbSchema`, `pbZodSchema` — generated from applied migrations) |
| `@tinycld/core/file-viewer/*` | file preview/icon helpers |
| `@tinycld/core/Providers` | the top-level `Providers` component |

### Sidebar primitives

`@tinycld/core/components/sidebar-primitives` exports the building blocks every feature package uses to compose its sidebar:

- `SidebarNav` — scrollable outer container.
- `SidebarActionButton` — primary action at the top (e.g. "Compose", "+ Create").
- `SidebarItem` — clickable row with icon/label/optional color dot.
- `SidebarHeading` — section title with optional action button.
- `SidebarDivider` — visual separator between groups.
- `SidebarSlot` — extension point. A host package renders `<SidebarSlot target="<own-slug>" slot="<name>" />` at the insertion point; other packages target that `(target, slot)` pair via `sidebarContributions` in their manifest. Contributions are wired statically by the generator and rendered under `<Suspense>`. See [Sidebar slots](https://tinycld.org/docs/anatomy/sidebar-slots).

The Go side is module `tinycld.org/core` (`server/`), exporting `coreserver`
plus subsystems (`notify`, `push`, `mailer`, `audit`, `textextract`,
`thumbnails`, `render`, `realtime`, `sharelink`) and core's PocketBase
migrations under `server/pb_migrations/`.

## Development

Core is typechecked + unit-tested as a workspace member — there is no separate
build. From the **workspace root** (`~/code/tinycld/`):

```sh
pnpm install          # links members + runs the app generator (postinstall)
npx vitest run       # runs core's unit tests as part of the suite
```

Members import core source directly via the `@tinycld/core/*` path alias
(resolved by the app's tsconfig `paths` for typecheck, the
`node_modules/@tinycld/core` symlink for Metro, and vitest aliases for tests).

Core's own `tsconfig.json` carries self-referential `@tinycld/core` /
`@tinycld/core/*` path aliases plus the `@tinycld/app-generated/*` alias and the
uniwind type augmentation, so `tsc -p tsconfig.json` typechecks core standalone.

## License

[AGPL-3.0-only](./LICENSE) — © Nathan Stitt and the TinyCld contributors.
