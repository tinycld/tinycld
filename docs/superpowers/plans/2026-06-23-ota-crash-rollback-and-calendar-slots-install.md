# OTA Crash-Rollback E2E + Calendar-Slots Install Bug Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Execution status (2026-06-24):** Tasks 1–5, 8, 10, 11 COMPLETE (committed on `fix/all-deploy-fixes`, commits `d099bab`..`f741a3e`; vitest 7/7, tsc clean, biome clean, `bash -n` OK, Go `TestPkgMigrate_CalendarSlots` 2/2 pass).
> **Task 8 outcome:** both Go tests PASS — the unit-level `applyNamedMigrations` path is already SAFE (sorts ascending + atomic rollback; a dependent-before-create migration fails loudly, leaving no half-built schema). So the table-creation bug is NOT pure-ordering → **Task 9 (conditional) was correctly SKIPPED**; the boot-timing failure mode is covered by the Task 10 harness guard.
> **Task 5 note:** the existing dry-run installs `todo` via a hardcoded, todo-specific playwright spec, so a new self-contained parameterized spec `tests/install/calendar-slots-install.spec.ts` was created (human-chosen) rather than generalizing the todo spec. `PKG_SPEC` default `github:stefnnn/tinycld-calendar-slots` corroborated by `pkg_seed_test.go`.
> **Task 6 (real Docker + sim run) DEFERRED to the human** — it needs a Mac, Docker, a booted simulator, and a one-time manual `/connect`, and its observed outcome gates the conditional **Task 7** (capture-gap fix, only if the GENERIC `last_error` string reproduces). Prereqs verified present: Docker running, `IPHONE_SIMULATOR_UDID` in `../.env`, Xcode 26.5, instrumentation (`recordBundleError` + rollback-reason commits) at branch HEAD.

**Goal:** Build an OTA crash-rollback E2E harness on the iOS simulator, use it to reproduce the live bug where installing `@tinycld/calendar-slots` crashes the OTA bundle and rolls back, and fix the related bug where the booking tables are sometimes not created on install.

**Architecture:** Extend the existing happy-path OTA harness (`scripts/ota-e2e/`) with a crash-rollback assertion path that drives a REAL `calendar-slots` install (mints a real `build-<ts>-ios` OTA bundle on a server container), boots a Release sim, and asserts the device either updates healthily OR rolls back with a *captured reason* in `pkg_bad_bundle.last_error` — the detail the manual repro failed to surface. Then investigate/fix the install-time migration ordering that can leave booking tables uncreated.

**Tech Stack:** TypeScript (tsx scripts, vitest), Go (PocketBase `coreserver`), bash driver, iOS simulator (`expo run:ios --configuration Release`), Swift/Kotlin native module (`app-updater`), Docker (server image).

**Key background (verified during planning):**
- The happy-path harness lives in `scripts/ota-e2e/` (`run-ota-e2e.ts`, `run-ota-dry-run.sh`, `server-bundle.ts`, `logs-poller.ts`). Its README explicitly lists "crash-rollback + server reconcile" as **not covered**.
- Package UP migrations are NOT applied during install; they run on the **new binary's post-swap boot** (`migrate_sync.go:55-57`, "applied by the new binary on boot"). If that boot crashes or a migration throws before `RunAllMigrations` completes, **tables are not created AND the bundle rolls back** — likely one root cause behind both reported symptoms.
- `calendar-slots` migration `1800000001` calls `app.findCollectionByNameOrId('booking_pages')` and adds fields; if `1800000000` (which creates `booking_pages`) didn't fully apply, `1800000001` throws → migration sequence fails → boot fails → rollback → no booking tables.
- The device rollback detail flows: native `rollbackToPrevious(reason)` writes `error.json` → `takeRevertedBundle()` returns `{id,hash,error}` → `reportRevertedBundle()` POSTs to `/api/app/update/report-bad` → server `recordBadBundle` upserts `pkg_bad_bundle.last_error`.

---

## File Structure

**Harness (Phase 1):**
- `scripts/ota-e2e/bad-bundle-poller.ts` (NEW) — read `pkg_bad_bundle` rows + assert a captured `last_error`, and read `pkg_registry`/collection existence to assert rollback landed. Mirrors `logs-poller.ts` style (plain fetch + superuser token).
- `scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts` (NEW) — unit tests for the new pure functions (parsing, polling, predicate logic) with injected fetch.
- `scripts/ota-e2e/run-ota-crash-rollback.ts` (NEW) — the crash-rollback assertion runner (sibling to `run-ota-e2e.ts`): precheck a newer bundle, start polling for EITHER a healthy flip OR a `pkg_bad_bundle` row with a non-empty `last_error`, boot the Release sim, and classify the outcome.
- `scripts/ota-e2e/run-ota-crash-rollback.sh` (NEW) — bash driver mirroring `run-ota-dry-run.sh` but installing `calendar-slots` (not `todo`) and invoking the crash-rollback runner.
- `scripts/ota-e2e/README.md` (MODIFY) — document the new crash-rollback flow; move it out of "Not covered".
- `package.json` (MODIFY) — add `test:e2e:ota:rollback` script.

