# Package Terms and Conditions, Part 2 (client gate, e2e, help) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The app keeps a package closed until the owner accepts its terms: the owner reads and decides in the setup wizard, on first launch, on the package route or in Settings; everybody else sees no entry for the package, or a waiting screen on its route.

**Architecture:** The server state exists (part 1): `pkg_registry.terms_version` / `terms_accepted_version`, the `pkg_terms` text and the `pkg_terms_decisions` rows. This part is client code only. One hook, `useTermsBlocked()`, gives the set of closed slugs. It is a constant in a build where no package declares terms, and it closes every package that declares terms until the registry has loaded. A cover over the package Tabs (`PackageTermsGate`) shows the terms to the owner and a waiting screen to everybody else. The nav, settings, help, search and sidebar-slot lists drop a closed package. The owner decides by inserting a `pkg_terms_decisions` row through pbtsdb.

**Tech Stack:** Expo Router, React Native + NativeWind, pbtsdb + TanStack DB live queries, `useMutation` from `@tinycld/core/lib/mutations`, vitest, Playwright.

**Spec:** the `covers` list of `id: p0c-package-terms` in `/Users/nas/code/tinycld/docs/superpowers/plans/2026-10-21-books-run-manifest.yaml`, with the human decisions of 2026-11-23 in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p0c-package-terms/plan-outline.md`. Rulings: `rulings.md` in the same folder. Part 1: `docs/plans/2026-11-23-package-terms-part1.md` (tasks 1-3 built at `237e8a1c`; tasks 4-6 dropped).

**This part covers:** `covers` items 2 (wizard Apps step), 3 (first launch for an owner who has not agreed), 4 as the human changed it (the client hides nav, routes, settings, help and search; non-owners see the waiting screen; the server refuses nothing), 5 (decline stays closed; Settings lets the owner read and accept later), 7 client half (a package with no terms is unchanged), 8 (no package named in core), the e2e list and the help topic. With part 1 this is every `covers` item.

## Global Constraints

- Repo: `/Users/nas/tmp/tinycld/books-reports/tinycld`, branch `books/p0c-package-terms`. All paths below are relative to it. Commands run from the repo root unless a step says otherwise.
- **The gate is in the client only (the human, 2026-11-23).** Add no collection rule, no Go hook, no `pkgaccess` call and no route check that reads terms state. This part changes no Go file and no migration.
- Core names no package. Unit tests use fictional packages (`acme`, `gizmos`, `zoo`, `alpha`, `beta`). E2E uses the stub package `termsstub`, which a script under `tests/scripts/` writes; no file under `core/`, `app/`, `scripts/` or `server/` names it.
- Do not write the words "hosting", "saas", "tenant" or "router" (as a product) in code or comments.
- Data goes through pbtsdb: `useStore`, `useLiveQuery`, `useMutation` from `@tinycld/core/lib/mutations`. No raw PocketBase call in a component or hook. In e2e a raw superuser write is allowed only for the fixtures the spec headers name (rulings 12 and 30).
- No `useEffect` + `setState` to sync data. No complex logic in JSX; a part that can be hidden takes `isVisible` and returns `null`. No raw hex colors. No `console.*`. No `any`. No biome-ignore.
- Web and native: every component here uses React Native primitives and core UI components only. `SearchPalette.web.tsx` is web-only today and stays so; native has no palette.
- E2E: no `page.goto()` for in-app navigation; use `login`, `loginAs`, and nav testIDs. A phone run is `page.setViewportSize({ width: 390, height: 844 })`.
- The implementer does not run e2e. Each e2e step ends with a typecheck and a biome check of the spec; the run is the driver's (see "For the driver").
- After each task: `pnpm exec biome check <the files you changed>` (no output of errors is the pass) and the typecheck the task names.
- Commit messages: no mention of any AI tool.

## Review Focus

Failure modes that types and a happy-path test do not catch.

1. **A closed package shows for a moment before its state is known.** `blockedSlugsOf(declared, rows, false)` must close every declaring package; `useAccessiblePackagesResult` must not be ready before the terms state is. Pinned by `terms-state.test.ts` ("closes every package that declares terms until the registry has loaded").
2. **A build with no terms package changes behavior.** `useTermsBlocked` must be the constant hook then and read no store (a test that renders `SidebarSlot` with no provider, `core/components/sidebar-primitives/__tests__/sidebar-slot-props.test.tsx`, fails otherwise). `openEntriesOf` returns the same array when nothing is closed.
3. **A decline of an older version marks the new version declined**, so the owner is never asked about new terms. Pinned by `terms-state.test.ts` ("lists pending packages and marks a decline of the current version").
4. **The launch redirect loops or hijacks deep links.** It lives only in `app/a/(app)/index.tsx`, only for the owner, only for an undecided package.
5. **An e2e spec changes the one terms state of the primary deployment** and opens or closes the stub under the other specs. `package-terms-blocked.spec.ts` writes nothing; `package-terms-owner.spec.ts` runs on the second deployment and puts `declined` back after each test. Two other specs sign the owner in on the second deployment in other workers (`server-switcher.spec.ts`, `popover-placement.spec.ts`); while the stub is undecided their owner lands on Settings → Package terms, which is inside the app shell. Both wait for the shell or the sign-in form only, not for a landing package (reviewer's reading); the driver's run includes them.
6. **The decisions query runs for a non-owner** (the list rule refuses a member; a refused query never reads ready). `usePackageTermsList` asks only when `isOwner`.

## Measurements this plan relies on

All from the planner's prototype of the whole part in a scratch worktree at `237e8a1c` (removed after):

- `tsc --noEmit -p tsconfig.json` in `core/` and in the repo root: no error. `biome check` of every file below: no error.
- `vitest run core/lib/packages/__tests__/terms-state.test.ts core/components/setup/wizard/steps/__tests__/apps-step.test.ts core/tests/unit/store-sync-modes.test.ts`: 3 files, 21 tests passed. A run of every `core` and `scripts` test with `HAS_TERMS` forced to `true`: 532 files, 5042 tests passed.
- `sidebar-slot-props.test.tsx` renders `SidebarSlot` with no store provider. A `SidebarSlot` that always calls `useTermsBlocked` failed there with "useStore must be used within the Provider" when a package declares terms. Hence `GatedContributions` (Task 10).
- The scaffolder (Task 8) ran in the scratch tree with a stand-in `pnpm`; the stub it wrote passed `tsc`, `biome check` and the generator's `resolveTerms` (version `1`, body 130 bytes).
- No e2e run was made by the planner. Three things only the e2e run shows are listed in "For the driver".

## File Structure

| File | Task | Responsibility |
|---|---|---|
| `core/lib/pocketbase.ts`, `core/tests/unit/store-sync-modes.test.ts` | 7 | collections `pkg_terms`, `pkg_terms_decisions` |
| `core/components/setup/PackageManager.tsx` | 7 | the registry insert carries the two terms fields |
| `core/lib/packages/terms-state.ts`, `__tests__/terms-state.test.ts` (new) | 7 | pure state functions |
| `core/lib/packages/use-package-terms.ts` (new) | 7 | live state, lists, the decision mutation |
| `tests/scripts/scaffold-terms-stub.ts` (new) | 8 | writes the `termsstub` package |
| `tests/e2e/package-terms-helpers.ts` (new) | 8 | stub constants, superuser client, reset, second-deployment login |
| `tests/e2e/helpers.ts` | 8 | `login` option `landed` for a route outside the shell |
| `tests/e2e/package-terms-blocked.spec.ts` (new) | 8, 9, 10 | what everybody sees (primary deployment, no writes) |
| `.github/workflows/ci.yml` | 8 | scaffold step |
| `core/components/package-terms/TermsDocument.tsx`, `PackageTermsReview.tsx`, `TermsWaitingScreen.tsx`, `PackageTermsGate.tsx` (new) | 9 | terms text, owner review, waiting screen, route cover |
| `core/components/workspace/PackageTabs.tsx`, `WorkspaceLayout.tsx`, `MobileDrawer.tsx` | 9 | cover mounted; no sidebar |
| `core/lib/use-accessible-packages.ts` | 9 | non-owners lose closed packages |
| `app/a/(app)/settings/index.tsx` | 10, 11 | panels hidden; the Package terms link |
| `app/a/(app)/settings/[...section].tsx`, `account/[...section].tsx`, `system/[...section].tsx` | 10 | panel routes hidden |
| `core/lib/help/use-help-topics.ts` | 10 | help hidden |
| `core/components/search-palette/SearchPalette.web.tsx`, `useSearchResults.ts` | 10 | search hidden |
| `core/components/sidebar-primitives/SidebarSlot.tsx` | 10 | slot contributions hidden |
| `core/components/package-terms/TermsDialog.tsx` (new) | 11 | terms in a dialog |
| `app/a/(app)/settings/package-terms.tsx` (new) | 11 | read and accept later |
| `app/a/(app)/index.tsx`, `app/a/(app)/settings/packages.tsx` | 11 | launch redirect; link |
| `tests/e2e/package-terms-owner.spec.ts` (new) | 11, 12 | the owner's decisions (second deployment) |
| `core/components/setup/wizard/steps/app-choices.ts`, `__tests__/apps-step.test.ts`, `AppsStep.tsx` | 12 | terms in the wizard |
| `core/help/package-terms.md` (new), `docs/packages.md` | 13 | help topic, client section |

A `diff` block below shows only the changed lines with their nearest context. Lines with `+` are added, lines with `-` are removed; make no other change in the file.

---

### Task 7: Client data layer

**Files:**
- Create: `core/lib/packages/terms-state.ts`
- Create: `core/lib/packages/__tests__/terms-state.test.ts`
- Create: `core/lib/packages/use-package-terms.ts`
- Modify: `core/lib/pocketbase.ts` (after `pkg_registry`; the `coreStores` list)
- Modify: `core/tests/unit/store-sync-modes.test.ts` (`CORE_COLLECTIONS`)
- Modify: `core/components/setup/PackageManager.tsx` (the insert in `RegisterPackageModal`)

**Interfaces:**
- Consumes: part 1's schema (`pkg_registry.terms_version`, `terms_accepted_version`; `pkg_terms`: `pkg`, `version`, `body`; `pkg_terms_decisions`: `pkg`, `terms_version`, `decision`, `user`, `created`); `packageRegistry` (`./static-registry`, each entry has `slug`, `name`, optional `terms`); `enabledStatusFor(on: boolean)` (`@tinycld/core/lib/setup/set-package-enabled`); `useCurrentRole(): { isOwner, isReady, … }`.
- Produces, in `terms-state.ts`: `type TermsState = 'none' | 'accepted' | 'pending'`; `interface TermsRow { slug; terms_version; terms_accepted_version }`; `interface DeclinedRow { pkg; terms_version }`; `interface PendingTerms { slug; name; version; isDeclined }`; `termsStateOf(row | undefined): TermsState`; `blockedSlugsOf(declared: readonly string[], rows: readonly TermsRow[], isReady: boolean): ReadonlySet<string>`; `pendingTermsOf(rows, declined, nameOf): PendingTerms[]`; `hasUndecidedTerms(pending): boolean`; `openEntriesOf<T>(entries, slugOf, blocked): T[]`.
- Produces, in `use-package-terms.ts`: `TERMS_PACKAGES: { slug: string; name: string }[]`; `declaresTerms(slug: string): boolean`; `interface TermsBlocked { blocked: ReadonlySet<string>; stateOf(slug): TermsState; isReady: boolean }`; `useTermsBlocked(): TermsBlocked`; `slugOfGroup(group: { pkgSlug: string }): string`; `slugOfEntry(entry: { slug: string }): string`; `useTermsOpen<T>(entries: readonly T[], slugOf: (entry: T) => string): T[]`; `interface PackageTermsEntry { id; slug; name; version; state: 'accepted' | 'pending'; isDeclined: boolean; isEnabled: boolean }`; `usePackageTermsList(): { entries: PackageTermsEntry[]; pending: PendingTerms[]; isReady: boolean }`; `interface TermsDecisionInput { entry: Pick<PackageTermsEntry, 'id' | 'slug' | 'version' | 'isEnabled'>; decision: 'accepted' | 'declined'; shouldEnable?: boolean }`; `useTermsDecision(options?: { onDecided?: (input: TermsDecisionInput) => void }): { decide(input: TermsDecisionInput): void; isPending: boolean }`.

- [ ] **Step 1: Regenerate the schema types**

Part 1 added a migration. `core/types/pbSchema.ts` is generated from the migrations and is gitignored.

Run: `pnpm run packages:generate`
Expected: exit 0. `command grep -c terms_accepted_version core/types/pbSchema.ts` prints a number above 0.

- [ ] **Step 2: Write the failing test**

Create `core/lib/packages/__tests__/terms-state.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
    blockedSlugsOf,
    hasUndecidedTerms,
    openEntriesOf,
    pendingTermsOf,
    termsStateOf,
} from '../terms-state'

const row = (slug: string, version: string, accepted: string) => ({
    slug,
    terms_version: version,
    terms_accepted_version: accepted,
})

describe('termsStateOf', () => {
    it('reads a package with no terms as none', () => {
        expect(termsStateOf(row('acme', '', ''))).toBe('none')
        expect(termsStateOf(undefined)).toBe('none')
    })
    it('reads an accepted current version as accepted', () => {
        expect(termsStateOf(row('acme', '2026-11-01', '2026-11-01'))).toBe('accepted')
    })
    it('reads a new terms version as pending again', () => {
        expect(termsStateOf(row('acme', '2026-12-01', '2026-11-01'))).toBe('pending')
        expect(termsStateOf(row('acme', '2026-11-01', ''))).toBe('pending')
    })
    it('ignores an accepted version left behind by a package that dropped its terms', () => {
        expect(termsStateOf(row('acme', '', '2026-11-01'))).toBe('none')
    })
})

describe('blockedSlugsOf', () => {
    const rows = [row('acme', 'v2', 'v1'), row('gizmos', 'v1', 'v1')]
    it('closes every package that declares terms until the registry has loaded', () => {
        expect([...blockedSlugsOf(['acme', 'gizmos'], [], false)]).toEqual(['acme', 'gizmos'])
    })
    it('closes only pending packages once the registry has loaded', () => {
        expect([...blockedSlugsOf(['acme', 'gizmos'], rows, true)]).toEqual(['acme'])
    })
    it('closes nothing when no package declares terms', () => {
        expect(blockedSlugsOf([], [], false).size).toBe(0)
        expect(blockedSlugsOf([], [], true).size).toBe(0)
    })
    it('opens a declared package that has no registry row', () => {
        expect(blockedSlugsOf(['zoo'], [], true).size).toBe(0)
    })
})

