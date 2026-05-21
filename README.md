# @tinycld/core

The shared runtime + UI library for the TinyCld ecosystem. A standalone git
repo, cloned as a workspace member sibling of the app shell and the feature
packages (it is no longer bundled inside the app shell).

It exposes the app-facing surface of core via `@tinycld/core/*` subpaths:
`~/lib/*`, `~/ui/*`, `~/components/*`, `~/types/*`, the top-level `Providers`,
and the runtime package-derivation modules under `lib/packages/`
(`config-types`, `derive-stores`, `derive-components`, `derive-seeds`,
`static-registry`) which the app consumes from the generated
`tinycld.config.ts`. The Go side (`server/`, module `tinycld.org/core`) provides
`coreserver` plus subsystems (notify, push, mailer, audit, textextract,
thumbnails) and core's PocketBase migrations.

## Layout

Clone the workspace members as siblings under one root:

```sh
git clone <app-remote>   ~/code/tinycld/new/app       # the app shell (member "app")
git clone <core-remote>  ~/code/tinycld/new/core      # this repo
git clone <feature-remote> ~/code/tinycld/new/contacts  # any feature package
```

## Development

Core is typechecked + unit-tested as a workspace member — there is no separate
build. From the **workspace root** (`~/code/tinycld/new/`):

```sh
npm install          # links members + runs the app generator (postinstall)
npx vitest run       # runs core's unit tests as part of the suite
```

Members import core source directly via the `@tinycld/core/*` path alias
(resolved by the app's tsconfig `paths` for typecheck, the
`node_modules/@tinycld/core` symlink for Metro, and vitest aliases for tests).

Core's own `tsconfig.json` carries self-referential `@tinycld/core` /
`@tinycld/core/*` path aliases plus the `@tinycld/app-generated/*` alias and the
uniwind type augmentation, so `tsc -p tsconfig.json` typechecks core standalone.