**Calendar-slots install bug (Phase 2):**
- `core/server/coreserver/pkg_migrate.go` (MODIFY, after investigation) — ensure package migrations apply atomically / fail loudly when a collection a later migration depends on is missing.
- `core/server/coreserver/pkg_migrate_test.go` (MODIFY) — add a Go test reproducing the dependent-migration-before-create failure.
- `scripts/ota-e2e/run-ota-crash-rollback.ts` (MODIFY) — add the "booking tables exist after a healthy install" assertion so the harness also guards the table-creation bug.

---

## PHASE 1 — Crash-Rollback OTA E2E Harness

### Task 1: Bad-bundle poller — pure parsing + predicate

**Files:**
- Create: `scripts/ota-e2e/bad-bundle-poller.ts`
- Test: `scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts
import { describe, expect, it, vi } from 'vitest'
import { extractBadBundles, pollForBadBundle, type BadBundleRow } from '../bad-bundle-poller'

describe('extractBadBundles', () => {
    it('returns rows with bundle_id, reports, and last_error', () => {
        const rows = extractBadBundles({
            items: [
                { bundle_id: 'build-1-ios', reports: 2, last_error: 'native rollback: crash-launch counter tripped (launches=2)' },
                { bundle_id: 'build-2-ios', reports: 1, last_error: '' },
            ],
        })
        expect(rows).toEqual([
            { bundleId: 'build-1-ios', reports: 2, lastError: 'native rollback: crash-launch counter tripped (launches=2)' },
            { bundleId: 'build-2-ios', reports: 1, lastError: '' },
        ])
    })

    it('tolerates a missing items array', () => {
        expect(extractBadBundles({})).toEqual([])
    })
})

describe('pollForBadBundle', () => {
    const noSleep = () => Promise.resolve()

    it('resolves with the row once the target bundle reports a non-empty last_error', async () => {
        const responses: BadBundleRow[][] = [
            [],
            [{ bundleId: 'build-9-ios', reports: 1, lastError: '' }],
            [{ bundleId: 'build-9-ios', reports: 1, lastError: 'native rollback: crash-launch counter tripped (launches=2)' }],
        ]
        const fetchRows = vi.fn(() => Promise.resolve(responses.shift() ?? []))
        const row = await pollForBadBundle({
            fetchRows,
            target: 'build-9-ios',
            timeoutMs: 10_000,
            intervalMs: 1_000,
            sleep: noSleep,
        })
        expect(row.lastError).toContain('crash-launch counter tripped')
    })

    it('rejects on timeout, surfacing the last-seen rows', async () => {
        const fetchRows = vi.fn(() => Promise.resolve([{ bundleId: 'build-9-ios', reports: 1, lastError: '' }]))
        await expect(
            pollForBadBundle({ fetchRows, target: 'build-9-ios', timeoutMs: 2_000, intervalMs: 1_000, sleep: noSleep })
        ).rejects.toThrow(/timed out.*build-9-ios/)
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/code/tinycld/tinycld && pnpm exec vitest run scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts`
Expected: FAIL with "Cannot find module '../bad-bundle-poller'".

- [ ] **Step 3: Write minimal implementation**

```ts
// scripts/ota-e2e/bad-bundle-poller.ts
// Reads the server's pkg_bad_bundle collection (the durable record of a device
// that crash-looped a bundle and rolled back) so the crash-rollback E2E can
// assert the rollback was REPORTED with a meaningful reason — not just that it
// happened. Mirrors logs-poller.ts: plain fetch + an injectable sleep for
// deterministic tests.

export interface BadBundleRow {
    bundleId: string
    reports: number
    lastError: string
}

interface BadBundleResponse {
    items?: Array<{ bundle_id?: string; reports?: number; last_error?: string }>
}

export function extractBadBundles(response: BadBundleResponse): BadBundleRow[] {
    return (response.items ?? []).map(it => ({
        bundleId: it.bundle_id ?? '',
        reports: it.reports ?? 0,
        lastError: it.last_error ?? '',
    }))
}

interface PollOpts {
    fetchRows: () => Promise<BadBundleRow[]>
    target: string
    timeoutMs: number
    intervalMs: number
    sleep?: (ms: number) => Promise<void>
    onPoll?: (rows: BadBundleRow[]) => void
}

const realSleep = (ms: number) => new Promise<void>(r => setTimeout(r, ms))

// Poll until the target bundle has a pkg_bad_bundle row with a NON-EMPTY
// last_error (the captured rollback reason / regex detail). A row with an empty
// last_error is the failure the manual repro hit — we treat it as "not yet
// captured" and keep waiting, then surface it in the timeout message.
export function pollForBadBundle(opts: PollOpts): Promise<BadBundleRow> {
    const { fetchRows, target, timeoutMs, intervalMs, sleep = realSleep, onPoll } = opts
    return new Promise<BadBundleRow>((resolve, reject) => {
        let lastSeen: BadBundleRow[] = []
        let elapsed = 0
        async function loop() {
            try {
                const rows = await fetchRows()
                lastSeen = rows
                onPoll?.(rows)
                const hit = rows.find(r => r.bundleId === target && r.lastError !== '')
                if (hit) {
                    resolve(hit)
                    return
                }
                elapsed += intervalMs
                if (elapsed >= timeoutMs) {
                    reject(
                        new Error(
                            `pollForBadBundle timed out after ${timeoutMs}ms waiting for a ` +
                                `pkg_bad_bundle row with bundle_id=${target} and a non-empty last_error; ` +
                                `last-seen=${JSON.stringify(lastSeen)}`
                        )
                    )
                    return
                }
                await sleep(intervalMs)
                await loop()
            } catch (err) {
                reject(err)
            }
        }
        void loop()
    })
}

// Fetch pkg_bad_bundle rows via the superuser API (same token flow as logs-poller).
export async function fetchBadBundles(serverUrl: string, token: string): Promise<BadBundleRow[]> {
    const res = await fetch(`${serverUrl}/api/collections/pkg_bad_bundle/records?sort=-updated&perPage=50`, {
        headers: { Authorization: token },
    })
    if (!res.ok) {
        throw new Error(`fetchBadBundles failed: ${res.status}`)
    }
    return extractBadBundles((await res.json()) as BadBundleResponse)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm exec vitest run scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add scripts/ota-e2e/bad-bundle-poller.ts scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts
git commit -m "test(ota-e2e): bad-bundle poller for crash-rollback assertion"
```

