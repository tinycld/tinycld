# Package Terms and Conditions, Part 1 (generator and server) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A package declares terms and conditions in its manifest; the server stores them, records the owner's decision, and refuses every request to a package whose current terms version the owner has not accepted.

**Architecture:** The manifest field `terms: { file, version }` is read by the generator, which puts `termsVersion` and `termsBody` on the package's row of `server/bundled-packages.json`. The boot sync copies the version to `pkg_registry.terms_version` and the text to the `pkg_terms` collection. The owner accepts by creating a `pkg_terms_decisions` row; a Go hook checks it and sets `pkg_registry.terms_accepted_version` in the same transaction. A package is **pending** while `terms_version != ""` and `terms_accepted_version != terms_version`. The new Go package `core/server/pkgterms` refuses a pending package on the record API, file downloads, realtime subscriptions, `/api/<slug>/` routes and DAV prefixes; `pkgaccess.WriteError`, `/api/search` and the report engine ask it too.

**Tech Stack:** TypeScript generator scripts (vitest), PocketBase fork (JS migration, Go hooks), Go 1.27 module `tinycld.org/core` at `core/server`.

**Spec:** the `covers` list of `id: p0c-package-terms` in `/Users/nas/code/tinycld/docs/superpowers/plans/2026-10-21-books-run-manifest.yaml`. Outline of both parts: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p0c-package-terms/plan-outline.md`. Rulings: `rulings.md` in the same folder.

**This part covers:** `covers` item 1 (manifest field, generator, docs), item 4 server half (the server refuses collections and routes), item 6 (acceptance records, audit log, new version needs a new acceptance), item 7 server half (a package with no terms is not touched), item 8 (fictional packages in tests). Part 2 (tasks 7-14) covers the client, the wizard, first launch, Settings, e2e and the help topic.

## Global Constraints

- Repo: `/Users/nas/tmp/tinycld/books-reports/tinycld`, branch `books/p0c-package-terms`. All paths below are relative to it.
- Core names no package. Tests and docs use fictional packages only: `acme`, `acme-pro`, `gizmos`, `trinkets`, `cogs`, `gadgets`, `zoo-*`, `plain`.
- Do not write the words "hosting", "saas", "tenant" or "router" (as a product) in code or comments.
- Never build SQL text from a variable. The one raw SQL condition in this plan (`pendingSQL`) is made of constants only.
- Server log calls use `logging.ForPackage`; this plan adds none in `pkgterms` (it returns errors).
- The migration `1720000000_create_pkg_registry.js` is released: do not edit it. The new migration is `2090000017_create_pkg_terms.js`.
- Go commands run from `core/server`. Run only the tests a step names; the full suite is the driver's.
- TS commands run from the repo root. Run `pnpm exec biome check <the files you changed>` before each commit of TS files, and `gofmt -l <the packages you changed>` before each commit of Go files (no output is the pass).
- `tests.ApiScenario` expects an empty body when `ExpectedContent` is empty: every `call(...)` in the tests gives at least one expected part.
- Commit messages: no mention of any AI tool.

## Review Focus

Failure modes the type system and a happy-path test do not catch. Each is pinned by a test in the task that owns it.

1. **The owner writes `terms_accepted_version` straight to `pkg_registry`** (its update rule admits the owner) and no decision is recorded. Pinned by `TestTheOwnerCannotWriteTheTermsFieldsOfTheRegistry` (Task 3).
2. **Turning the package off, or any other registry status, ends the refusal.** `pkgaccess` reads only `installed|bundled` rows; the terms guard must read every status. Pinned by `TestAPackageThatIsTurnedOffStaysRefused` (Task 4).
3. **The wrong owner by prefix:** `acme` pending must not refuse `acme_pro_notes` of the accepted `acme-pro`, and the reverse. Pinned by `TestCollectionErrorFindsTheOwnerByTheLongestSlug` (Task 4).
   The same holds for a way past the request hooks: an `expand` (`TestAnExpandDoesNotReadAPendingPackage`, Task 4) and a registry row with a longer slug (`TestTheOwnerCannotShadowAPendingPackage`, Task 4).
4. **A realtime topic that names the collection another way** (the collection id, `collection/<name>` change signals, `?options=`) passes the guard. Pinned by `TestARealtimeSubscriptionDropsTheTopicsOfAPendingPackage` (Task 4).
5. **A report reads a pending package through a link, or the boot check marks the package's saved reports broken.** Pinned by `TestRunRefusesACollectionOfAPendingPackage` and `TestCheckSavedDoesNotBreakTheReportsOfAPendingPackage` (Task 6).

Also check: a new terms version leaves `terms_accepted_version` alone and makes the package pending (`TestSyncBundledPackagesCarriesTheTerms`, Task 2; `TestANewVersionNeedsANewAcceptance`, Task 3).

## Measurements this plan relies on

- One guard check with nothing pending is one read of `pkg_registry`: `BenchmarkCollectionErrorNothingPending` (Task 4, 12 registry rows) gave 20.8 µs/op on the planner's machine (`go test ./pkgterms/ -run XXX -bench CollectionError -benchtime 2000x`). No cache is planned.
- The realtime subscribe handler of the fork calls `Client.Subscribe(e.Subscriptions...)` after the hooks (`third_party/pocketbase/apis/realtime.go`, the `OnRealtimeSubscribeRequest().Trigger` body), so a hook that changes `e.Subscriptions` before `e.Next()` changes what is subscribed. Checked by the Task 4 test, which reads `e.Subscriptions` in the trigger's final function.
- The record create handler checks the create rule before it triggers `OnRecordCreateRequest`: a member's create of a decision answers 400 from the rule (`TestOnlyTheOwnerDecides`, run by the planner).
- The file download route of the fork answers 404 to a signed-out request before `OnFileDownloadRequest` runs (planner's run of the Task 4 file test with a blank token). The file test therefore uses a member.
- Hook counts after all of part 1, from the planner's run of `TestRegisterBindsTheRecordedHandlerCounts`: `OnRecordCreate` 9, `OnRecordCreateRequest` 21, `OnRecordDeleteRequest` 21, `OnRecordUpdateRequest` 23, `OnRecordEnrich` 1, `OnRecordViewRequest` 1, `OnRecordsListRequest` 1, `OnFileDownloadRequest` 1, `OnRealtimeSubscribeRequest` 1, `OnServe` 42.

## File Structure

| File | Task | Responsibility |
|---|---|---|
| `core/lib/packages/types.ts` | 1 | `PackageTermsDecl`, `PackageManifest.terms` |
| `scripts/load-manifest.ts` | 1 | the generator's copy of the manifest type |
| `scripts/gen-terms.ts` (new) | 1 | `resolveTerms`: validate the declaration, read the file |
| `scripts/gen-server.ts` | 1 | `termsVersion` / `termsBody` on a bundled row |
| `scripts/generate.ts` | 1 | calls `resolveTerms` for each feature |
| `scripts/__tests__/gen-terms.test.ts` (new), `scripts/__tests__/gen-server.test.ts` | 1 | tests |
| `docs/packages.md` | 1, 6 | field reference, validation, the "Package terms" section |
| `core/server/pb_migrations/2090000017_create_pkg_terms.js` (new) | 2 | registry fields, `pkg_terms`, `pkg_terms_decisions` |
| `core/server/pkgterms/pkgterms.go` (new) | 2 | names, `Pending`, `Sync` |
| `core/server/pkgterms/pkgterms_test.go` (new) | 2 | test helpers of the package, `Pending` and `Sync` tests |
| `core/server/coreserver/pkg_seed.go` | 2 | boot sync of the terms |
| `core/server/coreserver/pkg_terms_test.go` (new) | 2 | boot sync tests |
| `core/server/pkgterms/decisions.go` (new), `decisions_test.go` (new) | 3 | `Register`, the decision hooks, the registry guards |
| `core/server/coreserver/server.go`, `audit.go`, `pkg_terms_audit_test.go` (new) | 3 | registration, audit |
| `core/server/coreserver/composition_parity_test.go` | 3, 4, 5 | golden hook counts |
| `core/server/pkgterms/guard.go` (new), `guard_test.go` (new) | 4 | `PendingError`, `Refusal`, `SlugError`, `CollectionError`, request guards |
| `core/server/pkgaccess/pkgaccess.go` | 4 | `WriteError` refuses a pending package |
| `core/server/pkgterms/routes.go` (new), `routes_test.go` (new) | 5 | route middleware |
| `core/server/davprefix/davprefix.go`, `owner_test.go` (new) | 5 | `Registry.Owner` |
| `core/server/search/register.go`, `terms_test.go` (new) | 6 | search skips pending packages |
| `core/server/reports/stmt.go`, `endpoint.go`, `terms_test.go` (new) | 6 | a report refuses a pending package's collection |
| `core/server/automation/endpoints.go`, `terms_test.go` (new) | 6 | an automation dry run refuses a pending package's collection |

---

### Task 1: Manifest `terms` field and the generator carry

**Files:**
- Modify: `core/lib/packages/types.ts` (end of `PackageManifest`, after `reportPanels`)
- Modify: `scripts/load-manifest.ts` (import line 4; end of `PackageManifest`)
- Create: `scripts/gen-terms.ts`
- Modify: `scripts/gen-server.ts` (imports; `BundledPkgInput`; `buildBundledPackages`)
- Modify: `scripts/generate.ts` (imports; after `validatePorts(portsFeatures)`; the `buildBundledPackages` call)
- Create: `scripts/__tests__/gen-terms.test.ts`
- Modify: `scripts/__tests__/gen-server.test.ts` (end of `describe('buildBundledPackages')`)
- Modify: `docs/packages.md` (field reference table; "Validation" list)

**Interfaces:**
- Consumes: `PackageManifest` (`scripts/load-manifest.ts`), `buildBundledPackages(features: BundledPkgInput[]): string` (`scripts/gen-server.ts`).
- Produces:
  - `export interface PackageTermsDecl { file: string; version: string }` in `core/lib/packages/types.ts`; `terms?: PackageTermsDecl` on both `PackageManifest` types.
  - `export function resolveTerms(packageDir: string, slug: string, terms: PackageManifest['terms']): ResolvedTerms | undefined`, `export interface ResolvedTerms { version: string; body: string }`, `export const TERMS_VERSION_PATTERN: RegExp`, `export const TERMS_MAX_BYTES = 200_000` in `scripts/gen-terms.ts`.
  - A row of `server/bundled-packages.json` carries `termsVersion: string` and `termsBody: string` only when the package has terms. Task 2 reads these two keys.

- [ ] **Step 1: Write the failing generator tests**

Create `scripts/__tests__/gen-terms.test.ts`:

```ts
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { resolveTerms, TERMS_MAX_BYTES } from '../gen-terms'

const dirs: string[] = []

// A fictional package directory with the given files.
function packageDir(files: Record<string, string>): string {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'gen-terms-'))
    dirs.push(dir)
    for (const [name, text] of Object.entries(files)) {
        fs.mkdirSync(path.dirname(path.join(dir, name)), { recursive: true })
        fs.writeFileSync(path.join(dir, name), text)
    }
    return dir
}

afterEach(() => {
    for (const dir of dirs.splice(0)) fs.rmSync(dir, { recursive: true, force: true })
})

describe('resolveTerms', () => {
    it('returns undefined for a package without terms', () => {
        expect(resolveTerms(packageDir({}), 'gizmos', undefined)).toBeUndefined()
    })

    it('reads the file and keeps the version', () => {
        const dir = packageDir({ 'TERMS.md': '# Gizmos terms\n\nBe kind.\n' })
        expect(resolveTerms(dir, 'gizmos', { file: 'TERMS.md', version: '2026-11-01' })).toEqual({
            version: '2026-11-01',
            body: '# Gizmos terms\n\nBe kind.',
        })
    })

    it('reads a file in a subdirectory of the package', () => {
        const dir = packageDir({ 'legal/terms.md': 'Text' })
        expect(resolveTerms(dir, 'gizmos', { file: 'legal/terms.md', version: '3' })?.body).toBe(
            'Text'
        )
    })

    it.each([
        ['an empty version', { file: 'TERMS.md', version: '' }, /gizmos: manifest\.terms version/],
        [
            'a version with a space',
            { file: 'TERMS.md', version: 'v 1' },
            /gizmos: manifest\.terms version/,
        ],
        [
            'a version of 51 characters',
            { file: 'TERMS.md', version: 'v'.repeat(51) },
            /gizmos: manifest\.terms version/,
        ],
        ['a file that is not markdown', { file: 'TERMS.txt', version: '1' }, /must be a \.md file/],
        ['a missing file', { file: 'MISSING.md', version: '1' }, /does not exist/],
        [
            'a file outside the package',
            { file: '../TERMS.md', version: '1' },
            /outside the package/,
        ],
        ['an empty file', { file: 'EMPTY.md', version: '1' }, /is empty/],
        ['a file that is too large', { file: 'BIG.md', version: '1' }, /is larger than/],
    ])('refuses %s', (_name, terms, message) => {
        const dir = packageDir({
            'TERMS.md': 'Text',
            'TERMS.txt': 'Text',
            'EMPTY.md': ' \n',
            'BIG.md': 'x'.repeat(TERMS_MAX_BYTES + 1),
        })
        expect(() => resolveTerms(dir, 'gizmos', terms)).toThrow(message)
    })

    it('refuses a directory named like a markdown file', () => {
        const dir = packageDir({ 'TERMS.md/inner.txt': 'x' })
        expect(() => resolveTerms(dir, 'gizmos', { file: 'TERMS.md', version: '1' })).toThrow(
            /does not exist/
        )
    })
})
```

In `scripts/__tests__/gen-server.test.ts`, add this test as the last `it` of `describe('buildBundledPackages', ...)` (after the test `core bundled row is named "TinyCld Base", has a server, and a source spec`):

```ts
    it('carries the terms of a package that has them, and no terms keys for one that has none', () => {
        const rows = JSON.parse(
            buildBundledPackages([
                {
                    manifest: {
                        name: 'Gizmos',
                        slug: 'gizmos',
                        version: '0.1.0',
                        description: '',
                        terms: { file: 'TERMS.md', version: '2026-11-01' },
                    },
                    terms: { version: '2026-11-01', body: '# Gizmos terms' },
                },
                {
                    manifest: {
                        name: 'Trinkets',
                        slug: 'trinkets',
                        version: '0.2.0',
                        description: '',
                    },
                },
            ])
        ) as Record<string, unknown>[]
        expect(rows[0]).toMatchObject({ termsVersion: '2026-11-01', termsBody: '# Gizmos terms' })
        expect(JSON.parse(rows[0]?.manifestJson as string).terms).toEqual({
            file: 'TERMS.md',
            version: '2026-11-01',
        })
        expect(rows[1]).not.toHaveProperty('termsVersion')
        expect(rows[1]).not.toHaveProperty('termsBody')
    })
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `pnpm exec vitest run scripts/__tests__/gen-terms.test.ts scripts/__tests__/gen-server.test.ts`
Expected: FAIL. `gen-terms.test.ts` cannot resolve `../gen-terms`; the new `gen-server` test fails on `toMatchObject` (no `termsVersion`).

- [ ] **Step 3: Add the manifest types**

In `core/lib/packages/types.ts`, replace the end of `PackageManifest`:

```ts
    reportPanels?: ReportPanelDecl[]
}
```

with:

```ts
    reportPanels?: ReportPanelDecl[]

    /**
     * Terms and conditions the owner must accept before anybody uses this
     * package (PackageTermsDecl). Omit it and the package needs no acceptance.
     */
    terms?: PackageTermsDecl
}

/**
 * A package's terms and conditions. `file` is a markdown file in the package,
 * relative to the package root (for example `'TERMS.md'`). `version` is a label
 * of 1 to 50 letters, digits, `.`, `_` or `-` that the author changes when the
 * text changes; core compares it for equality only. The owner accepts one
 * version: a new version blocks the package again until the owner accepts it.
 * The generator reads the file (scripts/gen-terms.ts), so the text is the text
 * of the installed package version.
 */
export interface PackageTermsDecl {
    file: string
    version: string
}
```