describe('pendingTermsOf', () => {
    const names = (slug: string) => ({ acme: 'Acme', gizmos: 'Gizmos' })[slug] ?? slug
    it('lists pending packages and marks a decline of the current version', () => {
        const rows = [row('acme', 'v2', 'v1'), row('gizmos', 'v1', ''), row('zoo', 'v1', 'v1')]
        const declined = [
            { pkg: 'gizmos', terms_version: 'v1' },
            // A decline of an older version does not count for the new one.
            { pkg: 'acme', terms_version: 'v1' },
        ]
        expect(pendingTermsOf(rows, declined, names)).toEqual([
            { slug: 'acme', name: 'Acme', version: 'v2', isDeclined: false },
            { slug: 'gizmos', name: 'Gizmos', version: 'v1', isDeclined: true },
        ])
    })
    it('asks the owner only while a pending package has no decision', () => {
        const declined = [{ slug: 'a', name: 'A', version: 'v1', isDeclined: true }]
        expect(hasUndecidedTerms(declined)).toBe(false)
        expect(
            hasUndecidedTerms([
                ...declined,
                { slug: 'b', name: 'B', version: 'v1', isDeclined: false },
            ])
        ).toBe(true)
        expect(hasUndecidedTerms([])).toBe(false)
    })
})

describe('openEntriesOf', () => {
    const groups = [{ pkgSlug: 'acme' }, { pkgSlug: 'gizmos' }]
    it('drops the entries of closed packages', () => {
        expect(openEntriesOf(groups, g => g.pkgSlug, new Set(['acme']))).toEqual([
            { pkgSlug: 'gizmos' },
        ])
    })
    it('returns the same list when nothing is closed', () => {
        expect(openEntriesOf(groups, g => g.pkgSlug, new Set())).toBe(groups)
    })
})
```

Run: `pnpm exec vitest run core/lib/packages/__tests__/terms-state.test.ts`
Expected: FAIL, the import of `../terms-state` does not resolve.

- [ ] **Step 3: Write the state functions**

Create `core/lib/packages/terms-state.ts`:

```ts
/**
 * The terms state of packages, from the registry rows. Pure: the hooks in
 * use-package-terms.ts feed it and every gate reads its result.
 *
 * The server owns the state (pkg_registry.terms_version and
 * terms_accepted_version, see core/server/pkgterms). The client only decides
 * what to show.
 */

export type TermsState = 'none' | 'accepted' | 'pending'

/** The registry fields the terms state is read from. */
export interface TermsRow {
    slug: string
    terms_version: string
    terms_accepted_version: string
}

/** A `declined` row of pkg_terms_decisions. */
export interface DeclinedRow {
    pkg: string
    terms_version: string
}

/** A package the owner has not accepted the current terms of. */
export interface PendingTerms {
    slug: string
    name: string
    version: string
    /** The owner declined this version. The package stays closed; no launch prompt. */
    isDeclined: boolean
}

/** A package with no registry row has no terms the owner can accept. */
export function termsStateOf(
    row: Pick<TermsRow, 'terms_version' | 'terms_accepted_version'> | undefined
): TermsState {
    if (!row || row.terms_version === '') return 'none'
    return row.terms_accepted_version === row.terms_version ? 'accepted' : 'pending'
}

/**
 * The slugs to keep closed. While the registry has not loaded, every package
 * that declares terms is closed: an open package must never show before its
 * state is known. A package that declares none is never in the set.
 */
export function blockedSlugsOf(
    declared: readonly string[],
    rows: readonly TermsRow[],
    isReady: boolean
): ReadonlySet<string> {
    if (!isReady) return new Set(declared)
    return new Set(rows.filter(r => termsStateOf(r) === 'pending').map(r => r.slug))
}

/** The pending packages, in registry order, each with its decline mark. */
export function pendingTermsOf(
    rows: readonly TermsRow[],
    declined: readonly DeclinedRow[],
    nameOf: (slug: string) => string
): PendingTerms[] {
    const declinedKeys = new Set(declined.map(d => `${d.pkg}\n${d.terms_version}`))
    return rows
        .filter(r => termsStateOf(r) === 'pending')
        .map(r => ({
            slug: r.slug,
            name: nameOf(r.slug),
            version: r.terms_version,
            isDeclined: declinedKeys.has(`${r.slug}\n${r.terms_version}`),
        }))
}

/** True when the owner must still be asked: a pending package with no decision. */
export function hasUndecidedTerms(pending: readonly PendingTerms[]): boolean {
    return pending.some(p => !p.isDeclined)
}

/** Drops the entries of closed packages from a per-package list. */
export function openEntriesOf<T>(
    entries: readonly T[],
    slugOf: (entry: T) => string,
    blocked: ReadonlySet<string>
): T[] {
    if (blocked.size === 0) return entries as T[]
    return entries.filter(e => !blocked.has(slugOf(e)))
}
```

Run: `pnpm exec vitest run core/lib/packages/__tests__/terms-state.test.ts`
Expected: 1 file, 12 tests passed.

- [ ] **Step 4: Register the two collections**

In `core/lib/pocketbase.ts`:

```diff
 const pkg_registry = newCollection('pkg_registry', {
     omitOnInsert: ['created', 'updated'],
     ...onDemand,
     ...indexing,
 })
 
+// The terms text of each package that declares terms. Read-only from the
+// client: the server writes it at boot (core/server/pkgterms).
+const pkg_terms = newCollection('pkg_terms', {
+    ...onDemand,
+    ...indexing,
+})
+
+// The owner's decisions on package terms. The owner inserts a row; a server
+// hook checks it and, for `accepted`, writes pkg_registry.terms_accepted_version.
+const pkg_terms_decisions = newCollection('pkg_terms_decisions', {
+    omitOnInsert: ['created'],
+    ...onDemand,
+    relations: { user: users },
+    ...indexing,
+})
+
 // Build-history catalog for the Admin panel's Builds tab. Read-only from the
```

and in the `coreStores` object:

```diff
     org_pkg_access,
     pkg_registry,
+    pkg_terms,
+    pkg_terms_decisions,
     pkg_build,
```

In `core/tests/unit/store-sync-modes.test.ts`, in `CORE_COLLECTIONS`:

```diff
     'pkg_registry',
+    'pkg_terms',
+    'pkg_terms_decisions',
     'pkg_build',
```

In `core/components/setup/PackageManager.tsx`, in the `pkg_registry` insert of `RegisterPackageModal` (the regenerated row type now has the two fields):

```diff
                 manifest_json: '',
                 has_server: false,
+                // A source that is not installed declares no terms yet; the boot
+                // sync writes the version when its package is installed.
+                terms_version: '',
+                terms_accepted_version: '',
             })