---

### Task 2: Collection-existence probe (booking tables + registry)

**Files:**
- Modify: `scripts/ota-e2e/bad-bundle-poller.ts` (add `collectionExists` + `registryStatus`)
- Test: `scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts` (add cases)

- [ ] **Step 1: Write the failing test** (append to the existing test file)

```ts
import { collectionExists } from '../bad-bundle-poller'

describe('collectionExists', () => {
    it('returns true on HTTP 200 (known collection)', async () => {
        const fetchFn = vi.fn(() => Promise.resolve({ status: 200, ok: true } as Response))
        expect(await collectionExists('http://x', 'tok', 'booking_pages', fetchFn)).toBe(true)
    })
    it('returns false on HTTP 404 (unknown collection)', async () => {
        const fetchFn = vi.fn(() => Promise.resolve({ status: 404, ok: false } as Response))
        expect(await collectionExists('http://x', 'tok', 'booking_pages', fetchFn)).toBe(false)
    })
    it('returns null on any other status (transient)', async () => {
        const fetchFn = vi.fn(() => Promise.resolve({ status: 503, ok: false } as Response))
        expect(await collectionExists('http://x', 'tok', 'booking_pages', fetchFn)).toBeNull()
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts`
Expected: FAIL with "collectionExists is not exported".

- [ ] **Step 3: Add the implementation** (append to `bad-bundle-poller.ts`)

```ts
// Existence probe for a collection: PocketBase returns 200 for a known
// collection's records endpoint (even when empty) and 404 for an unknown one.
// Any other status (transient mid-restart) returns null so the caller retries.
// Mirrors tests/install/todo-install.spec.ts collectionExists.
export async function collectionExists(
    serverUrl: string,
    token: string,
    name: string,
    fetchFn: typeof fetch = fetch
): Promise<boolean | null> {
    let res: Response
    try {
        res = await fetchFn(`${serverUrl}/api/collections/${name}/records?perPage=1`, {
            headers: { Authorization: token },
        })
    } catch {
        return null
    }
    if (res.status === 200) return true
    if (res.status === 404) return false
    return null
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm exec vitest run scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts`
Expected: PASS (7 tests total).

- [ ] **Step 5: Commit**

```bash
git add scripts/ota-e2e/bad-bundle-poller.ts scripts/ota-e2e/__tests__/bad-bundle-poller.test.ts
git commit -m "test(ota-e2e): collectionExists probe for booking-tables assertion"
```

---

### Task 3: Crash-rollback runner (outcome classifier)

**Files:**
- Create: `scripts/ota-e2e/run-ota-crash-rollback.ts`

This runner mirrors `run-ota-e2e.ts` but races TWO outcomes after boot: (a) healthy flip to the new bundle (via `pollForBundleId`), or (b) a reported rollback (via `pollForBadBundle`). It is invoked with `OTA_E2E_SKIP_BUILD=1` by the shell driver (Task 4), which handles the build/boot/connect.

- [ ] **Step 1: Write the runner** (no unit test — it is an orchestrator of already-tested pieces; the bash driver in Task 5 is the integration check)

