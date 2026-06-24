# OTA "Update-Is-Live" Assertion — Design

**Date:** 2026-06-24
**Status:** Approved (brainstorm), pending implementation plan

## Problem

The OTA E2E harness proves the device *received and applied* a new bundle and
*booted into it* — but only by reading the server's `q.currentId`, which the Go
update handler logs on every `/api/app/update` check. That `currentId` comes from
`AppUpdater.getCurrentBundleId()`, a **native** read of which bundle directory is
currently promoted. It proves the native loader picked the new bundle; it does
**not** prove the new bundle's **JavaScript executed, the real app tree mounted,
or anything became visible on screen**. A bundle that promotes but then fails to
execute/render its JS could still have its new `currentId` reported on a
subsequent native update check.

The reported user concern — *"have we verified the iOS app can actually receive
and apply an update, then boot afterwards with the update changes visible?"* — is
therefore **not** answered by the existing harness. This design adds that proof.

## Goal

After an OTA update, assert that:
1. the new bundle's **JS executed and the real provider tree mounted** (not just a
   placeholder/gate screen), and
2. a marker carrying the **running bundle id is present on screen** (a11y tree),

both keyed to the **new** `build-<ts>-ios` id, so the assertion can only pass when
the update is genuinely live and usable.

## Non-goals

- Pixel/OCR verification of arbitrary UI (we assert a known sentinel element, not
  screen text via OCR).
- Verifying a specific *package's* UI (todo/calendar-slots screens) — that would
  couple the test to package strings and require login+navigation. The sentinel
  is app-shell-level.
- Android (the harness is iOS-only today).
- Replacing the existing `currentId`-flip check — this is an **additional**,
  stronger layer on top of it.

## Constraints discovered during brainstorming

- **No `__DEV__` gating.** The OTA path only runs in a **Release** build
  (`expo run:ios --configuration Release`); anything behind `__DEV__` is compiled
  out and never fires. The boot signal + sentinel must be present in production
  builds.
- **`simctl` has no accessibility-tree query.** Machine-readable on-screen
  elements require external tooling. `idb` (`idb ui describe-all --json`) returns
  the a11y tree without an XCUITest target and is already installed on the dev
  machine (`~/.local/bin/idb` + `idb_companion`). It is treated as an **optional**
  dependency: absent ⇒ skip-with-log, never a false fail.
- **Mount timing matters.** The existing `MarkBundleHealthy` component mounts
  *inside* `<Providers>` on the resolved gate branch precisely so its effect fires
  only when the real provider tree (auth, data layer, stores) has committed — not
  when the blank gate placeholder renders. The new sentinel must mount at the same
  point for the same reason: "rendered" must mean the real app, not a placeholder.

## Architecture

Two parts: an always-on app-shell component that emits the signals, and a scripted
harness module that asserts them.

### Component 1 — `BundleSentinel` (app shell, production, always on)

A behavior-plus-tiny-UI component mounted **inside `<Providers>` on the resolved
branch of `app/_layout.tsx`, immediately after `<MarkBundleHealthy />`** — same
mount-timing contract.

On mount it:

1. **Logs to console unconditionally** (production included) a single distinctive,
   stable line:
   ```
   [tinycld] app-boot: rendered bundle id=<getCurrentBundleId()> hash=<shortHash>
   ```
   This fires only after the JS bundle executed and the real tree committed — the
   proof of execution+mount. `<shortHash>` is the first 12 chars of
   `getCurrentBundleHash()` (enough to disambiguate; full hash not needed on
   screen/log).

2. **Renders a near-invisible on-screen sentinel** — a tiny absolutely-positioned
   element carrying:
   - `accessibilityIdentifier="ota-bundle-sentinel"`
   - `accessibilityLabel="bundle:<id>"`

   Visible to the accessibility tree (so `idb` can find it), visually negligible.
   Concretely: a `<View>` absolutely positioned at the top-left with
   `pointerEvents="none"`, `width: 1, height: 1, opacity: 0` (or `0.01`), wrapping
   a `<Text>` whose content is the `bundle:<id>` string and carrying the
   `accessibilityIdentifier`/`accessibilityLabel` above. `opacity: 0` keeps it out
   of the visible UI while remaining in the a11y tree on iOS; if a future iOS
   release prunes fully-transparent nodes from the tree, bump to `opacity: 0.01`.
   No `__DEV__` gate.

The component lives in core (`tinycld/core/lib/`) alongside `mark-bundle-healthy`,
mirroring its file shape (a `useBundleSentinel` hook + a `BundleSentinel`
component). It depends only on the existing `AppUpdater` native module
(`getCurrentBundleId`, `getCurrentBundleHash`) and React.

**Production footprint:** one console.log per boot and one tiny always-mounted
element. No network, no `__DEV__`, no behavioral change to the app. The console
line is innocuous in production logs.

### Component 2 — Harness assertion modules (`scripts/ota-e2e/`)

Each module splits a **pure parser** (unit-tested, no IO) from a **thin IO
wrapper** (injectable for tests; integration-exercised by the real run), matching
the existing `bad-bundle-poller.ts` / `logs-poller.ts` style.

#### 2a. `boot-log-scraper.ts`
- `extractBootBundleId(logText: string): string | null` — pure. Regex-parses the
  `[tinycld] app-boot: rendered bundle id=<id> hash=<…>` line and returns the
  **last** (most recent) id, or `null` if none. Unit-tested.