In `scripts/load-manifest.ts`, replace the import on line 4:

```ts
import type { ReportPanelDecl } from '../core/lib/packages/types'
```

with:

```ts
import type { PackageTermsDecl, ReportPanelDecl } from '../core/lib/packages/types'
```

and replace:

```ts
    // Record panels beside a report. See ReportPanelDecl in core/lib/packages/types.ts.
    reportPanels?: ReportPanelDecl[]
}
```

with:

```ts
    // Record panels beside a report. See ReportPanelDecl in core/lib/packages/types.ts.
    reportPanels?: ReportPanelDecl[]
    // Terms the owner must accept before anybody uses the package. Validated
    // and read by scripts/gen-terms.ts. See PackageTermsDecl.
    terms?: PackageTermsDecl
}
```

- [ ] **Step 4: Write `scripts/gen-terms.ts`**

```ts
import * as fs from 'node:fs'
import * as path from 'node:path'
import type { PackageManifest } from './load-manifest'

// A terms version is an opaque label the package author changes when the text
// changes (a date or a number). It is compared for equality only.
export const TERMS_VERSION_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,49}$/
// Keep in step with the `body` max of pkg_terms
// (core/server/pb_migrations/2090000017_create_pkg_terms.js).
export const TERMS_MAX_BYTES = 200_000

export interface ResolvedTerms {
    version: string
    body: string
}

// Validate a manifest's `terms` and read its markdown file. Returns undefined
// for a package without terms. Throws with the package slug on a bad
// declaration, so a generate run stops before it writes a registry row that
// would block the package with no text to show.
export function resolveTerms(
    packageDir: string,
    slug: string,
    terms: PackageManifest['terms']
): ResolvedTerms | undefined {
    if (terms === undefined) return undefined
    const fail = (why: string): never => {
        throw new Error(`[generate] ${slug}: manifest.terms ${why}`)
    }
    if (typeof terms.version !== 'string' || !TERMS_VERSION_PATTERN.test(terms.version)) {
        fail(
            `version ${JSON.stringify(terms.version)} is not valid: use 1 to 50 letters, digits, '.', '_' or '-', starting with a letter or digit`
        )
    }
    if (typeof terms.file !== 'string' || !terms.file.endsWith('.md')) {
        fail(`file ${JSON.stringify(terms.file)} must be a .md file in the package`)
    }
    const root = path.resolve(packageDir)
    const file = path.resolve(root, terms.file)
    if (!file.startsWith(root + path.sep)) {
        fail(`file ${JSON.stringify(terms.file)} is outside the package`)
    }
    if (!fs.existsSync(file) || !fs.statSync(file).isFile()) {
        fail(`file ${JSON.stringify(terms.file)} does not exist`)
    }
    const body = fs.readFileSync(file, 'utf8').trim()
    if (body === '') fail(`file ${JSON.stringify(terms.file)} is empty`)
    if (Buffer.byteLength(body, 'utf8') > TERMS_MAX_BYTES) {
        fail(`file ${JSON.stringify(terms.file)} is larger than ${TERMS_MAX_BYTES} bytes`)
    }
    return { version: terms.version, body }
}
```

- [ ] **Step 5: Carry the terms on the bundled row**

In `scripts/gen-server.ts`, add the import between the `describe-packages` and `load-manifest` imports:

```ts
import type { ResolvedTerms } from './gen-terms'
```

In `interface BundledPkgInput`, after the `hasServer?: boolean` member, add:

```ts
    // The package's terms and conditions, already validated and read
    // (resolveTerms in gen-terms.ts). Omitted for a package without terms.
    terms?: ResolvedTerms
```

In `buildBundledPackages`, replace:

```ts
        ...(f.source ? { source: f.source } : {}),
    }))
```

with:

```ts
        ...(f.source ? { source: f.source } : {}),
        // Core's Go sync (SyncBundledPackages) copies these to
        // pkg_registry.terms_version and the pkg_terms collection.
        ...(f.terms ? { termsVersion: f.terms.version, termsBody: f.terms.body } : {}),
    }))
```

In `scripts/generate.ts`, add the import after the `./gen-server` import block (before `./gen-uniwind`):

```ts
import { resolveTerms } from './gen-terms'
```

Replace (in `main()`):

```ts
    validatePorts(portsFeatures)
    for (const f of features) {
```

with:

```ts
    validatePorts(portsFeatures)
    // Read each package's terms here, with the other checks, so a bad `terms`
    // stops the run before any file is written.
    const termsBySlug = new Map(
        features.map(f => [f.manifest.slug, resolveTerms(f.dir, f.manifest.slug, f.manifest.terms)])
    )
    for (const f of features) {
```

and in the `buildBundledPackages([...])` call, replace:

```ts
                source: resolveGitSource(f.dir, f.manifest.slug),
            })),
```

with:

```ts
                source: resolveGitSource(f.dir, f.manifest.slug),
                terms: termsBySlug.get(f.manifest.slug),
            })),
```

- [ ] **Step 6: Run the tests, biome and tsc**

Run: `pnpm exec vitest run scripts/__tests__/gen-terms.test.ts scripts/__tests__/gen-server.test.ts`
Expected: PASS, 2 files, 30 tests.

Run: `pnpm exec biome check scripts/gen-terms.ts scripts/__tests__/gen-terms.test.ts scripts/gen-server.ts scripts/generate.ts scripts/load-manifest.ts scripts/__tests__/gen-server.test.ts core/lib/packages/types.ts`
Expected: `Checked 7 files`, no errors.

Run: `pnpm exec tsc --noEmit -p tsconfig.json`
Expected: no output (about 15 s).

- [ ] **Step 7: Document the field**

In `docs/packages.md`, in the "Field reference" table, add this row after the `peerVersions` row:

```md
| `terms` | `{ file, version }` — terms and conditions the **owner must accept before anybody uses the package**. `file` is a markdown file in the package (e.g. `'TERMS.md'`); `version` is a label of 1-50 characters (`A-Z a-z 0-9 . _ -`) that you change when the text changes. Until the owner accepts the current version, the server refuses the package's collections and routes for everybody. See [Package terms](#package-terms-terms). Omit it and the package needs no acceptance. |
```

In the "Validation" section, add this bullet at the end of the "The generator fails fast on:" list (after the `notifications.events` bullet):

```md
- **`terms`**: a `version` that is not 1-50 characters of `A-Z a-z 0-9 . _ -` starting with a letter or digit; a `file` that is not a `.md` file, is outside the package, does not exist, is empty, or is larger than 200,000 bytes.
```

In section "G. Go server wiring", nothing changes. In section "Config & runtime derivations", add this paragraph at the end of the section (before the `### Validation` heading):

```md
`server/bundled-packages.json` also carries each package's terms: the generator reads `terms.file` and writes `termsVersion` and `termsBody` on the package's row (`scripts/gen-terms.ts`). A package without `terms` has neither key.
```

- [ ] **Step 8: Commit**

```bash
git add core/lib/packages/types.ts scripts/load-manifest.ts scripts/gen-terms.ts scripts/gen-server.ts scripts/generate.ts scripts/__tests__/gen-terms.test.ts scripts/__tests__/gen-server.test.ts docs/packages.md
git commit -m "feat(packages): manifest terms field, carried into bundled-packages.json"
```

---

### Task 2: Migration, `pkgterms` state, boot sync

**Files:**
- Create: `core/server/pb_migrations/2090000017_create_pkg_terms.js`
- Create: `core/server/pkgterms/pkgterms.go`
- Create: `core/server/pkgterms/pkgterms_test.go`
- Modify: `core/server/coreserver/pkg_seed.go` (imports; `bundledPackage`; the create and update branches of `SyncBundledPackages`)
- Create: `core/server/coreserver/pkg_terms_test.go`

**Interfaces:**
- Consumes: bundled row keys `termsVersion`, `termsBody` (Task 1); `rlstest.NewMigratedApp`, `rlstest.MigrationsDir`, `rlstest.SetPassword`; `usernames.Derive`; in `coreserver` tests `writeBundledJSON`, `withCwd`, `newRegistryOnlyApp` (`pkg_seed_test.go`).
- Produces:
  - Schema: `pkg_registry.terms_version`, `pkg_registry.terms_accepted_version` (text, max 50); collection `pkg_terms` (`pkg` unique, `version`, `body`, `created`, `updated`; list/view: signed in; no API writes); collection `pkg_terms_decisions` (`pkg`, `terms_version`, `decision` = `accepted|declined`, `user`, `created`; list/view: owner or admin; create: owner; no update, no delete; a plain index on `(pkg, terms_version, decision)`, not unique: a decline can be repeated, and a version the package comes back to can be accepted again).
  - Go, package `tinycld.org/core/pkgterms`: constants `RegistryCollection = "pkg_registry"`, `TermsCollection = "pkg_terms"`, `DecisionsCollection = "pkg_terms_decisions"`, `FieldTermsVersion = "terms_version"`, `FieldAcceptedVersion = "terms_accepted_version"`, `DecisionAccepted = "accepted"`, `DecisionDeclined = "declined"`; `func Pending(pkg *core.Record) bool`; `func Sync(app core.App, slug, version, body string) error`.
  - Test helpers in `pkgterms_test` (used by Tasks 3-5): `newApp(t testing.TB) *tests.TestApp`, `addPackage(t, app, slug, status, termsVersion, acceptedVersion string) *core.Record`, `addCollection(t, app, name string, fields ...core.Field) *core.Collection`, `addRecord(t, app, collection, body string) *core.Record`, `addUser(t, app, email, role string) *core.Record`, `tokenOf(t, rec) string`, `superuserToken(t, app) string`, `call(t *testing.T, app *tests.TestApp, method, url, token, body string, status int, parts ...string)`, `reload(t, app, rec) *core.Record`.

- [ ] **Step 1: Write the migration**

Create `core/server/pb_migrations/2090000017_create_pkg_terms.js`:

```js
/// <reference path="../pb_data/types.d.ts" />
//
// Package terms and conditions (manifest `terms`).
//
// pkg_registry.terms_version is the version the installed package declares,
// written at boot from bundled-packages.json. terms_accepted_version is the
// version the owner accepted, written only by the server when the owner
// creates an `accepted` row in pkg_terms_decisions (core/server/pkgterms).
// A package whose two values differ is refused to everybody.
//
// pkg_terms holds the text, one row per package, written only at boot. Any
// signed-in user may read it: a member reads the terms the owner accepted.
//
// pkg_terms_decisions is the record of each decision: package, terms version,
// user, time. Only the owner creates a row; nobody changes or removes one.
// A decision on terms that are already accepted is refused, so each acceptance
// row is one change from pending to accepted. A version can be accepted again
// after the package left it and came back to it.
//
// A new migration because 1720000000_create_pkg_registry.js is released.
migrate(
    app => {
        const SIGNED_IN = '@request.auth.id != "" && @request.auth.disabled != true'
        const ADMIN = `${SIGNED_IN} && (@request.auth.role = "owner" || @request.auth.role = "admin")`
        const OWNER = `${SIGNED_IN} && @request.auth.role = "owner"`

        const registry = app.findCollectionByNameOrId('pkg_registry')
        registry.fields.add(new TextField({ id: 'pr_terms_version', name: 'terms_version', max: 50 }))
        registry.fields.add(
            new TextField({ id: 'pr_terms_accepted', name: 'terms_accepted_version', max: 50 })
        )
        app.save(registry)

        app.save(
            new Collection({
                id: 'pbc_pkg_terms_01',
                name: 'pkg_terms',
                type: 'base',
                system: false,
                listRule: SIGNED_IN,
                viewRule: SIGNED_IN,
                createRule: null,
                updateRule: null,
                deleteRule: null,
                fields: [
                    { id: 'pt_pkg', name: 'pkg', type: 'text', required: true, max: 100 },
                    { id: 'pt_version', name: 'version', type: 'text', required: true, max: 50 },
                    // Keep max in step with TERMS_MAX_BYTES in scripts/gen-terms.ts.
                    { id: 'pt_body', name: 'body', type: 'text', required: true, max: 200000 },
                    { id: 'pt_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                    { id: 'pt_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
                ],
                indexes: ['CREATE UNIQUE INDEX `idx_pkg_terms_pkg` ON `pkg_terms` (`pkg`)'],
            })
        )

        app.save(
            new Collection({
                id: 'pbc_pkg_terms_dec_01',
                name: 'pkg_terms_decisions',
                type: 'base',
                system: false,
                listRule: ADMIN,
                viewRule: ADMIN,
                createRule: OWNER,
                updateRule: null,
                deleteRule: null,
                fields: [
                    { id: 'ptd_pkg', name: 'pkg', type: 'text', required: true, max: 100 },
                    { id: 'ptd_version', name: 'terms_version', type: 'text', required: true, max: 50 },
                    {
                        id: 'ptd_decision',
                        name: 'decision',
                        type: 'select',
                        required: true,
                        maxSelect: 1,
                        values: ['accepted', 'declined'],
                    },
                    // Not cascade: the record of an acceptance outlives the account.
                    {
                        id: 'ptd_user',
                        name: 'user',
                        type: 'relation',
                        required: false,
                        collectionId: '_pb_users_auth_',
                        cascadeDelete: false,
                        maxSelect: 1,
                    },
                    { id: 'ptd_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                ],
                indexes: [
                    'CREATE INDEX `idx_pkg_terms_decisions_pkg` ON `pkg_terms_decisions` (`pkg`, `terms_version`, `decision`)',
                ],
            })
        )
    },
    app => {
        for (const name of ['pkg_terms_decisions', 'pkg_terms']) {
            try {
                app.delete(app.findCollectionByNameOrId(name))
            } catch (e) {
                // may not exist
            }
        }
        const registry = app.findCollectionByNameOrId('pkg_registry')
        registry.fields.removeById('pr_terms_version')
        registry.fields.removeById('pr_terms_accepted')
        app.save(registry)
    }
)
```