```ts
// scripts/ota-e2e/run-ota-crash-rollback.ts
// Crash-rollback variant of run-ota-e2e.ts. After the Release sim boots on its
// embedded bundle and an OTA bundle is staged on the server, ONE of two things
// must happen, and we assert WHICH:
//   - HEALTHY: the app reloads into build-<ts>-ios and stays up (happy path).
//   - ROLLBACK: the app crash-loops the new bundle and reverts to embedded; the
//     server records a pkg_bad_bundle row whose last_error carries the captured
//     reason (native rollback reason / regex detail). An EMPTY last_error is a
//     FAILURE — it is exactly the gap the manual repro hit.
// Env knobs mirror run-ota-e2e.ts; OTA_E2E_EXPECT=healthy|rollback selects which
// outcome is the pass (default: rollback, the case we are chasing).
import { classifyBundleId, embeddedIdForVersion } from './identity'
import { fetchAppUpdateCurrentIds, pollForBundleId, superuserToken } from './logs-poller'
import { fetchBadBundles, pollForBadBundle } from './bad-bundle-poller'
import { precheckNewerBundle } from './server-bundle'
import { readFileSync } from 'node:fs'
import path from 'node:path'

const SERVER_URL = process.env.OTA_E2E_SERVER_URL ?? 'http://localhost:7200'
const SUPERUSER_EMAIL = process.env.OTA_E2E_SUPERUSER_EMAIL
const SUPERUSER_PASSWORD = process.env.OTA_E2E_SUPERUSER_PASSWORD
const EXPECT = (process.env.OTA_E2E_EXPECT ?? 'rollback') as 'healthy' | 'rollback'
const TIMEOUT_MS = Number(process.env.OTA_E2E_TIMEOUT_MS) || 240_000
const POLL_INTERVAL_MS = Number(process.env.OTA_E2E_POLL_INTERVAL_MS) || 3_000
const APP_DIR = path.resolve(import.meta.dirname, '..', '..')

function fail(msg: string): never {
    console.error(`\n[ota-rollback] FAIL: ${msg}\n`)
    process.exit(1)
}

function readAppVersion(): string {
    const raw = readFileSync(path.join(APP_DIR, 'app.json'), 'utf8')
    return (JSON.parse(raw) as { expo: { version: string } }).expo.version
}

async function main() {
    if (!SUPERUSER_EMAIL || !SUPERUSER_PASSWORD) {
        fail('OTA_E2E_SUPERUSER_EMAIL and OTA_E2E_SUPERUSER_PASSWORD must be set (see README).')
    }
    const appVersion = readAppVersion()
    const embeddedId = embeddedIdForVersion(appVersion)
    console.log(`[ota-rollback] app version ${appVersion} → embedded id ${embeddedId}; expecting ${EXPECT}`)

    const newId = await precheckNewerBundle({ serverUrl: SERVER_URL, runtimeVersion: appVersion, embeddedId })
    if (classifyBundleId(newId) !== 'server') fail(`Precheck returned unexpected bundle id: ${newId}`)
    console.log(`[ota-rollback] server offers new bundle ${newId}`)

    const token = await superuserToken(SERVER_URL, SUPERUSER_EMAIL, SUPERUSER_PASSWORD).catch(err =>
        fail(`superuser auth failed: ${(err as Error).message}`)
    )

    // Race the two terminal outcomes. Whichever resolves first decides the result;
    // we then check it against EXPECT.
    const healthy = pollForBundleId({
        fetchCurrentIds: () => fetchAppUpdateCurrentIds(SERVER_URL, token),
        target: newId,
        timeoutMs: TIMEOUT_MS,
        intervalMs: POLL_INTERVAL_MS,
    }).then(() => ({ kind: 'healthy' as const }))

    const rolledBack = pollForBadBundle({
        fetchRows: () => fetchBadBundles(SERVER_URL, token),
        target: newId,
        timeoutMs: TIMEOUT_MS,
        intervalMs: POLL_INTERVAL_MS,
        onPoll: rows => {
            const r = rows.find(x => x.bundleId === newId)
            if (r) console.log(`[ota-rollback]   pkg_bad_bundle[${newId}]: reports=${r.reports} last_error=${JSON.stringify(r.lastError)}`)
        },
    }).then(row => ({ kind: 'rollback' as const, row }))

    const outcome = await Promise.race([healthy, rolledBack]).catch(err => fail((err as Error).message))

    if (EXPECT === 'healthy') {
        if (outcome.kind === 'healthy') {
            console.log('\n[ota-rollback] PASS: app reloaded healthily into the new bundle.\n')
            process.exit(0)
        }
        fail(`expected a healthy update but the bundle rolled back: ${JSON.stringify(outcome)}`)
    } else {
        if (outcome.kind === 'rollback') {
            // The crux: the rollback must carry a CAPTURED reason, not the generic string.
            const generic = 'client rolled back: bundle failed to reach healthy'
            if (outcome.row.lastError === generic) {
                fail(
                    `rollback happened but last_error is the GENERIC string — the device did not ` +
                        `capture a reason (recordBundleError / RegExp shim / native rollback reason missing ` +
                        `from the build, or error.json was not written). last_error=${JSON.stringify(outcome.row.lastError)}`
                )
            }
            console.log(`\n[ota-rollback] PASS: rolled back with captured reason: ${outcome.row.lastError}\n`)
            process.exit(0)
        }
        fail('expected a crash-rollback but the bundle updated healthily (the crash did not reproduce).')
    }
}

main().catch(err => fail((err as Error).message))
```

