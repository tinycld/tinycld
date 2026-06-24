# OTA Boot-Beacon Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Replace the Release-blind `console.log` boot signal with a server **boot beacon** so the harness can prove, in a Release build, that the OTA'd bundle's JS actually executed and the real provider tree mounted.

**Architecture:** `BundleSentinel` POSTs `{ id, platform, hash }` to a new minimal `POST /api/app/boot` Go endpoint on mount (after the real provider tree commits, mirroring `MarkBundleHealthy`). The server logs `app-boot: rendered` with `bundle_id` — queryable via `_logs`, the same channel the existing `currentId`-flip assertion already reads and which provably works in Release. The harness reads the beacon from `_logs` and `assertUpdateIsLive` requires the new `build-<ts>-ios` id. The v1 console-scraper + a11y-sentinel are superseded.

**Tech Stack:** Go (PocketBase `coreserver`), TypeScript (RN app shell; tsx harness; vitest), iOS Release build.

**Spec:** `docs/superpowers/specs/2026-06-24-ota-update-visible-assertion-design.md` (see "Real-run findings & revision").

**Root cause (confirmed via systematic debugging):** RN Release does not route JS `console.*` to `os_log`; the `opacity:0` a11y sentinel isn't surfaced by `idb`. The OTA reload itself works + is healthy (proven by `boot.json` absence ⇒ `markHealthy()` ran). Fix = report over the server channel that works in Release.

**Patterns to mirror (read before implementing):**
- Server handler: `core/server/coreserver/app_updates.go` `g.POST("/update/report-bad", …)` (binds JSON, validates, logs at Info/Warn, returns JSON).
- Client POST: `core/lib/use-app-updates.ts` `reportRevertedBundle()` + `reportBadBundle()` (resolved-address gate, transport gate, fetch). The beacon mirrors this BUT fires in production Release (no `__DEV__`-only gate beyond the web/address gates).
- Harness poller: `scripts/ota-e2e/logs-poller.ts` `fetchAppUpdateCurrentIds` / `extractCurrentIds` (reads `_logs` filtered by message, pulls a `data` field).

---

## File Structure

**Server:**
- `core/server/coreserver/app_updates.go` (MODIFY) — add `POST /api/app/boot` handler logging `app-boot: rendered` with `q.bundleId`.
- `core/server/coreserver/app_updates_test.go` (MODIFY/CREATE) — Go test: POST to `/api/app/boot` returns 200 and the row/log is recorded.