(`pb_migrations` is outside biome's paths; there is no lint run for this file.)

- [ ] **Step 2: Write the failing `pkgterms` tests and the shared test helpers**

Create `core/server/pkgterms/pkgterms_test.go`:

```go
package pkgterms_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/pkgterms"
	"tinycld.org/core/rlstest"
	"tinycld.org/core/usernames"
)

// newApp is a database with every core migration: the real pkg_registry,
// pkg_terms and pkg_terms_decisions.
func newApp(t testing.TB) *tests.TestApp {
	t.Helper()
	return rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))
}

// addPackage stores the registry row of a fictional package. A blank terms
// version is a package with no terms.
func addPackage(t testing.TB, app core.App, slug, status, termsVersion, acceptedVersion string) *core.Record {
	t.Helper()
	registry, err := app.FindCollectionByNameOrId(pkgterms.RegistryCollection)
	if err != nil {
		t.Fatal(err)
	}
	pkg := core.NewRecord(registry)
	pkg.Set("name", slug)
	pkg.Set("slug", slug)
	pkg.Set("version", "1.0.0")
	pkg.Set("status", status)
	pkg.Set(pkgterms.FieldTermsVersion, termsVersion)
	pkg.Set(pkgterms.FieldAcceptedVersion, acceptedVersion)
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	return pkg
}

// addCollection stores a collection any signed-in user may read and write,
// so a refusal in a test is the terms guard and never a rule.
func addCollection(t testing.TB, app core.App, name string, fields ...core.Field) *core.Collection {
	t.Helper()
	authed := "@request.auth.id != ''"
	col := core.NewBaseCollection(name)
	col.Fields.Add(&core.TextField{Name: "body"})
	for _, f := range fields {
		col.Fields.Add(f)
	}
	col.ListRule, col.ViewRule, col.CreateRule, col.UpdateRule, col.DeleteRule = &authed, &authed, &authed, &authed, &authed
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	return col
}

func addRecord(t testing.TB, app core.App, collection, body string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("body", body)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func addUser(t testing.TB, app core.App, email, role string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.Set("username", usernames.Derive(email))
	u.Set("name", "Test")
	u.Set("role", role)
	u.SetVerified(true)
	u.SetPassword("Password123!")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func tokenOf(t testing.TB, rec *core.Record) string {
	t.Helper()
	token, err := rec.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func superuserToken(t testing.TB, app core.App) string {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(col)
	su.SetEmail("root@terms.test")
	rlstest.SetPassword(su, "Password123!")
	if err := app.Save(su); err != nil {
		t.Fatal(err)
	}
	return tokenOf(t, su)
}

// call sends one request to the app's API and checks the status and the
// parts of the body. A blank token is a signed-out request.
func call(t *testing.T, app *tests.TestApp, method, url, token, body string, status int, parts ...string) {
	t.Helper()
	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["Authorization"] = token
	}
	sc := tests.ApiScenario{
		Name: method + " " + url, Method: method, URL: url, Headers: headers,
		ExpectedStatus: status, ExpectedContent: parts,
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	if body != "" {
		sc.Body = strings.NewReader(body)
	}
	if status == http.StatusNoContent {
		sc.ExpectedContent = nil
	}
	sc.Test(t)
}

func reload(t testing.TB, app core.App, rec *core.Record) *core.Record {
	t.Helper()
	fresh, err := app.FindRecordById(rec.Collection().Name, rec.Id)
	if err != nil {
		t.Fatal(err)
	}
	return fresh
}

func TestPending(t *testing.T) {
	app := newApp(t)
	cases := []struct {
		name, slug, version, accepted string
		want                          bool
	}{
		{"a package with no terms", "plain", "", "", false},
		{"terms nobody accepted", "fresh", "2", "", true},
		{"the current version accepted", "agreed", "2", "2", false},
		{"an older version accepted", "moved", "2", "1", true},
		{"terms removed after an acceptance", "dropped", "", "1", false},
	}
	for _, c := range cases {
		pkg := addPackage(t, app, c.slug, "bundled", c.version, c.accepted)
		if got := pkgterms.Pending(pkg); got != c.want {
			t.Errorf("%s: Pending = %v, want %v", c.name, got, c.want)
		}
	}
}

func termsRow(t testing.TB, app core.App, slug string) *core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter(pkgterms.TermsCollection, "pkg = {:pkg}", "", 0, 0, map[string]any{"pkg": slug})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) > 1 {
		t.Fatalf("%d terms rows for %s, want at most one", len(rows), slug)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func TestSyncStoresUpdatesAndRemovesTheText(t *testing.T) {
	app := newApp(t)

	if err := pkgterms.Sync(app, "acme", "1", "# Acme terms"); err != nil {
		t.Fatal(err)
	}
	first := termsRow(t, app, "acme")
	if first == nil || first.GetString("version") != "1" || first.GetString("body") != "# Acme terms" {
		t.Fatalf("after the first sync: %v", first)
	}

	// The same terms again write nothing: `updated` does not move at a boot.
	if err := pkgterms.Sync(app, "acme", "1", "# Acme terms"); err != nil {
		t.Fatal(err)
	}
	if same := termsRow(t, app, "acme"); same.GetString("updated") != first.GetString("updated") {
		t.Fatal("a sync of unchanged terms wrote the row")
	}

	if err := pkgterms.Sync(app, "acme", "2", "# Acme terms, second"); err != nil {
		t.Fatal(err)
	}
	second := termsRow(t, app, "acme")
	if second.Id != first.Id || second.GetString("version") != "2" || second.GetString("body") != "# Acme terms, second" {
		t.Fatalf("after a new version: %v", second)
	}

	if err := pkgterms.Sync(app, "acme", "", ""); err != nil {
		t.Fatal(err)
	}
	if gone := termsRow(t, app, "acme"); gone != nil {
		t.Fatalf("terms removed from the manifest are still stored: %v", gone)
	}
	// A package that never had terms has nothing to remove.
	if err := pkgterms.Sync(app, "plain", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestSyncRefusesAVersionWithNoText(t *testing.T) {
	app := newApp(t)
	if err := pkgterms.Sync(app, "acme", "1", ""); err == nil {
		t.Fatal("a version with no text was stored")
	}
	if row := termsRow(t, app, "acme"); row != nil {
		t.Fatalf("stored: %v", row)
	}
}

// A database with no pkg_terms collection (a bare fixture) takes a package
// with no terms and refuses one that has them.
func TestSyncWithoutTheCollection(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := pkgterms.Sync(app, "plain", "", ""); err != nil {
		t.Fatalf("a package with no terms: %v", err)
	}
	if err := pkgterms.Sync(app, "acme", "1", "text"); err == nil {
		t.Fatal("terms with nowhere to store them gave no error")
	}
}
```

- [ ] **Step 3: Run the tests to see them fail**

Run (from `core/server`): `go test ./pkgterms/`
Expected: FAIL to build: `no non-test Go files in .../core/server/pkgterms`.

- [ ] **Step 4: Write `core/server/pkgterms/pkgterms.go`**

```go
// Package pkgterms holds the terms and conditions of packages: which version
// a package declares, which version the owner accepted, and the refusal of
// every request to a package whose current version is not accepted.
//
// A package declares terms in its manifest (`terms`). The generator carries
// the version and the text into bundled-packages.json; the boot sync writes
// the version to pkg_registry.terms_version and the text to pkg_terms (Sync).
// The owner accepts by creating a pkg_terms_decisions row; a hook checks it
// and writes pkg_registry.terms_accepted_version (Register). While the two
// versions differ the package is pending, and SlugError / CollectionError
// refuse it. A package with no terms has a blank terms_version and is never
// pending.
//
// The package imports no core package that asks it (pkgaccess, search,
// reports), so each of them can.
package pkgterms

import (
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

const (
	RegistryCollection  = "pkg_registry"
	TermsCollection     = "pkg_terms"
	DecisionsCollection = "pkg_terms_decisions"

	// FieldTermsVersion is the version the installed package declares.
	FieldTermsVersion = "terms_version"
	// FieldAcceptedVersion is the version the owner accepted.
	FieldAcceptedVersion = "terms_accepted_version"

	DecisionAccepted = "accepted"
	DecisionDeclined = "declined"
)

// Pending is true when the package has terms and the owner has not accepted
// their current version.
func Pending(pkg *core.Record) bool {
	version := pkg.GetString(FieldTermsVersion)
	return version != "" && pkg.GetString(FieldAcceptedVersion) != version
}

// Sync stores the terms text of one package, or removes it when the package
// has none (a blank version). It runs at boot for every bundled package.
//
// A database without the pkg_terms collection can hold no terms: that is an
// error for a package that has them, and nothing to do for one that has none.
func Sync(app core.App, slug, version, body string) error {
	col, err := app.FindCollectionByNameOrId(TermsCollection)
	if err != nil {
		if version == "" {
			return nil
		}
		return fmt.Errorf("pkgterms: package %s has terms but there is no %s collection: %w", slug, TermsCollection, err)
	}
	rows, err := app.FindRecordsByFilter(TermsCollection, "pkg = {:pkg}", "", 1, 0, map[string]any{"pkg": slug})
	if err != nil {
		return fmt.Errorf("pkgterms: read the terms of %s: %w", slug, err)
	}
	if version == "" {
		for _, row := range rows {
			if err := app.Delete(row); err != nil {
				return fmt.Errorf("pkgterms: remove the terms of %s: %w", slug, err)
			}
		}
		return nil
	}
	if body == "" {
		return errors.New("pkgterms: package " + slug + " has a terms version but no terms text")
	}
	row := core.NewRecord(col)
	if len(rows) > 0 {
		row = rows[0]
		if row.GetString("version") == version && row.GetString("body") == body {
			return nil
		}
	}
	row.Set("pkg", slug)
	row.Set("version", version)
	row.Set("body", body)
	if err := app.Save(row); err != nil {
		return fmt.Errorf("pkgterms: store the terms of %s: %w", slug, err)
	}
	return nil
}
```

- [ ] **Step 5: Run the `pkgterms` tests**

Run: `go vet ./pkgterms/ && go test ./pkgterms/`
Expected: `ok  	tinycld.org/core/pkgterms`.

- [ ] **Step 6: Write the failing boot sync tests**

Create `core/server/coreserver/pkg_terms_test.go`:

```go
package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgterms"
	"tinycld.org/core/rlstest"
)

func registryRow(t *testing.T, app core.App, slug string) *core.Record {
	t.Helper()
	row, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:slug}", map[string]any{"slug": slug})
	if err != nil {
		t.Fatalf("registry row of %s: %v", slug, err)
	}
	return row
}

func termsText(t *testing.T, app core.App, slug string) (version, body string, found bool) {
	t.Helper()
	rows, err := app.FindRecordsByFilter(pkgterms.TermsCollection, "pkg = {:pkg}", "", 0, 0, map[string]any{"pkg": slug})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return "", "", false
	}
	return rows[0].GetString("version"), rows[0].GetString("body"), true
}

// The boot sync carries a package's terms from bundled-packages.json into
// the registry and pkg_terms, never touches the accepted version, and so
// makes a package pending again when its terms version changes.
func TestSyncBundledPackagesCarriesTheTerms(t *testing.T) {
	app := rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))
	dir := t.TempDir()
	withCwd(t, dir)
	gizmos := bundledPackage{Name: "Gizmos", Slug: "gizmos", Version: "1.0.0", TermsVersion: "2026-01", TermsBody: "# Gizmos terms"}
	trinkets := bundledPackage{Name: "Trinkets", Slug: "trinkets", Version: "1.0.0"}

	writeBundledJSON(t, dir, []bundledPackage{gizmos, trinkets})
	SyncBundledPackages(app)

	row := registryRow(t, app, "gizmos")
	if row.GetString("terms_version") != "2026-01" || row.GetString("terms_accepted_version") != "" || !pkgterms.Pending(row) {
		t.Fatalf("a new package with terms: version %q, accepted %q", row.GetString("terms_version"), row.GetString("terms_accepted_version"))
	}
	if version, body, ok := termsText(t, app, "gizmos"); !ok || version != "2026-01" || body != "# Gizmos terms" {
		t.Fatalf("the stored terms: %q %q %v", version, body, ok)
	}
	if plain := registryRow(t, app, "trinkets"); plain.GetString("terms_version") != "" || pkgterms.Pending(plain) {
		t.Fatal("a package with no terms is pending")
	}
	if _, _, ok := termsText(t, app, "trinkets"); ok {
		t.Fatal("a package with no terms has a terms row")
	}

	// The owner accepts; the next boot with the same terms keeps that.
	row.Set("terms_accepted_version", "2026-01")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	SyncBundledPackages(app)
	if row = registryRow(t, app, "gizmos"); pkgterms.Pending(row) {
		t.Fatal("a boot with unchanged terms made the package pending")
	}

	// A new release with new terms: pending again, the text replaced.
	gizmos.Version, gizmos.TermsVersion, gizmos.TermsBody = "1.1.0", "2026-06", "# Gizmos terms, June"
	writeBundledJSON(t, dir, []bundledPackage{gizmos, trinkets})
	SyncBundledPackages(app)
	row = registryRow(t, app, "gizmos")
	if !pkgterms.Pending(row) || row.GetString("terms_version") != "2026-06" || row.GetString("terms_accepted_version") != "2026-01" {
		t.Fatalf("after new terms: version %q, accepted %q", row.GetString("terms_version"), row.GetString("terms_accepted_version"))
	}
	if version, body, _ := termsText(t, app, "gizmos"); version != "2026-06" || body != "# Gizmos terms, June" {
		t.Fatalf("the stored terms after a new version: %q %q", version, body)
	}

	// A release that drops its terms: nothing to accept, no text.
	gizmos.TermsVersion, gizmos.TermsBody = "", ""
	writeBundledJSON(t, dir, []bundledPackage{gizmos, trinkets})
	SyncBundledPackages(app)
	if row = registryRow(t, app, "gizmos"); row.GetString("terms_version") != "" || pkgterms.Pending(row) {
		t.Fatal("a package that dropped its terms is still pending")
	}
	if _, _, ok := termsText(t, app, "gizmos"); ok {
		t.Fatal("the text of dropped terms is still stored")
	}
}

// The seed tests' registry has no terms fields and no pkg_terms collection
// (newRegistryOnlyApp): a sync of packages with no terms still works there.
func TestSyncBundledPackagesWithoutTheTermsSchema(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	withCwd(t, dir)
	writeBundledJSON(t, dir, []bundledPackage{{Name: "Trinkets", Slug: "trinkets", Version: "1.0.0"}})
	SyncBundledPackages(app)
	if row := registryRow(t, app, "trinkets"); row.GetString("status") != "bundled" {
		t.Fatalf("status %q", row.GetString("status"))
	}
}
```

Run: `go vet ./coreserver/`
Expected: FAIL: `unknown field TermsVersion in struct literal of type bundledPackage`.

- [ ] **Step 7: Sync the terms at boot**

In `core/server/coreserver/pkg_seed.go`, add the import (its own group after the pocketbase import):

```go
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgterms"
)
```

Replace the end of the `bundledPackage` struct:

```go
	Source string `json:"source"`
}
```

with:

```go
	Source string `json:"source"`
	// TermsVersion and TermsBody are the package's terms and conditions
	// (manifest `terms`), read by the generator. Both blank for a package
	// with no terms.
	TermsVersion string `json:"termsVersion,omitempty"`
	TermsBody    string `json:"termsBody,omitempty"`
}

// syncTerms stores a bundled package's terms text after its registry row is
// saved. A failure is logged, not fatal, as for the row itself: the row then
// still carries terms_version, so the package stays refused, never open.
func syncTerms(app core.App, pkg bundledPackage) {
	if err := pkgterms.Sync(app, pkg.Slug, pkg.TermsVersion, pkg.TermsBody); err != nil {
		srvLog.Error("failed to store package terms", "slug", pkg.Slug, "err", err)
	}
}
```

In `SyncBundledPackages`, in the create branch, replace:

```go
				record.Set("manifest_json", pkg.ManifestJSON)
			}
			if err := app.Save(record); err != nil {
				srvLog.Error("failed to create bundled package registry row", "slug", pkg.Slug, "err", err)
			}
			continue
```

with:

```go
				record.Set("manifest_json", pkg.ManifestJSON)
			}
			// A new package with terms starts pending: terms_accepted_version
			// is blank until the owner accepts.
			record.Set(pkgterms.FieldTermsVersion, pkg.TermsVersion)
			if err := app.Save(record); err != nil {
				srvLog.Error("failed to create bundled package registry row", "slug", pkg.Slug, "err", err)
			}
			syncTerms(app, pkg)
			continue
```

and in the update branch, replace:

```go
			existing.Set("manifest_json", pkg.ManifestJSON)
		}
		if err := app.Save(existing); err != nil {
			srvLog.Error("failed to update bundled package registry row", "slug", pkg.Slug, "err", err)
		}
	}
```

with:

```go
			existing.Set("manifest_json", pkg.ManifestJSON)
		}
		// terms_accepted_version is never written here: a new terms version
		// then differs from it, and the package is pending again.
		existing.Set(pkgterms.FieldTermsVersion, pkg.TermsVersion)
		if err := app.Save(existing); err != nil {
			srvLog.Error("failed to update bundled package registry row", "slug", pkg.Slug, "err", err)
		}
		syncTerms(app, pkg)
	}
```

- [ ] **Step 8: Run the boot sync tests and the access-rule guard**

Run: `go vet ./coreserver/ && go test ./coreserver/ -run 'TestSyncBundledPackages|TestFindBundledPackagesJSON'`
Expected: `ok  	tinycld.org/core/coreserver`.

Run: `go test ./rlstest/`
Expected: `ok  	tinycld.org/core/rlstest` (it checks that every access rule of the core migrations, the two new collections included, requires a login).

Run: `gofmt -l pkgterms coreserver`
Expected: no output.

- [ ] **Step 9: Commit**

```bash
git add core/server/pb_migrations/2090000017_create_pkg_terms.js core/server/pkgterms/pkgterms.go core/server/pkgterms/pkgterms_test.go core/server/coreserver/pkg_seed.go core/server/coreserver/pkg_terms_test.go
git commit -m "feat(pkgterms): terms schema, state and boot sync"
```

---

### Task 3: Decisions, registry guards, audit

**Files:**
- Create: `core/server/pkgterms/decisions.go`
- Create: `core/server/pkgterms/decisions_test.go`
- Modify: `core/server/coreserver/server.go` (imports; `RegisterSharedCore`, after `pkgaccess.Register(app)`)
- Modify: `core/server/coreserver/audit.go` (imports; `RegisterAuditHooks`)
- Create: `core/server/coreserver/pkg_terms_audit_test.go`
- Modify: `core/server/coreserver/composition_parity_test.go` (`hostHookCounts`)

**Interfaces:**
- Consumes: Task 2's constants, `Pending`, schema and test helpers; `audit.RegisterCollection(app *pocketbase.PocketBase, name string, config *audit.CollectionConfig)`, `audit.LabelFromFields(fieldNames ...string)` (joins the values with `:`); in `coreserver` tests `bootedPocketBase(t) *pocketbase.PocketBase` (`autoupgrade_wiring_test.go`), `guestRLSUser(t, app, email, role)` (`guest_rls_test.go`), `waitFor(t, cond)` (`backup_api_test.go`), `registryRow` (Task 2).
- Produces:
  - `func Register(app core.App)` in `pkgterms`, called from `coreserver.RegisterSharedCore`. Tasks 4 and 5 each add one line to it.
  - Behavior: `POST /api/collections/pkg_terms_decisions/records` with `{pkg, terms_version, decision}` by the owner is the only way to accept or decline. The server sets `user`. `accepted` sets `pkg_registry.terms_accepted_version` in the same transaction. Refusals (400, in `data`): `pkg` → `unknown_package` or `no_terms`; `terms_version` → `not_current`; `decision` → `already_accepted` (any decision while the package is not pending). A decline can be repeated. Part 2's client reads these codes.
  - A non-superuser request cannot change `terms_version` or `terms_accepted_version` of `pkg_registry`, create a row with either set, change the `slug` of a pending row, or delete a pending row: 403 `The terms of a package are accepted on the terms screen.`
  - Audit: one `audit_logs` row per decision, `action` `created`, `resource_type` `pkg_terms_decisions`, `resource_label` `<pkg>:<terms_version>:<decision>`, `actor` the owner.

- [ ] **Step 1: Write the failing decision tests**

Create `core/server/pkgterms/decisions_test.go`:

```go
package pkgterms_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/pkgterms"
)

const decisionsURL = "/api/collections/pkg_terms_decisions/records"

type decisionEnv struct {
	app    *tests.TestApp
	pkg    *core.Record
	owner  *core.Record
	admin  *core.Record
	ownerT string
	adminT string
}

// newDecisionEnv is the app with the fictional package acme, pending on
// version 2 of its terms, and pkgterms registered.
func newDecisionEnv(t *testing.T) *decisionEnv {
	t.Helper()
	app := newApp(t)
	pkgterms.Register(app)
	env := &decisionEnv{app: app, pkg: addPackage(t, app, "acme", "bundled", "2", "")}
	env.owner = addUser(t, app, "owner@terms.test", "owner")
	env.admin = addUser(t, app, "admin@terms.test", "admin")
	env.ownerT, env.adminT = tokenOf(t, env.owner), tokenOf(t, env.admin)
	return env
}

func decisionBody(pkg, version, decision string) string {
	return `{"pkg":"` + pkg + `","terms_version":"` + version + `","decision":"` + decision + `"}`
}

func (e *decisionEnv) decisions(t *testing.T) []*core.Record {
	t.Helper()
	rows, err := e.app.FindRecordsByFilter(pkgterms.DecisionsCollection, "id != ''", "created", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestOwnerAcceptsTheTerms(t *testing.T) {
	e := newDecisionEnv(t)
	// The body names another user: the server records who decided.
	body := `{"pkg":"acme","terms_version":"2","decision":"accepted","user":"` + e.admin.Id + `"}`
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, body, http.StatusOK, `"decision":"accepted"`, `"user":"`+e.owner.Id+`"`)

	pkg := reload(t, e.app, e.pkg)
	if pkg.GetString(pkgterms.FieldAcceptedVersion) != "2" || pkgterms.Pending(pkg) {
		t.Fatalf("after the acceptance: accepted %q, pending %v", pkg.GetString(pkgterms.FieldAcceptedVersion), pkgterms.Pending(pkg))
	}
	rows := e.decisions(t)
	if len(rows) != 1 || rows[0].GetString("pkg") != "acme" || rows[0].GetString("terms_version") != "2" ||
		rows[0].GetString("user") != e.owner.Id || rows[0].GetDateTime("created").IsZero() {
		t.Fatalf("the record of the acceptance: %v", rows)
	}
}

func TestOnlyTheOwnerDecides(t *testing.T) {
	e := newDecisionEnv(t)
	member := tokenOf(t, addUser(t, e.app, "member@terms.test", "member"))
	for name, token := range map[string]string{"an admin": e.adminT, "a member": member, "signed out": ""} {
		t.Run(name, func(t *testing.T) {
			call(t, e.app, http.MethodPost, decisionsURL, token, decisionBody("acme", "2", "accepted"), http.StatusBadRequest, `"status":400`)
		})
	}
	if rows := e.decisions(t); len(rows) != 0 {
		t.Fatalf("a decision of somebody who is not the owner was stored: %v", rows)
	}
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("the package is no longer pending")
	}
}

func TestADecisionIsCheckedAgainstThePackage(t *testing.T) {
	e := newDecisionEnv(t)
	addPackage(t, e.app, "plain", "bundled", "", "")
	cases := []struct{ name, body, code string }{
		{"a version that is not current", decisionBody("acme", "1", "accepted"), `"terms_version":{"code":"not_current"`},
		{"a package that does not exist", decisionBody("nothing", "2", "accepted"), `"pkg":{"code":"unknown_package"`},
		{"a package with no terms", decisionBody("plain", "2", "accepted"), `"pkg":{"code":"no_terms"`},
		{"a decision that is neither", decisionBody("acme", "2", "maybe"), `"decision":{"code":"validation_`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, c.body, http.StatusBadRequest, c.code)
		})
	}
	if rows := e.decisions(t); len(rows) != 0 {
		t.Fatalf("a refused decision was stored: %v", rows)
	}
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a refused decision accepted the terms")
	}
}