- [ ] **Step 2: Typecheck the runner**

Run: `pnpm exec tsc --noEmit -p scripts/tsconfig.json 2>/dev/null || pnpm exec tinycld-pkg typecheck`
Expected: no errors referencing `run-ota-crash-rollback.ts`. (If `scripts/` is excluded from the app tsconfig, rely on the runtime `tsx` parse in Task 5.)

- [ ] **Step 3: Commit**

```bash
git add scripts/ota-e2e/run-ota-crash-rollback.ts
git commit -m "feat(ota-e2e): crash-rollback runner racing healthy-flip vs reported-rollback"
```

---

### Task 4: package.json script

**Files:**
- Modify: `tinycld/package.json` (scripts block)

- [ ] **Step 1: Add the script**

In `tinycld/package.json` `scripts`, directly after the existing `"test:e2e:ota": "tsx scripts/ota-e2e/run-ota-e2e.ts",` line, add:

```json
        "test:e2e:ota:rollback": "tsx scripts/ota-e2e/run-ota-crash-rollback.ts",
```

- [ ] **Step 2: Verify it parses**

Run: `node -e "require('./package.json').scripts['test:e2e:ota:rollback'] || process.exit(1)"`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add package.json
git commit -m "chore(ota-e2e): add test:e2e:ota:rollback script"
```

---

### Task 5: Bash driver — install calendar-slots + drive the rollback assertion

**Files:**
- Create: `scripts/ota-e2e/run-ota-crash-rollback.sh`

This mirrors `run-ota-dry-run.sh` but (a) installs `@tinycld/calendar-slots` instead of `todo`, and (b) invokes `run-ota-crash-rollback.ts`. Because `run-ota-dry-run.sh` is large and battle-tested, the driver SHELLS OUT to it for the container/build/boot/connect steps where possible, overriding only the install spec and the final assertion command.

- [ ] **Step 1: Inspect the reusable pieces of the existing driver**

Run: `grep -nE "run_phase|install @tinycld|test:e2e:ota|SKIP_INSTALL|precheck|SERVER_URL|ios-simulator|seed.*AsyncStorage|tinycld:server:app" scripts/ota-e2e/run-ota-dry-run.sh`
Expected: shows the install-spec invocation, the sim build/boot call, the AsyncStorage server-seed step, and the final assertion call. Note their exact forms — the new driver copies these verbatim, swapping the package slug and assertion script.

- [ ] **Step 2: Write the driver**

Create `scripts/ota-e2e/run-ota-crash-rollback.sh`. Copy the structure of `run-ota-dry-run.sh` (preamble, pre-flight, container build/boot, token scrape, sim build/boot, AsyncStorage seed) verbatim, making exactly these changes:

1. Header comment: describe the crash-rollback variant (installs calendar-slots, asserts rollback-with-captured-reason).
2. Where `run-ota-dry-run.sh` installs `@tinycld/todo`, install `@tinycld/calendar-slots` instead. Use the SAME install mechanism the dry-run uses (driving the install spec / the admin install endpoint). Set the package spec via a variable at the top:

```bash
PKG_SPEC="${PKG_SPEC:-github:stefnnn/tinycld-calendar-slots}"   # the bundle whose OTA crash we reproduce
```

3. Replace the final assertion line `pnpm run test:e2e:ota` with:

```bash
OTA_E2E_SERVER_URL="${SERVER_URL}" \
OTA_E2E_SUPERUSER_EMAIL="${ADMIN_USER_LOGIN}" \
OTA_E2E_SUPERUSER_PASSWORD="${ADMIN_USER_PW}" \
OTA_E2E_EXPECT="${OTA_E2E_EXPECT:-rollback}" \
OTA_E2E_SKIP_BUILD=1 \
pnpm run test:e2e:ota:rollback
```

4. Keep all env knobs (`IMAGE`, `KEEP`, `SKIP_INSTALL`, `SERVER_PORT`) identical so a `KEEP=1` container can be reused across runs.

- [ ] **Step 3: Make it executable**

Run: `chmod +x scripts/ota-e2e/run-ota-crash-rollback.sh`
Expected: no output.

- [ ] **Step 4: Shellcheck / dry parse**

Run: `bash -n scripts/ota-e2e/run-ota-crash-rollback.sh`
Expected: no syntax errors (exit 0).

- [ ] **Step 5: Commit**

```bash
git add scripts/ota-e2e/run-ota-crash-rollback.sh
git commit -m "feat(ota-e2e): bash driver installing calendar-slots + asserting rollback"
```

---

### Task 6: First real run — reproduce the bug

**Files:** none (execution + observation)

- [ ] **Step 1: Build a binary that DEFINITELY contains the instrumentation**

The crash-rollback assertion only works if the booted Release app contains the RegExp shim + native `recordBundleError` + rollback reasons. Confirm they are in the branch HEAD, then the driver's `ios-simulator.sh --prod` rebuilds from source (it runs `clean:native` → fresh prebuild → recompiles Swift).

Run: `git log --oneline -3 && grep -rl "recordBundleError" modules/app-updater/ios/AppUpdaterModule.swift`
Expected: HEAD includes the diag + rollback-reason commits; the symbol is present in source.

- [ ] **Step 2: Run the driver (real Docker + sim)**

Prereqs (from README): Docker running, `IPHONE_SIMULATOR_UDID` in `../.env`, Xcode, a one-time manual `/connect` to the test server on the sim.

Run:
```bash
cd ~/code/tinycld/tinycld
KEEP=1 OTA_E2E_EXPECT=rollback bash scripts/ota-e2e/run-ota-crash-rollback.sh
```
Expected: ONE of:
- **PASS** with `rolled back with captured reason: <detail>` — the harness reproduced the crash AND the instrumentation captured the reason. Record the exact `<detail>` (this names the bug). Proceed to Phase 2.
- **FAIL** `rollback happened but last_error is the GENERIC string` — reproduces your manual repro EXACTLY. This means `error.json` was not written; go to Step 3.
- **FAIL** `expected a crash-rollback but the bundle updated healthily` — the crash did not reproduce on the sim (env-dependent). Note it and proceed to Phase 2 anyway (the table bug is separate).

- [ ] **Step 3: If GENERIC string — inspect device + container state (diagnostic, not a fix yet)**

With `KEEP=1` the sim + container are still up.

Read the on-device updater state (the simulator's app container):
```bash
find ~/Library/Developer/CoreSimulator/Devices -path "*Application Support/app-updater/*.json" 2>/dev/null -exec sh -c 'echo "== $1 =="; cat "$1"' _ {} \;
```
Expected: shows `reverted.json` and whether `error.json` exists. If `reverted.json` exists but `error.json` does NOT, the bug is that `rollbackToPrevious` did not write it on this path — capture which path fired (check the container's `_logs` / the app console for the boot sequence). Record findings in the task; the fix lands in Phase 1.5 (Task 7).

- [ ] **Step 4: Commit findings (notes only)**

```bash
git commit --allow-empty -m "docs(ota-e2e): record first crash-rollback run outcome [details in commit body]"
```
(Put the observed `last_error` / file state in the commit body.)

---

### Task 7: (CONDITIONAL) Fix the capture gap if the GENERIC string reproduced

**Only do this task if Task 6 produced the generic string.** The likely cause: a rollback path writes `reverted.json` without a paired `error.json`, OR `error.json` is cleared before `takeRevertedBundle` reads it. The investigation in Task 6 Step 3 identifies which.

**Files:**
- Modify: `modules/app-updater/ios/AppUpdaterModule.swift`
- Modify: `modules/app-updater/android/src/main/java/org/tinycld/appupdater/AppUpdaterModule.kt`

- [ ] **Step 1: Reproduce the gap as a Swift-logic assertion in the harness**

Add a temporary assertion to `run-ota-crash-rollback.ts`: when the generic string is seen, also read (via the bash driver's `KEEP=1` container) the bundle id in `pkg_bad_bundle` and confirm `reports`. If `reports > 1`, the row predates this run (stale) — re-run with a fresh sim wipe to rule that out:

Run: `xcrun simctl uninstall "$IPHONE_SIMULATOR_UDID" org.tinycld.app` then re-run Task 6 Step 2.
Expected: a fresh `reports=1` row. If it STILL has the generic string with `reports=1`, the capture gap is real.

- [ ] **Step 2: Apply the fix indicated by the investigation**

Based on Task 6 Step 3 findings, the fix is one of:
  - If `error.json` is written but to a different `id` than `reverted.json`'s bundle: align them (write `error.json` with the SAME `curId` used for `reverted.json` — already the case in `rollbackToPrevious`; verify no other writer clobbers it).
  - If a non-`rollbackToPrevious` path produces the rollback: route it through `rollbackToPrevious(reason:)` so `error.json` is always co-written.
  - If `markHealthy`/`promotePendingIfAny` clears `error.json` before the reporting launch reads it: stop clearing it there (only `takeRevertedBundle` and `markHealthy` should clear, and only when appropriate).

Show the exact edit once the cause is known; do not guess here. (This step is intentionally cause-driven — the plan does not fabricate a fix for an unconfirmed cause.)

- [ ] **Step 3: Re-run Task 6 Step 2 to verify the captured reason now appears**

Expected: PASS with a non-generic `last_error`.

- [ ] **Step 4: Commit**

```bash
git add modules/app-updater/ios/AppUpdaterModule.swift modules/app-updater/android/src/main/java/org/tinycld/appupdater/AppUpdaterModule.kt
git commit -m "fix(app-updater): ensure every rollback path co-writes the crash detail"
```

---

## PHASE 2 — Calendar-Slots Install Bug (booking tables sometimes not created)

### Task 8: Reproduce the table-creation failure as a Go test

**Hypothesis (from planning):** package UP migrations apply on the new binary's post-swap boot (`migrate_sync.go`). `calendar-slots` migration `1800000001` depends on `booking_pages` existing (created by `1800000000`). 

IMPORTANT — verified during planning: `pkg_migrate.go:77` ALREADY applies migrations via `sortedCopy(files)` in ascending filename order, so plain "wrong order" is NOT the bug. The remaining candidates are: (a) a **partial apply** where `1800000000` fails partway (so `booking_pages` exists but is incomplete, or some of the 4 collections are missing) and the apply does NOT abort, leaving install reporting success with missing tables; (b) the apply is **interrupted by the boot crash** that Phase 1 reproduces (the migrations run on the SAME boot that crashes — strongly linking the two bugs); or (c) the JS executor **swallows a throw** from one migration so a later step proceeds against a half-built schema. Task 8 distinguishes these.

**Files:**
- Test: `core/server/coreserver/pkg_migrate_test.go`

- [ ] **Step 1: Read the existing migration-apply path + tests**

Run: `sed -n '1,80p' core/server/coreserver/pkg_migrate.go && grep -nE "func Test" core/server/coreserver/pkg_migrate_test.go`
Expected: shows how package JS migrations are discovered/ordered/applied and the existing test scaffolding (`newMigrateTestApp` helper, seen in `rebuild_test.go`).

- [ ] **Step 2: Write a failing test that applies the two calendar-slots migrations in order and asserts both collections exist**

```go
// In core/server/coreserver/pkg_migrate_test.go — add:
//
// Reproduces the reported "booking tables sometimes not created" bug: the two
// calendar-slots migrations MUST apply in filename order (create, then
// add-fields). If 1800000001 runs without 1800000000 having created
// booking_pages, findCollectionByNameOrId throws and the whole apply fails,
// leaving NO booking tables. This test pins the ordering + atomicity guarantee.
func TestPkgMigrate_CalendarSlots_OrderingCreatesAllTables(t *testing.T) {
    app := newMigrateTestApp(t)
    addCoreRelationTargets(t, app) // orgs + user_org collections the relations point at

    // Apply the package's migrations the same way install does, in filename order.
    files := []string{
        "1800000000_create_calendar-slots.js",
        "1800000001_calendar-slots-booking-config.js",
    }
    if err := applyPkgMigrationsInOrder(t, app, "calendar-slots", files); err != nil {
        t.Fatalf("calendar-slots migrations failed to apply: %v", err)
    }

    for _, name := range []string{"booking_pages", "booking_slot_types", "booking_availability", "bookings"} {
        if _, err := app.FindCollectionByNameOrId(name); err != nil {
            t.Fatalf("collection %s missing after install — booking tables not created: %v", name, err)
        }
    }
}
```

NOTE: `addCoreRelationTargets` and `applyPkgMigrationsInOrder` are test helpers. If equivalents already exist (check Step 1 output for `newMigrateTestApp`, `setRegistryRow`, `addPkgRegistryCollection`), reuse them; otherwise add minimal helpers in the same test file that (a) create stub `orgs`/`user_org` collections with the ids the migrations reference (`pbc_orgs_00001`, `pbc_user_org_01`), and (b) read each migration file from the calendar-slots checkout and run it through the same JS-migration executor the install path uses.

- [ ] **Step 3: Run the test to verify it fails (or passes — both are informative)**

Run: `cd core/server && go test ./coreserver/ -run TestPkgMigrate_CalendarSlots_OrderingCreatesAllTables -v`
Expected: EITHER it FAILS (reproduces the bug — proceed to Task 9) OR PASSES (the bug is NOT in pure ordering; it is timing/boot-crash-induced — proceed to Task 10, the harness-level guard, since the unit level cannot reproduce it).

- [ ] **Step 4: Commit**

```bash
git add core/server/coreserver/pkg_migrate_test.go
git commit -m "test(pkg-migrate): pin calendar-slots migration ordering creates all booking tables"
```

---

### Task 9: (CONDITIONAL) Fix migration ordering/atomicity if Task 8 failed

**Only if Task 8 reproduced the failure at the unit level.**

**Files:**
- Modify: `core/server/coreserver/pkg_migrate.go`

- [ ] **Step 1: Identify the exact defect from the test failure**

Likely candidates (confirm against the failure, do not guess). NOTE: ordering is already correct (`pkg_migrate.go:77` sorts), so do NOT add a sort — investigate these instead:
  - A migration failure not aborting the apply (partial apply leaves some of the 4 collections, the install still reports success).
  - The JS executor swallowing a `findCollectionByNameOrId` throw so a later migration proceeds against a half-built schema.
  - A field-add in `1800000000` failing silently so `booking_pages` exists but is missing fields a later step needs.

- [ ] **Step 2: Apply the minimal fix** (shown once the defect is confirmed; e.g. add a `sort.Strings(files)` before the apply loop, or wrap the apply in a transaction that rolls back on any migration error and propagates it so install fails loudly instead of silently leaving tables missing).

- [ ] **Step 3: Re-run the Task 8 test**

Run: `go test ./coreserver/ -run TestPkgMigrate_CalendarSlots_OrderingCreatesAllTables -v`
Expected: PASS.

- [ ] **Step 4: Run the full coreserver suite for regressions**

Run: `go test ./coreserver/ 2>&1 | tail -5`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add core/server/coreserver/pkg_migrate.go
git commit -m "fix(pkg-migrate): apply package migrations in order + abort install on failure"
```