```

- [ ] **Step 5: Write the hooks**

Create `core/lib/packages/use-package-terms.ts`:

```ts
import { eq, inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { enabledStatusFor } from '@tinycld/core/lib/setup/set-package-enabled'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { newRecordId } from 'pbtsdb/core'
import { useMemo } from 'react'
import { packageRegistry } from './static-registry'
import {
    blockedSlugsOf,
    openEntriesOf,
    type PendingTerms,
    pendingTermsOf,
    type TermsState,
    termsStateOf,
} from './terms-state'

/**
 * The packages of this build that declare terms (manifest `terms`). Static, so
 * a deployment with no such package sends no terms query at all, and a package
 * with terms is closed from the first render, before the registry has loaded.
 */
export const TERMS_PACKAGES = packageRegistry
    .filter(p => p.terms)
    .map(p => ({ slug: p.slug, name: p.name }))

const TERMS_SLUGS = TERMS_PACKAGES.map(p => p.slug)
const HAS_TERMS = TERMS_SLUGS.length > 0
const NO_SLUGS: ReadonlySet<string> = new Set()

const TERMS_SLUG_SET: ReadonlySet<string> = new Set(TERMS_SLUGS)

/** True for a package of this build that declares terms. Static: reads no store. */
export const declaresTerms = (slug: string) => TERMS_SLUG_SET.has(slug)

const nameOf = (slug: string) => TERMS_PACKAGES.find(p => p.slug === slug)?.name ?? slug

/** The registry rows of the packages that declare terms. One filter, so every caller shares one subscription. */
function useTermsRows() {
    const [pkgRegistry] = useStore('pkg_registry')
    const { data: rows = [], isReady } = useLiveQuery({
        queryKey: ['core', 'pkg-terms', 'rows'],
        query: query =>
            HAS_TERMS
                ? query
                      .from({ p: pkgRegistry })
                      .where(({ p }) => inArray(p.slug, TERMS_SLUGS))
                      .select(({ p }) => ({
                          id: p.id,
                          slug: p.slug,
                          status: p.status,
                          terms_version: p.terms_version,
                          terms_accepted_version: p.terms_accepted_version,
                      }))
                : null,
    })
    return { rows, isReady: !HAS_TERMS || isReady }
}

export interface TermsBlocked {
    /** Slugs of the packages nobody may use yet. Empty when no package declares terms. */
    blocked: ReadonlySet<string>
    stateOf: (slug: string) => TermsState
    isReady: boolean
}

const NOTHING_BLOCKED: TermsBlocked = { blocked: NO_SLUGS, stateOf: () => 'none', isReady: true }

function useNothingBlocked(): TermsBlocked {
    return NOTHING_BLOCKED
}

function useBlockedFromRegistry(): TermsBlocked {
    const { rows, isReady } = useTermsRows()
    return useMemo(
        () => ({
            blocked: blockedSlugsOf(TERMS_SLUGS, rows, isReady),
            stateOf: slug => termsStateOf(rows.find(r => r.slug === slug)),
            isReady,
        }),
        [rows, isReady]
    )
}

/**
 * Which packages are closed until the owner accepts their terms.
 *
 * Chosen once for the build: with no package that declares terms this is a
 * constant and reads no store, so every surface that asks (the rail, help,
 * settings, search, sidebar slots) behaves as it did before terms existed.
 */
export const useTermsBlocked: () => TermsBlocked = HAS_TERMS
    ? useBlockedFromRegistry
    : useNothingBlocked

/** The package of a settings or help group. Module-level, so useTermsOpen gets a stable reference. */
export const slugOfGroup = (group: { pkgSlug: string }) => group.pkgSlug

/** The package of an entry that carries its slug. */
export const slugOfEntry = (entry: { slug: string }) => entry.slug

/** A per-package list (settings groups, help groups, search packages) without the closed packages. */
export function useTermsOpen<T>(entries: readonly T[], slugOf: (entry: T) => string): T[] {
    const { blocked } = useTermsBlocked()
    return useMemo(() => openEntriesOf(entries, slugOf, blocked), [entries, slugOf, blocked])
}

export interface PackageTermsEntry {
    /** The pkg_registry row id. */
    id: string
    slug: string
    name: string
    version: string
    state: Exclude<TermsState, 'none'>
    isDeclined: boolean
    isEnabled: boolean
}

/**
 * Every package with terms and its state, for the owner: the Package terms
 * screen, the launch prompt and the setup wizard. The decisions query runs
 * only for the owner; nobody else may read the collection.
 */
export function usePackageTermsList(): {
    entries: PackageTermsEntry[]
    pending: PendingTerms[]
    isReady: boolean
} {
    const { rows, isReady: rowsReady } = useTermsRows()
    const { isOwner, isReady: roleReady } = useCurrentRole()
    const [decisions] = useStore('pkg_terms_decisions')
    const shouldReadDecisions = HAS_TERMS && isOwner
    const { data: declined = [], isReady: declinedReady } = useLiveQuery({
        queryKey: ['core', 'pkg-terms', 'declined', shouldReadDecisions],
        query: query =>
            shouldReadDecisions
                ? query
                      .from({ d: decisions })
                      .where(({ d }) => eq(d.decision, 'declined'))
                      .select(({ d }) => ({ pkg: d.pkg, terms_version: d.terms_version }))
                : null,
    })
    return useMemo(() => {
        const pending = pendingTermsOf(rows, declined, nameOf)
        const entries = rows.flatMap((r): PackageTermsEntry[] => {
            const state = termsStateOf(r)
            if (state === 'none') return []
            return [
                {
                    id: r.id,
                    slug: r.slug,
                    name: nameOf(r.slug),
                    version: r.terms_version,
                    state,
                    isDeclined: pending.some(p => p.slug === r.slug && p.isDeclined),
                    isEnabled: r.status !== 'disabled',
                },
            ]
        })
        return {
            entries,
            pending,
            isReady: !HAS_TERMS || (rowsReady && roleReady && (!isOwner || declinedReady)),
        }
    }, [rows, declined, rowsReady, roleReady, isOwner, declinedReady])
}

export interface TermsDecisionInput {
    entry: Pick<PackageTermsEntry, 'id' | 'slug' | 'version' | 'isEnabled'>
    decision: 'accepted' | 'declined'
    /** The setup wizard turns the app on with the acceptance. */
    shouldEnable?: boolean
}

/**
 * Records the owner's decision. The server checks the row and, for an
 * acceptance, writes pkg_registry.terms_accepted_version; the registry query
 * above then reads the package as accepted.
 */
export function useTermsDecision(
    options: { onDecided?: (input: TermsDecisionInput) => void } = {}
) {
    const [decisions, pkgRegistry] = useStore('pkg_terms_decisions', 'pkg_registry')
    const { user } = useAuth()
    const decide = useMutation<void, Error, TermsDecisionInput>({
        mutationFn: mutation(function* (input: TermsDecisionInput) {
            yield decisions.insert({
                id: newRecordId(),
                pkg: input.entry.slug,
                terms_version: input.entry.version,
                decision: input.decision,
                user: user.id,
            })
            if (input.decision === 'accepted' && input.shouldEnable && !input.entry.isEnabled) {
                yield pkgRegistry.update(input.entry.id, draft => {
                    draft.status = enabledStatusFor(true)
                })
            }
        }),
        onSuccess: (_data, input) => options.onDecided?.(input),
        onError: err => {
            captureException('core.pkg-terms', err)
            notify.emit({
                event: 'mutation.error',
                title: 'Could not save your decision',
                body: errorToString(err),
                data: { operation: 'core.pkg-terms', error: errorToString(err) },
            })
        },
    })
    return { decide: decide.mutate, isPending: decide.isPending }
}
```

- [ ] **Step 6: Verify**

Run: `pnpm exec vitest run core/lib/packages/__tests__/terms-state.test.ts core/tests/unit/store-sync-modes.test.ts`
Expected: 2 files passed, no failed test.

Run: `pnpm exec tsc --noEmit -p core/tsconfig.json && pnpm exec tsc --noEmit`
Expected: no output, exit 0.

Run: `pnpm exec biome check core/lib/packages/terms-state.ts core/lib/packages/__tests__/terms-state.test.ts core/lib/packages/use-package-terms.ts core/lib/pocketbase.ts core/tests/unit/store-sync-modes.test.ts core/components/setup/PackageManager.tsx`
Expected: `Checked 6 files`, no error.

- [ ] **Step 7: Commit**

```bash
git add core/lib/packages/terms-state.ts core/lib/packages/__tests__/terms-state.test.ts core/lib/packages/use-package-terms.ts core/lib/pocketbase.ts core/tests/unit/store-sync-modes.test.ts core/components/setup/PackageManager.tsx
git commit -m "feat(terms): client state of package terms and the owner's decision"
```

---

### Task 8: E2E stub package and helpers

**Files:**
- Create: `tests/scripts/scaffold-terms-stub.ts`
- Create: `tests/e2e/package-terms-helpers.ts`
- Create: `tests/e2e/package-terms-blocked.spec.ts` (first test only; Tasks 9 and 10 add the others)
- Modify: `tests/e2e/helpers.ts` (`login` takes `landed`)
- Modify: `.github/workflows/ci.yml` (one step after "Scaffold reports stub package")

**Interfaces:**
- Consumes: `login(page, { startAt? })` from `tests/e2e/helpers.ts` (this task adds the option `landed?: Locator`); `appHref(route)` from `@tinycld/core/lib/org-routes`; the generator of part 1 (reads the stub's `terms`).
- Produces: the workspace member `termsstub` (package `@tinycld/termsstub`, name "Terms Stub", nav testID `nav-termsstub`, landing testID `termsstub-landing`, settings link testID `settings-org-termsstub-general`, panel testID `termsstub-settings-panel`, help topic "Terms stub topic", terms version `1`, a seed that records a `declined` decision). In `package-terms-helpers.ts`: `TERMS_STUB` (`slug`, `name`, `version`, `sentence`, `landing`, `settingsLink`, `settingsPanel`, `helpTopic`), `PHONE_VIEWPORT`, `SECOND_ORIGIN: string`, `skipWithoutTermsStub(): boolean`, `termsSuperuser(server: 'primary' | 'second'): Promise<PocketBase>`, `resetStubTerms(pb: PocketBase, state: 'undecided' | 'declined'): Promise<void>`, `loginOnSecond(page: Page, route?: string, landed?: Locator): Promise<void>`, `packageNavEntries(page: Page): Locator`.

Why the seed declines: a package with undecided terms sends the owner to Settings → Package terms on every launch (Task 11). With a declined stub the other specs of the run land where they did before.

- [ ] **Step 1: Write the scaffolder**

Create `tests/scripts/scaffold-terms-stub.ts`:

```ts
#!/usr/bin/env tsx
/**
 * scaffold-terms-stub.ts — provisions the @tinycld/termsstub package into the
 * workspace that contains the app shell.
 *
 * Why a stub: a package can declare terms and conditions (manifest `terms`),
 * and nobody can use it until the owner accepts them. Core must not name a
 * package, and app-shell CI installs no feature package, so without a package
 * that declares terms the package-terms e2e (tests/e2e/package-terms-*.spec.ts)
 * has nothing to gate. The stub declares terms and one of each surface the gate
 * hides: a nav entry with a route, a settings panel, a help topic and a search
 * adapter.
 *
 * Its seed records a `declined` decision. So on a fresh e2e database the stub
 * is closed AND decided: the owner is not sent to Settings → Package terms on
 * every launch of every other spec (only an undecided package does that).
 *
 * Like scaffold-forms-host-stub.ts this script writes every file itself: no
 * bootstrap preset ships terms, and writing the files directly keeps the
 * fixture free of network access and of template drift.
 *
 * Invocation:
 *   - CI: `pnpm exec tsx tinycld/tests/scripts/scaffold-terms-stub.ts` from the
 *     workspace root, after `pnpm install` and before the bundle build, so the
 *     stub's route is in the exported bundle and its terms in the Go server.
 *   - Local: run once before `tinycld-pkg test:e2e`.
 *
 * Idempotent: re-running re-emits the sources, and installs only when the stub
 * is not yet a workspace member.
 *
 * Fixture identifiers the specs assert on (keep in sync with
 * tests/e2e/package-terms-helpers.ts): slug `termsstub`, name "Terms Stub",
 * terms version `1`, the sentence TERMS_SENTENCE in the terms text, landing
 * testID `termsstub-landing`, settings panel `general` (testID
 * `termsstub-settings-panel`), help topic "Terms stub topic".
 */

import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const STUB_DIR = 'termsstub'
export const STUB_SLUG = 'termsstub'
export const STUB_PACKAGE = '@tinycld/termsstub'
const TERMS_VERSION = '1'
const TERMS_SENTENCE = 'You agree to feed the stub once a day.'

// Every subprocess finishes in well under a minute; a bound turns a rare
// wedge into a loud failure instead of a silent runner burn.
const SUBPROCESS_TIMEOUT_MS = 5 * 60_000

function workspaceRoot(): string {
    // tinycld/tests/scripts/<this> → tinycld/ → workspace root
    return resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..')
}

function write(path: string, contents: string): void {
    mkdirSync(resolve(path, '..'), { recursive: true })
    writeFileSync(path, contents)
}

function writePackage(stubDir: string): void {
    const src = `./tinycld/${STUB_SLUG}`
    const pkg = {
        name: STUB_PACKAGE,
        version: '0.1.0',
        private: true,
        type: 'module',
        exports: {
            './package.json': './package.json',
            './manifest': './manifest.ts',
            './seed': `${src}/seed.ts`,
            './search-adapter': `${src}/search-adapter.tsx`,
            './screens/*': `${src}/screens/*.tsx`,
            './settings/*': `${src}/settings/*.tsx`,
        },
        peerDependencies: {},
    }
    write(join(stubDir, 'package.json'), `${JSON.stringify(pkg, null, 4)}\n`)
    // Written as biome formats it: the workspace lint covers every member, the stub included.
    write(
        join(stubDir, 'tsconfig.json'),
        `{
    "extends": "@tinycld/core/tsconfig.package-base.json",
    "compilerOptions": {
        "paths": {
            "~/tinycld/${STUB_SLUG}/*": ["./tinycld/${STUB_SLUG}/*"]
        }
    },
    "include": ["tinycld/**/*.ts", "tinycld/**/*.tsx"],
    "exclude": ["node_modules"]
}
`
    )
}

function writeManifest(stubDir: string): void {
    write(
        join(stubDir, 'manifest.ts'),
        `const manifest = {
    name: 'Terms Stub',
    slug: '${STUB_SLUG}',
    version: '0.1.0',
    description: 'app E2E package-terms stub',
    routes: { directory: 'screens' },
    nav: { label: 'Terms Stub', icon: 'file-check', order: 996 },
    terms: { file: 'TERMS.md', version: '${TERMS_VERSION}' },
    settings: [{ slug: 'general', label: 'Terms Stub Panel', component: 'settings/general' }],
    help: { directory: 'help' },
    search: { adapter: 'search-adapter' },
    seed: { script: 'seed' },
}

export default manifest
`
    )
    write(
        join(stubDir, 'TERMS.md'),
        `# Terms Stub terms and conditions

${TERMS_SENTENCE}

1. The stub is a test fixture.
2. It does nothing else.
`
    )
    write(
        join(stubDir, 'help', 'topic.md'),
        `---
title: Terms stub topic
summary: A help topic that shows only when the terms of the stub are accepted.
order: 1
---

To use the terms stub, the owner accepts its terms first.
`
    )
}

function writeSources(stubDir: string): void {
    const dir = join(stubDir, 'tinycld', STUB_SLUG)
    write(
        join(dir, 'screens', 'index.tsx'),
        `import { Text, View } from 'react-native'

export default function TermsStubIndex() {
    return (
        <View className="flex-1 bg-background p-4">
            <Text className="text-foreground" testID="termsstub-landing">
                Terms stub landing
            </Text>
        </View>
    )
}
`
    )
    write(
        join(dir, 'settings', 'general.tsx'),
        `import { Text, View } from 'react-native'

export default function TermsStubPanel() {
    return (
        <View className="p-4">
            <Text className="text-foreground" testID="termsstub-settings-panel">
                Terms stub panel
            </Text>
        </View>
    )
}
`
    )
    write(
        join(dir, 'search-adapter.tsx'),
        `import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useRouter } from 'expo-router'

// The palette calls this for every in-scope package while it is open, so it
// only takes handles. The stub has no server search source: only its scope
// chip matters to the specs.
export function useSearchActions() {
    const router = useRouter()
    const orgHref = useOrgHref()
    return { onSelect: () => router.push(orgHref('${STUB_SLUG}')) }
}
`
    )
    write(
        join(dir, 'seed.ts'),
        `import type PocketBase from 'pocketbase'

const SLUG = '${STUB_SLUG}'

// Records that the owner DECLINED the stub's terms, when nobody decided yet.
// A package with undecided terms sends the owner to Settings → Package terms
// on every launch, which would move every other spec's landing page. The
// package-terms specs put the state they need in place themselves.
export default async function seed(pb: PocketBase): Promise<void> {
    const row = await pb
        .collection('pkg_registry')
        .getFirstListItem(pb.filter('slug = {:slug}', { slug: SLUG }))
    const version = String(row.terms_version ?? '')
    if (version === '' || row.terms_accepted_version === version) return
    const decided = await pb.collection('pkg_terms_decisions').getList(1, 1, {
        filter: pb.filter('pkg = {:slug} && terms_version = {:version}', { slug: SLUG, version }),
    })
    if (decided.totalItems > 0) return
    await pb
        .collection('pkg_terms_decisions')
        .create({ pkg: SLUG, terms_version: version, decision: 'declined' })
}
`
    )
}

/** Registers the stub in package.json workspaces and pnpm-workspace.yaml; true when either changed. */
function ensureMember(wsRoot: string): boolean {
    const path = join(wsRoot, 'package.json')
    const pkg = JSON.parse(readFileSync(path, 'utf8'))
    const workspaces: string[] = Array.isArray(pkg.workspaces) ? pkg.workspaces : []
    const missing = !workspaces.includes(STUB_DIR)
    if (missing) {
        pkg.workspaces = [...workspaces, STUB_DIR]
        writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
    }
    const added = ensurePnpmMember(join(wsRoot, 'pnpm-workspace.yaml'))
    return missing || added
}

/** Adds `- <STUB_DIR>` to the packages: block. Returns false when already present. */
function ensurePnpmMember(yamlPath: string): boolean {
    const lines = readFileSync(yamlPath, 'utf8').split('\n')
    const pkgIdx = lines.findIndex(l => /^packages:\s*$/.test(l))
    if (pkgIdx === -1) throw new Error(`no packages: block in ${yamlPath}`)
    let lastEntry = pkgIdx
    for (let i = pkgIdx + 1; i < lines.length; i++) {
        const line = lines[i] ?? ''
        if (line.trim() === `- ${STUB_DIR}`) return false
        if (/^\s+-\s+/.test(line)) lastEntry = i
        else if (line.trim() !== '' && !line.startsWith(' ')) break
    }
    const indent = (lines[lastEntry] ?? '').match(/^\s*/)?.[0] ?? '  '
    lines.splice(lastEntry + 1, 0, `${indent}- ${STUB_DIR}`)
    writeFileSync(yamlPath, lines.join('\n'))
    return true
}

function install(wsRoot: string): void {
    // COREPACK_ENABLE_DOWNLOAD_PROMPT=0: corepack's "download pnpm?" prompt is
    // a silent stdin-wait in non-TTY-ish CI.
    const env = { ...process.env, COREPACK_ENABLE_DOWNLOAD_PROMPT: '0' }
    spawnSync('corepack', ['enable'], {
        cwd: wsRoot,
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
        env,
    })
    const r = spawnSync('pnpm', ['install', '--no-frozen-lockfile'], {
        cwd: wsRoot,
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
        env,
    })
    if (r.status !== 0) {
        throw new Error(`pnpm install failed (status ${r.status}); the terms stub will not link`)
    }
}

/**
 * Regenerate so the stub's route, settings panel, help topic, seed and terms
 * land in the generated config and in the server's package list. Without this
 * the package exists on disk and contributes nothing.
 */
function regenerateConfig(wsRoot: string): void {
    const r = spawnSync('pnpm', ['run', 'packages:generate'], {
        cwd: join(wsRoot, 'tinycld'),
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
    })
    if (r.status !== 0) {
        const cause = r.error ? `: ${r.error.message}` : ''
        throw new Error(`packages:generate failed${cause}; the terms stub will not be loaded`)
    }
}

function main(): void {
    const wsRoot = workspaceRoot()
    console.log(`[scaffold-terms-stub] workspace root: ${wsRoot}`)
    const stubDir = join(wsRoot, STUB_DIR)
    writePackage(stubDir)
    writeManifest(stubDir)
    writeSources(stubDir)
    if (ensureMember(wsRoot)) install(wsRoot)
    regenerateConfig(wsRoot)
    console.log(`[scaffold-terms-stub] done — ${STUB_DIR}/ ready at ${stubDir}`)
}

main()
```

Do not run the scaffolder: it runs `pnpm install` at the workspace root and adds a member. The driver runs it before the e2e run.

- [ ] **Step 2: Write the e2e helpers**

`login` waits for the app shell (`nav-home` or `nav-more`). A setup step renders outside the shell, so the wizard test of Task 12 needs another sign that the sign-in is done. In `tests/e2e/helpers.ts`:

```diff
 // every existing caller is unaffected.
-export async function login(page: Page, options: { startAt?: string } = {}) {
+//
+// `landed` replaces the app shell as the sign that the sign-in is done, for a
+// `startAt` route that renders outside the shell (the setup guide's steps).
+export async function login(page: Page, options: { startAt?: string; landed?: Locator } = {}) {
     await startTrackedSession(page)
```

```diff
-    const shellReady = appShell(page)
+    const shellReady = options.landed ?? appShell(page)
     // Match EITHER message the sign-in form can render for a failed attempt.
```

(`Locator` is already imported there.)

Create `tests/e2e/package-terms-helpers.ts`:

```ts
import * as fs from 'node:fs'
import * as path from 'node:path'
import type { Locator, Page } from '@playwright/test'
import { appHref } from '@tinycld/core/lib/org-routes'
import PocketBase from 'pocketbase'
import { login } from './helpers'

// The package-terms specs drive the termsstub package
// (tests/scripts/scaffold-terms-stub.ts): the one package of the e2e build that
// declares terms and conditions. Keep these in step with that script.
export const TERMS_STUB = {
    slug: 'termsstub',
    name: 'Terms Stub',
    version: '1',
    sentence: 'You agree to feed the stub once a day.',
    landing: 'termsstub-landing',
    settingsLink: 'settings-org-termsstub-general',
    settingsPanel: 'termsstub-settings-panel',
    helpTopic: 'Terms stub topic',
} as const

export const PHONE_VIEWPORT = { width: 390, height: 844 }

const PRIMARY_PORT = Number(process.env.E2E_PORT ?? 7200)
const SECOND_PORT = Number(process.env.E2E_PORT_2 ?? 7201)
/** The second e2e deployment: its own database, the same seeded users. */
export const SECOND_ORIGIN = `http://localhost:${SECOND_PORT}`

const ADMIN_LOGIN = process.env.ADMIN_USER_LOGIN || 'admin@tinycld.org'
const ADMIN_PASSWORD = process.env.ADMIN_USER_PW || 'AdminPass1234!'

// Same rule as skipWithoutFormsStub (helpers.ts): skip in a dev workspace
// without the stub, fail loudly on CI, where the scaffold step is mandatory.
// Pass it as a callback.
export function skipWithoutTermsStub(): boolean {
    const stubDir = path.resolve(import.meta.dirname, '..', '..', '..', TERMS_STUB.slug)
    if (fs.existsSync(stubDir)) return false
    if (process.env.CI) {
        throw new Error(
            'termsstub is not scaffolded but CI is set — the terms stub scaffold step must run before e2e.'
        )
    }
    return true
}

/** A PocketBase client signed in as the superuser of one e2e deployment. */
export async function termsSuperuser(server: 'primary' | 'second'): Promise<PocketBase> {
    const port = server === 'primary' ? PRIMARY_PORT : SECOND_PORT
    const pb = new PocketBase(`http://127.0.0.1:${port}`)
    pb.autoCancellation(false)
    await pb.collection('_superusers').authWithPassword(ADMIN_LOGIN, ADMIN_PASSWORD)
    return pb
}

/**
 * Puts the stub's terms in a known state: not accepted, and either undecided
 * or declined. A superuser write (see the header of
 * package-terms-owner.spec.ts for why): the app has no way to take back an
 * acceptance or a decline.
 */
export async function resetStubTerms(pb: PocketBase, state: 'undecided' | 'declined') {
    const filter = pb.filter('pkg = {:slug}', { slug: TERMS_STUB.slug })
    const decisions = await pb.collection('pkg_terms_decisions').getFullList({ filter })
    for (const decision of decisions) {
        await pb.collection('pkg_terms_decisions').delete(decision.id)
    }
    const row = await pb
        .collection('pkg_registry')
        .getFirstListItem(pb.filter('slug = {:slug}', { slug: TERMS_STUB.slug }))
    await pb.collection('pkg_registry').update(row.id, { terms_accepted_version: '' })
    if (state === 'undecided') return
    await pb.collection('pkg_terms_decisions').create({
        pkg: TERMS_STUB.slug,
        terms_version: TERMS_STUB.version,
        decision: 'declined',
    })
}

/**
 * Signs the owner (the shared test user) in on the second deployment. Give
 * `landed` for a route outside the app shell (a setup step): it is what shows
 * that the sign-in is done.
 */
export async function loginOnSecond(page: Page, route = '', landed?: Locator) {
    const startAt = `${SECOND_ORIGIN}${route === '' ? '/' : appHref(route)}`
    await login(page, { startAt, landed })
}

const FIXED_NAV = ['home', 'help', 'settings', 'sidebar-toggle', 'more']

/**
 * The package entries of the rail or the tab bar: every `nav-*` item but the
 * fixed ones. One of them visible shows that the user's package list has
 * loaded, so "no entry for the stub" is not read from an empty list.
 */
export function packageNavEntries(page: Page): Locator {
    const notFixed = FIXED_NAV.map(name => `:not([data-testid="nav-${name}"])`).join('')
    return page.locator(`[data-testid^="nav-"]${notFixed}`)
}
```

- [ ] **Step 3: Write the first e2e test**

It reads only (a read-only assertion through the record API): the server of the e2e run holds the stub's terms.

Create `tests/e2e/package-terms-blocked.spec.ts`:

```ts
import { expect, test } from '@playwright/test'
import { skipWithoutTermsStub, TERMS_STUB, termsSuperuser } from './package-terms-helpers'

// A package whose terms the owner has not accepted: what everybody sees.
//
// This file writes nothing. The termsstub package declares terms, and its seed
// records that the owner declined them, so on the shared e2e database the stub
// is closed for the whole run. The owner's decisions (accept, decline, the
// first launch, the setup wizard) are in package-terms-owner.spec.ts, on the
// second deployment.
//
// Not covered here: the gate is in the client only. The server answers a
// request for the stub's records and routes as for any other package, and no
// test here asserts a refusal.

test.describe('package terms: a package nobody may use yet', () => {
    test.skip(skipWithoutTermsStub)

    test('the server holds the terms version and text of the package', async () => {
        const pb = await termsSuperuser('primary')
        const row = await pb
            .collection('pkg_registry')
            .getFirstListItem(pb.filter('slug = {:slug}', { slug: TERMS_STUB.slug }))
        expect(row.terms_version).toBe(TERMS_STUB.version)
        expect(row.terms_accepted_version).toBe('')
        const terms = await pb
            .collection('pkg_terms')
            .getFirstListItem(pb.filter('pkg = {:slug}', { slug: TERMS_STUB.slug }))
        expect(terms.version).toBe(TERMS_STUB.version)
        expect(terms.body).toContain(TERMS_STUB.sentence)
    })
})
```

- [ ] **Step 4: Add the CI step**

In `.github/workflows/ci.yml`, after the step "Scaffold reports stub package" and before the comment "Shared e2e prep":

```yaml
      # The package-terms specs need a package that declares terms, and
      # app-shell CI installs none. Must precede the bundle build so the
      # stub's route is in the bundle and its terms are in the Go server.
      - name: Scaffold terms stub package
        working-directory: ws
        run: pnpm exec tsx tinycld/tests/scripts/scaffold-terms-stub.ts
```

- [ ] **Step 5: Verify**

Run: `pnpm exec tsc --noEmit`
Expected: no output, exit 0 (the app tsconfig includes `tests/e2e` and `tests/scripts`).

Run: `pnpm exec biome check tests/scripts/scaffold-terms-stub.ts tests/e2e/helpers.ts tests/e2e/package-terms-helpers.ts tests/e2e/package-terms-blocked.spec.ts`
Expected: `Checked 4 files`, no error.

- [ ] **Step 6: Commit**

```bash
git add tests/scripts/scaffold-terms-stub.ts tests/e2e/helpers.ts tests/e2e/package-terms-helpers.ts tests/e2e/package-terms-blocked.spec.ts .github/workflows/ci.yml
git commit -m "test(terms): stub package with terms and the e2e helpers"
```

---

### Task 9: Terms components and the route gate

**Files:**
- Create: `core/components/package-terms/TermsDocument.tsx`
- Create: `core/components/package-terms/PackageTermsReview.tsx`
- Create: `core/components/package-terms/TermsWaitingScreen.tsx`
- Create: `core/components/package-terms/PackageTermsGate.tsx`
- Modify: `core/components/workspace/PackageTabs.tsx`
- Modify: `core/components/workspace/WorkspaceLayout.tsx`
- Modify: `core/components/workspace/MobileDrawer.tsx`
- Modify: `core/lib/use-accessible-packages.ts`
- Modify: `tests/e2e/package-terms-blocked.spec.ts` (three tests)

**Interfaces:**
- Consumes (Task 7): `useTermsBlocked`, `usePackageTermsList`, `useTermsDecision`, `PackageTermsEntry`. Existing: `MarkdownRenderer` (`@tinycld/core/components/help/MarkdownRenderer`; props `body`, `translateModifierKeys`, `shortcutTableHeuristic`), `ServerActionButton` and `ButtonText` (`@tinycld/core/ui/button`), `useWorkspaceStore(s => s.activePkgSlug)`, `PackageAccessDenied` (the cover this one copies).
- Produces: `TermsDocument({ slug: string })` (testIDs `pkg-terms-body`, `pkg-terms-missing`); `PackageTermsReview({ entry: PackageTermsEntry })` (testIDs `pkg-terms-review`, `pkg-terms-agree`, `pkg-terms-decline`, `pkg-terms-declined`); `TermsWaitingScreen({ name: string })` (testID `pkg-terms-waiting`); `PackageTermsGate()`; `useActivePkgTermsBlocked(): boolean`. `useAccessiblePackagesResult()` drops closed packages for every role but the owner.

Why a cover and not a redirect: the Tabs keep the other packages' screens frozen and mounted; a cover over the content area (as `PackageAccessDenied`) leaves that as it is. The package's own screen still mounts under the cover; the cover only hides it. That is the client-only gate the human chose.

- [ ] **Step 1: Write the e2e tests**

In `tests/e2e/package-terms-blocked.spec.ts`, replace the two import lines with:

```ts
import { expect, test } from '@playwright/test'
import { appHref } from '@tinycld/core/lib/org-routes'
import { login, loginAs, TEST_COLLABORATOR_PASSWORD, TEST_COLLABORATOR_USERNAME } from './helpers'
import {
    PHONE_VIEWPORT,
    packageNavEntries,
    skipWithoutTermsStub,
    TERMS_STUB,
    termsSuperuser,
} from './package-terms-helpers'
```

add this line before `test.describe(`:

```ts
const STUB_ROUTE = appHref(TERMS_STUB.slug)

```

and add these tests after the first test, inside the `describe`:

```ts

    test('a member has no nav entry and sees the waiting screen on the route', async ({ page }) => {
        await loginAs(page, TEST_COLLABORATOR_USERNAME, TEST_COLLABORATOR_PASSWORD, {
            startAt: STUB_ROUTE,
        })
        await expect(page.getByTestId('pkg-terms-waiting')).toBeVisible()
        await expect(page.getByTestId('pkg-terms-waiting')).toContainText(
            'Waiting for the owner to accept the terms'
        )
        // The member cannot decide.
        await expect(page.getByTestId('pkg-terms-agree')).toHaveCount(0)
        // The member's rail has loaded (an entry of another package shows),
        // and the stub is not in it.
        await expect(packageNavEntries(page).first()).toBeVisible()
        await expect(page.getByTestId(`nav-${TERMS_STUB.slug}`)).toHaveCount(0)
    })

    test('a member on a phone sees the waiting screen and no tab', async ({ page }) => {
        await page.setViewportSize(PHONE_VIEWPORT)
        await loginAs(page, TEST_COLLABORATOR_USERNAME, TEST_COLLABORATOR_PASSWORD, {
            startAt: STUB_ROUTE,
        })
        await expect(page.getByTestId('pkg-terms-waiting')).toBeVisible()
        // The tab bar has loaded, and neither it nor the More list has the stub.
        await expect(packageNavEntries(page).first()).toBeVisible()
        await page.getByTestId('nav-more').click()
        await expect(page.getByTestId(`nav-${TERMS_STUB.slug}`)).toHaveCount(0)
    })

    test('the owner keeps the nav entry and sees the terms with the declined note', async ({
        page,
    }) => {
        await login(page)
        await page.getByTestId(`nav-${TERMS_STUB.slug}`).click()
        const review = page.getByTestId('pkg-terms-review')
        await expect(review).toBeVisible()
        await expect(review.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await expect(review.getByTestId('pkg-terms-declined')).toBeVisible()
        await expect(review.getByTestId('pkg-terms-agree')).toBeVisible()
    })
```

`packageNavEntries` needs one other package in the member's nav; the e2e workspace always has one (CI scaffolds the forms-host and reports stubs). `startAt` is the initial load of `login` / `loginAs`, not an in-app `goto`. The phone test sets the viewport before the load because a member has no nav entry to reach the route with.

These tests fail until the steps below are done (no `pkg-terms-waiting`, no `pkg-terms-review`). The implementer does not run them.

- [ ] **Step 2: Write the terms text component**

Create `core/components/package-terms/TermsDocument.tsx`:

```tsx
import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { MarkdownRenderer } from '@tinycld/core/components/help/MarkdownRenderer'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { Text, View } from 'react-native'

/**
 * The terms text of one package, as the installed server holds it
 * (pkg_terms, written at boot). Any signed-in user may read it.
 */
export function TermsDocument({ slug }: { slug: string }) {
    const [pkgTerms] = useStore('pkg_terms')
    const { data: rows = [], isReady } = useLiveQuery({
        queryKey: ['core', 'pkg-terms', 'text', slug],
        query: query => query.from({ t: pkgTerms }).where(({ t }) => eq(t.pkg, slug)),
    })
    const text = rows[0]

    if (!isReady) return null
    if (!text) {
        return (
            <Text testID="pkg-terms-missing" className="text-sm text-muted-foreground">
                The text of these terms is not available. Try again later.
            </Text>
        )
    }
    return (
        <View testID="pkg-terms-body" className="gap-2">
            <Text className="text-xs text-muted-foreground">Version {text.version}</Text>
            {/* The author's own text: no shortcut glyph translation, no help table layout. */}
            <MarkdownRenderer
                body={text.body}
                translateModifierKeys={false}
                shortcutTableHeuristic={false}
            />
        </View>
    )
}
```

- [ ] **Step 3: Write the owner's review and the waiting screen**

Create `core/components/package-terms/PackageTermsReview.tsx`:

```tsx
import type { PackageTermsEntry } from '@tinycld/core/lib/packages/use-package-terms'
import { useTermsDecision } from '@tinycld/core/lib/packages/use-package-terms'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { ScrollView, Text, View } from 'react-native'
import { TermsDocument } from './TermsDocument'

/**
 * What the owner sees in place of a package whose terms are not accepted:
 * the terms, Agree and Decline. Agree has no keyboard shortcut on purpose:
 * an acceptance of terms is a deliberate press.
 */
export function PackageTermsReview({ entry }: { entry: PackageTermsEntry }) {
    const { decide, isPending } = useTermsDecision()
    return (
        <View testID="pkg-terms-review" className="flex-1 bg-background">
            <ScrollView
                className="flex-1"
                contentContainerClassName="w-full max-w-[720px] self-center gap-3 p-5"
            >
                <Text className="text-xl font-bold text-foreground">
                    {entry.name}: terms and conditions
                </Text>
                <Text className="text-sm text-muted-foreground">
                    Nobody in this workspace can use {entry.name} until you agree to these terms.
                </Text>
                <DeclinedNote isVisible={entry.isDeclined} />
                <TermsDocument slug={entry.slug} />
            </ScrollView>
            <View className="flex-row justify-end gap-3 border-t border-border p-4">
                <DeclineButton
                    isVisible={!entry.isDeclined}
                    isDisabled={isPending}
                    onPress={() => decide({ entry, decision: 'declined' })}
                />
                <ServerActionButton
                    testID="pkg-terms-agree"
                    isDisabled={isPending}
                    onPress={() => decide({ entry, decision: 'accepted' })}
                >
                    <ButtonText>Agree</ButtonText>
                </ServerActionButton>
            </View>
        </View>
    )
}

function DeclinedNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text
            testID="pkg-terms-declined"
            className="rounded-md border border-border bg-surface-secondary p-3 text-sm text-foreground"
        >
            You declined these terms. The package stays off until you agree.
        </Text>
    )
}

function DeclineButton({
    isVisible,
    isDisabled,
    onPress,
}: {
    isVisible: boolean
    isDisabled: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <ServerActionButton
            testID="pkg-terms-decline"
            variant="outline"
            isDisabled={isDisabled}
            onPress={onPress}
        >
            <ButtonText>Decline</ButtonText>
        </ServerActionButton>
    )
}
```

Create `core/components/package-terms/TermsWaitingScreen.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Hourglass } from 'lucide-react-native'
import { Text, View } from 'react-native'

/** What everybody but the owner sees in place of a package whose terms are not accepted. */
export function TermsWaitingScreen({ name }: { name: string }) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <View
            testID="pkg-terms-waiting"
            className="flex-1 items-center justify-center gap-3 bg-background p-6"
        >
            <Hourglass size={28} color={mutedColor} />
            <Text className="text-center text-base font-bold text-foreground">
                Waiting for the owner to accept the terms
            </Text>
            <Text className="max-w-[420px] text-center text-sm leading-5 text-muted-foreground">
                {name} cannot be used until the owner of this workspace accepts its terms and
                conditions. Ask the owner to open {name}.
            </Text>
        </View>
    )
}
```

- [ ] **Step 4: Write the cover**

Create `core/components/package-terms/PackageTermsGate.tsx`:

```tsx
import {
    type PackageTermsEntry,
    usePackageTermsList,
    useTermsBlocked,
} from '@tinycld/core/lib/packages/use-package-terms'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { StyleSheet, View } from 'react-native'
import { PackageTermsReview } from './PackageTermsReview'
import { TermsWaitingScreen } from './TermsWaitingScreen'