func TestADeclineKeepsThePackagePendingAndCanBeFollowedByAnAcceptance(t *testing.T) {
	e := newDecisionEnv(t)
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "declined"), http.StatusOK, `"decision":"declined"`)
	pkg := reload(t, e.app, e.pkg)
	if !pkgterms.Pending(pkg) || pkg.GetString(pkgterms.FieldAcceptedVersion) != "" {
		t.Fatal("a decline accepted the terms")
	}

	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "accepted"), http.StatusOK, `"decision":"accepted"`)
	if pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("the acceptance after a decline did not accept")
	}
	// An acceptance is final for its version: no decline, no second acceptance.
	for _, decision := range []string{"declined", "accepted"} {
		call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", decision), http.StatusBadRequest, `"decision":{"code":"already_accepted"`)
	}
	if rows := e.decisions(t); len(rows) != 2 {
		t.Fatalf("%d decision rows, want the decline and the acceptance", len(rows))
	}
}

// The owner declines as often as the terms are shown, and accepts a version
// again when the package left it and came back to it (an older release put
// back).
func TestADeclineAndAReturnedVersionCanBeDecidedAgain(t *testing.T) {
	e := newDecisionEnv(t)
	for range 2 {
		call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "declined"), http.StatusOK, `"decision":"declined"`)
	}
	for _, version := range []string{"2", "3", "2"} {
		pkg := reload(t, e.app, e.pkg)
		pkg.Set(pkgterms.FieldTermsVersion, version)
		if err := e.app.Save(pkg); err != nil {
			t.Fatal(err)
		}
		call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", version, "accepted"), http.StatusOK, `"terms_version":"`+version+`"`)
		if pkgterms.Pending(reload(t, e.app, e.pkg)) {
			t.Fatalf("version %s is not accepted", version)
		}
	}
	if rows := e.decisions(t); len(rows) != 5 {
		t.Fatalf("%d decision rows, want two declines and three acceptances", len(rows))
	}
}

func TestANewVersionNeedsANewAcceptance(t *testing.T) {
	e := newDecisionEnv(t)
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "accepted"), http.StatusOK, `"terms_version":"2"`)

	// The boot sync writes the version of the new package release.
	pkg := reload(t, e.app, e.pkg)
	pkg.Set(pkgterms.FieldTermsVersion, "3")
	if err := e.app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a new terms version did not make the package pending")
	}
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "accepted"), http.StatusBadRequest, `"not_current"`)
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "3", "accepted"), http.StatusOK, `"terms_version":"3"`)
	pkg = reload(t, e.app, e.pkg)
	if pkgterms.Pending(pkg) || pkg.GetString(pkgterms.FieldAcceptedVersion) != "3" {
		t.Fatal("the acceptance of the new version did not accept")
	}
}

// A decision stored by the server with no request (a seed) follows the same
// rule: an acceptance sets the accepted version.
func TestASeededAcceptanceAccepts(t *testing.T) {
	e := newDecisionEnv(t)
	col, err := e.app.FindCollectionByNameOrId(pkgterms.DecisionsCollection)
	if err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(col)
	row.Set("pkg", "acme")
	row.Set("terms_version", "2")
	row.Set("decision", pkgterms.DecisionAccepted)
	if err := e.app.Save(row); err != nil {
		t.Fatal(err)
	}
	if pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a seeded acceptance did not accept")
	}
}

func TestNobodyChangesOrRemovesADecision(t *testing.T) {
	e := newDecisionEnv(t)
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "declined"), http.StatusOK, `"decision":"declined"`)
	id := e.decisions(t)[0].Id
	call(t, e.app, http.MethodPatch, decisionsURL+"/"+id, e.ownerT, `{"decision":"accepted"}`, http.StatusForbidden, `"status":403`)
	call(t, e.app, http.MethodDelete, decisionsURL+"/"+id, e.ownerT, "", http.StatusForbidden, `"status":403`)
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("an edit of a decision accepted the terms")
	}
}

func TestTheOwnerCannotWriteTheTermsFieldsOfTheRegistry(t *testing.T) {
	e := newDecisionEnv(t)
	url := "/api/collections/pkg_registry/records/" + e.pkg.Id
	const refusal = "The terms of a package are accepted on the terms screen."
	call(t, e.app, http.MethodPatch, url, e.ownerT, `{"terms_accepted_version":"2"}`, http.StatusForbidden, refusal)
	call(t, e.app, http.MethodPatch, url, e.ownerT, `{"terms_version":""}`, http.StatusForbidden, refusal)
	call(t, e.app, http.MethodDelete, url, e.ownerT, "", http.StatusForbidden, refusal)
	call(t, e.app, http.MethodPost, "/api/collections/pkg_registry/records", e.ownerT,
		`{"name":"Other","slug":"other","status":"available","terms_accepted_version":"1"}`, http.StatusForbidden, refusal)
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a registry write accepted the terms")
	}

	// The slug decides which collections and routes the row owns.
	call(t, e.app, http.MethodPatch, url, e.ownerT, `{"slug":"other"}`, http.StatusForbidden, refusal)

	// The owner still turns the package off and on, and a superuser repairs.
	call(t, e.app, http.MethodPatch, url, e.ownerT, `{"status":"disabled"}`, http.StatusOK, `"status":"disabled"`)
	call(t, e.app, http.MethodPatch, url, superuserToken(t, e.app), `{"terms_accepted_version":"2"}`, http.StatusOK, `"terms_accepted_version":"2"`)
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `go vet ./pkgterms/`
Expected: FAIL: `undefined: pkgterms.Register`.

- [ ] **Step 3: Write `core/server/pkgterms/decisions.go`**

```go
package pkgterms

import (
	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

// Register binds everything that keeps a package with unaccepted terms out
// of use. Bound once by coreserver's shared registration.
func Register(app core.App) {
	registerDecisions(app)
	registerRegistryGuards(app)
}

// registerDecisions makes a pkg_terms_decisions row the one way to accept or
// decline. The create rule admits only the owner; the request hook records
// who decided; the model hook checks the row against the package and, for an
// acceptance, writes the accepted version in the same transaction, so a
// stored `accepted` row and the registry never disagree.
func registerDecisions(app core.App) {
	app.OnRecordCreateRequest(DecisionsCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			if e.Auth == nil || e.Auth.GetString("role") != "owner" {
				return e.ForbiddenError("Only the owner decides on the terms of a package.", nil)
			}
			// The client does not choose whose decision this is.
			e.Record.Set("user", e.Auth.Id)
		}
		return e.Next()
	})

	app.OnRecordCreate(DecisionsCollection).BindFunc(func(e *core.RecordEvent) error {
		slug := e.Record.GetString("pkg")
		version := e.Record.GetString("terms_version")
		decision := e.Record.GetString("decision")
		original := e.App
		defer func() { e.App = original }()
		return original.RunInTransaction(func(txApp core.App) error {
			pkg, err := txApp.FindFirstRecordByFilter(RegistryCollection, "slug = {:slug}", dbx.Params{"slug": slug})
			if err != nil {
				return validation.Errors{"pkg": validation.NewError("unknown_package", "There is no such package.")}
			}
			current := pkg.GetString(FieldTermsVersion)
			if current == "" {
				return validation.Errors{"pkg": validation.NewError("no_terms", "The package has no terms.")}
			}
			if version != current {
				return validation.Errors{"terms_version": validation.NewError("not_current", "This is not the current version of the terms.")}
			}
			if !Pending(pkg) {
				return validation.Errors{"decision": validation.NewError("already_accepted", "The terms are already accepted.")}
			}
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			if decision != DecisionAccepted {
				return nil
			}
			pkg.Set(FieldAcceptedVersion, version)
			return txApp.Save(pkg)
		})
	})
}

// registerRegistryGuards keeps the terms fields of pkg_registry out of the
// owner's reach through the record API: the update rule of pkg_registry
// admits the owner (the owner turns packages on and off), so without this an
// owner could write terms_accepted_version and leave no decision record. A
// pending row cannot be deleted through the record API either: with no row
// the package has no owner of its collections and nothing is refused.
func registerRegistryGuards(app core.App) {
	const refusal = "The terms of a package are accepted on the terms screen."
	app.OnRecordCreateRequest(RegistryCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() && (e.Record.GetString(FieldTermsVersion) != "" || e.Record.GetString(FieldAcceptedVersion) != "") {
			return e.ForbiddenError(refusal, nil)
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest(RegistryCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			before := e.Record.Original()
			for _, field := range []string{FieldTermsVersion, FieldAcceptedVersion} {
				if e.Record.GetString(field) != before.GetString(field) {
					return e.ForbiddenError(refusal, nil)
				}
			}
			// The slug names the collections and routes the row owns: a new
			// slug on a pending row would end the refusal with no decision.
			if Pending(before) && e.Record.GetString("slug") != before.GetString("slug") {
				return e.ForbiddenError(refusal, nil)
			}
		}
		return e.Next()
	})
	app.OnRecordDeleteRequest(RegistryCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() && Pending(e.Record) {
			return e.ForbiddenError(refusal, nil)
		}
		return e.Next()
	})
}
```