**App shell:**
- `core/lib/bundle-sentinel.tsx` (MODIFY) — replace the `console.log` boot line with a `postBootBeacon(serverUrl, …)` POST on mount (web/address gated). Keep the a11y sentinel View (harmless; may aid a future on-screen check) but it is no longer machine-asserted.
- `core/lib/app-updater/client.ts` (MODIFY, if that's where checkForUpdate/reportBadBundle live) — add a `postBootBeacon` client fn mirroring `reportBadBundle`.
- `core/tests/unit/bundle-sentinel.test.tsx` (MODIFY) — update tests: assert `postBootBeacon` is called with the bundle id on mount; drop the `bootLogLine`/console expectations (or keep `bootLogLine` only if still used).

**Harness:**
- `scripts/ota-e2e/boot-beacon-poller.ts` (NEW) — `extractBootBeaconIds(logsResponse)` (pure) + `fetchBootBeaconIds(serverUrl, token)` reading `_logs` filtered to `message='app-boot: rendered'`, pulling `data['q.bundleId']`. Mirrors logs-poller.ts.
- `scripts/ota-e2e/__tests__/boot-beacon-poller.test.ts` (NEW).
- `scripts/ota-e2e/update-is-live.ts` (MODIFY) — replace `scrapeBootBundleId` with a poll over `fetchBootBeaconIds` (retry until the new id appears); drop the `idb` a11y branch (or keep it behind an opt-in env, NOT required). Signature still `assertUpdateIsLive(serverUrl, token, newId, fail, log)` — note it now needs serverUrl+token, not udid.
- `scripts/ota-e2e/run-ota-e2e.ts` + `run-ota-crash-rollback.ts` (MODIFY) — pass `(SERVER_URL, token, newId, fail, log)` to the new `assertUpdateIsLive`.
- `scripts/ota-e2e/boot-log-scraper.ts` + `a11y-sentinel.ts` (DELETE or leave unused) — superseded. Delete to avoid dead code, along with their tests.
- `scripts/ota-e2e/README.md` (MODIFY) — update the "Update-is-live" section: server beacon, not console/idb.

---

## Task 1: Server `POST /api/app/boot` endpoint (Go, TDD)

**Files:**
- Modify: `core/server/coreserver/app_updates.go`
- Test: `core/server/coreserver/app_updates_test.go`

- [ ] **Step 1: Read the existing handler + test scaffolding.** `grep -n 'report-bad\|OnServe\|func Test\|reportBadBody\|BindBody' core/server/coreserver/app_updates.go core/server/coreserver/app_updates_test.go`. Note how `report-bad` binds the body, validates platform, logs, and returns JSON, and how existing tests spin up a test app + issue a request.

- [ ] **Step 2: Write the failing Go test** modeling a boot POST. Mirror the report-bad test: start the coreserver test app, `POST /api/app/boot` with `{"id":"build-9-ios","platform":"ios","hash":"abc"}`, assert HTTP 200 and `{"ok":true}`. (If the existing tests assert on logs via a captured logger, assert the `app-boot: rendered` line; otherwise assert the 200 + body and rely on the harness integration for the log-read.) Name it `TestAppBoot_RecordsBeacon`.

- [ ] **Step 3: Run it, confirm FAIL** (404/route missing): `cd core/server && go test ./coreserver/ -run TestAppBoot_RecordsBeacon -v`.

- [ ] **Step 4: Implement the handler** in the `g := e.Router.Group("/api/app")` block, after the `report-bad` handler:

```go
// A freshly-booted bundle's JS posts here once the real provider tree has
// mounted (BundleSentinel) — the proof the new bundle EXECUTED and rendered, not
// just that the native loader promoted it. Public, like the rest of /api/app
// (the app may post pre-auth). Logged at Info so the OTA e2e can read the beacon
// from _logs; console.log can't be observed in a Release build, which is why this
// server beacon exists.
g.POST("/boot", func(re *core.RequestEvent) error {
    var body struct {
        ID       string `json:"id"`
        Platform string `json:"platform"`
        Hash     string `json:"hash"`
    }
    if err := re.BindBody(&body); err != nil {
        return re.BadRequestError("invalid body", err)
    }
    if body.ID == "" {
        return re.BadRequestError("id is required", nil)
    }
    app.Logger().Info("app-boot: rendered",
        "q.bundleId", body.ID,
        "q.platform", body.Platform,
        "q.hash", body.Hash,
        "remoteAddr", re.Request.RemoteAddr,
    )
    return re.JSON(http.StatusOK, map[string]any{"ok": true})
})
```

- [ ] **Step 5: Run the test, confirm PASS.** Then `go test ./coreserver/ 2>&1 | tail -5` (no regressions) + `gofmt -l core/server/coreserver/app_updates.go` (empty).

- [ ] **Step 6: Commit** `git add core/server/coreserver/app_updates.go core/server/coreserver/app_updates_test.go && git commit -m "feat(server): POST /api/app/boot beacon — logs app-boot: rendered for OTA e2e"`. (Only those two files.)

---

## Task 2: `postBootBeacon` client fn (app, TDD)

**Files:**
- Modify: `core/lib/app-updater/client.ts` (or wherever `reportBadBundle` lives — `grep -rn "export async function reportBadBundle\|export async function checkForUpdate" core/lib`)
- Test: the existing client test file for that module (mirror its `reportBadBundle` test)

- [ ] **Step 1: Locate `reportBadBundle`** and read its signature/impl: `grep -rn "reportBadBundle\|checkForUpdate" core/lib/app-updater/`.

- [ ] **Step 2: Write a failing unit test** for `postBootBeacon({ serverUrl, platform, id, hash, fetchFn })` mirroring the `reportBadBundle` test: assert it POSTs to `${serverUrl}/api/app/boot` with the JSON body and resolves on ok. Use an injected `fetchFn` mock.

- [ ] **Step 3: Run, confirm FAIL.**

- [ ] **Step 4: Implement `postBootBeacon`** in the same module, mirroring `reportBadBundle`:

```ts
// POST a boot beacon once the running bundle's JS has mounted the real tree, so
// the OTA e2e can confirm (from the server's _logs) that the NEW bundle actually
// executed in a Release build — where console.log is not observable. Best-effort:
// a failure must never disrupt the app, so callers swallow/capture errors.
export async function postBootBeacon(opts: {
    serverUrl: string
    platform: 'ios' | 'android'
    id: string
    hash: string
    fetchFn?: typeof fetch
}): Promise<void> {
    const { serverUrl, platform, id, hash, fetchFn = fetch } = opts
    const res = await fetchFn(`${serverUrl}/api/app/boot`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id, platform, hash }),
    })
    if (!res.ok) throw new Error(`postBootBeacon failed: ${res.status}`)
}
```

- [ ] **Step 5: Run, confirm PASS.** biome + tsc clean.

- [ ] **Step 6: Commit** `feat(app-updater): postBootBeacon client for the OTA boot beacon`.

---

## Task 3: BundleSentinel posts the beacon instead of console.log

**Files:**
- Modify: `core/lib/bundle-sentinel.tsx`
- Modify: `core/tests/unit/bundle-sentinel.test.tsx`

- [ ] **Step 1: Update the test.** Replace the console expectation with: on mount (non-web), `postBootBeacon` is called once with `{ id: getCurrentBundleId(), hash: getCurrentBundleHash(), platform }`. Mock `postBootBeacon`, `app-updater`, and the resolved-address module. Keep the `formatSentinelLabel` test. Drop/keep `bootLogLine` per whether it's still referenced (if unused, remove it + its test).

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Rewrite `useBundleSentinel`** to post the beacon, gated like `reportRevertedBundle` (web no-op; resolved-address gate via `getResolvedAddress()`; transport gate via `isUpdateTransportAllowed`); NO `__DEV__`-only gate (must run in prod Release). Swallow errors via `captureException` (boot beacon is best-effort, like report-bad). Keep the `BundleSentinel` a11y View (it's harmless and may aid a future on-screen check) but it is no longer the asserted signal.

```tsx
import { getResolvedAddress } from '@tinycld/core/lib/...'      // same source reportRevertedBundle uses
import { isUpdateTransportAllowed, postBootBeacon } from '@tinycld/core/lib/app-updater/client'
import { captureException } from '@tinycld/core/lib/errors'

export function useBundleSentinel(): void {
    useEffect(() => {
        if (Platform.OS === 'web') return
        const serverUrl = getResolvedAddress()
        if (!serverUrl || !isUpdateTransportAllowed(serverUrl)) return
        void postBootBeacon({
            serverUrl,
            platform: Platform.OS === 'ios' ? 'ios' : 'android',
            id: AppUpdater.getCurrentBundleId(),
            hash: AppUpdater.getCurrentBundleHash(),
        }).catch(err => captureException('bundle-sentinel.boot-beacon', err))
    }, [])
}
```

(Verify the exact import paths of `getResolvedAddress` / `isUpdateTransportAllowed` against `use-app-updates.ts`; reuse the same sources.)

- [ ] **Step 4: Run, confirm PASS.** biome + tsc clean. Remove `bootLogLine` if now unused (and its test) so there's no dead code.

- [ ] **Step 5: Commit** `feat(app-shell): BundleSentinel posts a server boot beacon (Release-observable)`.

---

## Task 4: boot-beacon poller (harness, TDD)

**Files:**
- Create: `scripts/ota-e2e/boot-beacon-poller.ts`
- Test: `scripts/ota-e2e/__tests__/boot-beacon-poller.test.ts`

- [ ] **Step 1: Failing test** for `extractBootBeaconIds(response)` (pure — pulls `data['q.bundleId']` from each `_logs` item) + `fetchBootBeaconIds(serverUrl, token)`. Mirror `logs-poller.test.ts` / `bad-bundle-poller.test.ts` exactly (injected fetch where needed). Cover: extracts ids in order; tolerates missing items/field.

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Implement** mirroring `fetchAppUpdateCurrentIds`/`extractCurrentIds` but filter `message='app-boot: rendered'` and read `data['q.bundleId']`:

```ts
interface LogsResponse { items?: Array<{ data?: Record<string, unknown> }> }

export function extractBootBeaconIds(response: LogsResponse): string[] {
    const ids: string[] = []
    for (const item of response.items ?? []) {
        const id = item.data?.['q.bundleId']
        if (typeof id === 'string' && id.length > 0) ids.push(id)
    }
    return ids
}

export async function fetchBootBeaconIds(serverUrl: string, token: string): Promise<string[]> {
    const filter = encodeURIComponent("message='app-boot: rendered'")
    const res = await fetch(`${serverUrl}/api/logs?filter=${filter}&sort=-created&perPage=20`, {
        headers: { Authorization: token },
    })
    if (!res.ok) throw new Error(`fetchBootBeaconIds failed: ${res.status}`)
    return extractBootBeaconIds((await res.json()) as LogsResponse)
}
```

- [ ] **Step 4: Run, confirm PASS.** biome + tsc clean.

- [ ] **Step 5: Commit** `test(ota-e2e): boot-beacon poller reading app-boot: rendered from _logs`.

---

## Task 5: Rewire `assertUpdateIsLive` to the beacon; drop console/idb

**Files:**
- Modify: `scripts/ota-e2e/update-is-live.ts`
- Modify: `scripts/ota-e2e/run-ota-e2e.ts`, `scripts/ota-e2e/run-ota-crash-rollback.ts`
- Modify: `scripts/ota-e2e/__tests__/update-is-live.test.ts`
- Delete: `scripts/ota-e2e/boot-log-scraper.ts` (+ test), `scripts/ota-e2e/a11y-sentinel.ts` (+ test)

- [ ] **Step 1: Rewrite `assertUpdateIsLive`** to `(serverUrl, token, newId, fail, log)`: poll `fetchBootBeaconIds(serverUrl, token)` (reuse the `pollForBundleId` helper from logs-poller, or a bounded retry loop) until `newId` appears; on timeout, `fail` naming the beacon never arrived (the new bundle's JS didn't execute/mount). Remove the `scrapeBootBundleId` + `idbAvailable`/`queryA11ySentinel` logic entirely.

- [ ] **Step 2: Update its unit test** to mock `fetchBootBeaconIds` (or inject a fetcher) and assert pass-on-id / fail-on-timeout.

- [ ] **Step 3: Update both runners** to call `await assertUpdateIsLive(SERVER_URL, token, newId, fail, m => console.log(...))` after the flip — they already have `SERVER_URL`, `token`, `newId`. Remove the `SIM_UDID`-guard branch (the beacon needs no udid); keep running it unconditionally after the flip.

- [ ] **Step 4: Delete the superseded modules** `boot-log-scraper.ts`, `a11y-sentinel.ts`, and their tests. `grep -rn "boot-log-scraper\|a11y-sentinel\|scrapeBootBundleId\|queryA11ySentinel\|idbAvailable" scripts/` must return nothing after.

- [ ] **Step 5: Verify** `pnpm exec vitest run scripts/ota-e2e/__tests__/ && pnpm exec tsc --noEmit && pnpm exec biome check scripts/ota-e2e/`. All green.

- [ ] **Step 6: Commit** `refactor(ota-e2e): assert update-is-live via server boot beacon; drop Release-blind console/idb signals`.

---

## Task 6: Docs

**Files:** Modify `scripts/ota-e2e/README.md`

- [ ] **Step 1: Rewrite the "Update-is-live assertion" section** to describe the server boot beacon (`BundleSentinel` → `POST /api/app/boot` → `app-boot: rendered` in `_logs` → harness reads it), and remove the console/`idb` description + the optional-`idb` dependency note. State WHY (console.log + opacity:0 a11y aren't observable in Release).

- [ ] **Step 2: Commit** `docs(ota-e2e): document the server boot-beacon update-is-live proof`.

---

## Task 7: Real-run verification

**Files:** none (execution)

This requires a fresh Docker image build (the app + server changed), so it is the FULL pipeline. The `app.config.ts`/`package.json` version fixes are already in place.

- [ ] **Step 1: Rebuild + run** (no IMAGE reuse — the image must include the new server endpoint + app beacon):
  ```bash
  cd ~/code/tinycld/tinycld
  docker rm -f tinycld-ota-server 2>/dev/null
  KEEP=1 bash scripts/ota-e2e/run-ota-dry-run.sh 2>&1 | tee /tmp/ota-beacon-run.log
  ```
- [ ] **Step 2: Expected:** `precheck OK` → Release build → flip → `boot-log proof` (now from the beacon) showing the new `build-<ts>-ios`, then PASS. If the beacon doesn't arrive: with `KEEP=1`, check the server `_logs` for `app-boot: rendered` and the device for the POST (the app posts only after the real tree commits + address is resolved).
- [ ] **Step 3: Commit findings** (empty commit with the observed proof lines in the body).

---

## Self-Review notes
- **Root cause addressed:** the fix replaces the two Release-blind channels (console.log, opacity:0 a11y) with a server beacon over the channel proven to work in Release (`_logs`), exactly the mechanism the existing `currentId`-flip assertion already uses. This is a root-cause fix, not a symptom patch.
- **Type/name consistency:** server logs `q.bundleId`; poller reads `data['q.bundleId']` — must match (pinned by Task 1 + Task 4 tests). `assertUpdateIsLive` signature changes from `(udid,newId,...)` to `(serverUrl,token,newId,...)` — both runners updated in Task 5.
- **No dead code:** Task 5 deletes the superseded boot-log-scraper + a11y-sentinel modules and confirms no references remain.
- **Known unknown:** exact import paths of `getResolvedAddress`/`isUpdateTransportAllowed`/`reportBadBundle` — Task 2/3 verify against `use-app-updates.ts` rather than assume.
