# Project Guidelines

## Documentation drift

When you change a code-style or data-access rule also update the matching task page in the `web` repo at `web/src/content/docs/tasks/<file>.md`. The two are the canonical source for their respective audiences (devs and coding agents read this file during development; human contributors read the website at https://tinycld.org/docs), and they must not drift.

## Overview
This repo is the runnable Expo + PocketBase app shell. `@tinycld/core` is a
**separate sibling repo** at `~/code/tinycld/core/` (not bundled here). The
ecosystem is an npm workspace rooted at `~/code/tinycld/`; the app shell, core,
and every feature package (`@tinycld/contacts`, `@tinycld/mail`,
`@tinycld/calendar`, `@tinycld/drive`, `@tinycld/calc`, `@tinycld/text`,
`@tinycld/google-takeout-import`) are workspace members at the same level.
`pnpm install` at the workspace root runs the postinstall, whose
`link-members` step creates `node_modules/@tinycld/<name>` symlinks for every
present member (pnpm only links depended-on members itself, so link-members
covers the feature siblings nothing depends on).

The Go side of core is module `tinycld.org/core` (at `core/server/`), exporting
`coreserver` (registration orchestrator) plus subsystems `notify`, `push`,
`mailer`, `audit`, `textextract`, `thumbnails`, `render`, `realtime`,
`sharelink`. Core's PocketBase migrations live at `core/server/pb_migrations/`.
The app server (`server/main.go`) is module `tinycld.org/app` and consumes core
via `replace tinycld.org/core => ../core/server` (rewritten into `server/go.work`
by the generator).

Each feature sibling lives in its own git repo (`tinycld/<name>` on GitHub) and
is gitignored at the workspace root. The package generator (`scripts/generate.ts`)
walks `getPackages()` and wires installed members into the routes, registry, Go
server, and PocketBase migrations.

Validate changes from inside the member you touched: `pnpm exec tinycld-pkg check`
(biome + tsc + vitest, scoped to that member). For an ecosystem-wide sweep,
run from `app/`: `pnpm run pkg:check` (every member) or `pnpm run checks`
(biome + app typecheck). The Go side runs `pnpm run test:server`, which uses
the `no_ui` build tag so PocketBase's admin UI routes are skipped during tests
(PB v0.37+ panics on duplicate route registration when an `OnServe` handler
binds a fixed pattern across multiple test scenarios that share an app). The
shipped server binary is still built without `no_ui` so the admin UI is
available in production.

## Code Style & Patterns

- Strive for simplicity and clarity
- Keep JSX minimal: No complex ternary operators, map functions, or calculations inside the return statement.
- Move logic out: All state management, event handling, and data processing must be in custom hooks (useFeatureName) or helper functions outside the main component function.
- Co-locate, don't embed: If logic is used only in the component, define it just above the JSX, keep the JSX clean of declarations and other logic
- Extract: If a sub-section of a function or JSX is complex, break it into separate, smaller parts.
- Conditional visibility: Instead of hiding/showing large blocks using `{condition && <Component />}`, have the component accept an `isVisible` prop and return null when it shouldn't render.
- Comments: Only add comments that explain "why", not "what". Avoid trivial comments like `// Delete users` before `deleteFrom('user')` or `// Create profile` before `insertInto('profile')`. If the code is self-explanatory, no comment is needed.
- Testing: Write unit tests for new features. Only mock using helpers in tests/unit.helpers.tsx as needed, do not mock out any of our own components or actions.
- Run quality checks after any changes:
   - From inside the member: `pnpm exec tinycld-pkg check` (biome + tsc + vitest, scoped)
   - From `app/` for an ecosystem-wide sweep: `pnpm run checks` (biome + tsc) and `pnpm run pkg:test:unit` (vitest across all members)
- Embrace Type Inference: Do not over-specify types, allow TypeScript to infer types whenever possible.
  - DO NOT USE `any` to pass type checks, even with biome ignore comments.
- Biome enforces 4-space indentation, single quotes, ES5 trailing commas, and no superfluous semicolons.
- Components export PascalCase React components (`CustomerList.tsx`), hooks use camelCase with a `use` prefix, and utility modules use kebab-case file names.
- This app supports both light and dark modes. Do not use raw hex color values — use `className` with semantic Tailwind tokens (e.g., `className="text-foreground bg-background"`) or `useThemeColor('foreground')` for non-className contexts (Lucide icons, RN Pressable style props).
- Keep hooks pure and side-effect free; call them at the top level of React components.
- **Avoid `useState` and `useEffect`** — almost always there is a better primitive:
  - Form fields → `useForm` + zod (see **Forms** section)
  - Server/async data → `useLiveQuery`
  - Mutations → `useMutation` from `@tinycld/core/lib/mutations`
  - Derived values → use `.select()` on liveQuery expressions to return computed values
  - Responding to prop/state changes → compute during render (not `useEffect` + `setState`)
  - DOM refs / imperative handles → `useRef`
  - Shared UI state (sidebar, dialogs, popovers, compose mode) → Zustand stores (see **Zustand Stores** section)
  - Only reach for `useState` when you have genuinely local, synchronous UI state (e.g. a modal open/closed toggle, an accordion expanded state) that no other component needs. If you find yourself pairing `useState` with `useEffect` to sync or transform data, that's a signal to use a better pattern.
- Use React Hook Form + Zod for forms — see the **Forms** section below for details.
- Captured exceptions should captured using `captureException` which can be imported from `@tinycld/core/lib/errors`
- Do not embed logic inside JSX. Prefer early return, assignment to variable that's inserted into JSX or other patterns to keep the code clean.
- Do not edit types/pbSchema.ts or types/pbZodSchema.ts, they are auto-generated whenever the database is migrated and edits will not be saved
- After developing a feature, offer to add it to the website's docs and feature sections 

## Data Queries & Mutations
- **ALWAYS use pbtsdb** for all PocketBase data queries and mutations - never use PocketBase directly in components
- Import collections with `useStore('collection1', 'collection2')` from `pbtsdb` - it uses variadic arguments and returns a tuple array
  - Example: `const [tagsCollection] = useStore('tags')`
  - Example: `const [jobsCollection, addressesCollection] = useStore('jobs', 'addresses')`
- **Always use `useOrgLiveQuery`** from `@tinycld/core/lib/pocketbase` for org-scoped data queries instead of raw `useLiveQuery`. It automatically provides `OrgScope` (`orgId`, `userOrgId`, `orgSlug`) to the query callback, disables queries until org context loads (preventing cross-org data flash), and auto-includes `orgId`/`userOrgId` in the dependency array. Only use raw `useLiveQuery` in bootstrap hooks that `useOrgLiveQuery` itself depends on (`use-org-info`, `use-current-role`, `use-current-user-org`) or for genuinely user-level (non-org) queries like theme preferences.
  ```ts
  const { data: items } = useOrgLiveQuery((query, { orgId }) =>
      query.from({ item: itemsCollection }).where(({ item }) => eq(item.org, orgId))
  )
  ```
- Use TanStack DB operators (`eq`, `and`, `or`, `gt`, `lt`, etc.) from `@tanstack/db` for type-safe filtering
- Query syntax: `.from()`, `.where()`, `.orderBy()`, `.join()`, `.select()` - follows TanStack DB patterns
- **Prefer inline queries** — write `useStore` + `useOrgLiveQuery` directly in the screen component rather than wrapping them in custom hooks. This keeps the data flow visible where it's used. Only extract a shared hook when the exact same query is needed in 3+ screens.
- **Mutations** — use `useMutation` from `@tinycld/core/lib/mutations` (not directly from `@tanstack/react-query`). It supports generator-based mutation functions that automatically await pbtsdb `Transaction` objects:
  ```ts
  const create = useMutation({
      mutationFn: function* (data) {
          yield contactsCollection.insert({ id: newRecordId(), ...data })
      },
      onSuccess: () => router.back(),
      onError: handleMutationErrorsWithForm({ setError, getValues }),
  })
  ```
- For multi-step mutations, yield each Transaction sequentially, or yield an array for parallel execution
- Use `performMutations` from `@tinycld/core/lib/mutations` when you need to await Transactions inside an async function
- All collections are configured in `lib/pocketbase.ts` with expand relations
- Reference documentation:
   - Expo Router: https://docs.expo.dev/router/introduction/
   - pbtsdb: https://github.com/nathanstitt/pbtsdb/blob/main/llms.txt
   - TanStack DB: https://tanstack.com/db/latest/docs/overview

## Zustand Stores (UI State)
- Use Zustand stores for **shared UI state** — sidebar open/closed, dialog targets, compose mode, popover state, visible calendar IDs, active sections. Do NOT use Zustand for server data (use `useLiveQuery`), form state (use `useForm`), mutations (use `useMutation`), or URL state (use Expo Router params).
- Import `create` and persistence helpers from `@tinycld/core/lib/store`:
  ```ts
  import { create, persist, asyncStorage } from '@tinycld/core/lib/store'
  ```
- Store files live in `core/lib/stores/` (shared) or `<sibling>/tinycld/<slug>/stores/` (package-scoped).
- Existing core stores: `workspace-store.ts` (sidebar, drawer, active package), `auth-store.ts` (user session). Theme preference is stored in PocketBase via `useThemePreference()` from `lib/use-theme-preference.ts`, not in a Zustand store.
- Existing package stores (inside each sibling repo): `mail/stores/` (compose, thread list), `drive/stores/` (UI dialogs, search), `calendar/stores/` (popover, visible calendars).
- Use `persist` middleware with `partialize` when only some fields need AsyncStorage persistence:
  ```ts
  export const useMyStore = create<MyState>()(
      persist(
          (set) => ({
              persisted: true,
              transient: false,
              toggle: () => set((s) => ({ persisted: !s.persisted })),
          }),
          {
              name: 'tinycld_my_store',
              storage: asyncStorage,
              partialize: (s) => ({ persisted: s.persisted }),
          }
      )
  )
  ```
- Use selectors for granular subscriptions — `useMyStore(s => s.field)` only re-renders when `field` changes.
- Keep mutations in React hooks (not in stores) — they need reactive data from `useLiveQuery` and TanStack Query's `isPending`/`isError` tracking. Compose stores + mutation hooks in a slim `useFeature()` hook when a component needs both.
- Do NOT use React Context for new shared UI state — use a Zustand store instead.

## Logging
- Use the centralized logger from `lib/logger.ts` instead of console.log
- Import with: `import { log } from '@/lib/logger'`
- Available log levels: `log.debug()`, `log.info()`, `log.warn()`, `log.error()`
- `log.error()` automatically sends errors to Sentry for tracking
- Use `log.debug()` for verbose development info, `log.info()` for general information, `log.warn()` for warnings, and `log.error()` for errors
- The logger shows timestamps and colors in development mode

## Scripts Reference
- `pnpm run dev` starts Expo + PocketBase together (generator runs first). It reuses whatever is in `server/pb_data` — it does not seed.
- `pnpm run db:reset` wipes `server/pb_data`, re-runs migrations, and seeds a test user + org. **Run this once on a fresh checkout to get something to log in with.** It prints a boxed login summary at the end (see "Logging in for local dev" below).
- `pnpm run db:seed` seeds into the current database without wiping it; also prints the login summary.
- `pnpm run typecheck` runs `tinycld-pkg typecheck` (tsc for this member).
- `pnpm run checks` runs lint and typechecks (ecosystem-wide biome + app tsc).
- `pnpm run lint` (or `pnpm run lint:fix`) runs Biome over the app and every present sibling. Biome lives only in the app shell; `app/biome.json` is the single config for the app **and** every member. Sibling repos do not ship their own `biome.json` or `lint`/`checks` scripts — `pnpm run lint` walks the sibling dirs at their real workspace-root filesystem paths.
- `pnpm run pkg:check`, `pnpm run pkg:test:unit`, `pnpm run pkg:test:e2e` run the corresponding `tinycld-pkg` command across every present member.
- `pnpm run test:e2e` and `pnpm run test:server` cover the Playwright suite and supporting services.
   - never start or kill servers when running Playwright. It will manage it's own service and test data. If you see network errors or other issues, stop and ask for advice
- `pnpm run test:e2e <test file>` will run a single test.  This will also start the dev server for testing.
- `pnpm run export:web` runs `expo export --platform web` for production web builds.
- `pnpm run export:ios` and `pnpm run export:android` produce platform-specific exports (used by the docker/EAS build pipelines, not for local launch).

## PocketBase Notes
- Local data lives in `server/pb_data/`; reset via `tests/pb-test-server` scripts when fixtures fall out of sync.
- Keep migrations in `server/pb_migrations/` and describe manual steps in the PR body.
- Create api routes only as a last resort and after discussion. Prefer to create records using standard useMutation with pbtsdb stores.  If needed we can use golang hooks to observe and modify records as they're created/modified
- Go server hooks (e.g. CardDAV in the `@tinycld/contacts` sibling's `server/` directory) use SDK methods that bypass PocketBase API rules — they implement equivalent authorization manually. When changing API rules on a collection, check if a Go hook also accesses that collection and update its filters to match.

## Logging in for local dev
- After `pnpm run db:reset` (or `pnpm run db:seed`) the script prints a boxed summary of the credentials to use — you don't need to read the seed script.
- Two accounts exist: the **app user** `user@tinycld.org` (what you sign in to TinyCld with) and the **PocketBase superuser** `admin@tinycld.org` (the `/_/` admin UI and the `/setup` superuser dashboard).
- Both passwords are **generated randomly on first create** and printed in the box. To pin known passwords (e.g. to match an existing `.env` or share across resets), set `TEST_USER_PW` (app user) and `ADMIN_USER_PW` (superuser) in `app/.env`, or pass `--user-pw`. CI sets both so logins are deterministic — don't change that contract.
- Override the login emails with `TEST_USER_LOGIN` / `ADMIN_USER_LOGIN` in `app/.env`.
- There's no first-run `…/setup?token=…` link locally: `db:reset` creates the superuser up front (so it can seed), so PocketBase isn't on its first run. That token flow is only for an empty self-hosted instance. The `/setup` link `db:reset` prints goes to the superuser login → dashboard instead.

## Users & Organizations
- Users belong to orgs via the `user_org` junction table (many-to-many)
- Roles (`admin`, `clerical`, `workforce`) are per-org—a user can have different roles in different orgs
- Use `useOrgInfo()` from `@tinycld/core/lib/use-org-info` to get `{ orgSlug, orgId, org }` for the current org context
- `useOrgSlug()` from `@tinycld/core/lib/use-org-slug` returns just the org slug — on web it reads from `OrgSlugContext` (set by `[orgSlug]/_layout.tsx`), on native it reads from AsyncStorage
- `navigateToOrg(orgSlug)` from `@tinycld/core/lib/org-url` does a same-origin path navigation to `/a/<orgSlug>`
- Session helpers like `getRoleForOrg(session, orgSlug)` provide role lookups

## Routing & Navigation
- **Org context comes from the URL path**: `/a/<orgSlug>/<service>` — e.g. `/a/acme/contacts`, `/a/acme/mail`, `/a/acme/settings/profile`
- All org-scoped routes use the `/a/[orgSlug]/` prefix in file-system routing
- Use `useOrgHref()` from `@tinycld/core/lib/org-routes` for **type-safe** org-scoped navigation. Pass short paths (without the `/a/[orgSlug]` prefix) — misspellings are caught at compile time:
  ```tsx
  const orgHref = useOrgHref()
  router.push(orgHref('contacts/new'))
  router.push(orgHref('contacts/[id]', { id: contact.id }))
  router.push(orgHref('mail', { folder: 'sent' }))
  <Link href={orgHref('mail/[id]', { id: threadId })} />
  ```
- **Never** use `as OneRouter.Href` casts for org routes — always use `useOrgHref()`
- For dynamic package navigation where the slug is a runtime value (e.g. rail/tab bar), use the resolved URL string: `` `/a/${orgSlug}/${pkgSlug}` ``
- Use `useOrgInfo()` or `useOrgSlug()` to get the current org — `useOrgSlug()` reads from context on web

## Package System
- Feature packages live in **sibling git repos** at `~/code/tinycld/{contacts,mail,calendar,drive,calc,text,google-takeout-import}/` and are **pnpm workspace members** (listed in `pnpm-workspace.yaml`) of a workspace root at `~/code/tinycld/`. `@tinycld/core` is its own sibling repo at `~/code/tinycld/core/` (also a workspace member). The postinstall's `link-members` step creates `node_modules/@tinycld/<name>` symlinks for every member; pnpm itself only links depended-on members, so this covers the rest.
- `tinycld.packages.ts::getPackages()` enumerates the workspace member siblings that carry a `manifest.ts`, plus bundled core — that set is the source of truth. To add a feature package, clone it as a sibling of the app shell, add it to `pnpm-workspace.yaml`'s `packages:` list, and run `pnpm install` at the **workspace root** (`~/code/tinycld/`). There is no `packages:link`/`packages:install` — the workspace install does the linking.
- `pnpm run packages:generate` (runs as the workspace-root `postinstall`, and before `dev`) wires linked feature packages into the app. It is now a **thin** step — most wiring moved to runtime imports. It:
  - Re-exports package screens into `app/a/[orgSlug]/{slug}/` (org-scoped routes) and public routes into `app/<path>` (Expo Router needs files on disk).
  - Writes `tinycld.config.ts` (the installed-package source of truth — a typed `definePackageEntry` array) and `tinycld.seeds.ts` (Node-only seed list, kept out of the app bundle).
  - Writes `lib/generated/package-help.ts` (frontmatter-parsed help topics) and `lib/generated/uniwind-sources.css` (Tailwind `@source` roots).
  - Symlinks migrations/hooks into `server/pb_migrations/` and `server/pb_hooks/`, and generates the Go server wiring. Core's migrations are symlinked in via a separate explicit pass (core has no `manifest.ts`).
  - It NO LONGER generates collections/registry/sidebars/providers/settings/seeds files — those are derived at runtime from `tinycld.config.ts` (see `core/lib/packages/{derive-stores,static-registry,derive-components,derive-seeds}.ts`).
- Each feature package provides: `manifest.ts`, optionally `types.ts` (schema types), `collections.ts`, `screens/`, `public-screens/`, `pb-migrations/`, `pb-hooks/`, `seed.ts`, `tests/`, `settings/` (settings-panel contributions).
- Manifest fields `routes`, `publicRoutes`, and `nav` are all optional — a package can contribute only a settings panel (see `@tinycld/google-takeout-import`) with none of these.
- The type system is fully integrated — package `types.ts` exports a `{PascalSlug}Schema` type. `generate-config.ts` composes these into `MergedPackageSchema` (a literal intersection), which `pocketbase.ts` intersects with core's `Schema` so `useStore('packageCollection')` is strongly typed end-to-end. (The literal intersection — not a `typeof tinycldConfig` derivation — avoids a circular type reference through `coreStores`.)
- Package screens run in the app's bundle context and can import from the host app using `~/` and from core using `@tinycld/core/...`.
- `lib/generated/`, `app/a/[orgSlug]/*/`, and `app/p/*/` are gitignored; `app/a/[orgSlug]/_layout.tsx`, `app/a/[orgSlug]/settings/*`, and `app/p/_layout.tsx` are hand-written app files (force-add to git).
- **Install at the workspace root (`~/code/tinycld/`), not inside a sibling.** Members declare framework deps as `peerDependencies` with no own `dependencies`; pnpm's `nodeLinker: hoisted` flattens shared deps into the workspace-root `node_modules/` so every member resolves a single copy of `react`, `react-native`, `pbtsdb`, etc. (an npm-like flat tree). A stray `pnpm install` (or any other PM) inside a sibling would create a duplicate `node_modules` there and reintroduce "Type X is not assignable to type X" errors — keep siblings free of their own `node_modules`/lockfile. Metro watches the workspace root and Vitest aliases resolve members through that hoisted root install.
- Metro bundler resolves workspace members via a `watchFolders` entry for the workspace root in `metro.config.cjs`. The 388-line custom resolver is gone — npm's `node_modules/@tinycld/*` symlinks + Metro's default resolver handle member subpaths (`.ts`/`.tsx`/dir-index) with no singleton pins. Vitest still needs `@tinycld/core/*` path aliases because Vite's exports resolution lacks Metro's directory-index fallback.
- **Tailwind/Uniwind class scanning across linked packages is wired up by the generator.** Tailwind v4's scanner respects `.gitignore`, and the symlinks (and `node_modules` installs) for linked packages live inside gitignored paths. Without help, any utility class used **only** inside a linked package (e.g. `mr-3`, `bg-green-500`) silently produces no CSS rule — the className lands on the DOM element but has no styles. The generator writes one absolute `@source "<package-real-path>";` line per linked package into `lib/generated/uniwind-sources.css`, which `global.css` imports. The file regenerates on every `packages:link` / `packages:unlink`, so newly-linked packages (siblings, `node_modules`-installed third-party, or arbitrary checkouts) work automatically. Diagnose missing styles by checking `document.styleSheets` in DevTools for a `.your-class { ... }` rule; if missing, run `pnpm run packages:generate` and inspect `lib/generated/uniwind-sources.css`.
- Sibling-package tests run inside each sibling via its own `pnpm exec tinycld-pkg test` / `tinycld-pkg test:e2e` (each sibling has a `vitest.config.ts` and optional `playwright.config.ts` that merge with the canonical configs in `app/`). From `app/` you can run them across every present member with `pnpm run pkg:test:unit` / `pnpm run pkg:test:e2e`.
- Runtime hooks: `usePackages()` and `usePackage(slug)` from `@tinycld/core/lib/packages/use-packages`.
- Full documentation: `docs/packages.md` (in this repo, or in core's docs subtree).

## In-app help
- Packages contribute help via a `help/` directory of `<id>.md` files. Each file is a markdown document with a YAML frontmatter block (`title`, `summary` required; `tags: [..]` and `order: N` optional). The filename (without `.md`) is the topic ID. Declare it in `manifest.ts` with `help: { directory: 'help' }`. The generator writes `lib/generated/package-help.ts`; topics surface in the global hub at `/a/[orgSlug]/help`, the per-package help screen, and the right-slide drawer.
- `@tinycld/core` contributes baseline topics from `core/help/` (in the sibling repo). The generator includes core explicitly the same way it symlinks core's migrations.
- **Whenever you implement or significantly change a user-facing feature, add or update a help topic for it.** The feature is not "done" until a user can find out how to use it from inside the app.
- Open the drawer to a specific topic from anywhere with `openHelp('<pkg>:<id>')` from `@tinycld/core/lib/help/open-help`. Render `<HelpIcon topic="<pkg>:<id>" />` from `@tinycld/core/components/help/HelpIcon` next to UI controls. Cross-link between topics inside markdown bodies with `[label](help://<pkg>:<id>)` — the renderer intercepts that scheme and reopens the drawer instead of navigating away.
- Permalinks: `/a/[orgSlug]/help/[pkg]/[topic]` is a real route — shareable in conversation. The hub has full-text search (substring, weighted: title > tags > summary > body).

## Forms and other components
- All form UI components live in `ui/form/` and are exported from `~/ui/form`
- The barrel export re-exports `useForm`, `Control`, `Controller`, `zodResolver`, and `z` so screens only need one import:
  ```tsx
  import { useForm, zodResolver, z, TextInput, FormErrorSummary } from '~/ui/form'
  ```
- Available components: `TextInput`, `TextAreaInput`, `NumberInput`, `SelectInput`, `Toggle`, `FormErrorSummary`
- Each input accepts a generic `control` and type-safe `name` via `Path<T>` — pass the `control` from `useForm()` and field names are autocompleted
- Define a Zod schema per form, pass it via `zodResolver(schema)` to `useForm()`, and let TypeScript infer the form type from `defaultValues` — do not manually specify the generic
- Use `mode: 'onChange'` for real-time validation as the user types
- Show `<FormErrorSummary errors={errors} isEnabled={isSubmitted} />` above fields to surface all errors after first submit
- **Always use `useMutation` from `@tinycld/core/lib/mutations`** for form submissions — this ensures pbtsdb errors bubble up to the form via `onError`:
  ```ts
  const create = useMutation({
      mutationFn: function* (data) {
          yield collection.insert({ id: newRecordId(), ...data })
      },
      onSuccess: () => router.back(),
      onError: handleMutationErrorsWithForm({ setError, getValues }),
  })
  const onSubmit = handleSubmit((data) => create.mutate(data))
  ```
- Use `create.isPending` to disable the submit button and show loading state
- Error utilities in `lib/errors.ts`: `errorToString()`, `extractValidationErrors()`, `handleMutationErrorsWithForm()`, `captureException()`
- Do NOT use manual `useState` for form fields — always use `useForm` + the form components
- For complex forms, extract a `useFeatureForm()` hook that wraps `useForm` with schema, defaults, and submit logic
- See `@tinycld/contacts`'s `screens/new.tsx` (in the contacts sibling repo) for a reference implementation
- When developing a feature for a package, consider if the components you are adding would be of use to other packages. If so add them to the top-level ./components and offer to update other packages to use them.
- **UI Framework: Gluestack UI + Uniwind (Tailwind v4 for React Native)**
  - Use React Native primitives (`View`, `Text`, `Pressable`, `ScrollView`) with Tailwind `className` for styling via Uniwind.
  - Complex UI components (Modal, ActionSheet, AlertDialog) are built with Gluestack UI's headless `create*` factories in `ui/` — they use `@gluestack-ui/core` internally and are styled with Uniwind + `tva()`.
  - The custom `Menu` component in `ui/menu/` provides a compound-component API (`Menu`, `Menu.Trigger`, `Menu.Portal`, `Menu.Overlay`, `Menu.Content`, `Menu.Item`, `Menu.ItemTitle`, `Menu.Label`, `Separator`). It portals to `document.body` on web.
  - Layout: `View` + `className="flex-row"` replaces XStack; plain `View` is column by default (replaces YStack).
  - Text: `Text` + `className="text-sm text-foreground"` — use semantic color tokens (`text-foreground`, `text-muted`, `text-accent`, `text-danger`).
  - Colors via className: `bg-background`, `bg-surface-secondary`, `border-border`, `text-muted`, `bg-accent`, `text-accent-foreground`, `bg-danger-soft`, `text-danger`.
  - Colors via hook (for non-className contexts like Lucide icons, RN style props): import `useThemeColor` from `@tinycld/core/lib/use-app-theme`. This provides both built-in tokens (`foreground`, `background`, `muted-foreground`, etc.) and custom app tokens (`rail-background`, `rail-text`, `rail-active-text`, `sidebar-background`, `active-indicator`, `hover-background`).
  - Tuple form for multiple colors: `const [fg, bg] = useThemeColor(['foreground', 'background'])`.
  - Custom theme tokens are defined in `global.css` under `@variant light` / `@variant dark`.
  - Do not use `StyleSheet.create` — prefer `className` or inline `style` objects.
  - **Prefer `className` over `useThemeColor` + inline `style`.** When both work, the className form is shorter, declarative, and survives token renames. Reach for `useThemeColor` only when className isn't an option:
    - Props that take a literal color string (Lucide icons' `color`, `<RefreshControl tintColor>`, gradient stops, `shadowColor`).
    - Computed colors with opacity, e.g. `` `${activeIndicator}12` `` for a semi-transparent overlay (NativeWind's `bg-primary/10` covers many cases — use it when it does).
    - Style-callback APIs that don't accept className (`Pressable`'s `style={({ pressed }) => …}`).
    - Animated styles via reanimated where the value must be a JS string.

## Documentation & Support
- Gluestack UI: https://v5.gluestack.io/llms.txt
- Uniwind (Tailwind for RN): https://docs.uniwind.dev
- Expo documentation: https://docs.expo.dev/llms-full.txt
- PocketBase reference: https://raw.githubusercontent.com/Suryapratap-R/pocketbase-llm-txt/refs/heads/main/llms-full.txt
- PocketBase TS helper docs: https://raw.githubusercontent.com/satohshi/pocketbase-ts/refs/heads/master/README.md

## Project Structure & Module Organization
Expo routes live under `app/`, with organization screens in `app/a/[orgSlug]/` and shared layouts in `_layout.tsx`. Shared UI lives in `components/` and `ui/`, hooks in `hooks/`, and domain utilities in `lib/` and `constants/`. Static assets stay in `assets/` and `public/`. PocketBase data, migrations, and hooks live in `pb_data/`, `pb_migrations/`, and `pb_hooks/`. Tests and automation land in `tests/`, covering Playwright, Vitest, and Docker helpers.