---

### Task 10: Harness-level guard — booking tables exist after a healthy install

Whether or not Task 9 ran, add an end-to-end guard so the harness catches the table-creation bug in the real install path (which includes the boot-time apply the unit test cannot exercise).

**Files:**
- Modify: `scripts/ota-e2e/run-ota-crash-rollback.ts`

- [ ] **Step 1: Add the assertion (runs only on the healthy outcome)**

In `run-ota-crash-rollback.ts`, after a `healthy` outcome (and ALSO when run with `OTA_E2E_EXPECT=healthy`), assert the four booking collections exist via `collectionExists`:

```ts
import { collectionExists } from './bad-bundle-poller'

// After determining outcome.kind === 'healthy' (and before exit 0), verify the
// install actually created the package's schema — the "booking tables sometimes
// not created" bug surfaces as a healthy install with MISSING collections.
async function assertBookingTables(serverUrl: string, token: string): Promise<void> {
    const required = ['booking_pages', 'booking_slot_types', 'booking_availability', 'bookings']
    for (const name of required) {
        // Tolerate a brief post-restart window: retry a few times before failing.
        let exists: boolean | null = null
        for (let i = 0; i < 10; i++) {
            exists = await collectionExists(serverUrl, token, name)
            if (exists === true) break
            await new Promise(r => setTimeout(r, 2_000))
        }
        if (exists !== true) {
            fail(`booking table '${name}' was NOT created by the calendar-slots install (exists=${exists}) — the table-creation bug reproduced.`)
        }
    }
    console.log('[ota-rollback] booking tables present: ' + required.join(', '))
}
```

