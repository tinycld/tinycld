# OTA Happy-Path Native E2E — Design

**Date:** 2026-06-12
**Status:** Approved design, ready for implementation plan
**Scope:** Walking skeleton — one locally-run test proving the OTA update pipe on an iOS simulator.

## Goal

Provide a single, repeatable, **locally-run** test that proves the full over-the-air (OTA) update pipe works end-to-end on a real iOS simulator:

1. A **Release** build of the app launches on its embedded JS bundle.
2. It auto-checks the connected server for a newer bundle (the existing `useAppUpdates` behavior).
3. It downloads, stages, and reloads into the newer server-served bundle on its own.
4. The harness observes that the app is now running the **new** bundle id (not the embedded one).

This is the **happy path only**. Healthy-mark persistence across relaunch and crash-rollback are explicitly out of scope for the skeleton; they layer onto the same harness later (see Future Work).

### Why this scope

The app already runs as a full web SPA with mature Playwright E2E covering business logic and UI flows. The native E2E gap is purely **native-only integration points** — things absent from the web build. OTA was chosen as the first target because it ties directly into already-built infrastructure (`ios-simulator.sh --prod`, the `app-updater` native module, `tests/install/run-todo-install.sh`) and the shipped rollback reconciler work. The happy path is the smallest end-to-end proof that the OTA pipe is wired correctly.

## Key constraints (from the existing code)

These are facts established by reading the current implementation; the design depends on them.

- **Release build is mandatory.** Per `scripts/ios-simulator.sh`, only a Release configuration bakes JS into the embedded `main.jsbundle` with no Metro, which is the only path where `AppDelegate.bundleURL()` falls through to `AppUpdaterBundle.stagedBundleURL()`. A Debug/Metro build never exercises the OTA loader. The harness builds Release via `ios-simulator.sh --prod`.
- **The update flow is server-driven and fully automatic.** `core/lib/use-app-updates.ts` (`useAppUpdates`) fires a check on launch (after a 3s delay) and on every `AppState → 'active'` transition. If the server offers a manifest, the hook downloads → stages → shows a toast → calls `AppUpdater.reload()` on its own. **There is no UI to drive** — so the test is environment orchestration plus a thin observation step, not UI automation. No Maestro/Detox needed for the skeleton.
- **Transport gating.** `isUpdateTransportAllowed` (`core/lib/app-updater/client.ts`) refuses to check/download over plaintext unless the host is loopback (`localhost`/`127.0.0.1`) or `EXPO_PUBLIC_ALLOW_INSECURE_UPDATES` is set. Pointing the simulator at `http://localhost:<port>` satisfies the loopback exemption — no TLS required for the harness.
- **The server contract already exists.** `GET /api/app/update?platform=&runtimeVersion=&currentId=&currentHash=` (`core/server/coreserver/app_updates.go`) returns `204` when up-to-date / no match, or a manifest JSON when a newer bundle exists for the client's `platform`+`runtimeVersion`. "Up to date" is true when `currentId` matches the bundle id **or** `currentHash` matches the bundle hash.
- **The client reports its current bundle id on every check.** The hook passes `AppUpdater.getCurrentBundleId()` as `currentId`. After a reload promotes the new bundle, the next automatic check carries the **new** id.
- **The server logs every check at Info level.** The `/api/app/update` handler emits a structured log line per request including `q.currentId` and `server.bundles`. This is the observation channel (see below) — no DB access or app/server changes needed.
- **New bundles are produced by the install/rebuild pipeline.** `core/server/coreserver/rebuild_pipeline.go` / `pkg_build.go` mint `pkg_build` records whose `bundles` field feeds `/api/app/update`. `tests/install/run-todo-install.sh` already drives this pipeline. The harness reuses it rather than inventing bundle staging.

## Architecture — orchestration harness, not a test framework

Three small units, each independently understandable and testable. No new native code, no instrumented test build, no Appium/Detox/Maestro for the skeleton.

### Unit 1 — `build-release-sim` (shell, wraps existing tooling)

- **Does:** Boots the target iOS simulator and installs a Release build.
- **How:** Thin wrapper over `scripts/ios-simulator.sh --prod` (which already runs `expo run:ios --configuration Release`). Reads the simulator UDID from `../.env` (`IPHONE_SIMULATOR_UDID`) as the existing script does, or accepts `--udid`.
- **Input:** simulator UDID.
- **Output:** app installed + booted on the sim; the **embedded bundle id** `E` captured for the before/after comparison (`embedded-<version>` from `app.json`).
- **Owns nothing new** — it is a call over existing tooling.