- `scrapeBootBundleId(udid, sinceSeconds, spawnFn?): Promise<string | null>` —
  shells:
  ```
  xcrun simctl spawn <udid> log show \
    --predicate 'eventMessage CONTAINS "app-boot: rendered"' \
    --last <sinceSeconds>s --style compact
  ```
  feeds stdout to `extractBootBundleId`. `spawnFn` injectable for tests.

#### 2b. `a11y-sentinel.ts`
- `findSentinelBundleId(tree: unknown): string | null` — pure. Walks the
  `idb ui describe-all` JSON, finds the element whose identifier is
  `ota-bundle-sentinel`, parses the id out of its `AXLabel`/label
  (`bundle:<id>`). Returns the id or `null`. Unit-tested with a captured fixture.
- `queryA11ySentinel(udid, runner?): Promise<string | null>` — runs
  `idb ui describe-all --udid <udid> --json`, parses via `findSentinelBundleId`.
  **If `idb` is not on PATH, returns `null` after logging a clear
  "idb not found — skipping on-screen sentinel check" note** (skip, not fail).
  `runner` injectable for tests.

#### 2c. Wiring into the runner
In `run-ota-crash-rollback.ts` on the `EXPECT=healthy` + `outcome.kind==='healthy'`
branch (after the existing `assertBookingTables`), add an
`assertUpdateIsLive(udid, newId)` step that:
- scrapes the boot log and asserts the parsed id === `newId` (the new
  `build-<ts>-ios`) — **required**; failure names expected vs seen.
- queries the a11y sentinel and asserts its id === `newId` — **required when `idb`
  is present**; **skipped-with-log when absent** (so the harness still runs on a
  machine without `idb`).
- both checks use a bounded retry loop (first render may lag the `currentId` flip),
  mirroring `assertBookingTables`' retry shape.

The simulator UDID reaches the runner via `IPHONE_SIMULATOR_UDID` (already
exported by the driver). The driver (`run-ota-crash-rollback.sh`) needs no change
beyond ensuring that env var is forwarded to the assertion step (it already is for
the build steps).

Optionally, the same `assertUpdateIsLive` can be wired into the happy-path
`run-ota-e2e.ts` after its flip assertion (low cost, same proof). This is a
stretch item, not required for the core goal.

## Data flow

```
Release build (embedded bundle)  ──boot──▶ BundleSentinel logs id=embedded-<v>, renders sentinel(embedded-<v>)
        │
   OTA install stages build-<ts>-ios, app reloads
        ▼
New bundle boots ──▶ BundleSentinel logs id=build-<ts>-ios, renders sentinel(build-<ts>-ios)
        │
   harness: scrapeBootBundleId(udid) ─▶ build-<ts>-ios  ✅ (JS executed + mounted)
   harness: queryA11ySentinel(udid)  ─▶ build-<ts>-ios  ✅ (on screen)  [or skip if no idb]
```

## Error handling

- **Wrong id seen** (e.g. still `embedded-<v>`): fail naming expected `newId` vs
  observed, indicating the new JS never executed/rendered even though `currentId`
  flipped natively — exactly the gap this design targets.
- **No boot-log line at all**: fail — the app never reached the sentinel mount
  (crashed before the real tree committed, or stuck on the gate). Distinct message
  from "wrong id".
- **`idb` absent**: the a11y check logs a skip and does not fail; the boot-log
  check still runs (it needs only `simctl`, always present in this environment).
- **Transient `idb`/`simctl` error**: treated as retry within the bounded loop;
  only a clean negative result after retries fails (a11y) or, for `idb` errors
  specifically, downgrades to skip with a logged reason.

## Testing

- **Unit (vitest, no device):** `extractBootBundleId` and `findSentinelBundleId`
  with fixtures (a real `log show` blob and a captured `idb describe-all` JSON).
  These run in CI like the existing poller tests.
- **Integration (device, scripted):** the two IO wrappers and `assertUpdateIsLive`
  are exercised by the real `run-ota-crash-rollback.sh` run — no separate ad-hoc
  invocation. Re-runnable on demand.
- **No new `__DEV__`-gated paths**, so the Release build the OTA path needs is
  unaffected.

## File inventory

App shell (core):
- `tinycld/core/lib/bundle-sentinel.tsx` (NEW) — `useBundleSentinel` + `BundleSentinel`.
- `tinycld/app/_layout.tsx` (MODIFY) — mount `<BundleSentinel />` after `<MarkBundleHealthy />`.
- a core unit test for the sentinel's id/label formatting (pure helper, if extracted).

Harness:
- `scripts/ota-e2e/boot-log-scraper.ts` (NEW) + `__tests__/boot-log-scraper.test.ts`.
- `scripts/ota-e2e/a11y-sentinel.ts` (NEW) + `__tests__/a11y-sentinel.test.ts`.
- `scripts/ota-e2e/run-ota-crash-rollback.ts` (MODIFY) — add `assertUpdateIsLive`.
- `scripts/ota-e2e/README.md` (MODIFY) — document the new assertion + the `idb`
  optional dependency.
- (stretch) `scripts/ota-e2e/run-ota-e2e.ts` (MODIFY) — same assertion on the
  happy path.

## Open questions

None blocking. The happy-path wiring (2c stretch) is optional and can be decided
during planning.