Call `await assertBookingTables(SERVER_URL, token)` in the `EXPECT === 'healthy'` + `outcome.kind === 'healthy'` branch before `process.exit(0)`.

- [ ] **Step 2: Run the healthy variant against a server where calendar-slots installed cleanly**

Run:
```bash
KEEP=1 OTA_E2E_EXPECT=healthy bash scripts/ota-e2e/run-ota-crash-rollback.sh
```
Expected: PASS with "booking tables present" — OR a clear FAIL naming the missing table (reproduces the bug end-to-end).

- [ ] **Step 3: Commit**

```bash
git add scripts/ota-e2e/run-ota-crash-rollback.ts
git commit -m "test(ota-e2e): assert calendar-slots install creates all booking tables"
```

---

### Task 11: Documentation

**Files:**
- Modify: `scripts/ota-e2e/README.md`

- [ ] **Step 1: Document the new flow**

In `scripts/ota-e2e/README.md`:
  - Add a section "Crash-rollback E2E (`test:e2e:ota:rollback`)" describing: it installs calendar-slots, boots the Release sim, and asserts EITHER a healthy flip (with booking tables present) OR a rollback with a captured `last_error`. Document `OTA_E2E_EXPECT=healthy|rollback` and `PKG_SPEC`.
  - In the "Not covered (future work)" list, REMOVE "crash-rollback + server reconcile" (now covered) and keep the rest (Android, on-screen UI assertions).