The transaction pattern (swap `e.App` to the transaction's app for `e.Next()`, restore it after) is the one `core/server/groups/hooks.go` uses in `nextInTx`.

- [ ] **Step 4: Run the decision tests**

Run: `go vet ./pkgterms/ && go test ./pkgterms/`
Expected: `ok  	tinycld.org/core/pkgterms`.

- [ ] **Step 5: Write the failing audit test**

Create `core/server/coreserver/pkg_terms_audit_test.go`:

```go
package coreserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgterms"
)

// A decision is in the audit log: the package, the version and the decision
// in the label, the owner as the actor.
func TestATermsDecisionIsAudited(t *testing.T) {
	app := bootedPocketBase(t)
	RegisterAuditHooks(app)
	pkgterms.Register(app)
	registry, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	pkg := core.NewRecord(registry)
	pkg.Set("name", "Gizmos")
	pkg.Set("slug", "gizmos")
	pkg.Set("version", "1.0.0")
	pkg.Set("status", "bundled")
	pkg.Set("terms_version", "2026-01")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	owner := guestRLSUser(t, app, "terms-owner@test.local", "owner")

	// The create request of the record API, as its handler triggers it: the
	// audit hook and the pkgterms hooks run, then the record is saved.
	decisions, err := app.FindCollectionByNameOrId(pkgterms.DecisionsCollection)
	if err != nil {
		t.Fatal(err)
	}
	decision := core.NewRecord(decisions)
	decision.Set("pkg", "gizmos")
	decision.Set("terms_version", "2026-01")
	decision.Set("decision", pkgterms.DecisionAccepted)
	request := &core.RequestEvent{App: app, Auth: owner}
	request.Request = httptest.NewRequest(http.MethodPost, "/api/collections/pkg_terms_decisions/records", nil)
	request.Response = httptest.NewRecorder()
	event := &core.RecordRequestEvent{RequestEvent: request, Collection: decisions, Record: decision}
	err = app.OnRecordCreateRequest().Trigger(event, func(e *core.RecordRequestEvent) error {
		return e.App.Save(e.Record)
	})
	if err != nil {
		t.Fatal(err)
	}
	if pkgterms.Pending(registryRow(t, app, "gizmos")) {
		t.Fatal("the decision did not accept the terms")
	}

	var logs []*core.Record
	waitFor(t, func() bool {
		logs, _ = app.FindRecordsByFilter("audit_logs", "resource_type = 'pkg_terms_decisions'", "", 0, 0)
		return len(logs) > 0
	})
	if len(logs) != 1 || logs[0].GetString("action") != "created" || logs[0].GetString("resource_label") != "gizmos:2026-01:accepted" {
		t.Fatalf("the audit row: %d rows, action %q, label %q", len(logs), logs[0].GetString("action"), logs[0].GetString("resource_label"))
	}
	if actor := logs[0].GetString("actor"); actor != owner.Id {
		t.Fatalf("the actor is %q, want the owner %q", actor, owner.Id)
	}
}
```

Run: `go test ./coreserver/ -run TestATermsDecisionIsAudited`
Expected: FAIL: `condition was never met within 5s` (the collection is not audited yet).

- [ ] **Step 6: Register `pkgterms` and the audit**

In `core/server/coreserver/server.go`, add the import after `"tinycld.org/core/pkgaccess"`:

```go
	"tinycld.org/core/pkgterms"
```

and in `RegisterSharedCore`, replace:

```go
	pkgaccess.Register(app)
	// Saved reports and their shares.
```

with (keep the rest of that comment as it is):

```go
	pkgaccess.Register(app)
	// Package terms: the owner's decisions, and the refusal of every package
	// whose terms the owner has not accepted. Shared: a package is refused
	// wherever it runs.
	pkgterms.Register(app)
	// Saved reports and their shares.
```

In `core/server/coreserver/audit.go`, replace the import block:

```go
import (
	"github.com/pocketbase/pocketbase"
	"tinycld.org/core/audit"
)
```

with:

```go
import (
	"github.com/pocketbase/pocketbase"
	"tinycld.org/core/audit"
	"tinycld.org/core/pkgterms"
)
```

and in `RegisterAuditHooks`, after the line `audit.RegisterCollections(app, []string{"label_assignments", "org_pkg_access"}, nil)`, add:

```go
	// The owner's decision on a package's terms: which package, which
	// version, accepted or declined. The actor and the time are the log's own.
	audit.RegisterCollection(app, pkgterms.DecisionsCollection, &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromFields("pkg", "terms_version", "decision"),
	})
```

- [ ] **Step 7: Update the golden hook counts**

In `core/server/coreserver/composition_parity_test.go`, in `hostHookCounts`, change these four values (keep each line's existing comment and add the note given):

| Key | Old | New | Add to the comment |
|---|---|---|---|
| `"OnRecordCreate"` | 8 | 9 | `, and the check of a package terms decision (shared, via pkgterms.Register)` |
| `"OnRecordCreateRequest"` | 16 | 19 | `// incl. pkgterms: who decided, the registry's terms fields; and the audit of a terms decision` |
| `"OnRecordDeleteRequest"` | 18 | 20 | `// incl. pkgterms: a pending registry row; and the audit of a terms decision` |
| `"OnRecordUpdateRequest"` | 19 | 21 | `// incl. pkgterms: the registry's terms fields; and the audit of a terms decision` |

Run: `gofmt -l coreserver` and fix the alignment it reports (`gofmt -d coreserver/composition_parity_test.go` shows it).

- [ ] **Step 8: Run the tests**

Run: `go vet ./coreserver/ && go test ./coreserver/ -run 'TestATermsDecisionIsAudited|TestRegisterBindsTheRecordedHandlerCounts|Composition|Parity|TestSyncBundledPackages'`
Expected: `ok  	tinycld.org/core/coreserver`.

- [ ] **Step 9: Commit**

```bash
git add core/server/pkgterms/decisions.go core/server/pkgterms/decisions_test.go core/server/coreserver/server.go core/server/coreserver/audit.go core/server/coreserver/pkg_terms_audit_test.go core/server/coreserver/composition_parity_test.go
git commit -m "feat(pkgterms): the owner's decision record, registry guards and audit"
```

---

### Task 4: Collection enforcement

**Files:**
- Create: `core/server/pkgterms/guard.go`
- Create: `core/server/pkgterms/guard_test.go`
- Modify: `core/server/pkgterms/decisions.go` (`Register`)
- Modify: `core/server/pkgaccess/pkgaccess.go` (imports; `WriteError`)
- Modify: `core/server/coreserver/composition_parity_test.go` (`hostHookCounts`)

**Interfaces:**
- Consumes: Task 2's constants, `Pending`, test helpers; Task 3's `Register`; `pkgaccess.WriteError(app core.App, user *core.Record, slug string) error`, `pkgaccess.CanWrite`.
- Produces (package `pkgterms`):
  - `type PendingError struct{ Slug string }`, `Error()` = `The owner has not accepted the terms of the <slug> package.`
  - `func Refusal(err error) error`: a `*PendingError` becomes `apis.NewForbiddenError(err.Error(), nil)` (a 403 `*router.ApiError`); any other error, and nil, come back unchanged.
  - `func SlugError(app core.App, slug string) error`: nil, `*PendingError`, or the registry read error.
  - `func CollectionError(app core.App, collection string) error`: the same for the package that owns the collection (`<slug>` or `<slug>_*`, dashes as underscores, longest slug first, registry rows of every status).
  - Every non-superuser request (signed out too) to a pending package's collection is refused with the 403: record list, view, create, update, delete, file download. A realtime subscribe call loses the topics of a pending package's collections and keeps the others, with no error: a client that subscribed while the package was pending gets no events after the acceptance, so part 2's client must subscribe again (reload the collections) when the owner accepts.
  - An `expand` that reaches a pending package's collection, by a relation field or a `_via_` back-relation, fails the request with the 403 for a non-superuser.
  - A non-superuser cannot create a `pkg_registry` row, or change a row's `slug`, to a slug that is or extends the slug of a pending package (403 with the pending package's message).
  - `pkgaccess.WriteError` returns the same 403 for a pending package (non-nil, non-superuser user), before the access level.

- [ ] **Step 1: Write the failing guard tests**

Create `core/server/pkgterms/guard_test.go`:

```go
package pkgterms_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/pkgaccess"
	"tinycld.org/core/pkgterms"
)

const acmeRefusal = "The owner has not accepted the terms of the acme package."

type guardEnv struct {
	app    *tests.TestApp
	pkg    *core.Record
	note   *core.Record
	owner  *core.Record
	ownerT string
	memT   string
}

// newGuardEnv is the app with the fictional package acme (pending on
// version 2), its collection acme_notes with one record, a collection no
// package owns, and pkgterms registered.
func newGuardEnv(t *testing.T) *guardEnv {
	t.Helper()
	app := newApp(t)
	pkgterms.Register(app)
	e := &guardEnv{app: app, pkg: addPackage(t, app, "acme", "bundled", "2", "")}
	addCollection(t, app, "acme_notes")
	addCollection(t, app, "freeform_scratch")
	e.note = addRecord(t, app, "acme_notes", "hello")
	addRecord(t, app, "freeform_scratch", "free")
	e.owner = addUser(t, app, "owner@terms.test", "owner")
	e.ownerT = tokenOf(t, e.owner)
	e.memT = tokenOf(t, addUser(t, app, "member@terms.test", "member"))
	return e
}

func (e *guardEnv) accept(t *testing.T) {
	t.Helper()
	pkg := reload(t, e.app, e.pkg)
	pkg.Set(pkgterms.FieldAcceptedVersion, pkg.GetString(pkgterms.FieldTermsVersion))
	if err := e.app.Save(pkg); err != nil {
		t.Fatal(err)
	}
}

const notesURL = "/api/collections/acme_notes/records"

func TestAPendingPackageRefusesEveryRecordRequest(t *testing.T) {
	e := newGuardEnv(t)
	one := notesURL + "/" + e.note.Id
	for name, token := range map[string]string{"a member": e.memT, "the owner": e.ownerT} {
		t.Run(name, func(t *testing.T) {
			call(t, e.app, http.MethodGet, notesURL, token, "", http.StatusForbidden, acmeRefusal)
			call(t, e.app, http.MethodGet, one, token, "", http.StatusForbidden, acmeRefusal)
			call(t, e.app, http.MethodPost, notesURL, token, `{"body":"new"}`, http.StatusForbidden, acmeRefusal)
			call(t, e.app, http.MethodPatch, one, token, `{"body":"changed"}`, http.StatusForbidden, acmeRefusal)
			call(t, e.app, http.MethodDelete, one, token, "", http.StatusForbidden, acmeRefusal)
		})
	}
	if n, _ := e.app.CountRecords("acme_notes"); n != 1 {
		t.Fatalf("%d notes, want the one seeded", n)
	}
	if got := reload(t, e.app, e.note).GetString("body"); got != "hello" {
		t.Fatalf("the note was changed to %q", got)
	}
}

// A collection open to the public (a share page reads one with no session)
// is refused too: nobody uses the package.
func TestAPendingPackageRefusesASignedOutRequestToAPublicCollection(t *testing.T) {
	e := newGuardEnv(t)
	open := addCollection(t, e.app, "acme_open")
	public := ""
	open.ListRule, open.ViewRule, open.CreateRule, open.UpdateRule, open.DeleteRule = &public, &public, &public, &public, &public
	if err := e.app.Save(open); err != nil {
		t.Fatal(err)
	}
	rec := addRecord(t, e.app, "acme_open", "shared")
	all := "/api/collections/acme_open/records"
	one := all + "/" + rec.Id
	call(t, e.app, http.MethodGet, all, "", "", http.StatusForbidden, acmeRefusal)
	call(t, e.app, http.MethodGet, one, "", "", http.StatusForbidden, acmeRefusal)
	call(t, e.app, http.MethodPost, all, "", `{"body":"new"}`, http.StatusForbidden, acmeRefusal)
	call(t, e.app, http.MethodPatch, one, "", `{"body":"changed"}`, http.StatusForbidden, acmeRefusal)
	call(t, e.app, http.MethodDelete, one, "", "", http.StatusForbidden, acmeRefusal)

	e.accept(t)
	call(t, e.app, http.MethodGet, all, "", "", http.StatusOK, `"body":"shared"`)
}

func TestTheRefusalEndsWithTheAcceptance(t *testing.T) {
	e := newGuardEnv(t)
	call(t, e.app, http.MethodGet, notesURL, e.memT, "", http.StatusForbidden, acmeRefusal)
	e.accept(t)
	call(t, e.app, http.MethodGet, notesURL, e.memT, "", http.StatusOK, `"body":"hello"`)
	call(t, e.app, http.MethodPost, notesURL, e.memT, `{"body":"new"}`, http.StatusOK, `"body":"new"`)
}

func TestASuperuserAndOtherCollectionsAreNotRefused(t *testing.T) {
	e := newGuardEnv(t)
	call(t, e.app, http.MethodGet, notesURL, superuserToken(t, e.app), "", http.StatusOK, `"body":"hello"`)
	call(t, e.app, http.MethodGet, "/api/collections/freeform_scratch/records", e.memT, "", http.StatusOK, `"body":"free"`)
	// The terms themselves stay readable: a member reads what the owner must accept.
	if err := pkgterms.Sync(e.app, "acme", "2", "# Acme terms"); err != nil {
		t.Fatal(err)
	}
	call(t, e.app, http.MethodGet, "/api/collections/pkg_terms/records", e.memT, "", http.StatusOK, `"body":"# Acme terms"`)
}

// Turning the package off does not end the refusal: the owner of a
// collection is read over the registry rows of every status.
func TestAPackageThatIsTurnedOffStaysRefused(t *testing.T) {
	for _, status := range []string{"disabled", "available", "installed"} {
		t.Run(status, func(t *testing.T) {
			e := newGuardEnv(t)
			pkg := reload(t, e.app, e.pkg)
			pkg.Set("status", status)
			if err := e.app.Save(pkg); err != nil {
				t.Fatal(err)
			}
			call(t, e.app, http.MethodGet, notesURL, e.memT, "", http.StatusForbidden, acmeRefusal)
		})
	}
}

func TestCollectionErrorFindsTheOwnerByTheLongestSlug(t *testing.T) {
	app := newApp(t)
	addPackage(t, app, "acme", "bundled", "2", "")
	addPackage(t, app, "acme-pro", "bundled", "1", "1")
	addPackage(t, app, "zeta", "bundled", "1", "1")
	addPackage(t, app, "zeta-plus", "bundled", "5", "")
	addPackage(t, app, "plain", "bundled", "", "")
	cases := []struct {
		collection, pending string
	}{
		{"acme", "acme"},
		{"acme_notes", "acme"},
		{"acmenotes", ""},
		{"acme_pro", ""},
		{"acme_pro_notes", ""},
		{"zeta_items", ""},
		{"zeta_plus_items", "zeta-plus"},
		{"plain_things", ""},
		{"users", ""},
	}
	for _, c := range cases {
		err := pkgterms.CollectionError(app, c.collection)
		var pending *pkgterms.PendingError
		got := ""
		if errors.As(err, &pending) {
			got = pending.Slug
		} else if err != nil {
			t.Fatalf("%s: %v", c.collection, err)
		}
		if got != c.pending {
			t.Errorf("%s: refused for %q, want %q", c.collection, got, c.pending)
		}
	}
}

func TestSlugErrorAndRefusal(t *testing.T) {
	app := newApp(t)
	addPackage(t, app, "acme", "disabled", "2", "1")
	addPackage(t, app, "plain", "bundled", "", "")
	for _, slug := range []string{"plain", "nothing"} {
		if err := pkgterms.SlugError(app, slug); err != nil {
			t.Errorf("%s: %v", slug, err)
		}
	}
	err := pkgterms.SlugError(app, "acme")
	var pending *pkgterms.PendingError
	if !errors.As(err, &pending) || pending.Slug != "acme" {
		t.Fatalf("acme: %v", err)
	}
	var api *router.ApiError
	if !errors.As(pkgterms.Refusal(err), &api) || api.Status != http.StatusForbidden || api.Message != acmeRefusal {
		t.Fatalf("Refusal = %v", pkgterms.Refusal(err))
	}
	other := errors.New("disk")
	if pkgterms.Refusal(other) != other || pkgterms.Refusal(nil) != nil {
		t.Fatal("Refusal changed an error that is not a pending package")
	}
}

// A database whose registry has no terms fields, or no registry at all (a
// bare fixture), has no pending package.
func TestADatabaseWithoutTermsFieldsRefusesNothing(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := pkgterms.CollectionError(app, "acme_notes"); err != nil {
		t.Fatalf("no registry: %v", err)
	}
	registry := core.NewBaseCollection(pkgterms.RegistryCollection)
	registry.Fields.Add(&core.TextField{Name: "slug"})
	if err := app.Save(registry); err != nil {
		t.Fatal(err)
	}
	if err := pkgterms.CollectionError(app, "acme_notes"); err != nil {
		t.Fatalf("a registry without the fields: %v", err)
	}
	if err := pkgterms.SlugError(app, "acme"); err != nil {
		t.Fatalf("a registry without the fields: %v", err)
	}
}

func TestAPendingPackageRefusesItsFiles(t *testing.T) {
	app := newApp(t)
	pkgterms.Register(app)
	pkg := addPackage(t, app, "acme", "bundled", "2", "")
	col := addCollection(t, app, "acme_files", &core.FileField{Name: "doc", MaxSelect: 1, MaxSize: 1 << 20})
	file, err := filesystem.NewFileFromBytes([]byte("secret"), "doc.txt")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("doc", file)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	url := "/api/files/acme_files/" + rec.Id + "/" + rec.GetString("doc")
	member := tokenOf(t, addUser(t, app, "member@terms.test", "member"))

	call(t, app, http.MethodGet, url, member, "", http.StatusForbidden, acmeRefusal)

	pkg.Set(pkgterms.FieldAcceptedVersion, "2")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	call(t, app, http.MethodGet, url, member, "", http.StatusOK, "secret")
}

func subscribeAs(t *testing.T, app core.App, auth *core.Record, topics ...string) []string {
	t.Helper()
	event := &core.RealtimeSubscribeRequestEvent{
		RequestEvent:  &core.RequestEvent{App: app, Auth: auth},
		Subscriptions: topics,
	}
	var kept []string
	err := app.OnRealtimeSubscribeRequest().Trigger(event, func(e *core.RealtimeSubscribeRequestEvent) error {
		kept = e.Subscriptions
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return kept
}

func TestARealtimeSubscriptionDropsTheTopicsOfAPendingPackage(t *testing.T) {
	e := newGuardEnv(t)
	notes, err := e.app.FindCollectionByNameOrId("acme_notes")
	if err != nil {
		t.Fatal(err)
	}
	topics := []string{
		"acme_notes",
		"acme_notes/*",
		"acme_notes/" + e.note.Id,
		`acme_notes/*?options={"query":{"filter":"body != ''"}}`,
		notes.Id + "/*",
		"collection/acme_notes",
		"freeform_scratch/*",
		"collection/freeform_scratch",
		"users/" + e.owner.Id,
		"a-custom-topic",
	}
	free := []string{"freeform_scratch/*", "collection/freeform_scratch", "users/" + e.owner.Id, "a-custom-topic"}

	if got := subscribeAs(t, e.app, e.owner, topics...); !slices.Equal(got, free) {
		t.Fatalf("the owner keeps %q, want %q", got, free)
	}
	if got := subscribeAs(t, e.app, nil, topics...); !slices.Equal(got, free) {
		t.Fatalf("a signed-out client keeps %q, want %q", got, free)
	}
	superusers, err := e.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	if got := subscribeAs(t, e.app, core.NewRecord(superusers), topics...); !slices.Equal(got, topics) {
		t.Fatalf("a superuser keeps %q, want every topic", got)
	}
	e.accept(t)
	if got := subscribeAs(t, e.app, e.owner, topics...); !slices.Equal(got, topics) {
		t.Fatalf("after the acceptance the owner keeps %q, want every topic", got)
	}
}

// pkgaccess.WriteError is the write gate of the protocol servers and of the
// automation actions, which do not pass the record API.
func TestWriteErrorRefusesAPendingPackage(t *testing.T) {
	e := newGuardEnv(t)
	err := pkgaccess.WriteError(e.app, e.owner, "acme")
	var api *router.ApiError
	if !errors.As(err, &api) || api.Status != http.StatusForbidden || api.Message != acmeRefusal {
		t.Fatalf("the owner on a pending package: %v", err)
	}
	if pkgaccess.CanWrite(e.app, e.owner, "acme") {
		t.Fatal("CanWrite admits a pending package")
	}
	e.accept(t)
	if err := pkgaccess.WriteError(e.app, e.owner, "acme"); err != nil {
		t.Fatalf("after the acceptance: %v", err)
	}
}

// An expand reads a related collection with no request hook of that
// collection. A relation to a pending package, and a back-relation from it,
// give none of its records.
func TestAnExpandDoesNotReadAPendingPackage(t *testing.T) {
	app := newApp(t)
	pkgterms.Register(app)
	pkg := addPackage(t, app, "acme", "bundled", "2", "")
	scratch := addCollection(t, app, "freeform_scratch")
	notes := addCollection(t, app, "acme_notes", &core.RelationField{Name: "scratch", CollectionId: scratch.Id, MaxSelect: 1})
	scratch.Fields.Add(&core.RelationField{Name: "note", CollectionId: notes.Id, MaxSelect: 1})
	if err := app.Save(scratch); err != nil {
		t.Fatal(err)
	}
	free := addRecord(t, app, "freeform_scratch", "free")
	note := addRecord(t, app, "acme_notes", "hidden text")
	note.Set("scratch", free.Id)
	if err := app.Save(note); err != nil {
		t.Fatal(err)
	}
	free.Set("note", note.Id)
	if err := app.Save(free); err != nil {
		t.Fatal(err)
	}
	member := tokenOf(t, addUser(t, app, "member@terms.test", "member"))
	const all = "/api/collections/freeform_scratch/records"

	for _, expand := range []string{"note", "acme_notes_via_scratch"} {
		t.Run(expand, func(t *testing.T) {
			res := httpGet(t, app, all+"?expand="+expand, member)
			if strings.Contains(res, "hidden text") {
				t.Fatalf("the expand gave a record of the pending package: %s", res)
			}
		})
	}
	// Without the expand the collection itself is read as before.
	call(t, app, http.MethodGet, all, member, "", http.StatusOK, `"body":"free"`)

	pkg.Set(pkgterms.FieldAcceptedVersion, "2")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	call(t, app, http.MethodGet, all+"?expand=note", member, "", http.StatusOK, `"body":"hidden text"`)
	call(t, app, http.MethodGet, all+"?expand=acme_notes_via_scratch", member, "", http.StatusOK, `"body":"hidden text"`)
}

// httpGet returns the status and body of one GET, for a test that pins
// what a response does not hold.
func httpGet(t *testing.T, app *tests.TestApp, url, token string) string {
	t.Helper()
	base, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	mux, err := base.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return strconv.Itoa(rec.Code) + " " + rec.Body.String()
}

// CollectionError gives a collection to the longest slug. The owner may
// write registry rows, so a row with a longer slug and no terms must not
// take the collections of a pending package.
func TestTheOwnerCannotShadowAPendingPackage(t *testing.T) {
	e := newGuardEnv(t)
	const registryURL = "/api/collections/pkg_registry/records"
	for _, slug := range []string{"acme-notes", "acme_notes", "acme"} {
		call(t, e.app, http.MethodPost, registryURL, e.ownerT,
			`{"name":"Shadow","slug":"`+slug+`","status":"available"}`, http.StatusForbidden, acmeRefusal)
	}
	plain := addPackage(t, e.app, "plain", "bundled", "", "")
	call(t, e.app, http.MethodPatch, registryURL+"/"+plain.Id, e.ownerT, `{"slug":"acme-notes"}`, http.StatusForbidden, acmeRefusal)
	if err := pkgterms.CollectionError(e.app, "acme_notes"); err == nil {
		t.Fatal("a registry write ended the refusal")
	}

	// A slug that only starts with the same letters owns other collections.
	call(t, e.app, http.MethodPost, registryURL, e.ownerT,
		`{"name":"Acmeplus","slug":"acmeplus","status":"available"}`, http.StatusOK, `"slug":"acmeplus"`)
	call(t, e.app, http.MethodPatch, registryURL+"/"+plain.Id, e.ownerT, `{"slug":"plainer"}`, http.StatusOK, `"slug":"plainer"`)
	e.accept(t)
	call(t, e.app, http.MethodPost, registryURL, e.ownerT,
		`{"name":"Notes","slug":"acme-notes","status":"available"}`, http.StatusOK, `"slug":"acme-notes"`)
}

// One check with nothing pending is one read of the registry. The plan
// records what this gave (20.8 µs/op).
func BenchmarkCollectionErrorNothingPending(b *testing.B) {
	app := newApp(b)
	for _, slug := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		addPackage(b, app, "pkg-"+slug, "bundled", "1", "1")
	}
	b.ResetTimer()
	for b.Loop() {
		if err := pkgterms.CollectionError(app, "pkg_a_notes"); err != nil {
			b.Fatal(err)
		}
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `go vet ./pkgterms/`
Expected: FAIL: `undefined: pkgterms.CollectionError` (and `PendingError`, `SlugError`, `Refusal`).

- [ ] **Step 3: Write `core/server/pkgterms/guard.go`**

```go
package pkgterms

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// PendingError is the refusal of a package whose terms the owner has not
// accepted.
type PendingError struct {
	Slug string
}

func (e *PendingError) Error() string {
	return fmt.Sprintf("The owner has not accepted the terms of the %s package.", e.Slug)
}

// Refusal is err as an HTTP handler returns it: a PendingError becomes a 403
// with its message; every other error is returned as it is.
func Refusal(err error) error {
	var pending *PendingError
	if errors.As(err, &pending) {
		return apis.NewForbiddenError(pending.Error(), nil)
	}
	return err
}

// pendingSQL selects the packages that are pending. The same condition as
// Pending. No variable: both sides are columns or constants.
const pendingSQL = "[[" + FieldTermsVersion + "]] != '' AND [[" + FieldAcceptedVersion + "]] != [[" + FieldTermsVersion + "]]"

// hasTermsFields is false for a database that cannot hold terms: no registry,
// or a registry from before the terms fields (a bare test fixture). Such a
// database has no pending package.
func hasTermsFields(app core.App) bool {
	col, err := app.FindCachedCollectionByNameOrId(RegistryCollection)
	if err != nil {
		return false
	}
	return col.Fields.GetByName(FieldTermsVersion) != nil && col.Fields.GetByName(FieldAcceptedVersion) != nil
}

// pendingPackages reads the registry rows of every pending package, of any
// status: a package the owner turned off is still pending.
func pendingPackages(app core.App) ([]*core.Record, error) {
	if !hasTermsFields(app) {
		return nil, nil
	}
	var rows []*core.Record
	if err := app.RecordQuery(RegistryCollection).AndWhere(dbx.NewExp(pendingSQL)).All(&rows); err != nil {
		// A failed read is not permission to use a package that may be pending.
		return nil, fmt.Errorf("pkgterms: read the pending packages: %w", err)
	}
	return rows, nil
}

// SlugError returns nil when the package may be used, a *PendingError when
// its terms are not accepted, and the read error when the registry cannot be
// read.
func SlugError(app core.App, slug string) error {
	rows, err := pendingPackages(app)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.GetString("slug") == slug {
			return &PendingError{Slug: slug}
		}
	}
	return nil
}

// collectionPrefix is a slug as collection names carry it.
func collectionPrefix(slug string) string {
	return strings.ReplaceAll(slug, "-", "_")
}

func owns(slug, collection string) bool {
	prefix := collectionPrefix(slug)
	return prefix != "" && (collection == prefix || strings.HasPrefix(collection, prefix+"_"))
}

// CollectionError is SlugError for the package that owns a collection. A
// collection belongs to the package whose slug is its name or its name
// prefix (dashes as underscores), the longest slug first, as in pkgaccess,
// but over the registry rows of every status. A collection no package owns
// is never refused.
func CollectionError(app core.App, collection string) error {
	rows, err := pendingPackages(app)
	if err != nil {
		return err
	}
	candidate := ""
	for _, row := range rows {
		slug := row.GetString("slug")
		if owns(slug, collection) && len(slug) > len(candidate) {
			candidate = slug
		}
	}
	if candidate == "" {
		return nil
	}
	// A pending package matches by name. A package with a longer slug that is
	// not pending may be the true owner (`acme` pending, `acme-pro` accepted,
	// collection acme_pro_notes), so the owner is read over every package.
	var all []*core.Record
	if err := app.RecordQuery(RegistryCollection).All(&all); err != nil {
		return fmt.Errorf("pkgterms: read the packages: %w", err)
	}
	for _, row := range all {
		slug := row.GetString("slug")
		if owns(slug, collection) && len(slug) > len(candidate) {
			if Pending(row) {
				candidate = slug
				continue
			}
			return nil
		}
	}
	return &PendingError{Slug: candidate}
}

// topicCollection is the collection a realtime topic names: "<collection>",
// "<collection>/<id>", "<collection>/*", each with an optional "?options=",
// and the change-signal form "collection/<collection>". The collection is a
// name or an id.
func topicCollection(app core.App, topic string) string {
	topic, _, _ = strings.Cut(topic, "?")
	first, rest, _ := strings.Cut(topic, "/")
	if first == "collection" && rest != "" {
		first, _, _ = strings.Cut(rest, "/")
	}
	col, err := app.FindCachedCollectionByNameOrId(first)
	if err != nil {
		return ""
	}
	return col.Name
}

// registerGuards binds the refusal on every way a request reaches a
// collection's records: list, view, create, update, delete, a file of a
// record, and a realtime subscription. A superuser is not refused: the
// dashboard, the seeds and a repair need the rows.
func registerGuards(app core.App) {
	record := func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			if err := CollectionError(e.App, e.Collection.Name); err != nil {
				return Refusal(err)
			}
		}
		return e.Next()
	}
	app.OnRecordViewRequest().BindFunc(record)
	app.OnRecordCreateRequest().BindFunc(record)
	app.OnRecordUpdateRequest().BindFunc(record)
	app.OnRecordDeleteRequest().BindFunc(record)

	app.OnRecordsListRequest().BindFunc(func(e *core.RecordsListRequestEvent) error {
		if !e.HasSuperuserAuth() {
			if err := CollectionError(e.App, e.Collection.Name); err != nil {
				return Refusal(err)
			}
		}
		return e.Next()
	})

	app.OnFileDownloadRequest().BindFunc(func(e *core.FileDownloadRequestEvent) error {
		if !e.HasSuperuserAuth() {
			if err := CollectionError(e.App, e.Collection.Name); err != nil {
				return Refusal(err)
			}
		}
		return e.Next()
	})

	// An expand reads the related collection with its view rule only: no
	// request hook of that collection runs. The enrich hook does run for each
	// expanded record, so the refusal is here. Only for an expand: the other
	// records of a response passed one of the guards above.
	app.OnRecordEnrich().BindFunc(func(e *core.RecordEnrichEvent) error {
		info := e.RequestInfo
		if info == nil || info.Context != core.RequestInfoContextExpand || (info.Auth != nil && info.Auth.IsSuperuser()) {
			return e.Next()
		}
		if err := CollectionError(e.App, e.Record.Collection().Name); err != nil {
			return Refusal(err)
		}
		return e.Next()
	})

	// A subscribe call names every topic the client wants. The topics of a
	// pending package are dropped and the others kept: one refused topic must
	// not end the client's other subscriptions.
	app.OnRealtimeSubscribeRequest().BindFunc(func(e *core.RealtimeSubscribeRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		kept := make([]string, 0, len(e.Subscriptions))
		for _, topic := range e.Subscriptions {
			name := topicCollection(e.App, topic)
			if name != "" {
				err := CollectionError(e.App, name)
				var pending *PendingError
				if errors.As(err, &pending) {
					continue
				}
				if err != nil {
					return err
				}
			}
			kept = append(kept, topic)
		}
		e.Subscriptions = kept
		return e.Next()
	})
}

// shadowError refuses a registry slug that would own collections of a
// pending package: CollectionError gives a collection to the longest slug, so
// a row `acme-notes` with no terms would take acme_notes from a pending
// `acme`. The slug of the pending package itself is refused too.
func shadowError(app core.App, slug string) error {
	rows, err := pendingPackages(app)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if owns(row.GetString("slug"), collectionPrefix(slug)) {
			return Refusal(&PendingError{Slug: row.GetString("slug")})
		}
	}
	return nil
}

// registerSlugGuards keeps a non-superuser from giving a registry row a slug
// that shadows a pending package. The owner may create and update registry
// rows (the rule of pkg_registry admits the owner).
func registerSlugGuards(app core.App) {
	app.OnRecordCreateRequest(RegistryCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			if err := shadowError(e.App, e.Record.GetString("slug")); err != nil {
				return err
			}
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest(RegistryCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() && e.Record.GetString("slug") != e.Record.Original().GetString("slug") {
			if err := shadowError(e.App, e.Record.GetString("slug")); err != nil {
				return err
			}
		}
		return e.Next()
	})
}
```

In `core/server/pkgterms/decisions.go`, replace the body of `Register`:

```go
func Register(app core.App) {
	registerDecisions(app)
	registerRegistryGuards(app)
}
```

with:

```go
func Register(app core.App) {
	registerDecisions(app)
	registerRegistryGuards(app)
	registerGuards(app)
	registerSlugGuards(app)
}
```

- [ ] **Step 4: Refuse in `pkgaccess.WriteError`**

In `core/server/pkgaccess/pkgaccess.go`, replace the import block's last group:

```go
	"tinycld.org/core/logging"
)
```

with:

```go
	"tinycld.org/core/logging"
	"tinycld.org/core/pkgterms"
)
```

and in `WriteError`, replace:

```go
	if user == nil || user.IsSuperuser() {
		return nil
	}
	level, err := LevelFor(app, user, slug)
```

with:

```go
	if user == nil || user.IsSuperuser() {
		return nil
	}
	// Before the level: nobody, the owner too, writes to a package whose
	// terms the owner has not accepted. This is what refuses the protocol
	// servers (DAV, IMAP/SMTP) and the automation actions, which do not pass
	// the record API where pkgterms binds its own guards.
	if err := pkgterms.SlugError(app, slug); err != nil {
		return pkgterms.Refusal(err)
	}
	level, err := LevelFor(app, user, slug)
```

- [ ] **Step 5: Run the tests**

Run: `go vet ./pkgterms/ ./pkgaccess/ && go test ./pkgterms/ ./pkgaccess/`
Expected: `ok` for both packages.

Run: `go test ./coreserver/ -run 'TestPkgAccess'`
Expected: `ok  	tinycld.org/core/coreserver` (the package-access guard tests still pass: their `demo` package has no terms).

- [ ] **Step 6: Update the golden hook counts**

In `core/server/coreserver/composition_parity_test.go`, in `hostHookCounts`:

| Key | Old | New | Comment |
|---|---|---|---|
| `"OnRecordCreateRequest"` | 19 | 21 | add `, the guard of a pending package, the registry slug guard` after `the registry's terms fields` |
| `"OnRecordDeleteRequest"` | 20 | 21 | add `, the guard of a pending package` after `a pending registry row` |
| `"OnRecordUpdateRequest"` | 21 | 23 | add `, the guard of a pending package, the registry slug guard` after `the registry's terms fields` |

and add five keys, each in its alphabetical place:

```go
	"OnRecordEnrich":                      1, // the package terms guard of an expand (shared, via pkgterms.Register)
```

(after `"OnRecordDeleteRequest"`, before `"OnRecordUpdate"`),

```go
	"OnFileDownloadRequest":               1, // the package terms guard (shared, via pkgterms.Register)
```

(after `"OnCollectionValidate"`),

```go
	"OnRealtimeSubscribeRequest":          1, // the package terms guard (shared, via pkgterms.Register)
```

(after `"OnModelValidate"`),

```go
	"OnRecordViewRequest":                 1,  // the package terms guard (shared, via pkgterms.Register)
	"OnRecordsListRequest":                1,  // the package terms guard (shared, via pkgterms.Register)
```

(after `"OnRecordValidate"`, before `"OnServe"`). Run `gofmt -l coreserver`; when it names the file, apply the spacing `gofmt -d coreserver/composition_parity_test.go` shows (it aligns the comments of adjacent lines).

Run: `go test ./coreserver/ -run 'TestRegisterBindsTheRecordedHandlerCounts|Composition|Parity'`
Expected: `ok  	tinycld.org/core/coreserver`.

- [ ] **Step 7: Commit**

```bash
git add core/server/pkgterms/guard.go core/server/pkgterms/guard_test.go core/server/pkgterms/decisions.go core/server/pkgaccess/pkgaccess.go core/server/coreserver/composition_parity_test.go
git commit -m "feat(pkgterms): refuse the collections of a package with unaccepted terms"
```

---

### Task 5: Route enforcement

**Files:**
- Create: `core/server/pkgterms/routes.go`
- Create: `core/server/pkgterms/routes_test.go`
- Modify: `core/server/pkgterms/decisions.go` (`Register`)
- Modify: `core/server/davprefix/davprefix.go` (new method before `ForApp`)
- Create: `core/server/davprefix/owner_test.go`
- Modify: `core/server/coreserver/composition_parity_test.go` (`"OnServe"`)

**Interfaces:**
- Consumes: Task 4's `SlugError`, `Refusal`; Task 3's `Register`; Task 2's test helpers and Task 4's `acmeRefusal` constant (`guard_test.go`); `davprefix.New() *Registry`, `davprefix.ForApp(app) *Registry`, `(*Registry).Claim(slug, prefix string) error`, the registry's `claimed map[string]string` (prefix → slug).
- Produces:
  - `func (r *Registry) Owner(path string) string` in `davprefix`.
  - Every request, of anybody (a superuser too), to `/api/<slug>`, `/api/<slug>/…` or a DAV prefix of a pending package answers the 403 `The owner has not accepted the terms of the <slug> package.` Part 2's client relies on this message.

- [ ] **Step 1: Write the failing `davprefix` test, then `Owner`**

Create `core/server/davprefix/owner_test.go`:

```go
package davprefix

import "testing"

func TestOwnerFindsThePackageOfAPath(t *testing.T) {
	r := New()
	for prefix, slug := range map[string]string{"/gizmos": "gizmos", "/gizmos/shared": "cogs", "/dav/files": "gadgets"} {
		if err := r.Claim(slug, prefix); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]string{
		"/gizmos":             "gizmos",
		"/gizmos/":            "gizmos",
		"/gizmos/ann/book":    "gizmos",
		"/gizmos/shared":      "cogs",
		"/gizmos/shared/team": "cogs",
		"/gizmos/sharedx":     "gizmos",
		"/gizmosx":            "",
		"/dav/files/a.txt":    "gadgets",
		"/dav":                "",
		"/api/gizmos":         "",
		"/":                   "",
		"":                    "",
	}
	for path, want := range cases {
		if got := r.Owner(path); got != want {
			t.Errorf("Owner(%q) = %q, want %q", path, got, want)
		}
	}
}
```

Run: `go vet ./davprefix/`
Expected: FAIL: `r.Owner undefined`.

In `core/server/davprefix/davprefix.go`, add this method between `Claim` and the comment `// ForApp returns the app's registry, creating it on first use.`:

```go
// Owner returns the slug that claimed the prefix a request path is under
// (the path is the prefix or continues it with "/"), the longest prefix
// first, or "" when no claimed prefix holds the path. Prefixes are claimed
// at registration, before the server takes requests, so a request reads the
// registry without a lock.
func (r *Registry) Owner(path string) string {
	best, slug := "", ""
	for prefix, owner := range r.claimed {
		if (path == prefix || strings.HasPrefix(path, prefix+"/")) && len(prefix) > len(best) {
			best, slug = prefix, owner
		}
	}
	return slug
}
```

(`strings` is already imported there.)

Run: `go test ./davprefix/`
Expected: `ok  	tinycld.org/core/davprefix`.

- [ ] **Step 2: Write the failing route test**

Create `core/server/pkgterms/routes_test.go`:

```go
package pkgterms_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/davprefix"
	"tinycld.org/core/pkgterms"
)

// A package mounts its routes at /api/<slug>/ and its DAV sources at the
// prefixes it claims. Both are refused while its terms are pending, before
// the route's own handler runs.
func TestAPendingPackageRefusesItsRoutes(t *testing.T) {
	app := newApp(t)
	pkgterms.Register(app)
	pkg := addPackage(t, app, "acme", "bundled", "2", "")
	addPackage(t, app, "plain", "bundled", "", "")
	if err := davprefix.ForApp(app).Claim("acme", "/acme-dav"); err != nil {
		t.Fatal(err)
	}
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		pong := func(re *core.RequestEvent) error { return re.String(http.StatusOK, "pong") }
		e.Router.GET("/api/acme/ping", pong)
		e.Router.GET("/api/acme", pong)
		e.Router.GET("/api/acmeplus/ping", pong)
		e.Router.GET("/api/plain/ping", pong)
		e.Router.GET("/acme-dav/{path...}", pong)
		e.Router.GET("/acme-davx/ping", pong)
		return e.Next()
	})
	owner := tokenOf(t, addUser(t, app, "owner@terms.test", "owner"))
	root := superuserToken(t, app)

	for _, url := range []string{"/api/acme/ping", "/api/acme", "/acme-dav/calendars/1"} {
		for name, token := range map[string]string{"the owner": owner, "a superuser": root, "signed out": ""} {
			t.Run(url+" "+name, func(t *testing.T) {
				call(t, app, http.MethodGet, url, token, "", http.StatusForbidden, acmeRefusal)
			})
		}
	}
	for _, url := range []string{"/api/acmeplus/ping", "/api/plain/ping", "/acme-davx/ping"} {
		call(t, app, http.MethodGet, url, "", "", http.StatusOK, "pong")
	}
	call(t, app, http.MethodGet, "/api/health", "", "", http.StatusOK, `"code":200`)

	pkg.Set(pkgterms.FieldAcceptedVersion, "2")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	call(t, app, http.MethodGet, "/api/acme/ping", "", "", http.StatusOK, "pong")
	call(t, app, http.MethodGet, "/acme-dav/calendars/1", "", "", http.StatusOK, "pong")
}
```

Run: `go test ./pkgterms/ -run TestAPendingPackageRefusesItsRoutes`
Expected: FAIL: the pending routes answer 200 `pong`, want 403.

- [ ] **Step 3: Write `core/server/pkgterms/routes.go`**

```go
package pkgterms

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/davprefix"
)

// routeSlug names the package a request path belongs to: the first segment
// under /api/ (a package mounts its routes at /api/<slug>/), or the package
// that claimed the DAV prefix the path is under. Blank for neither. A first
// segment that is no package (collections, realtime, search) is returned too:
// SlugError finds no pending package of that name.
func routeSlug(app core.App, path string) string {
	if rest, ok := strings.CutPrefix(path, "/api/"); ok {
		slug, _, _ := strings.Cut(rest, "/")
		return slug
	}
	return davprefix.ForApp(app).Owner(path)
}

// registerRoutes refuses the routes of a pending package. It is a middleware
// of the whole route tree, so it runs before the route's own handler and its
// auth: the refusal is the same for everybody, a superuser too (a package
// route is not a repair tool, and the middleware then needs no loaded auth).
func registerRoutes(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			if slug := routeSlug(re.App, re.Request.URL.Path); slug != "" {
				if err := SlugError(re.App, slug); err != nil {
					return Refusal(err)
				}
			}
			return re.Next()
		})
		return e.Next()
	})
}
```

In `core/server/pkgterms/decisions.go`, add the fourth line to `Register`:

```go
func Register(app core.App) {
	registerDecisions(app)
	registerRegistryGuards(app)
	registerGuards(app)
	registerSlugGuards(app)
	registerRoutes(app)
}
```

Run: `go vet ./pkgterms/ && go test ./pkgterms/`
Expected: `ok  	tinycld.org/core/pkgterms`.

- [ ] **Step 4: Update the golden hook count**

In `core/server/coreserver/composition_parity_test.go`, in `hostHookCounts`, replace the `"OnServe"` line's value `41` with `42` and its comment's start

```
// incl. the batch settings applied on every start, the classifier routes and GET /api/seats (all shared, via RegisterSharedCore);
```

with

```
// incl. the batch settings applied on every start, the classifier routes, GET /api/seats and the route guard of a package with pending terms (all shared, via RegisterSharedCore);
```

(the rest of the comment stays).

Run: `gofmt -l pkgterms davprefix coreserver`
Expected: no output.

Run: `go test ./coreserver/ -run 'TestRegisterBindsTheRecordedHandlerCounts|Composition|Parity'`
Expected: `ok  	tinycld.org/core/coreserver`.

- [ ] **Step 5: Commit**

```bash
git add core/server/pkgterms/routes.go core/server/pkgterms/routes_test.go core/server/pkgterms/decisions.go core/server/davprefix/davprefix.go core/server/davprefix/owner_test.go core/server/coreserver/composition_parity_test.go
git commit -m "feat(pkgterms): refuse the routes of a package with unaccepted terms"
```

---

### Task 6: Search and reports, and the docs of the enforcement

**Files:**
- Modify: `core/server/search/register.go` (imports; `handleSearch`)
- Create: `core/server/search/terms_test.go`
- Modify: `core/server/reports/stmt.go` (imports; `ruleRead`; `showsEveryRow`)
- Modify: `core/server/reports/endpoint.go` (imports; `runError`)
- Create: `core/server/reports/terms_test.go`
- Modify: `core/server/automation/endpoints.go` (imports; `dryRun`; the `/api/automation/dry-run` handler)
- Create: `core/server/automation/terms_test.go`
- Modify: `docs/packages.md` (new section after "Collection naming and package access enforcement")

**Interfaces:**
- Consumes: Task 4's `SlugError`, `CollectionError`, `PendingError`; in `search` tests `fakeSource(slug string, order int) Source`, `slugsOf`, `equalSlugs` (existing test helpers of the package); in `reports` tests `newZoo(t)`, `z.token(t, "ann")`, `z.post(t, name, token, body, status, parts...)`, `runBodyOf(feedingsByDay)`, `newSavedZoo(t)`, `CheckSaved(app)` (existing).
- Produces:
  - `/api/search` gives no results from a pending package.
  - A report run answers 403 with the `PendingError` message when its source, a union part, or a linked collection belongs to a pending package. The boot check (`CheckSaved`) is not changed.
  - `POST /api/automation/dry-run` answers the 403 when the trigger's collection belongs to a pending package (consumes `engineApp(t)` of the `automation` tests: app, engine, user, a `tickets` collection and the trigger `tickets:ticket-created`).

- [ ] **Step 1: Write the failing search test**

Create `core/server/search/terms_test.go`:

```go
package search

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/rlstest"
)

// A package whose terms the owner has not accepted gives no search results:
// its source is dropped before the fan-out, for every reader.
func TestSearchDropsTheSourcesOfAPendingPackage(t *testing.T) {
	app := rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))
	registry, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	add := func(slug, termsVersion, accepted string) *core.Record {
		pkg := core.NewRecord(registry)
		pkg.Set("name", slug)
		pkg.Set("slug", slug)
		pkg.Set("version", "1.0.0")
		pkg.Set("status", "bundled")
		pkg.Set("terms_version", termsVersion)
		pkg.Set("terms_accepted_version", accepted)
		if err := app.Save(pkg); err != nil {
			t.Fatal(err)
		}
		return pkg
	}
	gizmos := add("gizmos", "2", "")
	add("cogs", "1", "1")
	add("gadgets", "", "")
	all := []Source{fakeSource("gizmos", 5), fakeSource("cogs", 12), fakeSource("gadgets", 25)}

	got, err := withoutPendingTerms(app, all)
	if err != nil {
		t.Fatal(err)
	}
	if !equalSlugs(slugsOf(got), []string{"cogs", "gadgets"}) {
		t.Fatalf("searched %v, want [cogs gadgets]", slugsOf(got))
	}

	gizmos.Set("terms_accepted_version", "2")
	if err := app.Save(gizmos); err != nil {
		t.Fatal(err)
	}
	if got, err = withoutPendingTerms(app, all); err != nil || !equalSlugs(slugsOf(got), []string{"gizmos", "cogs", "gadgets"}) {
		t.Fatalf("after the acceptance searched %v (%v), want every source", slugsOf(got), err)
	}
}
```

Run: `go vet ./search/`
Expected: FAIL: `undefined: withoutPendingTerms`.

- [ ] **Step 2: Drop the sources of a pending package**

In `core/server/search/register.go`, add the import after `"tinycld.org/core/oauth"`:

```go
	"tinycld.org/core/pkgterms"
```

In `handleSearch`, replace:

```go
	sources := selectSources(RegisteredSources(), query.Slugs, granted)
	resp := Aggregate(re.Request.Context(), app, re.Auth.Id, query, sources)
	return re.JSON(http.StatusOK, resp)
}
```

with:

```go
	sources, err := withoutPendingTerms(app, selectSources(RegisteredSources(), query.Slugs, granted))
	if err != nil {
		return err
	}
	resp := Aggregate(re.Request.Context(), app, re.Auth.Id, query, sources)
	return re.JSON(http.StatusOK, resp)
}

// withoutPendingTerms drops the sources of a package whose terms the owner
// has not accepted: such a package gives no results to anybody. A source reads
// its package's tables in Go, past the record API where pkgterms refuses, so
// the search asks here. A registry that cannot be read fails the search: a
// failed read is not permission to search a package that may be pending.
func withoutPendingTerms(app core.App, sources []Source) ([]Source, error) {
	kept := make([]Source, 0, len(sources))
	for _, src := range sources {
		err := pkgterms.SlugError(app, src.Slug)
		var pending *pkgterms.PendingError
		if errors.As(err, &pending) {
			continue
		}
		if err != nil {
			return nil, err
		}
		kept = append(kept, src)
	}
	return kept, nil
}
```

(`errors` is already imported there.)

Run: `go vet ./search/ && go test ./search/`
Expected: `ok  	tinycld.org/core/search`.

- [ ] **Step 3: Write the failing report tests**

Create `core/server/reports/terms_test.go`:

```go
package reports

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// pendingPackage stores the registry row of a fictional package whose terms
// the owner has not accepted. The package owns the collections its slug
// prefixes (dashes as underscores): "zoo-animals" owns zoo_animals.
func pendingPackage(t *testing.T, app core.App, slug string) *core.Record {
	t.Helper()
	registry, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	pkg := core.NewRecord(registry)
	pkg.Set("name", slug)
	pkg.Set("slug", slug)
	pkg.Set("version", "1.0.0")
	pkg.Set("status", "bundled")
	pkg.Set("terms_version", "2")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	return pkg
}

// A report is one more way to read a package's rows, past the record API.
// It answers 403 when its source belongs to a pending package, and when a
// column only reaches such a package through a link.
func TestRunRefusesACollectionOfAPendingPackage(t *testing.T) {
	cases := []struct{ name, slug, refusal string }{
		{"the report's source", "zoo-feedings", "The owner has not accepted the terms of the zoo-feedings package."},
		{"a collection reached by a link", "zoo-animals", "The owner has not accepted the terms of the zoo-animals package."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			z := newZoo(t)
			Register(z.app)
			ann := z.token(t, "ann")
			pkg := pendingPackage(t, z.app, c.slug)
			z.post(t, "pending", ann, runBodyOf(feedingsByDay), http.StatusForbidden, c.refusal)

			pkg.Set("terms_accepted_version", "2")
			if err := z.app.Save(pkg); err != nil {
				t.Fatal(err)
			}
			z.post(t, "accepted", ann, runBodyOf(feedingsByDay), http.StatusOK, `"kind":"total"`)
		})
	}
}

// The boot check of the saved reports does not read as a reader, so a
// pending package does not mark its reports broken: they run again as soon
// as the owner accepts.
func TestCheckSavedDoesNotBreakTheReportsOfAPendingPackage(t *testing.T) {
	s := newSavedZoo(t)
	before, err := CheckSaved(s.app)
	if err != nil {
		t.Fatal(err)
	}
	pendingPackage(t, s.app, "zoo")
	after, err := CheckSaved(s.app)
	if err != nil {
		t.Fatal(err)
	}
	if before.Checked == 0 || after.Checked != before.Checked || len(after.Broken) != len(before.Broken) || after.Written != 0 {
		t.Fatalf("checked %d then %d, broken %d then %d, written %d", before.Checked, after.Checked, len(before.Broken), len(after.Broken), after.Written)
	}
}
```

Run: `go test ./reports/ -run 'TestRunRefusesACollectionOfAPendingPackage|TestCheckSavedDoesNotBreak'`
Expected: FAIL: `TestRunRefusesACollectionOfAPendingPackage` gets 200, want 403. `TestCheckSavedDoesNotBreakTheReportsOfAPendingPackage` passes now and must still pass after Step 4: it pins where the check must not go.

- [ ] **Step 4: Refuse in the report reader**

In `core/server/reports/stmt.go`, add the import as its own group:

```go
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgterms"
)
```

In `(*stmt).ruleRead`, replace:

```go
			return ruleRead{}, ruleRead{}, fmt.Errorf("reports: union source %q is not registered", key)
		}
		return unionForms(s.app, src, s.info, s.ask)
	}
	col, err := s.app.FindCachedCollectionByNameOrId(name)
	if err != nil {
		return ruleRead{}, ruleRead{}, fmt.Errorf("reports: collection %q: %w", name, err)
	}
	form, err := ruleFormOf(s.app, col, s.info)
```

with:

```go
			return ruleRead{}, ruleRead{}, fmt.Errorf("reports: union source %q is not registered", key)
		}
		for _, part := range src.Parts {
			if err := pkgterms.CollectionError(s.app, part.Collection); err != nil {
				return ruleRead{}, ruleRead{}, err
			}
		}
		return unionForms(s.app, src, s.info, s.ask)
	}
	col, err := s.app.FindCachedCollectionByNameOrId(name)
	if err != nil {
		return ruleRead{}, ruleRead{}, fmt.Errorf("reports: collection %q: %w", name, err)
	}
	// A reader's statement reads no collection of a package whose terms the
	// owner has not accepted. Here and not in ruleFormOf or lookupSource: the
	// boot check of the sources and of the saved reports (checkRule, bind)
	// must still pass, or a pending package would mark its reports broken.
	if err := pkgterms.CollectionError(s.app, col.Name); err != nil {
		return ruleRead{}, ruleRead{}, err
	}
	form, err := ruleFormOf(s.app, col, s.info)
```

In `(*stmt).showsEveryRow`, replace:

```go
	every := false
	if col.ListRule != nil {
```

with:

```go
	every := false
	// A collection of a package with unaccepted terms never drops its join:
	// the join reads the rule CTE, and ruleRead refuses there.
	if col.ListRule != nil && pkgterms.CollectionError(s.app, col.Name) == nil {
```

In `core/server/reports/endpoint.go`, add the import after `"tinycld.org/core/logging"`:

```go
	"tinycld.org/core/pkgterms"
```

and in `runError`, replace:

```go
	var specErr *SpecError
	switch {
	case errors.As(err, &specErr):
```

with:

```go
	var specErr *SpecError
	var pendingTerms *pkgterms.PendingError
	switch {
	case errors.As(err, &pendingTerms):
		return re.ForbiddenError(pendingTerms.Error(), nil)
	case errors.As(err, &specErr):
```

Run: `go vet ./reports/ && go test ./reports/ -run 'TestRunRefusesACollectionOfAPendingPackage|TestCheckSavedDoesNotBreak|TestRunEndpoint|TestCheckSaved'`
Expected: `ok  	tinycld.org/core/reports`.

Run: `gofmt -l search reports`
Expected: no output.

- [ ] **Step 5: Refuse in the automation dry run**

A dry run gives a signed-in user the id and summary of up to 50 records of the trigger's collection, read in Go on a core route.

Create `core/server/automation/terms_test.go`:

```go
package automation

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgterms"
)

// A dry run reads the records of a trigger's collection in Go. It reads
// none of a package whose terms the owner has not accepted.
func TestDryRunRefusesACollectionOfAPendingPackage(t *testing.T) {
	app, eng, u := engineApp(t)
	registry, err := app.FindCollectionByNameOrId(pkgterms.RegistryCollection)
	if err != nil {
		t.Fatal(err)
	}
	pkg := core.NewRecord(registry)
	pkg.Set("name", "Tickets")
	pkg.Set("slug", "tickets")
	pkg.Set("version", "1.0.0")
	pkg.Set("status", "bundled")
	pkg.Set(pkgterms.FieldTermsVersion, "2")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	tickets, _ := app.FindCollectionByNameOrId("tickets")
	ticket := core.NewRecord(tickets)
	ticket.Set("title", "urgent")
	ticket.Set("user", u.Id)
	if err := app.Save(ticket); err != nil {
		t.Fatal(err)
	}
	ast := ConditionsAST{Match: "all"}

	res, err := eng.dryRun(u, "tickets:ticket-created", nil, ast)
	var pending *pkgterms.PendingError
	if !errors.As(err, &pending) || pending.Slug != "tickets" || res.Total != 0 || len(res.Matches) != 0 {
		t.Fatalf("a dry run on a pending package: %v, total %d, matches %d", err, res.Total, len(res.Matches))
	}

	pkg.Set(pkgterms.FieldAcceptedVersion, "2")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	if res, err = eng.dryRun(u, "tickets:ticket-created", nil, ast); err != nil || res.Total != 1 {
		t.Fatalf("after the acceptance: %v, total %d", err, res.Total)
	}
}
```

Run: `go test ./automation/ -run TestDryRunRefusesACollectionOfAPendingPackage`
Expected: FAIL: `a dry run on a pending package: <nil>, total 1, matches 1`.

In `core/server/automation/endpoints.go`, replace the import block:

```go
import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)
```

with:

```go
import (
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgterms"
)
```

In `(*Engine).dryRun`, replace:

```go
	records, err := e.app.FindRecordsByFilter(trigger.Collection, filter, "-created", 50, 0, params)
```

with:

```go
	// A dry run reads the trigger's records in Go, past the record API where
	// pkgterms refuses a package whose terms the owner has not accepted.
	if err := pkgterms.CollectionError(e.app, trigger.Collection); err != nil {
		return out, err
	}
	records, err := e.app.FindRecordsByFilter(trigger.Collection, filter, "-created", 50, 0, params)
```

and in the `/api/automation/dry-run` handler, replace:

```go
			res, err := engine.dryRun(re.Auth, body.Trigger, body.TriggerConfig, body.Conditions)
			if err != nil {
```

with:

```go
			res, err := engine.dryRun(re.Auth, body.Trigger, body.TriggerConfig, body.Conditions)
			var pendingTerms *pkgterms.PendingError
			if errors.As(err, &pendingTerms) {
				return pkgterms.Refusal(err)
			}
			if err != nil {
```

Run: `gofmt -l automation; go vet ./automation/ && go test ./automation/ -run 'TestDryRun'`
Expected: `ok  	tinycld.org/core/automation`.

- [ ] **Step 6: Document the enforcement**

In `docs/packages.md`, add this section after the last paragraph of "### Collection naming and package access enforcement" and before "### Manifest variation across shipped packages":

````md
### Package terms (`terms`)

A package that declares `terms: { file: 'TERMS.md', version: '2026-11-01' }` cannot be used until the **owner** accepts that version. Core does all of it; the package ships only the markdown file and the version label.

- **State.** The boot sync writes the declared version to `pkg_registry.terms_version` and the text to the `pkg_terms` collection (one row per package, readable by every signed-in user). `pkg_registry.terms_accepted_version` is the version the owner accepted. The package is **pending** while `terms_version` is not blank and differs from `terms_accepted_version`.
- **Decision.** The owner creates a `pkg_terms_decisions` row (`pkg`, `terms_version`, `decision`: `accepted` or `declined`). The server records the user and the time, writes the row to the audit log, and on `accepted` sets `terms_accepted_version`. A decline changes nothing else: the package stays pending and the owner can accept later. Nobody edits or deletes a decision, and the owner cannot write the two registry fields directly.
- **A new version.** Change `terms.version` when the text changes. After the upgrade the package is pending again until the owner accepts the new version. A change of the text with the same `version` does not ask again.
- **What the server refuses while pending** (`core/server/pkgterms`), with a 403 `The owner has not accepted the terms of the <slug> package.`:
  - every record API request and file download on the package's collections (`<slug>` or `<slug>_*`), for everybody but a superuser, signed out too;
  - realtime subscriptions to those collections (the topics are dropped);
  - every route under `/api/<slug>/` and under a DAV prefix the package claimed, for everybody;
  - `pkgaccess.WriteError` / `CanWrite` (the write gate of protocol servers and automation actions);
  - an `expand` that reaches one of those collections from another collection;
  - `/api/search` results of the package, any report that reads one of its collections, and an automation dry run on one of its triggers.
- **Slugs.** While a package is pending, the owner cannot change its registry `slug`, and cannot give another registry row a slug that is or extends it (`acme` pending refuses a new `acme-notes`).
- **What it does not refuse.** A write with no request: a package's own Go code, a cron, a seed. A package that runs **its own protocol listener** (anything that is not the record API, an `/api/<slug>/` route or a claimed DAV prefix) must ask before it serves a client:

  ```go
  if err := pkgterms.SlugError(app, "<slug>"); err != nil {
      return err // *pkgterms.PendingError, or the registry read error
  }
  ```

- **No terms.** A package without `terms` has a blank `terms_version`, is never pending, and nothing above applies to it.
````

- [ ] **Step 7: Commit**

```bash
git add core/server/search/register.go core/server/search/terms_test.go core/server/reports/stmt.go core/server/reports/endpoint.go core/server/reports/terms_test.go core/server/automation/endpoints.go core/server/automation/terms_test.go docs/packages.md
git commit -m "feat(pkgterms): no search results and no reports from a package with unaccepted terms"
```

---

## For the driver

- Open for the human (rulings 13 and 14): a `filter` or `sort` on another collection that goes through a relation into a pending package is not refused (it shows no field, but a member can test a value); a package whose slug is a core route segment (`collections`, `realtime`) is not refused by the generator.
- NEEDS-RUN after Task 6: the full Go suites of `coreserver`, `reports`, `automation`, `pkgterms`, `pkgaccess`, `search`, `davprefix`, `rlstest` (the planner ran only the tests named in the steps), and `pnpm run checks` (it includes `check:core-isolation`).
- The planner compiled and ran the final state of all six tasks together. The states between tasks (`Register` with two, then three calls; the hook counts of Tasks 3 and 4) were derived from the final run, not run separately: when the parity test prints a different number at Task 3 or 4, the printed number is the one to record.
- Part 2 (tasks 7-14) is in the outline. It needs the human's answer on ruling 12 (a superuser write in e2e to reset the stub package's terms state) before its e2e tasks.