/**
 * True when the ACTIVE package is closed for its terms. Also true while the
 * state of a package that declares terms is not known yet, so its screen and
 * sidebar never show first. Shared by the cover below, WorkspaceLayout and
 * MobileDrawer, which hide the package sidebar for the same state.
 */
export function useActivePkgTermsBlocked(): boolean {
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const { blocked } = useTermsBlocked()
    return activePkgSlug != null && blocked.has(activePkgSlug)
}

type Cover =
    | { kind: 'none' }
    | { kind: 'loading' }
    | { kind: 'review'; entry: PackageTermsEntry }
    | { kind: 'waiting'; name: string }

function useCover(): Cover {
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const isBlocked = useActivePkgTermsBlocked()
    const { entries, isReady } = usePackageTermsList()
    const { isOwner } = useCurrentRole()
    if (!isBlocked) return { kind: 'none' }
    const entry = entries.find(e => e.slug === activePkgSlug)
    if (!isReady || !entry) return { kind: 'loading' }
    return isOwner ? { kind: 'review', entry } : { kind: 'waiting', name: entry.name }
}

/**
 * Covers the package content area while the owner has not accepted the
 * package's terms. On top of the Tabs navigator, as PackageAccessDenied is, so
 * the other packages keep their frozen state.
 */
export function PackageTermsGate() {
    const cover = useCover()
    if (cover.kind === 'none') return null
    return (
        <View className="bg-background" style={StyleSheet.absoluteFill}>
            <CoverBody cover={cover} />
        </View>
    )
}

