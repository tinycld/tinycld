# OTA Update-Is-Live Assertion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove that after an OTA update the new bundle's JS actually executed, the real provider tree mounted, and a marker carrying the running bundle id is present on screen — not merely that the native `currentId` flipped.

**Architecture:** An always-on (production-included, web no-op) `BundleSentinel` component in the app shell logs a distinctive `app-boot: rendered` line to the console and renders an a11y-visible sentinel carrying the running bundle id, mounted inside `<Providers>` next to the existing `MarkBundleHealthy` (so it fires only on the real tree's commit). A scripted harness adds two pure-parser + injectable-IO modules — a `simctl log show` boot-log scraper and an `idb ui describe-all` a11y-sentinel reader — wired into the crash-rollback runner's healthy path. `idb` is an optional dependency: absent ⇒ skip-with-log, never a false fail.

**Tech Stack:** TypeScript (React Native / Expo app shell; tsx scripts; vitest), `xcrun simctl log show`, Facebook `idb` (`idb ui describe-all --json`), bash driver.

**Spec:** `docs/superpowers/specs/2026-06-24-ota-update-visible-assertion-design.md`

**Key facts (verified during planning):**
- The OTA path runs only in a **Release** build, so the boot-log signal must NOT be `__DEV__`-gated (it would be compiled out). It must, however, no-op on **web** (the `app-updater` module is stubbed there: `getCurrentBundleId()` returns `'web'`).
- `AppUpdater` is imported as `import AppUpdater from 'app-updater'`; it exposes `getCurrentBundleId(): string` and `getCurrentBundleHash(): string`. The web stub (`modules/app-updater/index.web.ts`) returns `'web'` / `''`.
- The component to mirror is `core/lib/mark-bundle-healthy.ts` + `lib/use-mark-bundle-healthy.tsx`; `<MarkBundleHealthy />` is mounted inside `<Providers>` on the `resolved` branch of `app/_layout.tsx` (the only place that proves the real tree committed, not the blank gate).
- `simctl` has NO accessibility query; `idb` (`~/.local/bin/idb`, `idb_companion` via brew) returns the a11y tree. `idb ui describe-all --json` emits a flat list of elements; field keys may appear as `AXLabel`/`AXIdentifier` or normalized `label`/`type` depending on idb version — parse defensively.
- The harness pollers to mirror for style (pure parser + injectable IO + `Authorization: token` conventions): `scripts/ota-e2e/bad-bundle-poller.ts`, `scripts/ota-e2e/logs-poller.ts`.

---

## File Structure

**App shell (core):**
- `core/lib/bundle-sentinel.tsx` (NEW) — `formatSentinelLabel`/`bootLogLine` pure helpers + `useBundleSentinel` hook + `BundleSentinel` component.
- `core/tests/unit/bundle-sentinel.test.tsx` (NEW) — unit tests for the pure helpers.
- `app/_layout.tsx` (MODIFY) — mount `<BundleSentinel />` after `<MarkBundleHealthy />` inside `<Providers>`.

**Harness:**
- `scripts/ota-e2e/boot-log-scraper.ts` (NEW) — `extractBootBundleId` (pure) + `scrapeBootBundleId` (IO).
- `scripts/ota-e2e/__tests__/boot-log-scraper.test.ts` (NEW).
- `scripts/ota-e2e/a11y-sentinel.ts` (NEW) — `findSentinelBundleId` (pure) + `queryA11ySentinel` (IO, idb-optional).
- `scripts/ota-e2e/__tests__/a11y-sentinel.test.ts` (NEW).
- `scripts/ota-e2e/run-ota-crash-rollback.ts` (MODIFY) — add `assertUpdateIsLive` on the healthy path.
- `scripts/ota-e2e/README.md` (MODIFY) — document the assertion + the optional `idb` dependency.
- `scripts/ota-e2e/run-ota-e2e.ts` (MODIFY, OPTIONAL/stretch — Task 8) — same assertion on the happy path.

---

## Task 1: Bundle-sentinel pure helpers

**Files:**
- Create: `core/lib/bundle-sentinel.tsx`
- Test: `core/tests/unit/bundle-sentinel.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// core/tests/unit/bundle-sentinel.test.tsx
import { describe, expect, it } from 'vitest'
import { bootLogLine, formatSentinelLabel, shortHash } from '@tinycld/core/lib/bundle-sentinel'

describe('shortHash', () => {
    it('takes the first 12 chars', () => {
        expect(shortHash('abcdef0123456789aaaa')).toBe('abcdef012345')
    })
    it('returns empty for an empty hash', () => {
        expect(shortHash('')).toBe('')
    })
})

describe('formatSentinelLabel', () => {
    it('prefixes the bundle id with bundle:', () => {
        expect(formatSentinelLabel('build-123-ios')).toBe('bundle:build-123-ios')
    })
})

describe('bootLogLine', () => {
    it('emits the stable, scrapeable boot line with id and short hash', () => {
        expect(bootLogLine('build-123-ios', 'abcdef0123456789')).toBe(
            '[tinycld] app-boot: rendered bundle id=build-123-ios hash=abcdef012345'
        )
    })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd ~/code/tinycld/tinycld && pnpm exec vitest run core/tests/unit/bundle-sentinel.test.tsx`
Expected: FAIL with "Cannot find module '@tinycld/core/lib/bundle-sentinel'" (or "does not provide an export").

- [ ] **Step 3: Write the pure helpers**

```tsx
// core/lib/bundle-sentinel.tsx
// BundleSentinel emits, on every real-tree mount (production included), a proof
// that the running bundle's JS actually executed and rendered — closing the gap
// where the native currentId flip alone does not prove the new JS ran/rendered.
// It logs a distinctive, scrapeable console line AND renders an accessibility-
// visible (visually negligible) element carrying the running bundle id, so an
// external harness can assert BOTH "JS executed + mounted" (console) and "on
// screen" (a11y tree). Mounted next to MarkBundleHealthy inside <Providers> so it
// fires only when the real provider tree commits, not the blank gate placeholder.
//
// NOT __DEV__-gated: the OTA path runs only in Release builds, where __DEV__ code
// is stripped. It DOES no-op on web (the app-updater module is stubbed there).

// First 12 chars of the bundle hash — enough to disambiguate; the full hash is
// unnecessary on screen / in the log.
export function shortHash(hash: string): string {
    return hash.slice(0, 12)
}

// The accessibilityLabel the harness reads back from the a11y tree.
export function formatSentinelLabel(bundleId: string): string {
    return `bundle:${bundleId}`
}

// The single console line the boot-log scraper greps for. Stable by contract:
// scripts/ota-e2e/boot-log-scraper.ts parses exactly this shape.
export function bootLogLine(bundleId: string, hash: string): string {
    return `[tinycld] app-boot: rendered bundle id=${bundleId} hash=${shortHash(hash)}`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `pnpm exec vitest run core/tests/unit/bundle-sentinel.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add core/lib/bundle-sentinel.tsx core/tests/unit/bundle-sentinel.test.tsx
git commit -m "feat(app-shell): bundle-sentinel pure helpers (boot-log line + a11y label)"
```

---

## Task 2: Bundle-sentinel hook + component

**Files:**
- Modify: `core/lib/bundle-sentinel.tsx`

This adds the React hook (console log on mount) and the component (renders the a11y sentinel). No unit test for the rendering itself — it is an integration concern verified by the harness; the pure helpers it uses are already tested. The hook's console call is exercised indirectly.

- [ ] **Step 1: Append the hook + component**

Append to `core/lib/bundle-sentinel.tsx`:

```tsx
import AppUpdater from 'app-updater'
import { useEffect } from 'react'
import { Platform, Text, View } from 'react-native'

// Logs the boot line exactly once per mount. Production-included (no __DEV__
// guard); no-ops on web where the native updater is stubbed and there is no OTA.
export function useBundleSentinel(): void {
    useEffect(() => {
        if (Platform.OS === 'web') return
        // console.log (not console.debug): `simctl log show` captures default
        // os_log level; debug is filtered out unless verbose logging is enabled.
        console.log(bootLogLine(AppUpdater.getCurrentBundleId(), AppUpdater.getCurrentBundleHash()))
    }, [])
}

// BundleSentinel logs the boot proof and renders a visually-negligible,
// accessibility-visible element carrying the running bundle id, so a harness can
// assert the update is live on screen. Renders nothing meaningful on web.
export function BundleSentinel(): React.JSX.Element | null {
    useBundleSentinel()
    if (Platform.OS === 'web') return null
    const label = formatSentinelLabel(AppUpdater.getCurrentBundleId())
    // opacity:0 keeps it out of the visible UI while remaining in the iOS a11y
    // tree; pointerEvents:none so it never intercepts touches. If a future iOS
    // prunes fully-transparent nodes, bump opacity to 0.01.
    return (
        <View
            accessibilityIdentifier="ota-bundle-sentinel"
            accessibilityLabel={label}
            pointerEvents="none"
            style={{ position: 'absolute', top: 0, left: 0, width: 1, height: 1, opacity: 0 }}
        >
            <Text accessibilityLabel={label}>{label}</Text>
        </View>
    )
}
```

- [ ] **Step 2: Typecheck**

Run: `pnpm exec tsc --noEmit`
Expected: exit 0, no errors referencing `bundle-sentinel.tsx`. (If `React.JSX.Element` is not in scope, add `import type React from 'react'` — but with the project's react-native types `React.JSX.Element` resolves under the automatic JSX runtime; verify before changing.)

- [ ] **Step 3: Lint**

Run: `pnpm exec biome check core/lib/bundle-sentinel.tsx`
Expected: clean (fix any formatting at the source; no biome-ignore).

- [ ] **Step 4: Re-run the helper tests (no regressions)**

Run: `pnpm exec vitest run core/tests/unit/bundle-sentinel.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add core/lib/bundle-sentinel.tsx
git commit -m "feat(app-shell): BundleSentinel component (boot-log + a11y sentinel)"
```

---

## Task 3: Mount BundleSentinel in the app shell

**Files:**
- Modify: `app/_layout.tsx`

- [ ] **Step 1: Add the import**

In `app/_layout.tsx`, alongside the existing core component imports (near the `NewVersionToast` / `AppErrorBoundary` imports), add:

```tsx
import { BundleSentinel } from '@tinycld/core/lib/bundle-sentinel'
```

- [ ] **Step 2: Mount it inside `<Providers>`, after `<MarkBundleHealthy />`**

In the `resolved` return branch, change:

```tsx
    return (
        <Providers>
            <MarkBundleHealthy />
            <Slot />
            <NewVersionToast />
        </Providers>
    )
```

to:

```tsx
    return (
        <Providers>
            <MarkBundleHealthy />
            <BundleSentinel />
            <Slot />
            <NewVersionToast />
        </Providers>
    )
```

- [ ] **Step 3: Typecheck**

Run: `pnpm exec tsc --noEmit`
Expected: exit 0.

- [ ] **Step 4: Lint**

Run: `pnpm exec biome check app/_layout.tsx`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add app/_layout.tsx
git commit -m "feat(app-shell): mount BundleSentinel in the resolved provider tree"
```

---

## Task 4: Boot-log scraper — pure parser

**Files:**
- Create: `scripts/ota-e2e/boot-log-scraper.ts`
- Test: `scripts/ota-e2e/__tests__/boot-log-scraper.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// scripts/ota-e2e/__tests__/boot-log-scraper.test.ts
import { describe, expect, it, vi } from 'vitest'
import { extractBootBundleId, scrapeBootBundleId } from '../boot-log-scraper'

describe('extractBootBundleId', () => {
    it('returns the bundle id from a boot line', () => {
        const log = 'foo\n[tinycld] app-boot: rendered bundle id=build-123-ios hash=abcdef012345\nbar'
        expect(extractBootBundleId(log)).toBe('build-123-ios')
    })

    it('returns the LAST id when multiple boot lines are present', () => {
        const log =
            '[tinycld] app-boot: rendered bundle id=embedded-1.13.7 hash=aaaa\n' +
            '[tinycld] app-boot: rendered bundle id=build-999-ios hash=bbbb\n'
        expect(extractBootBundleId(log)).toBe('build-999-ios')
    })

    it('returns null when no boot line is present', () => {
        expect(extractBootBundleId('nothing to see here')).toBeNull()
    })
})

describe('scrapeBootBundleId', () => {
    it('feeds simctl log output through the parser', async () => {
        const spawnFn = vi.fn(() =>
            Promise.resolve('[tinycld] app-boot: rendered bundle id=build-7-ios hash=cafebabe1234')
        )
        const id = await scrapeBootBundleId('UDID-1', 120, spawnFn)
        expect(id).toBe('build-7-ios')
        expect(spawnFn).toHaveBeenCalledOnce()
    })

    it('returns null when the spawn yields no boot line', async () => {
        const spawnFn = vi.fn(() => Promise.resolve('some unrelated log output'))
        expect(await scrapeBootBundleId('UDID-1', 120, spawnFn)).toBeNull()
    })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm exec vitest run scripts/ota-e2e/__tests__/boot-log-scraper.test.ts`
Expected: FAIL with "Cannot find module '../boot-log-scraper'".

- [ ] **Step 3: Write the implementation**

```ts
// scripts/ota-e2e/boot-log-scraper.ts
// Reads the device console for the BundleSentinel boot line
// (core/lib/bundle-sentinel.tsx -> bootLogLine), which the app logs ONLY after
// the real provider tree commits. Finding it with the new build-<ts>-ios id is
// proof the new bundle's JS executed and mounted — stronger than the native
// currentId flip. Pure parser + an injectable spawn for deterministic tests,
// mirroring logs-poller.ts / bad-bundle-poller.ts.

import { spawn } from 'node:child_process'

// Parse the LAST `[tinycld] app-boot: rendered bundle id=<id> hash=<...>` line's
// id from a console blob (most recent boot wins). null when absent.
export function extractBootBundleId(logText: string): string | null {
    const re = /\[tinycld\] app-boot: rendered bundle id=(\S+) hash=/g
    let last: string | null = null
    for (const m of logText.matchAll(re)) {
        last = m[1]
    }
    return last
}

type SpawnFn = (udid: string, sinceSeconds: number) => Promise<string>

// Default spawn: `simctl log show` filtered to the boot line over the last N
// seconds, compact style. log show (not `log stream`) returns and exits, which is
// what we want for a one-shot scrape.
const realSpawn: SpawnFn = (udid, sinceSeconds) =>
    new Promise<string>((resolve, reject) => {
        const child = spawn('xcrun', [
            'simctl',
            'spawn',
            udid,
            'log',
            'show',
            '--predicate',
            'eventMessage CONTAINS "app-boot: rendered"',
            '--last',
            `${sinceSeconds}s`,
            '--style',
            'compact',
        ])
        let out = ''
        let err = ''
        child.stdout.on('data', d => {
            out += d
        })
        child.stderr.on('data', d => {
            err += d
        })
        child.on('error', reject)
        child.on('close', code => {
            // log show exits non-zero on some transient conditions; surface stderr
            // but still resolve with whatever stdout we captured so the caller's
            // retry loop can decide (an empty string parses to null).
            if (code !== 0 && out === '') reject(new Error(`simctl log show exited ${code}: ${err}`))
            else resolve(out)
        })
    })

// Scrape once and parse. Returns the boot bundle id, or null if no boot line was
// captured in the window. The caller retries.
export async function scrapeBootBundleId(
    udid: string,
    sinceSeconds: number,
    spawnFn: SpawnFn = realSpawn
): Promise<string | null> {
    const out = await spawnFn(udid, sinceSeconds)
    return extractBootBundleId(out)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `pnpm exec vitest run scripts/ota-e2e/__tests__/boot-log-scraper.test.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: Lint + typecheck**

Run: `pnpm exec biome check scripts/ota-e2e/boot-log-scraper.ts scripts/ota-e2e/__tests__/boot-log-scraper.test.ts && pnpm exec tsc --noEmit`
Expected: biome clean; tsc exit 0.

- [ ] **Step 6: Commit**

```bash
git add scripts/ota-e2e/boot-log-scraper.ts scripts/ota-e2e/__tests__/boot-log-scraper.test.ts
git commit -m "test(ota-e2e): boot-log scraper for the app-boot rendered line"
```

---

## Task 5: A11y-sentinel reader — pure parser + idb-optional IO

**Files:**
- Create: `scripts/ota-e2e/a11y-sentinel.ts`
- Test: `scripts/ota-e2e/__tests__/a11y-sentinel.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// scripts/ota-e2e/__tests__/a11y-sentinel.test.ts
import { describe, expect, it, vi } from 'vitest'
import { findSentinelBundleId, queryA11ySentinel } from '../a11y-sentinel'

// idb ui describe-all --json emits a flat array of element objects. Field keys
// vary by idb version, so the parser tolerates AXIdentifier/AXLabel and the
// normalized identifier/label spellings.
const treeAX = [
    { AXIdentifier: 'some-button', AXLabel: 'Tap me' },
    { AXIdentifier: 'ota-bundle-sentinel', AXLabel: 'bundle:build-55-ios' },
]
const treeNormalized = [
    { identifier: 'ota-bundle-sentinel', label: 'bundle:build-77-ios' },
]

describe('findSentinelBundleId', () => {
    it('finds the sentinel by AXIdentifier and parses bundle:<id>', () => {
        expect(findSentinelBundleId(treeAX)).toBe('build-55-ios')
    })
    it('tolerates the normalized identifier/label spelling', () => {
        expect(findSentinelBundleId(treeNormalized)).toBe('build-77-ios')
    })
    it('returns null when no sentinel element is present', () => {
        expect(findSentinelBundleId([{ AXIdentifier: 'x', AXLabel: 'y' }])).toBeNull()
    })
    it('returns null on a non-array (defensive)', () => {
        expect(findSentinelBundleId({})).toBeNull()
        expect(findSentinelBundleId(null)).toBeNull()
    })
})

describe('queryA11ySentinel', () => {
    it('returns the sentinel id from the idb runner output', async () => {
        const runner = vi.fn(() => Promise.resolve(JSON.stringify(treeAX)))
        expect(await queryA11ySentinel('UDID-1', runner)).toBe('build-55-ios')
    })
    it('returns null (skip) when the runner reports idb is unavailable', async () => {
        const runner = vi.fn(() => Promise.resolve(null))
        expect(await queryA11ySentinel('UDID-1', runner)).toBeNull()
    })
    it('returns null when the output is not valid JSON', async () => {
        const runner = vi.fn(() => Promise.resolve('not json'))
        expect(await queryA11ySentinel('UDID-1', runner)).toBeNull()
    })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm exec vitest run scripts/ota-e2e/__tests__/a11y-sentinel.test.ts`
Expected: FAIL with "Cannot find module '../a11y-sentinel'".

- [ ] **Step 3: Write the implementation**

```ts
// scripts/ota-e2e/a11y-sentinel.ts
// Reads the iOS accessibility tree via `idb ui describe-all --json` and extracts
// the BundleSentinel element's bundle id (accessibilityIdentifier
// "ota-bundle-sentinel", label "bundle:<id>"). Asserting this id == the new
// build-<ts>-ios proves the update is live ON SCREEN, not just executed.
//
// idb is an OPTIONAL dependency: when it is not installed, queryA11ySentinel
// returns null (skip), so the harness still runs on a machine without idb. Pure
// parser + injectable runner, mirroring the other ota-e2e modules.

import { spawn } from 'node:child_process'

interface A11yElement {
    AXIdentifier?: string
    identifier?: string
    AXLabel?: string
    label?: string
}

function elementId(el: A11yElement): string | undefined {
    return el.AXIdentifier ?? el.identifier
}
function elementLabel(el: A11yElement): string | undefined {
    return el.AXLabel ?? el.label
}

// Walk the flat element list, find ota-bundle-sentinel, parse bundle:<id> out of
// its label. Defensive against non-array input and missing fields.
export function findSentinelBundleId(tree: unknown): string | null {
    if (!Array.isArray(tree)) return null
    for (const el of tree as A11yElement[]) {
        if (elementId(el) === 'ota-bundle-sentinel') {
            const label = elementLabel(el) ?? ''
            const m = /^bundle:(.+)$/.exec(label)
            if (m) return m[1]
        }
    }
    return null
}

// Returns the raw `idb ui describe-all --json` stdout, or null if idb is not
// installed / not runnable (the optional-dependency skip path).
type IdbRunner = (udid: string) => Promise<string | null>

const realRunner: IdbRunner = udid =>
    new Promise<string | null>(resolve => {
        const child = spawn('idb', ['ui', 'describe-all', '--udid', udid, '--json'])
        let out = ''
        child.stdout.on('data', d => {
            out += d
        })
        // ENOENT (idb absent) or any spawn error -> skip (null), never throw.
        child.on('error', () => resolve(null))
        child.on('close', code => resolve(code === 0 ? out : null))
    })

// Query the a11y tree and extract the sentinel id. null means EITHER idb is
// unavailable OR the sentinel is not (yet) present — the caller distinguishes by
// retrying and, on a persistent null, logging a skip vs failing per the idb
// availability it was told about.
export async function queryA11ySentinel(
    udid: string,
    runner: IdbRunner = realRunner
): Promise<string | null> {
    const out = await runner(udid)
    if (out === null) return null
    try {
        return findSentinelBundleId(JSON.parse(out))
    } catch {
        return null
    }
}

// Whether idb is on PATH — lets the runner decide skip (absent) vs fail (present
// but no sentinel). Spawns `idb --help` once.
export function idbAvailable(): Promise<boolean> {
    return new Promise<boolean>(resolve => {
        const child = spawn('idb', ['--help'])
        child.on('error', () => resolve(false))
        child.on('close', code => resolve(code === 0))
    })
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `pnpm exec vitest run scripts/ota-e2e/__tests__/a11y-sentinel.test.ts`
Expected: PASS (7 tests).

- [ ] **Step 5: Lint + typecheck**

Run: `pnpm exec biome check scripts/ota-e2e/a11y-sentinel.ts scripts/ota-e2e/__tests__/a11y-sentinel.test.ts && pnpm exec tsc --noEmit`
Expected: biome clean; tsc exit 0.

- [ ] **Step 6: Commit**

```bash
git add scripts/ota-e2e/a11y-sentinel.ts scripts/ota-e2e/__tests__/a11y-sentinel.test.ts
git commit -m "test(ota-e2e): a11y-sentinel reader (idb describe-all, idb-optional)"
```

---

## Task 6: Wire `assertUpdateIsLive` into the crash-rollback runner

**Files:**
- Modify: `scripts/ota-e2e/run-ota-crash-rollback.ts`

This adds the assertion to the `EXPECT==='healthy'` + `outcome.kind==='healthy'` branch, after the existing `assertBookingTables` call. It reads the sim UDID from `IPHONE_SIMULATOR_UDID` (already exported by the driver).

- [ ] **Step 1: Add imports**

In `scripts/ota-e2e/run-ota-crash-rollback.ts`, add to the existing import block:

```ts
import { scrapeBootBundleId } from './boot-log-scraper'
import { idbAvailable, queryA11ySentinel } from './a11y-sentinel'
```

- [ ] **Step 2: Add a UDID constant near the other env reads**

After the existing `const POLL_INTERVAL_MS = ...` line, add:

```ts
const SIM_UDID = process.env.IPHONE_SIMULATOR_UDID
```

- [ ] **Step 3: Add the `assertUpdateIsLive` function** (after `assertBookingTables`)

```ts
// After a HEALTHY update, prove the NEW bundle's JS actually executed + rendered
// (boot-log) and is present ON SCREEN (a11y sentinel) — not just that the native
// currentId flipped. The boot-log check requires only simctl (always present).
// The a11y check needs idb: if idb is absent we SKIP it with a loud log (never a
// false fail); if idb IS present, a missing/mismatched sentinel FAILS.
async function assertUpdateIsLive(udid: string, newId: string): Promise<void> {
    // Boot-log: retry to allow first render to lag the currentId flip.
    let bootId: string | null = null
    for (let i = 0; i < 15; i++) {
        bootId = await scrapeBootBundleId(udid, 180).catch(() => null)
        if (bootId === newId) break
        await new Promise(r => setTimeout(r, 2_000))
    }
    if (bootId !== newId) {
        fail(
            `boot-log proof missing: expected the app to log 'app-boot: rendered ... id=${newId}' ` +
                `after the update, but saw id=${JSON.stringify(bootId)}. The new bundle's JS did not ` +
                `execute/mount even though the native currentId flipped.`
        )
    }
    console.log(`[ota-rollback] boot-log proof: new bundle JS executed + mounted (id=${newId})`)

    // A11y sentinel: skip-with-log if idb is unavailable; otherwise require it.
    if (!(await idbAvailable())) {
        console.log('[ota-rollback] idb not found — skipping on-screen sentinel check (boot-log proof stands)')
        return
    }
    let sentinelId: string | null = null
    for (let i = 0; i < 15; i++) {
        sentinelId = await queryA11ySentinel(udid)
        if (sentinelId === newId) break
        await new Promise(r => setTimeout(r, 2_000))
    }
    if (sentinelId !== newId) {
        fail(
            `on-screen sentinel proof missing: idb is present but the a11y element ` +
                `'ota-bundle-sentinel' did not carry id=${newId} (saw ${JSON.stringify(sentinelId)}). ` +
                `The update did not render visibly.`
        )
    }
    console.log(`[ota-rollback] on-screen sentinel proof: update visible (id=${newId})`)
}
```

- [ ] **Step 4: Call it on the healthy path**

In the `EXPECT === 'healthy'` + `outcome.kind === 'healthy'` branch, after the existing `await assertBookingTables(SERVER_URL, token)` line and before `process.exit(0)`, insert:

```ts
            if (!SIM_UDID) {
                console.log(
                    '[ota-rollback] IPHONE_SIMULATOR_UDID unset — skipping update-is-live assertion'
                )
            } else {
                await assertUpdateIsLive(SIM_UDID, newId)
            }
```

- [ ] **Step 5: Typecheck + lint**

Run: `pnpm exec tsc --noEmit && pnpm exec biome check scripts/ota-e2e/run-ota-crash-rollback.ts`
Expected: tsc exit 0; biome clean (fix wrap-only formatting at source).

- [ ] **Step 6: Commit**

```bash
git add scripts/ota-e2e/run-ota-crash-rollback.ts
git commit -m "test(ota-e2e): assert update-is-live (boot-log + a11y sentinel) on healthy path"
```

---

## Task 7: Document the assertion + the idb optional dependency

**Files:**
- Modify: `scripts/ota-e2e/README.md`

- [ ] **Step 1: Add a subsection under the Crash-rollback E2E section**

In `scripts/ota-e2e/README.md`, inside the "Crash-rollback E2E (`test:e2e:ota:rollback`)" section, after the HEALTHY/ROLLBACK bullet list, add:

```markdown
On the **HEALTHY** outcome the harness additionally asserts the update is
genuinely *live* (not just that the native `currentId` flipped):

- **boot-log proof** — the app logs `[tinycld] app-boot: rendered bundle id=<id>
  hash=<…>` from `BundleSentinel` only after the real provider tree mounts; the
  harness scrapes the device console (`simctl log show`) and requires the new
  `build-<ts>-ios` id. Proves the new bundle's JS executed + mounted.
- **on-screen sentinel proof** — `BundleSentinel` renders an accessibility element
  (`ota-bundle-sentinel`, label `bundle:<id>`); the harness reads the a11y tree
  with [`idb`](https://fbidb.io/) (`idb ui describe-all --json`) and requires the
  new id. Proves the update is visible on screen.

**`idb` is an OPTIONAL dependency.** Install it (`brew install idb-companion` +
`pipx install fb-idb`, or it ships at `~/.local/bin/idb`) to get the on-screen
sentinel check. When `idb` is absent the harness logs a skip and relies on the
boot-log proof alone — it never fails for a missing `idb`.
```

- [ ] **Step 2: Commit**

```bash
git add scripts/ota-e2e/README.md
git commit -m "docs(ota-e2e): document the update-is-live assertion + optional idb dep"
```

---

## Task 8: (OPTIONAL / stretch) Wire the assertion into the happy-path runner

Only do this if the happy-path harness (`run-ota-e2e.ts`) should also prove
update-is-live. It is the same proof on the simpler todo install. Skip if the
crash-rollback healthy path is sufficient.

**Files:**
- Modify: `scripts/ota-e2e/run-ota-e2e.ts`

- [ ] **Step 1: Add imports + a UDID-guarded call after the flip passes**

`run-ota-e2e.ts` already reads `SIM_UDID` (`const SIM_UDID = process.env.IPHONE_SIMULATOR_UDID`) and computes `newId` via precheck. Add near the other `./` imports:

```ts
import { scrapeBootBundleId } from './boot-log-scraper'
import { idbAvailable, queryA11ySentinel } from './a11y-sentinel'
```

Then factor the `assertUpdateIsLive` body into a shared spot OR copy a slimmed
inline version. To keep the two runners DRY, MOVE `assertUpdateIsLive` into a new
shared module `scripts/ota-e2e/update-is-live.ts` (export it), import it in BOTH
runners, and pass a `fail`/`log` pair in (each runner has its own `fail`). Concretely:

Create `scripts/ota-e2e/update-is-live.ts`:

```ts
// Shared update-is-live assertion used by both ota-e2e runners. The caller passes
// its own fail(msg): never (each runner formats its own prefix) so the proof
// logic lives in one place.
import { scrapeBootBundleId } from './boot-log-scraper'
import { idbAvailable, queryA11ySentinel } from './a11y-sentinel'

export async function assertUpdateIsLive(
    udid: string,
    newId: string,
    fail: (msg: string) => never,
    log: (msg: string) => void
): Promise<void> {
    let bootId: string | null = null
    for (let i = 0; i < 15; i++) {
        bootId = await scrapeBootBundleId(udid, 180).catch(() => null)
        if (bootId === newId) break
        await new Promise(r => setTimeout(r, 2_000))
    }
    if (bootId !== newId) {
        fail(
            `boot-log proof missing: expected id=${newId} after the update, saw ${JSON.stringify(bootId)}. ` +
                `The new bundle's JS did not execute/mount even though currentId flipped.`
        )
    }
    log(`boot-log proof: new bundle JS executed + mounted (id=${newId})`)

    if (!(await idbAvailable())) {
        log('idb not found — skipping on-screen sentinel check (boot-log proof stands)')
        return
    }
    let sentinelId: string | null = null
    for (let i = 0; i < 15; i++) {
        sentinelId = await queryA11ySentinel(udid)
        if (sentinelId === newId) break
        await new Promise(r => setTimeout(r, 2_000))
    }
    if (sentinelId !== newId) {
        fail(
            `on-screen sentinel proof missing: idb present but 'ota-bundle-sentinel' did not carry ` +
                `id=${newId} (saw ${JSON.stringify(sentinelId)}). The update did not render visibly.`
        )
    }
    log(`on-screen sentinel proof: update visible (id=${newId})`)
}
```

Then in BOTH `run-ota-crash-rollback.ts` (replacing the Task-6 inline copy) and
`run-ota-e2e.ts`, import and call it:

```ts
import { assertUpdateIsLive } from './update-is-live'
// ... where the flip/healthy outcome is confirmed and SIM_UDID is known:
if (SIM_UDID) await assertUpdateIsLive(SIM_UDID, newId, fail, m => console.log(`[ota-e2e] ${m}`))
```

(In the crash-rollback runner pass a `[ota-rollback]`-prefixed logger; delete the
inline `assertUpdateIsLive` from Task 6 so there is one definition.)

- [ ] **Step 2: Move the tests/checks**

Run: `pnpm exec vitest run scripts/ota-e2e/__tests__/ && pnpm exec tsc --noEmit && pnpm exec biome check scripts/ota-e2e/`
Expected: all unit tests pass; tsc exit 0; biome clean.

- [ ] **Step 3: Commit**

```bash
git add scripts/ota-e2e/update-is-live.ts scripts/ota-e2e/run-ota-crash-rollback.ts scripts/ota-e2e/run-ota-e2e.ts
git commit -m "refactor(ota-e2e): share assertUpdateIsLive across both runners + wire happy path"
```

---

## Task 9: Real run — verify on the simulator

**Files:** none (execution + observation)

This is the actual end-to-end verification the whole feature exists to enable.
Requires Docker + a booted sim + the manual `/connect` pre-step (see README).

- [ ] **Step 1: Run the happy-path driver (proves receive → apply → boot → VISIBLE)**

Run:
```bash
cd ~/code/tinycld/tinycld
KEEP=1 bash scripts/ota-e2e/run-ota-dry-run.sh
```
(If Task 8 was done, the happy-path runner now also asserts update-is-live.)
Expected: the existing flip PASS, plus — if Task 8 done — the boot-log + sentinel
proofs. If Task 8 was NOT done, run the healthy crash-rollback variant instead:
```bash
KEEP=1 OTA_E2E_EXPECT=healthy bash scripts/ota-e2e/run-ota-crash-rollback.sh
```
Expected: `boot-log proof: new bundle JS executed + mounted` and either
`on-screen sentinel proof: update visible` or the idb-skip log.

- [ ] **Step 2: Capture the result**

Record the exact harness output (the proof lines). If the sentinel check FAILED
while the boot-log PASSED, that's the informative case: JS ran but the element
wasn't found — capture the `idb ui describe-all --json` output for the booted sim
to diagnose the a11y field spelling, and adjust `findSentinelBundleId` if a real
field-name variant was missed (then re-run).

- [ ] **Step 3: Commit findings (notes only)**

```bash
git commit --allow-empty -m "docs(ota-e2e): record update-is-live real-run outcome [details in body]"
```
(Put the observed proof lines in the commit body.)

---

## Self-Review notes

- **Spec coverage:** Component 1 (BundleSentinel: console boot-log + a11y sentinel, no `__DEV__`, web no-op, mounted after MarkBundleHealthy) = Tasks 1–3. Component 2a (boot-log-scraper) = Task 4. Component 2b (a11y-sentinel, idb-optional) = Task 5. Component 2c (wire into healthy path with retries) = Task 6. Docs + optional idb dep = Task 7. Stretch happy-path wiring = Task 8. Real-run verification = Task 9.
- **Type consistency:** `extractBootBundleId`/`scrapeBootBundleId` (Task 4) reused in Task 6/8; `findSentinelBundleId`/`queryA11ySentinel`/`idbAvailable` (Task 5) reused in Task 6/8; `bootLogLine`/`formatSentinelLabel`/`shortHash` (Task 1) used by the component (Task 2). The boot line literal in `bootLogLine` (Task 1) and the regex in `extractBootBundleId` (Task 4) MUST match — both use `[tinycld] app-boot: rendered bundle id=<id> hash=<…>`; the test in Task 4 pins this. The a11y identifier `ota-bundle-sentinel` and label `bundle:<id>` are defined in Task 2 and parsed in Task 5 (pinned by both tests).
- **No placeholders:** every code step shows complete code; the only conditional is Task 8 (explicitly optional) and Task 9 (real-device, observation-based, with a concrete diagnostic fallback).
- **Known unknown surfaced, not hidden:** the exact `idb ui describe-all --json` field spelling (`AXIdentifier`/`AXLabel` vs `identifier`/`label`) — the parser tolerates BOTH, and Task 9 Step 2 captures a real tree to confirm/adjust rather than assuming.
```