### Unit 2 — `stage-newer-bundle` (TS, reuses the install harness)

- **Does:** Brings up a server and drives it to produce a **newer** bundle for the app's `runtimeVersion`, so `/api/app/update` answers with a manifest (status `new`) rather than `204`.
- **How:** Reuses the `tests/install/` package-install/rebuild path that already generates staged native bundles and writes a `pkg_build` record with `status = 'current'`. Server bound to `http://localhost:<port>` so the client's loopback transport exemption applies.
- **Input:** none beyond harness config (port).
- **Output:** running server URL `http://localhost:<port>`; the **expected new bundle id** `N` for the iOS platform (`build-<ts>-ios`).
- **Precondition asserted before booting the app:** a direct `GET /api/app/update?platform=ios&runtimeVersion=<v>&currentId=<E>` must return `200` with `id = N` (and `N != E`). If it returns `204`, fail here — see Error Handling.

### Unit 3 — `assert-reloaded` (TS)

- **Does:** After the app is pointed at the server, waits for the automatic check → download → stage → reload cycle, then asserts the app is running bundle `N`.
- **How (observation channel A — server-side signal):** Tail the server's Info log and watch the `app-update: request` lines' `q.currentId` field. Initially the app reports `E`; after the reload promotes the staged bundle, a subsequent automatic check reports `q.currentId = N`. Assertion: observe `q.currentId == N` within the timeout. This exploits the existing logging + the fact that the client always transmits its current id — **zero app or server changes**.
- **Input:** server log stream/path; `E`, `N`, timeout.
- **Output:** pass (`N` observed) / fail (timeout with last-observed id).

## Data flow

```
ios-simulator.sh --prod
    └─> app installed + booted on sim   (embedded id = E = embedded-<version>)

stage-newer-bundle  (reuse tests/install pipeline)
    └─> server @ http://localhost:P serving manifest  (new ios bundle id = N = build-<ts>-ios, N != E)
    └─> precheck: GET /api/app/update?...currentId=E  →  200 {id: N}

app (pointed at P)
    └─> useAppUpdates: GET /api/app/update  →  manifest N
    └─> download → stage → toast → AppUpdater.reload()
    └─> next automatic check reports currentId = N

assert-reloaded  (tail server log)
    └─> observe q.currentId flips E → N   ✅ PASS
```

## Error handling

No silent failures. Each likely failure has an unambiguous, actionable message.

- **Simulator UDID unset / not booted** → fail fast, reusing `ios-simulator.sh`'s existing message.
- **Server returns `204` at the precheck** (no newer bundle staged — the most likely failure) → fail in Unit 2 **before** booting the app, dumping the server's known bundles for context.
- **Reload never observed within timeout** → fail in Unit 3 with the **last-observed `q.currentId`** plus a hint: check the build is genuinely Release, the host is loopback (transport gating), and `__DEV__`/web guards aren't short-circuiting `useAppUpdates`.
- **No silent caps:** if the harness gives up waiting, it logs how long it waited and what it last saw.

## Testing & boundaries

- **iOS simulator only** for the skeleton. `ios-simulator.sh` is iOS-specific.
- **Local-only.** Not wired into CI (iOS simulators don't run on Linux runners). Run by hand, pre-release.
- **Reuses** the existing install-harness server and bundle pipeline; introduces no new bundle-building path.
- **No new native code, no instrumented build, no UI-driving framework.**

## Future work (explicitly out of scope for the skeleton)

These reuse Units 1–3 with additions:

- **Healthy-mark persistence:** after the happy-path reload, relaunch and assert it stays on `N` (no spurious rollback) — confirms `markBundleHealthy` ran.
- **Rollback flow:** stage a bundle that crashes before `markBundleHealthy`; assert native crash-rollback reverts and the server reconciles to `rolled_back` (exercises `ReconcileRolledBackInstall` / `.rollback-pending` / `exit(75)`). The highest-value flow and eventual real target.
- **Android:** swap Unit 1 for an `adb` / `expo run:android --variant release` equivalent; Units 2–3 unchanged.
- **On-screen assertions:** if a future flow needs visible UI state, introduce **Maestro** at that point. Deferred until a flow actually needs it.