function CoverBody({ cover }: { cover: Cover }) {
    if (cover.kind === 'review') return <PackageTermsReview entry={cover.entry} />
    if (cover.kind === 'waiting') return <TermsWaitingScreen name={cover.name} />
    return null
}
```

- [ ] **Step 5: Mount the cover and hide the sidebar**

In `core/components/workspace/PackageTabs.tsx`:

```diff
 import { packageRegistry } from '../../lib/packages/static-registry'
+import { PackageTermsGate } from '../package-terms/PackageTermsGate'
 import { PackageAccessDenied } from './PackageAccessDenied'
```

```diff
                 {SCREENS}
             </Tabs>
+            {/* Under the access cover: a member with no access reads that first. */}
+            <PackageTermsGate />
             <PackageAccessDenied />
```

In `core/components/workspace/WorkspaceLayout.tsx`:

```diff
 import { NotificationDrawer } from '@tinycld/core/components/NotificationDrawer'
+import { useActivePkgTermsBlocked } from '@tinycld/core/components/package-terms/PackageTermsGate'
 import { FilePickerSheetHost } from '@tinycld/core/file-viewer/FilePickerSheetHost'
```

```diff
     const activePkgDenied = useActivePkgDenied()
+    const activePkgTermsBlocked = useActivePkgTermsBlocked()
     const insets = useDeviceInsets()
```

```diff
     // A denied package's sidebar sits OUTSIDE the PackageAccessDenied overlay,
-    // so hide it here for the same state.
-    const hasSidebar = !isMobile && activePkg?.sidebar != null && !activePkgDenied
+    // so hide it here for the same state. The same holds for a package whose
+    // terms the owner has not accepted (PackageTermsGate).
+    const hasSidebar =
+        !isMobile && activePkg?.sidebar != null && !activePkgDenied && !activePkgTermsBlocked
```

In `core/components/workspace/MobileDrawer.tsx`:

```diff
+import { useActivePkgTermsBlocked } from '@tinycld/core/components/package-terms/PackageTermsGate'
 import { shouldSettleDrawerOpen } from '@tinycld/core/components/workspace/drawer-release'
```

```diff
     const pkg = usePackage(activePkgSlug ?? '')
+    const isTermsBlocked = useActivePkgTermsBlocked()
     const translateX = useSharedValue(-PANEL_WIDTH)
```

```diff
-    if (!pkg?.sidebar) return null
+    // A package whose terms the owner has not accepted shows no sidebar here
+    // either (PackageTermsGate covers its screen).
+    if (!pkg?.sidebar || isTermsBlocked) return null
```

- [ ] **Step 6: Drop closed packages from the nav of everybody but the owner**

In `core/lib/use-accessible-packages.ts`:

```diff
 import { ACTIVE_PKG_STATUSES } from '@tinycld/core/lib/packages/registry-predicates'
+import { useTermsBlocked } from '@tinycld/core/lib/packages/use-package-terms'
 import { usePackages } from '@tinycld/core/lib/packages/use-packages'
```

```diff
     const [pkgRegistryCollection] = useStore('pkg_registry')
+    const { blocked: termsBlocked, isReady: termsReady } = useTermsBlocked()
```

```diff
     // isReady rather than the record count.