- [ ] **Step 2: Commit**

```bash
git add scripts/ota-e2e/README.md
git commit -m "docs(ota-e2e): document the crash-rollback + booking-tables e2e"
```

---

## Self-Review notes

- **Spec coverage:** Phase 1 (crash-rollback harness) = Tasks 1–7; Phase 2 (calendar-slots table bug) = Tasks 8–10; docs = Task 11. The "capture the reason" gap from the manual repro is asserted in Task 3 (generic-string → FAIL) and fixed conditionally in Task 7. The table bug is reproduced at unit (Task 8) and e2e (Task 10) levels.
- **Conditional tasks** (7, 9) are explicitly cause-driven and do NOT fabricate a fix for an unconfirmed root cause — they require the reproduction's findings first. This is deliberate: the investigation is part of the work, per the brief.
- **Type consistency:** `BadBundleRow` ({bundleId, reports, lastError}) is defined in Task 1 and reused in Tasks 3/10; `collectionExists(serverUrl, token, name, fetchFn?)` defined in Task 2 and used in Task 10; `pollForBadBundle`/`fetchBadBundles` defined in Task 1 and used in Task 3.
- **Known unknowns surfaced, not hidden:** the exact install mechanism in `run-ota-dry-run.sh` (Task 5 Step 1 inspects it rather than assuming), and whether the table bug is pure-ordering vs boot-timing (Task 8 Step 3 branches on the result).