-    const registryLoaded = registryReady && installedReady
+    // The terms state is part of "loaded": until it is known, a package that
+    // declares terms is closed (useTermsBlocked), and a list built before that
+    // would drop it and then show it.
+    const registryLoaded = registryReady && installedReady && termsReady
```

```diff
-    // Admins/owners see everything active; they have no per-user overrides to
-    // wait on, so they're ready as soon as the registry has loaded.
-    if (role === 'owner' || role === 'admin') {
-        return { packages: filtered, isReady: registryLoaded }
-    }
+    // The owner keeps the entry of a package whose terms are not accepted: its
+    // route shows the terms. Nobody else gets an entry for it.
+    if (role === 'owner') return { packages: filtered, isReady: registryLoaded }
+    const open = filtered.filter(pkg => !termsBlocked.has(pkg.slug))
+
+    // Admins see everything active and open; they have no per-user overrides
+    // to wait on, so they're ready as soon as the registry has loaded.
+    if (role === 'admin') return { packages: open, isReady: registryLoaded }
```

```diff
-    const accessible = filtered.filter(pkg => {
+    const accessible = open.filter(pkg => {
         const access = overrideMap.get(pkg.slug)
```

- [ ] **Step 7: Verify**

Run: `pnpm exec tsc --noEmit -p core/tsconfig.json && pnpm exec tsc --noEmit`
Expected: no output, exit 0.

Run: `pnpm exec vitest run core/components/workspace core/lib/__tests__`
Expected: no failed test.

Run: `pnpm exec biome check core/components/package-terms core/components/workspace/PackageTabs.tsx core/components/workspace/WorkspaceLayout.tsx core/components/workspace/MobileDrawer.tsx core/lib/use-accessible-packages.ts tests/e2e/package-terms-blocked.spec.ts`
Expected: no error.

- [ ] **Step 8: Commit**

```bash
git add core/components/package-terms core/components/workspace/PackageTabs.tsx core/components/workspace/WorkspaceLayout.tsx core/components/workspace/MobileDrawer.tsx core/lib/use-accessible-packages.ts tests/e2e/package-terms-blocked.spec.ts
git commit -m "feat(terms): cover a package until the owner accepts its terms"
```

---

### Task 10: Hidden surfaces (settings, help, search, sidebar slots)

**Files:**
- Modify: `app/a/(app)/settings/index.tsx`
- Modify: `app/a/(app)/settings/[...section].tsx`
- Modify: `app/a/(app)/settings/account/[...section].tsx`
- Modify: `app/a/(app)/settings/system/[...section].tsx`
- Modify: `core/lib/help/use-help-topics.ts`
- Modify: `core/components/search-palette/SearchPalette.web.tsx`
- Modify: `core/components/search-palette/useSearchResults.ts`
- Modify: `core/components/sidebar-primitives/SidebarSlot.tsx`
- Modify: `tests/e2e/package-terms-blocked.spec.ts` (two tests)

**Interfaces:**
- Consumes (Task 7): `useTermsOpen`, `useTermsBlocked`, `slugOfGroup`, `slugOfEntry`, `declaresTerms`. Existing: `packageSettings`, `packageAccountSettings`, `packageSystemSettings`, `packageSidebarContributions`, `SidebarContributionEntry` (`@tinycld/core/lib/packages/derive-components`; each group has `pkgSlug`, each contribution `contributorSlug`); `searchPackages` (`@tinycld/core/lib/search/registry`), `SearchPackage` (`@tinycld/core/lib/search/types`).
- Produces: `useSearchResults(parsed: ParsedQuery, packages: SearchPackage[])` (a second parameter); `visibleGroups(groups, managed, blocked)` exported from `use-help-topics.ts`. A closed package has no settings link, no settings panel route ("Settings not found"), no help topic, no search chip, no search rows and no sidebar-slot contribution, for every role.

The search endpoint still answers for a closed package (the server refuses nothing by terms state); the client drops its rows.

- [ ] **Step 1: Write the e2e tests**

In `tests/e2e/package-terms-blocked.spec.ts`, add after the last test, inside the `describe`:

```ts

    test('settings, help and search do not show the package', async ({ page }) => {
        await login(page)

        await page.getByTestId('nav-settings').click()
        await expect(page.getByTestId('settings-index')).toBeVisible()
        await expect(page.getByTestId(TERMS_STUB.settingsLink)).toHaveCount(0)

        await page.getByTestId('nav-help').click()
        const searchBox = page.getByPlaceholder('Search help topics')
        await searchBox.fill('Terms stub')
        await expect(searchBox).toHaveValue('Terms stub')
        await expect(page.getByText(TERMS_STUB.helpTopic)).toHaveCount(0)

        // The palette: a package name and a colon make a scope chip, but not
        // for a closed package. The text stays plain text.
        await searchBox.fill('')
        await page.getByTestId('nav-settings').click()
        await page.keyboard.press('/')
        const palette = page.getByPlaceholder('Search everywhere…')
        await expect(palette).toBeVisible()
        await page.keyboard.type(`${TERMS_STUB.slug}:`)
        await expect(palette).toHaveValue(`${TERMS_STUB.slug}:`)
        await expect(page.getByTestId(`search-chip-${TERMS_STUB.slug}`)).toHaveCount(0)
    })

    test('the settings panel of the package reads not found', async ({ page }) => {
        await login(page, { startAt: appHref(`settings/${TERMS_STUB.slug}/general`) })
        await expect(page.getByText('Settings not found')).toBeVisible()
        await expect(page.getByTestId(TERMS_STUB.settingsPanel)).toHaveCount(0)
    })
```

The first test runs as the owner: the owner is the role that sees the most, so what the owner does not see nobody sees. Both fail until the steps below are done. The implementer does not run them.

- [ ] **Step 2: Hide the settings links**

In `app/a/(app)/settings/index.tsx`:

```diff
 } from '@tinycld/core/lib/packages/derive-components'
+import { slugOfGroup, useTermsOpen } from '@tinycld/core/lib/packages/use-package-terms'
 import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
```

in `PackageAccountLinks`:

```diff
-    const panels = packageAccountSettings.flatMap(group =>
+    // A package whose terms the owner has not accepted lists no panels, here
+    // and in the two package groups below.
+    const groups = useTermsOpen(packageAccountSettings, slugOfGroup)
+    const panels = groups.flatMap(group =>
         group.panels.map(panel => ({ group, panel, Icon: getIcon(group.icon ?? '') }))
```

in `AdminSettings` (the hook goes before the `if (!isVisible) return null`):

```diff
 function AdminSettings({ isVisible, isOwner }: { isVisible: boolean; isOwner: boolean }) {
     const foregroundColor = useThemeColor('foreground')
     const orgHref = useOrgHref()
     const router = useRouter()
+    const packageGroups = useTermsOpen(packageSettings, slugOfGroup)
 
     if (!isVisible) return null
```

```diff
-            {packageSettings.map(group => {
+            {packageGroups.map(group => {
                 const Icon = getIcon(group.icon ?? '')
```

in `SystemSettings`:

```diff
     const managed = useManagedSettingPrefixes()
+    const systemGroups = useTermsOpen(packageSystemSettings, slugOfGroup)
```

```diff
-    const packagePanels = packageSystemSettings.flatMap(group =>
+    const packagePanels = systemGroups.flatMap(group =>
         group.panels
```

- [ ] **Step 3: Hide the settings panel routes**

Each of the three routes waits for the terms state. Until it is known every package with terms is closed, and the panel of an accepted package would read "Settings not found" first.

In `app/a/(app)/settings/[...section].tsx`:

```diff
 import { packageSettings } from '@tinycld/core/lib/packages/derive-components'
+import {
+    slugOfGroup,
+    useTermsBlocked,
+    useTermsOpen,
+} from '@tinycld/core/lib/packages/use-package-terms'
 import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
```

```diff
     const bgColor = useThemeColor('background')
+    // A package whose terms the owner has not accepted has no panels: its URL
+    // reads "Settings not found".
+    const groups = useTermsOpen(packageSettings, slugOfGroup)
+    const { isReady: termsReady } = useTermsBlocked()
+
+    // Until the terms state is known every package with terms is closed: wait,
+    // or an open package's panel would read "Settings not found" first.
+    if (!termsReady) return null
 
     if (!isAdmin) {
```

```diff
-    return <PackagePanelScreen groups={packageSettings} />
+    return <PackagePanelScreen groups={groups} />
```

In `app/a/(app)/settings/account/[...section].tsx`:

```diff
 import { packageAccountSettings } from '@tinycld/core/lib/packages/derive-components'
+import {
+    slugOfGroup,
+    useTermsBlocked,
+    useTermsOpen,
+} from '@tinycld/core/lib/packages/use-package-terms'
 
 export default function PackageAccountSettingsSection() {
-    return <PackagePanelScreen groups={packageAccountSettings} />
+    // A package whose terms the owner has not accepted has no panels.
+    const groups = useTermsOpen(packageAccountSettings, slugOfGroup)
+    const { isReady: termsReady } = useTermsBlocked()
+    // Wait for the terms state: until then every package with terms is closed.
+    if (!termsReady) return null
+    return <PackagePanelScreen groups={groups} />
 }
```

In `app/a/(app)/settings/system/[...section].tsx`:

```diff
 import { resolvePanel } from '@tinycld/core/lib/packages/resolve-panel'
+import {
+    slugOfGroup,
+    useTermsBlocked,
+    useTermsOpen,
+} from '@tinycld/core/lib/packages/use-package-terms'
 import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
```

```diff
     const [pkgSlug, panelSlug] = segments
 
+    // A package whose terms the owner has not accepted has no panels.
+    const groups = useTermsOpen(packageSystemSettings, slugOfGroup)
     const match = useMemo(
-        () => resolvePanel(packageSystemSettings, pkgSlug, panelSlug),
-        [pkgSlug, panelSlug]
+        () => resolvePanel(groups, pkgSlug, panelSlug),
+        [groups, pkgSlug, panelSlug]
     )
+    const { isReady: termsReady } = useTermsBlocked()
+
+    // Wait for the terms state: until then every package with terms is closed.
+    if (!termsReady) return null
 
     if (!match) {
```

- [ ] **Step 4: Hide the help topics**

In `core/lib/help/use-help-topics.ts`:

```diff
 import { useMemo } from 'react'
+import { useTermsBlocked } from '../packages/use-package-terms'
 import { isManagedPrefix, useManagedSettingPrefixes } from '../use-managed-settings'
```

```diff
 // set of instructions for a screen that is not here.
-function visibleGroups(managed: readonly string[]): HelpGroup[] {
-    if (managed.length === 0) return allGroups
+//
+// A package whose terms the owner has not accepted contributes no topics: its
+// help describes screens nobody can open yet.
+export function visibleGroups(
+    groups: readonly HelpGroup[],
+    managed: readonly string[],
+    blocked: ReadonlySet<string>
+): HelpGroup[] {
+    if (managed.length === 0 && blocked.size === 0) return groups as HelpGroup[]
     const out: HelpGroup[] = []
-    for (const group of allGroups) {
+    for (const group of groups) {
+        if (blocked.has(group.pkgSlug)) continue
         const topics = group.topics.filter(t => !isManagedPrefix(managed, t.keyPrefix))
```

```diff
     const managed = useManagedSettingPrefixes()
-    return useMemo(() => visibleGroups(managed), [managed])
+    const { blocked } = useTermsBlocked()
+    return useMemo(() => visibleGroups(allGroups, managed, blocked), [managed, blocked])
```

- [ ] **Step 5: Hide the package in search**

In `core/components/search-palette/useSearchResults.ts`:

```diff
 import { buildSections, type SearchSection } from '@tinycld/core/lib/search/build-sections'
-import { searchPackages } from '@tinycld/core/lib/search/registry'
-import type { ParsedQuery, SearchRow } from '@tinycld/core/lib/search/types'
+import type { ParsedQuery, SearchPackage, SearchRow } from '@tinycld/core/lib/search/types'
```

```diff
-export function useSearchResults(parsed: ParsedQuery): {
+export function useSearchResults(
+    parsed: ParsedQuery,
+    /** The packages to show rows of: the searchable ones that are open. */
+    packages: SearchPackage[]
+): {
     sections: SearchSection[]
```

```diff
     // server-side now).
+    //
+    // The server answers for every package it knows. A package that is not in
+    // `packages` (its terms are not accepted) shows no rows.
+    const openSlugs = new Set(packages.map(p => p.slug))
+    const rows = (data?.rows ?? []).filter(row => openSlugs.has(row.slug))
     const rowsBySlug: Record<string, SearchRow[]> = {}
-    for (const row of data?.rows ?? []) {
+    for (const row of rows) {
         rowsBySlug[row.slug] = rowsBySlug[row.slug] ?? []
```

```diff
-        sections: buildSections(rowsBySlug, searchPackages, parsed.chips, data?.rows ?? []),
+        sections: buildSections(rowsBySlug, packages, parsed.chips, rows),
```

In `core/components/search-palette/SearchPalette.web.tsx`:

```diff
 import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
+import { slugOfEntry, useTermsOpen } from '@tinycld/core/lib/packages/use-package-terms'
 import type { SearchSection } from '@tinycld/core/lib/search/build-sections'
```

```diff
-const INSTALLED_SLUGS = searchPackages.map(p => p.slug)
-
 // SearchPalette renders the global cross-package command palette. Listens to
```

in `OpenSearchPalette`:

```diff
-    const parsed = useMemo(() => parseQuery(text, INSTALLED_SLUGS), [text])
-    const { sections, partial } = useSearchResults(parsed)
+    // A package whose terms the owner has not accepted is not searchable: no
+    // scope chip, no rows, no selection handler.
+    const openPackages = useTermsOpen(searchPackages, slugOfEntry)
+    const openSlugs = useMemo(() => openPackages.map(p => p.slug), [openPackages])
+    const parsed = useMemo(() => parseQuery(text, openSlugs), [text, openSlugs])
+    const { sections, partial } = useSearchResults(parsed, openPackages)
```

```diff
-            {searchPackages.map(pkg => (
+            {openPackages.map(pkg => (
                 <PackageActions key={pkg.slug} slug={pkg.slug} onReady={registerHandler} />
```

```diff
-                        <FooterHints chips={parsed.chips} remainder={remainder} partial={partial} />
+                        <FooterHints
+                            chips={parsed.chips}
+                            remainder={remainder}
+                            partial={partial}
+                            packages={openPackages}
+                        />
```

and at `FooterHints`:

```diff
     partial: string[]
+    /** The searchable packages that are open; only these are offered as a scope. */
+    packages: readonly { slug: string }[]
 }
```

```diff
-function FooterHints({ chips, remainder, partial }: FooterHintsProps) {
+function FooterHints({ chips, remainder, partial, packages }: FooterHintsProps) {
```

```diff
-    const pending = searchPackages.find(
-        p => p.slug === lastWord.toLowerCase() && !chips.includes(p.slug)
-    )
+    const pending = packages.find(p => p.slug === lastWord.toLowerCase() && !chips.includes(p.slug))
```

- [ ] **Step 6: Hide the sidebar-slot contributions**

A slot asks for the terms state only when it holds a contribution of a package that declares terms. `sidebar-slot-props.test.tsx` renders `SidebarSlot` with no store provider and must keep passing.

In `core/components/sidebar-primitives/SidebarSlot.tsx`:

```diff
 } from '@tinycld/core/lib/packages/derive-components'
+import { declaresTerms, useTermsOpen } from '@tinycld/core/lib/packages/use-package-terms'
 import { type ComponentType, Suspense } from 'react'
```

```diff
     if (!entries || entries.length === 0) return null
-    return (
-        <>
-            {entries.map(entry => (
-                <SlotContribution key={entry.contributorSlug} entry={entry} props={props} />
-            ))}
-        </>
-    )
+    // Only a slot that holds a contribution of a package with terms asks for
+    // the terms state; every other slot reads no store.
+    if (entries.some(entry => declaresTerms(entry.contributorSlug))) {
+        return <GatedContributions entries={entries} props={props} />
+    }
+    return <Contributions entries={entries} props={props} />
+}
+
+interface ContributionsProps {
+    entries: readonly SidebarContributionEntry[]
+    props?: SlotProps
 }
+
+function Contributions({ entries, props }: ContributionsProps) {
+    return entries.map(entry => (
+        <SlotContribution key={entry.contributorSlug} entry={entry} props={props} />
+    ))
+}
+
+/** Leaves out the contributions of a package whose terms the owner has not accepted. */
+function GatedContributions({ entries, props }: ContributionsProps) {
+    const open = useTermsOpen(entries, contributorOf)
+    return <Contributions entries={open} props={props} />
+}
+
+const contributorOf = (entry: SidebarContributionEntry) => entry.contributorSlug
```

- [ ] **Step 7: Verify**

Run: `pnpm exec tsc --noEmit -p core/tsconfig.json && pnpm exec tsc --noEmit`
Expected: no output, exit 0.

Run: `pnpm exec vitest run core/components/sidebar-primitives core/components/search-palette core/lib/help core/lib/search`
Expected: no failed test.

Run: `pnpm exec biome check "app/a/(app)/settings" core/lib/help/use-help-topics.ts core/components/search-palette core/components/sidebar-primitives/SidebarSlot.tsx tests/e2e/package-terms-blocked.spec.ts`
Expected: no error.

- [ ] **Step 8: Commit**

```bash
git add "app/a/(app)/settings" core/lib/help/use-help-topics.ts core/components/search-palette core/components/sidebar-primitives/SidebarSlot.tsx tests/e2e/package-terms-blocked.spec.ts
git commit -m "feat(terms): hide the settings, help and search of a package with unaccepted terms"
```

---

### Task 11: Settings → Package terms and the first launch

**Files:**
- Create: `core/components/package-terms/TermsDialog.tsx`
- Create: `app/a/(app)/settings/package-terms.tsx`
- Modify: `app/a/(app)/index.tsx`
- Modify: `app/a/(app)/settings/index.tsx` (the link in `OwnerLinks`)
- Modify: `app/a/(app)/settings/packages.tsx` (a link)
- Create: `tests/e2e/package-terms-owner.spec.ts` (four tests; Task 12 adds the wizard test)

**Interfaces:**
- Consumes (Task 7): `usePackageTermsList`, `useTermsDecision`, `PackageTermsEntry`, `TERMS_PACKAGES`, `hasUndecidedTerms`. Task 9: `TermsDocument`. Task 8: `loginOnSecond`, `resetStubTerms`, `termsSuperuser`, `TERMS_STUB`, `PHONE_VIEWPORT`, `skipWithoutTermsStub`. Existing: `Dialog` (`@tinycld/core/ui/dialog`: `isOpen`, `onClose`, `title`, `size`, `testID`; `Dialog.Body`, `Dialog.Footer`, `Dialog.CancelButton`, `Dialog.ActionButton`), `SettingsLink` (local to `settings/index.tsx`; takes `label`, `testID`, `onPress`, `icon`), `useNavigateBack`, `APP_PREFIX` (`@tinycld/core/lib/org-routes`).
- Produces: `TermsDialog({ entry: PackageTermsEntry | null; onClose(): void; onDecide(entry, decision: 'accepted' | 'declined'): void; isSubmitting: boolean })` (testIDs `pkg-terms-dialog`, `pkg-terms-dialog-close`, `pkg-terms-dialog-agree`, `pkg-terms-dialog-decline`); the route `settings/package-terms` (testIDs `settings-package-terms`, `pkg-terms-row-<slug>`, `pkg-terms-state-<slug>` with the text `Accepted`, `Declined` or `Not accepted yet`, `pkg-terms-open-<slug>`, `pkg-terms-empty`); the Settings link testID `settings-link-package-terms`; the Packages screen link testID `packages-terms-link`.

The dialog's open state is a `useState` slug in the screen: local UI state no other component reads. There is no form: the two actions are buttons, so no zod schema applies. Agree has no keyboard shortcut on purpose (ruling 28).

- [ ] **Step 1: Write the e2e spec**

Create `tests/e2e/package-terms-owner.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { APP_PREFIX } from '@tinycld/core/lib/org-routes'
import type PocketBase from 'pocketbase'
import {
    loginOnSecond,
    PHONE_VIEWPORT,
    resetStubTerms,
    skipWithoutTermsStub,
    TERMS_STUB,
    termsSuperuser,
} from './package-terms-helpers'

// The owner's decisions on the terms of a package: the first launch, decline,
// accept later from Settings, and the Apps step of the setup wizard.
//
// FIXTURES BY SUPERUSER WRITE (approved by the human 2026-11-23).
//
// 1. The terms state of the termsstub package is reset before every test and
//    put back to "declined" after it. Why the UI cannot do it: an acceptance
//    is final in the app, and a decline can only become an acceptance. No
//    screen takes either back, so without the reset only the first test of a
//    database could ever see an undecided package.
// 2. The wizard test creates the `setup.wizard` settings row. Why the UI cannot
//    do it: only the two server paths that create the first owner write that
//    row, and the e2e database is seeded with its users.
// 3. The wizard test turns the stub off first (registry status `disabled`) and
//    puts the status back after. Why the UI cannot do it: the Apps step opens
//    the terms of an app with unaccepted terms and does not toggle it. With
//    the app off, "Agree turns the app on" is shown by the test.
//
// What this therefore does NOT cover: the state a real deployment is in right
// after the server first registers a package with terms (the boot-time sync,
// covered by the Go tests of core/server/pkgterms), and the wizard as a new
// deployment opens it (tests/standalone/first-run-wizard.spec.ts). Every
// decision under test is made through the UI.
//
// This file runs on the SECOND deployment. The terms state of a package is
// one value for the whole deployment, so on the primary one these tests would
// open and close the stub under every other spec of the run. The tests of
// this file run in order in one worker.
//
// Not covered: the gate is in the client only; no test asserts that the
// server refuses a request.

async function openStub(page: Page) {
    await page.getByTestId(`nav-${TERMS_STUB.slug}`).click()
}

test.describe('package terms: the owner decides', () => {
    test.skip(skipWithoutTermsStub)

    let pb: PocketBase

    test.beforeAll(async () => {
        pb = await termsSuperuser('second')
    })

    test.beforeEach(async () => {
        await resetStubTerms(pb, 'undecided')
    })

    test.afterEach(async () => {
        await resetStubTerms(pb, 'declined')
    })

    test('the first launch sends the owner to the package terms', async ({ page }) => {
        await loginOnSecond(page)
        await expect(page.getByTestId('settings-package-terms')).toBeVisible()
        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText(
            'Not accepted yet'
        )

        await page.getByTestId(`pkg-terms-open-${TERMS_STUB.slug}`).click()
        const dialog = page.getByTestId('pkg-terms-dialog')
        await expect(dialog.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await dialog.getByTestId('pkg-terms-dialog-agree').click()

        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText('Accepted')
        await openStub(page)
        await expect(page.getByTestId(TERMS_STUB.landing)).toBeVisible()
        await expect(page.getByTestId('pkg-terms-review')).toHaveCount(0)
    })

    test('a declined package stays closed, and the next launch does not ask again', async ({
        page,
    }) => {
        await loginOnSecond(page, TERMS_STUB.slug)
        const review = page.getByTestId('pkg-terms-review')
        await expect(review.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await review.getByTestId('pkg-terms-decline').click()
        await expect(review.getByTestId('pkg-terms-declined')).toBeVisible()
        await expect(review.getByTestId('pkg-terms-decline')).toHaveCount(0)

        // A new launch: the owner lands on a package, not on the terms. Wait
        // for the launch redirect to settle before reading where it went.
        await loginOnSecond(page)
        await expect
            .poll(() => new URL(page.url()).pathname)
            .toMatch(new RegExp(`^${APP_PREFIX}/[^/]+`))
        expect(page.url()).not.toContain('package-terms')
    })

    test('the owner accepts later from Settings, and the package opens with its surfaces', async ({
        page,
    }) => {
        await resetStubTerms(pb, 'declined')
        await loginOnSecond(page)
        await page.getByTestId('nav-settings').click()
        await expect(page.getByTestId(TERMS_STUB.settingsLink)).toHaveCount(0)
        await page.getByTestId('settings-link-package-terms').click()
        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText('Declined')

        await page.getByTestId(`pkg-terms-open-${TERMS_STUB.slug}`).click()
        await page.getByTestId('pkg-terms-dialog-agree').click()
        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText('Accepted')

        // Accepted terms can still be read; there is nothing left to decide.
        await page.getByTestId(`pkg-terms-open-${TERMS_STUB.slug}`).click()
        await expect(page.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await expect(page.getByTestId('pkg-terms-dialog-agree')).toHaveCount(0)
        await page.getByTestId('pkg-terms-dialog-close').click()

        await page.getByTestId('nav-settings').click()
        await page.getByTestId(TERMS_STUB.settingsLink).click()
        await expect(page.getByTestId(TERMS_STUB.settingsPanel)).toBeVisible()

        await page.getByTestId('nav-help').click()
        await page.getByPlaceholder('Search help topics').fill('Terms stub')
        await expect(page.getByText(TERMS_STUB.helpTopic).first()).toBeVisible()

        await openStub(page)
        await expect(page.getByTestId(TERMS_STUB.landing)).toBeVisible()
    })

    test('on a phone the owner reads and accepts the terms on the package route', async ({
        page,
    }) => {
        await page.setViewportSize(PHONE_VIEWPORT)
        await loginOnSecond(page, TERMS_STUB.slug)
        const review = page.getByTestId('pkg-terms-review')
        await expect(review.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await review.getByTestId('pkg-terms-agree').click()
        await expect(page.getByTestId('pkg-terms-review')).toHaveCount(0)
        await expect(page.getByTestId(TERMS_STUB.landing)).toBeVisible()
    })
})
```

The tests fail until the steps below are done (no `settings-package-terms`, no launch redirect). The implementer does not run them.

- [ ] **Step 2: Write the dialog**

Create `core/components/package-terms/TermsDialog.tsx`:

```tsx
import type { PackageTermsEntry } from '@tinycld/core/lib/packages/use-package-terms'
import { Dialog } from '@tinycld/core/ui/dialog'
import { TermsDocument } from './TermsDocument'

interface TermsDialogProps {
    /** The package whose terms show; null keeps the dialog closed. */
    entry: PackageTermsEntry | null
    onClose: () => void
    onDecide: (entry: PackageTermsEntry, decision: 'accepted' | 'declined') => void
    isSubmitting: boolean
}

/**
 * The terms of one package in a dialog (a sheet on a phone): the setup wizard
 * and Settings → Package terms. Accepted terms show with Close only. Agree has
 * no keyboard shortcut on purpose.
 */
export function TermsDialog({ entry, onClose, onDecide, isSubmitting }: TermsDialogProps) {
    if (!entry) return null
    return (
        <Dialog
            isOpen
            onClose={onClose}
            title={`${entry.name}: terms and conditions`}
            size="xl"
            testID="pkg-terms-dialog"
        >
            <Dialog.Body>
                <TermsDocument slug={entry.slug} />
            </Dialog.Body>
            <Dialog.Footer>
                <Dialog.CancelButton
                    onPress={onClose}
                    label={entry.state === 'accepted' ? 'Close' : 'Not now'}
                    isDisabled={isSubmitting}
                    testID="pkg-terms-dialog-close"
                />
                <DecisionButtons entry={entry} onDecide={onDecide} isSubmitting={isSubmitting} />
            </Dialog.Footer>
        </Dialog>
    )
}

function DecisionButtons({
    entry,
    onDecide,
    isSubmitting,
}: {
    entry: PackageTermsEntry
    onDecide: TermsDialogProps['onDecide']
    isSubmitting: boolean
}) {
    if (entry.state === 'accepted') return null
    return (
        <>
            <DeclineAction
                isVisible={!entry.isDeclined}
                isDisabled={isSubmitting}
                onPress={() => onDecide(entry, 'declined')}
            />
            <Dialog.ActionButton
                label="Agree"
                onPress={() => onDecide(entry, 'accepted')}
                isDisabled={isSubmitting}
                testID="pkg-terms-dialog-agree"
            />
        </>
    )
}

function DeclineAction({
    isVisible,
    isDisabled,
    onPress,
}: {
    isVisible: boolean
    isDisabled: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <Dialog.ActionButton
            label="Decline"
            onPress={onPress}
            isDisabled={isDisabled}
            isDestructive
            testID="pkg-terms-dialog-decline"
        />
    )
}
```

- [ ] **Step 3: Write the Settings screen**

Create `app/a/(app)/settings/package-terms.tsx`:

```tsx
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { TermsDialog } from '@tinycld/core/components/package-terms/TermsDialog'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import {
    type PackageTermsEntry,
    usePackageTermsList,
    useTermsDecision,
} from '@tinycld/core/lib/packages/use-package-terms'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ArrowLeft } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Owner-only: the owner alone accepts terms (the create rule of
// pkg_terms_decisions). The owner lands here on launch while a package waits
// for a decision (app/a/(app)/index.tsx), and reads or accepts terms here later.
export default function PackageTermsSettings() {
    const { isOwner, isReady: roleReady } = useCurrentRole()
    const { entries, isReady } = usePackageTermsList()
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const fgColor = useThemeColor('foreground')
    // Which package's terms are open. Local: no other component needs it.
    const [openSlug, setOpenSlug] = useState<string | null>(null)
    const { decide, isPending } = useTermsDecision({ onDecided: () => setOpenSlug(null) })
    const openEntry = entries.find(e => e.slug === openSlug) ?? null

    // `role` reads null while it loads; see settings/packages.tsx.
    if (!roleReady) return null

    if (!isOwner) {
        return (
            <View className="flex-1 items-center justify-center bg-background p-5">
                <DocumentTitle pkg="Settings" title="Package terms" />
                <Text className="text-base text-muted-foreground">
                    Only the owner can accept the terms of a package.
                </Text>
            </View>
        )
    }

    const rows = entries.map(entry => (
        <TermsRow key={entry.slug} entry={entry} onOpen={() => setOpenSlug(entry.slug)} />
    ))

    return (
        <ScrollView
            className="flex-1 bg-background"
            contentContainerStyle={{ flexGrow: 1 }}
            testID="settings-package-terms"
        >
            <DocumentTitle pkg="Settings" title="Package terms" />
            <View className="w-full max-w-[720px] gap-4 p-5">
                <Pressable onPress={navigateBack} className="self-start">
                    <ArrowLeft size={24} color={fgColor} />
                </Pressable>
                <Text className="text-[22px] font-bold text-foreground">Package terms</Text>
                <Text className="text-sm text-muted-foreground">
                    A package with terms and conditions can be used only after you agree to them.
                    Until then nobody in this workspace can open it.
                </Text>
                <EmptyNote isVisible={isReady && entries.length === 0} />
                <View className="gap-3">{rows}</View>
            </View>
            <TermsDialog
                entry={openEntry}
                onClose={() => setOpenSlug(null)}
                onDecide={(entry, decision) => decide({ entry, decision })}
                isSubmitting={isPending}
            />
        </ScrollView>
    )
}

function EmptyNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="pkg-terms-empty" className="text-sm text-muted-foreground">
            No installed package has terms and conditions.
        </Text>
    )
}

const STATE_LABEL = {
    accepted: 'Accepted',
    declined: 'Declined',
    undecided: 'Not accepted yet',
} as const

function stateKeyOf(entry: PackageTermsEntry): keyof typeof STATE_LABEL {
    if (entry.state === 'accepted') return 'accepted'
    return entry.isDeclined ? 'declined' : 'undecided'
}

function TermsRow({ entry, onOpen }: { entry: PackageTermsEntry; onOpen: () => void }) {
    const stateKey = stateKeyOf(entry)
    return (
        <View
            testID={`pkg-terms-row-${entry.slug}`}
            className="flex-row flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-surface-secondary p-4"
        >
            <View className="min-w-[160px] flex-1 gap-0.5">
                <Text className="text-base font-semibold text-foreground">{entry.name}</Text>
                <Text className="text-xs text-muted-foreground">Version {entry.version}</Text>
                <Text
                    testID={`pkg-terms-state-${entry.slug}`}
                    className="text-sm text-muted-foreground"
                >
                    {STATE_LABEL[stateKey]}
                </Text>
            </View>
            <Button
                testID={`pkg-terms-open-${entry.slug}`}
                variant={entry.state === 'accepted' ? 'outline' : 'default'}
                onPress={onOpen}
            >
                <ButtonText>{entry.state === 'accepted' ? 'Read' : 'Review'}</ButtonText>
            </Button>
        </View>
    )
}
```

The rows are cards that wrap (`flex-wrap`), so the list needs no sideways scroll at any width.

- [ ] **Step 4: Add the launch redirect**

In `app/a/(app)/index.tsx`:

```diff
 import { useOrgHref } from '@tinycld/core/lib/org-routes'
+import { hasUndecidedTerms } from '@tinycld/core/lib/packages/terms-state'
+import { usePackageTermsList } from '@tinycld/core/lib/packages/use-package-terms'
 import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
```

```diff
-    const { isGuest } = useCurrentRole()
+    const { isGuest, isOwner } = useCurrentRole()
+    const terms = usePackageTermsList()
     const orgHref = useOrgHref()
```

```diff
-    if (!isReady || !landingLoaded) return null
+    if (!isReady || !landingLoaded || !terms.isReady) return null
+
+    // An owner who has not decided on a package's terms is asked first: a
+    // package added after setup, terms skipped in the wizard, or a new terms
+    // version. A declined package does not ask again; Settings → Package terms
+    // still lists it. Only this launch route asks: a deep link is not
+    // interrupted, and the package's own route shows its terms.
+    if (isOwner && hasUndecidedTerms(terms.pending)) {
+        const termsHref = orgHref('settings/package-terms')
+        trace('OrgIndex redirect to package terms', { href: JSON.stringify(termsHref) })
+        return <Redirect href={termsHref} />
+    }
 
     if (first) {
```

- [ ] **Step 5: Add the links**

In `app/a/(app)/settings/index.tsx`, change the import Task 10 added and add the icon:

```diff
-import { slugOfGroup, useTermsOpen } from '@tinycld/core/lib/packages/use-package-terms'
+import {
+    slugOfGroup,
+    TERMS_PACKAGES,
+    useTermsOpen,
+} from '@tinycld/core/lib/packages/use-package-terms'
```

```diff
     DatabaseBackup,
+    FileCheck,
     HardDrive,
```

in `OwnerLinks`, after the "Packages" link:

```diff
                 icon={<Package size={20} color={foregroundColor} />}
             />
+            <PackageTermsLink
+                isVisible={TERMS_PACKAGES.length > 0}
+                onPress={() => router.push(orgHref('settings/package-terms'))}
+                color={foregroundColor}
+            />
             <SettingsLink
                 label="Build History"
```

and after the `OwnerLinks` function:

```tsx
// Listed only when a package of this build declares terms.
function PackageTermsLink({
    isVisible,
    onPress,
    color,
}: {
    isVisible: boolean
    onPress: () => void
    color: string
}) {
    if (!isVisible) return null
    return (
        <SettingsLink
            label="Package terms"
            testID="settings-link-package-terms"
            onPress={onPress}
            icon={<FileCheck size={20} color={color} />}
        />
    )
}
```

In `app/a/(app)/settings/packages.tsx`:

```diff
 import { useOrgHref } from '@tinycld/core/lib/org-routes'
+import { TERMS_PACKAGES } from '@tinycld/core/lib/packages/use-package-terms'
 import { pb } from '@tinycld/core/lib/pocketbase'
```

```diff
 import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
+import { useRouter } from 'expo-router'
 import { ArrowLeft } from 'lucide-react-native'
```

```diff
     const fgColor = useThemeColor('foreground')
+    const router = useRouter()
```

```diff
                     <ArrowLeft size={24} color={fgColor} />
                 </Pressable>
+                <PackageTermsLink
+                    isVisible={TERMS_PACKAGES.length > 0}
+                    onPress={() => router.push(orgHref('settings/package-terms'))}
+                />
```

and at the end of the file:

```tsx

// Shown only when a package of this build declares terms.
function PackageTermsLink({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Pressable onPress={onPress} testID="packages-terms-link" className="self-start">
            <Text className="text-sm text-primary underline">
                Read or accept the terms and conditions of packages
            </Text>
        </Pressable>
    )
}
```

- [ ] **Step 6: Verify**

Run: `pnpm exec tsc --noEmit -p core/tsconfig.json && pnpm exec tsc --noEmit`
Expected: no output, exit 0.

Run: `pnpm exec biome check core/components/package-terms/TermsDialog.tsx "app/a/(app)/settings/package-terms.tsx" "app/a/(app)/index.tsx" "app/a/(app)/settings/index.tsx" "app/a/(app)/settings/packages.tsx" tests/e2e/package-terms-owner.spec.ts`
Expected: `Checked 6 files`, no error.

- [ ] **Step 7: Commit**

```bash
git add core/components/package-terms/TermsDialog.tsx "app/a/(app)/settings/package-terms.tsx" "app/a/(app)/index.tsx" "app/a/(app)/settings/index.tsx" "app/a/(app)/settings/packages.tsx" tests/e2e/package-terms-owner.spec.ts
git commit -m "feat(terms): Settings screen for package terms and the first-launch prompt"
```

---

### Task 12: Setup wizard Apps step

**Files:**
- Modify: `core/components/setup/wizard/steps/app-choices.ts`
- Modify: `core/components/setup/wizard/steps/__tests__/apps-step.test.ts`
- Modify: `core/components/setup/wizard/steps/AppsStep.tsx`
- Modify: `tests/e2e/package-terms-owner.spec.ts` (the wizard test)

**Interfaces:**
- Consumes: Task 7 `useTermsBlocked`, `usePackageTermsList`, `useTermsDecision` (`shouldEnable`); Task 11 `TermsDialog`. Existing: `AppCard({ choice, widthClass, onToggle })` in `AppsStep.tsx` (`accessibilityRole="checkbox"`, `accessibilityState={{ checked }}`, testID `setup-app-<slug>`); `CORE_STEP_IDS.apps`, `expectSetupStep(page, stepId)` from `core/e2e-setup-helpers`.
- Produces: `AppChoice.needsTerms: boolean`; `appChoicesOf(rows, bundled, termsPending?: ReadonlySet<string>)`; the hint testID `setup-app-terms-<slug>`. A card of an app with unaccepted terms shows off; a press opens the terms; Agree records the decision and turns the app on; "Not now" leaves it off.

- [ ] **Step 1: Write the failing unit test**

In `core/components/setup/wizard/steps/__tests__/apps-step.test.ts`, add `needsTerms: false,` after `isOn: true,` and after `isOn: false,` in the two expected objects of the first test, and add this test after it:

```ts
    it('shows an app with terms the owner has not accepted as off, whatever its status', () => {
        const rows = [
            { id: 'r1', slug: 'alpha', status: 'bundled' },
            { id: 'r2', slug: 'beta', status: 'bundled' },
        ]
        const choices = appChoicesOf(rows, bundled, new Set(['beta']))
        expect(choices.map(c => [c.slug, c.isOn, c.needsTerms])).toEqual([
            ['alpha', true, false],
            ['beta', false, true],
        ])
    })
```

Run: `pnpm exec vitest run core/components/setup/wizard/steps/__tests__/apps-step.test.ts`
Expected: FAIL, the first test (no `needsTerms` in the result) and the new one.

- [ ] **Step 2: Carry the terms state in the choices**

In `core/components/setup/wizard/steps/app-choices.ts`:

```diff
     icon: string
     isOn: boolean
+    /** The app has terms the owner has not accepted: it is off until the owner agrees. */
+    needsTerms: boolean
 }
```

```diff
         nav?: { icon?: string }
-    }[]
+    }[],
+    /** Slugs of the apps whose terms the owner has not accepted. */
+    termsPending: ReadonlySet<string> = new Set()
 ): AppChoice[] {
```

```diff
         if (!row || !pkg.nav || pkg.slug === CORE_SLUG) return []
+        const needsTerms = termsPending.has(pkg.slug)
         return [
```

```diff
                 icon: pkg.nav.icon ?? '',
-                isOn: row.status !== 'disabled',
+                isOn: row.status !== 'disabled' && !needsTerms,
+                needsTerms,
             },
```

Run: `pnpm exec vitest run core/components/setup/wizard/steps/__tests__/apps-step.test.ts`
Expected: no failed test.

- [ ] **Step 3: Write the e2e test**

In `tests/e2e/package-terms-owner.spec.ts`, add the import after the `import type PocketBase` line:

```ts
import { CORE_STEP_IDS, setupStep } from '../../core/e2e-setup-helpers'
```

add before `async function openStub`:

```ts
const WIZARD_KEY = 'setup.wizard'

```

and add after the last test, inside the outer `describe`:

```ts

    test.describe('the setup wizard', () => {
        // Set only when this run made the row: a row left by a stopped run is
        // used as it is and stays.
        let madeWizardRowId = ''
        let stubRowId = ''
        let stubStatus = ''

        test.beforeAll(async () => {
            // A wizard that is complete: its steps stay open by their URL, and
            // nobody on this deployment is sent into the wizard meanwhile.
            const settings = pb.collection('system_settings')
            const found = await settings.getList(1, 1, {
                filter: pb.filter('key = {:key}', { key: WIZARD_KEY }),
            })
            if (found.totalItems === 0) {
                const now = new Date().toISOString()
                const value = JSON.stringify({
                    startedAt: now,
                    acknowledged: [],
                    skipped: [],
                    completedAt: now,
                })
                madeWizardRowId = (await settings.create({ key: WIZARD_KEY, value })).id
            }

            // The app is off before the owner agrees, so the test shows that
            // the acceptance turns it on.
            const stub = await pb
                .collection('pkg_registry')
                .getFirstListItem(pb.filter('slug = {:slug}', { slug: TERMS_STUB.slug }))
            stubRowId = stub.id
            stubStatus = String(stub.status)
            await pb.collection('pkg_registry').update(stubRowId, { status: 'disabled' })
        })

        test.afterAll(async () => {
            if (stubRowId !== '') {
                await pb.collection('pkg_registry').update(stubRowId, { status: stubStatus })
            }
            if (madeWizardRowId !== '') {
                await pb.collection('system_settings').delete(madeWizardRowId)
            }
        })

        test('the Apps step shows the terms, and agreeing turns the app on', async ({ page }) => {
            const step = setupStep(page, CORE_STEP_IDS.apps)
            // A setup step renders outside the app shell: the step itself
            // shows that the sign-in is done.
            await loginOnSecond(page, 'setup/core.apps', step)
            await expect(step).toBeVisible()
            const card = page.getByTestId(`setup-app-${TERMS_STUB.slug}`)
            await expect(card).toHaveAttribute('aria-checked', 'false')
            await expect(page.getByTestId(`setup-app-terms-${TERMS_STUB.slug}`)).toBeVisible()

            // Closing the terms leaves the app off.
            await card.click()
            const dialog = page.getByTestId('pkg-terms-dialog')
            await expect(dialog.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
            await dialog.getByTestId('pkg-terms-dialog-close').click()
            await expect(card).toHaveAttribute('aria-checked', 'false')

            await card.click()
            await page.getByTestId('pkg-terms-dialog-agree').click()
            await expect(card).toHaveAttribute('aria-checked', 'true')
            await expect(page.getByTestId(`setup-app-terms-${TERMS_STUB.slug}`)).toHaveCount(0)
            // Read-only check: the acceptance also turned the app on.
            const stub = await pb.collection('pkg_registry').getOne(stubRowId)
            expect(stub.status).not.toBe('disabled')
        })
    })
```

The stub is turned off first so that the card's `aria-checked="true"` at the end needs both writes of `useTermsDecision` (the decision and the status); with the app already on, the acceptance alone would pass the test. The wizard row is complete on purpose: an open row sends every owner of the second deployment into the wizard and breaks the other specs that sign in there; a complete row sends nobody, and `SetupStepScreen` still renders a step by its URL when a row exists.

- [ ] **Step 4: Open the terms from the card**

In `core/components/setup/wizard/steps/AppsStep.tsx`:

```diff
 import { useLiveQuery } from '@tanstack/react-db'
+import { TermsDialog } from '@tinycld/core/components/package-terms/TermsDialog'
 import { useAutoUpgrade } from '@tinycld/core/components/setup/use-auto-upgrade'
```

```diff
 import { packageRegistry } from '@tinycld/core/lib/packages/static-registry'
+import {
+    usePackageTermsList,
+    useTermsBlocked,
+    useTermsDecision,
+} from '@tinycld/core/lib/packages/use-package-terms'
 import { pb, useStore } from '@tinycld/core/lib/pocketbase'
```

```diff
 import { Check } from 'lucide-react-native'
+import { useState } from 'react'
 import { Pressable, Text, View } from 'react-native'
```

at the end of `useAppChoices`, and a new hook after it:

```diff
-    return { choices: appChoicesOf(rows, packageRegistry), toggle: toggle.mutate }
+    // An app with terms the owner has not accepted shows as off, whatever its
+    // status: it turns on when the owner agrees in the terms dialog.
+    const { blocked } = useTermsBlocked()
+    return { choices: appChoicesOf(rows, packageRegistry, blocked), toggle: toggle.mutate }
+}
+
+/** The terms dialog of this step: which app's terms are open, and the decision. */
+function useAppTerms() {
+    const { entries } = usePackageTermsList()
+    // Local: only this step opens the dialog.
+    const [openSlug, setOpenSlug] = useState<string | null>(null)
+    const close = () => setOpenSlug(null)
+    const { decide, isPending } = useTermsDecision({ onDecided: close })
+    return {
+        entry: entries.find(e => e.slug === openSlug) ?? null,
+        open: setOpenSlug,
+        close,
+        isPending,
+        // Agreeing here also turns the app on: that is what the owner pressed the card for.
+        decide: (entry: (typeof entries)[number], decision: 'accepted' | 'declined') =>
+            decide({ entry, decision, shouldEnable: true }),
+    }
 }
```

in `AppCard`, and a new component after it:

```diff
                     {choice.description}
                 </Text>
+                <TermsHint isVisible={choice.needsTerms} slug={choice.slug} />
             </View>
```

```tsx
function TermsHint({ isVisible, slug }: { isVisible: boolean; slug: string }) {
    if (!isVisible) return null
    return (
        <Text testID={`setup-app-terms-${slug}`} className="text-xs font-medium text-primary">
            Has terms and conditions. Press to read them.
        </Text>
    )
}
```

and in `AppsStep`:

```diff
     const { choices, toggle } = useAppChoices()
+    const terms = useAppTerms()
+    const press = (c: AppChoice) => (c.needsTerms ? terms.open(c.slug) : toggle(c))
```

```diff
-        <AppCard key={c.slug} choice={c} widthClass={widthClass} onToggle={toggle} />
+        <AppCard key={c.slug} choice={c} widthClass={widthClass} onToggle={press} />
```

```diff
-                lead="These apps come with your server. Clear an app to hide it from everyone. You can show it again, or add more apps, at any time in Settings → Packages."
+                lead="These apps come with your server. Clear an app to hide it from everyone. You can show it again, or add more apps, at any time in Settings → Packages. An app with terms and conditions turns on when you agree to them."
```

```diff
             <SetupContinueButton onPress={next} requiresServer={false} />
+            <TermsDialog
+                entry={terms.entry}
+                onClose={terms.close}
+                onDecide={terms.decide}
+                isSubmitting={terms.isPending}
+            />
         </View>
```

- [ ] **Step 5: Verify**

Run: `pnpm exec vitest run core/components/setup/wizard`
Expected: no failed test.

Run: `pnpm exec tsc --noEmit -p core/tsconfig.json && pnpm exec tsc --noEmit`
Expected: no output, exit 0.

Run: `pnpm exec biome check core/components/setup/wizard/steps tests/e2e/package-terms-owner.spec.ts`
Expected: no error.

- [ ] **Step 6: Commit**

```bash
git add core/components/setup/wizard/steps/app-choices.ts core/components/setup/wizard/steps/__tests__/apps-step.test.ts core/components/setup/wizard/steps/AppsStep.tsx tests/e2e/package-terms-owner.spec.ts
git commit -m "feat(terms): the setup wizard shows and takes the acceptance of package terms"
```

---

### Task 13: Help topic and docs

**Files:**
- Create: `core/help/package-terms.md`
- Modify: `docs/packages.md` (a new section before "### Manifest variation across shipped packages")

**Interfaces:**
- Consumes: the screens and labels of Tasks 9-12.
- Produces: the help topic `core:package-terms`.

- [ ] **Step 1: Write the help topic**

Create `core/help/package-terms.md`:

```md
---
title: Package terms and conditions
summary: Read and accept the terms of a package before your workspace uses it
tags: [packages, terms, conditions, accept, decline, owner, legal]
order: 57
---

## What this does

Some packages come with terms and conditions. Nobody in the workspace can use such a package until the **owner** agrees to its terms. Until then the package has no entry for members and admins, and its settings, help topics and search results do not show.

## To accept the terms of a package

Only the owner can accept terms.

1. Open **Settings**, then **Package terms**.
2. Find the package and press **Review**.
3. Read the terms, then press **Agree**.

The package opens for everybody at once. You can also open the package from the navigation: the owner sees the terms there, with **Agree** and **Decline**.

When you sign in and a package still waits for your decision, the app opens **Package terms** first.

## During setup

In the setup guide, the **Choose your apps** step marks an app that has terms. Press the app to read the terms. **Agree** turns the app on. **Not now** leaves it off; you can accept later in **Settings**.

## If you decline

Press **Decline** to record that you do not agree. The package stays closed and the app does not ask again on sign-in. To accept later, open **Settings**, then **Package terms**, press **Review**, then **Agree**.

## When the terms change

When an update of a package brings new terms, the package closes again until the owner agrees to the new version. The app asks the owner at the next sign-in.

## To read terms you accepted

Open **Settings**, then **Package terms**, and press **Read** on the package.

## If you are not the owner

When you open a link to a package whose terms are not accepted, you see **Waiting for the owner to accept the terms**. Ask the owner of your workspace to open the package.

## What is recorded

Each decision (agree or decline) is stored with the package, the version of the terms, the owner's account and the time. It also shows in the audit log.
```

- [ ] **Step 2: Document the client gate**

In `docs/packages.md`, add this section before the heading "### Manifest variation across shipped packages":

```md
### Package terms (`terms`): how the gate works

A package that declares `terms: { file: 'TERMS.md', version: '2026-11-01' }` is closed in the app until the **owner** accepts that version. Core does all of it; the package ships only the markdown file and the version label.

- **State.** The boot sync writes the declared version to `pkg_registry.terms_version` and the text to the `pkg_terms` collection. `pkg_registry.terms_accepted_version` is the version the owner accepted. The package is **pending** while `terms_version` is not blank and differs from `terms_accepted_version`.
- **Decision.** The owner's client inserts a `pkg_terms_decisions` row (`pkg`, `terms_version`, `decision`: `accepted` or `declined`). The server records the user and the time, writes the audit log, and on `accepted` sets `terms_accepted_version`. A decline changes nothing else. Nobody edits or deletes a decision, and a non-superuser cannot write the two registry fields.
- **A new version.** Change `terms.version` when the text changes. After the upgrade the package is pending again. A change of the text with the same `version` does not ask again.
- **The gate is in the client.** `useTermsBlocked()` (`core/lib/packages/use-package-terms.ts`) gives the pending slugs. While a package is pending:
  - its route shows the terms with Agree / Decline to the owner and a waiting screen to everybody else (`PackageTermsGate`); its sidebar is hidden;
  - admins, members and guests have no nav entry for it; the owner keeps the entry;
  - its settings links and panels, help topics, search scope and rows, and sidebar-slot contributions are hidden for everybody;
  - the setup wizard's Apps step shows it off, and Agree in the terms dialog turns it on;
  - an owner who has not decided lands on Settings → Package terms at launch. A declined package does not ask again.
- **The server does not enforce terms.** No collection rule, route or search source reads the terms state: a request for a pending package's records or routes is answered as for any other package. Do not rely on terms as access control.
- **No terms.** A package without `terms` has a blank `terms_version` and is never pending. A build in which no package declares terms runs no terms query.
```

- [ ] **Step 3: Verify**

Run: `pnpm run packages:generate`
Expected: exit 0.

Run: `command grep -c 'package-terms' lib/generated/package-help.ts`
Expected: a number above 0.

Run: `pnpm exec vitest run core/lib/help scripts/__tests__/e2e-select.test.ts`
Expected: no failed test.

- [ ] **Step 4: Commit**

```bash
git add core/help/package-terms.md docs/packages.md
git commit -m "docs(terms): help topic and package docs for package terms"
```

---

## For the driver

- **NEEDS-RUN (e2e), after Task 13.** Prerequisite, once, from the lane workspace root (`/Users/nas/tmp/tinycld/books-reports`): `pnpm exec tsx tinycld/tests/scripts/scaffold-terms-stub.ts`. It writes the `termsstub` member, runs `pnpm install` at the root and `packages:generate`. Then one command for the two new files and the two other specs that use the second deployment: `E2E_PORT=7220 E2E_PORT_2=7221 E2E_IMAP_PORT=1220 E2E_IMAP_PORT_2=1221 pnpm exec tinycld-pkg test:e2e tinycld/tests/e2e/package-terms-blocked.spec.ts tinycld/tests/e2e/package-terms-owner.spec.ts tinycld/tests/e2e/server-switcher.spec.ts tinycld/tests/e2e/popover-placement.spec.ts`. Expected: no failed test; 11 tests in the two new files (6 + 5), and by a count of `test(` lines 15 + 3 in the two others.
- **Five things only that run shows** (the planner made no e2e run): (d) after the sign-in at `setup/core.apps` the app returns to that step, so `login` with `landed` passes; (e) the Apps step lists the stub while its status is `disabled`; (a) the superuser update of `pkg_registry.terms_accepted_version` to blank in `resetStubTerms` passes the part 1 hooks; (b) after Agree the registry row change reaches the client by realtime, so the cover goes away with no reload; (c) the seeded collaborator has default access to the stub (no `org_pkg_access` row is needed). A failure of one of these is a defect to fix at its source, not a reason to change the assertion.
- **After the stub exists, the full e2e suite must still pass**: the stub adds one nav entry for the owner (order 996) and the seed keeps it declined, so no other spec meets the launch redirect.
- **For the human (CLAUDE.md fixture list).** `tests/e2e/package-terms-owner.spec.ts` uses three superuser fixture writes: the reset of the stub's terms state (ruling 12, approved 2026-11-23), and for the wizard test the creation of a completed `setup.wizard` row and the stub's registry status set to `disabled` (ruling 30, new: not yet approved). The stub's seed also writes one `declined` decision as the superuser. The workspace `CLAUDE.md` lists the specs that use this exception; it needs an entry for this spec.
- Ruling 14 (part 1) stays open for the human: the generator does not refuse a package whose slug is a core route segment.

