# OTA happy-path native E2E (local-only, iOS)

Proves the over-the-air update pipe end-to-end on an iOS simulator: a **Release**
build launches on its embedded JS bundle, auto-checks **its cached/connected
server** (established by the manual connect pre-step below), and reloads into a
newer server-served bundle. The harness observes the flip via the server's
structured `_logs` (`app-update: request` records) over the API — no app/server
changes, no Maestro/Detox.

## What it does

1. Reads the embedded bundle id from `app.json` (`embedded-<version>`).
2. Prechecks `GET /api/app/update` — the server must already offer a newer iOS
   bundle (`build-<ts>-ios`). Fails loudly if not (status 204).
3. Builds + boots a Release sim via `scripts/ios-simulator.sh --prod`. The app
   resolves its server from its own cached value (established by the manual
   connect pre-step), not from the harness.
4. Polls the server's structured `_logs` (`GET /api/logs`) and passes when a
   logged `app-update: request` reports `q.currentId` equal to the new bundle id.

## Prerequisites (manual, one-time per run)

- **One-time per fresh simulator: connect the app to the test server manually.**
  The app does not accept a server address via env — it resolves a server from
  its own cached value (AsyncStorage) or the in-app `/connect` screen. So before
  the first run on a given simulator:
    1. Boot the sim and open TinyCld.
    2. On the `/connect` screen, enter the test server URL **explicitly** —
       `http://localhost:7200` (the on-screen prefill is `http://localhost:7100`,
       a DIFFERENT port; do not use it unless that's actually your server).
    3. Once connected, the address is cached in AsyncStorage and reused by the
       Release rebuild the harness boots. You only redo this if you wipe the sim.
- A booted iOS simulator; its UDID in the workspace-root `.env` as
  `IPHONE_SIMULATOR_UDID` (same var `ios-simulator.sh` uses).
- Xcode + the iOS toolchain (Release build runs `expo run:ios`).
- A **running local server on a loopback host** (e.g. `http://localhost:7200`,
  the `expo:test` port) which holds a newer iOS bundle than the app's embedded
  one. Two ways to get the bundle:
  - Trigger a server-side rebuild/install (the path
    `tests/install/run-todo-install.sh` drives), which runs `expo export
    --platform ios` and writes a `pkg_build` record; or
  - Point at an install-harness container that already built one.
- **PB superuser credentials** for that server — the harness reads `/api/logs`
  with them. No log-file capture is needed.

## Run

```sh
cd ~/code/tinycld/tinycld
OTA_E2E_SERVER_URL=http://localhost:7200 \
OTA_E2E_SUPERUSER_EMAIL=admin@example.com \
OTA_E2E_SUPERUSER_PASSWORD=... \
pnpm run test:e2e:ota
```

## Env knobs

| Var | Default | Meaning |
|---|---|---|
| `OTA_E2E_SERVER_URL` | `http://localhost:7200` | Server the harness prechecks + polls `_logs` on. Must match the server the app was manually connected to (the harness does not set the app's server). |
| `OTA_E2E_SUPERUSER_EMAIL` | _(required)_ | PB superuser identity used to read `/api/logs`. |
| `OTA_E2E_SUPERUSER_PASSWORD` | _(required)_ | PB superuser password. |
| `IPHONE_SIMULATOR_UDID` | from `.env` | Target simulator. |
| `OTA_E2E_TIMEOUT_MS` | `180000` | Max wait for the reload flip. |
| `OTA_E2E_POLL_INTERVAL_MS` | `3000` | Delay between `_logs` polls. |

## Interpreting failures

- **204 at precheck** → no newer bundle staged; the server build/export step
  didn't run or produced no iOS bundle.
- **superuser auth failed** → check `OTA_E2E_SUPERUSER_*` creds and that the
  server is up.
- **timed out, last-seen ids show only `embedded-…`** → the app never reloaded.
  Check: build is genuinely Release (not Debug/Metro), server host is loopback
  (transport gating), `__DEV__`/web guards aren't active.
- **timed out, and `_logs` shows no `app-update: request` records at all** → the
  app never reached the server (most often: it's still on the `/connect` screen —
  do the manual connect pre-step), OR the server's `Logs.MinLevel` was raised above
  Info so the request isn't persisted to `_logs` (default settings persist Info —
  only an issue if someone changed it).
- **build/boot exited non-zero** → an `expo run:ios --configuration Release`
  failure; see the inline build output.

## Crash-rollback E2E (`test:e2e:ota:rollback`)

A sibling harness that exercises the *unhappy* OTA path. Instead of the happy-path
`todo` install, it installs **`@tinycld/calendar-slots`** (an external `github:`
package, via the in-app installer UI driven by
`tests/install/calendar-slots-install.spec.ts`), which mints a `build-<ts>-ios`
bundle. It then boots the Release sim and races **two** terminal outcomes,
asserting which one happened:

- **HEALTHY** — the app reloads into the new bundle and stays up. The harness then
  asserts the install actually created all four booking collections
  (`booking_pages`, `booking_slot_types`, `booking_availability`, `bookings`) —
  guarding the "booking tables sometimes not created on install" bug — and runs the
  **update-is-live assertion** (below).
- **ROLLBACK** — the app crash-loops the new bundle and reverts to embedded; the
  server records a `pkg_bad_bundle` row whose `last_error` carries a **captured
  reason** (the native rollback reason / regex detail). An empty or the generic
  `last_error` is a FAILURE — that's exactly the gap the manual repro hit.

`OTA_E2E_EXPECT` selects which outcome is the pass (default `rollback`, the case
we're chasing). The driver mirrors the happy-path one but uses a **distinct**
container name (`tinycld-ota-rollback-server`), port (`7091`), and log dir
(`ota-crash-rollback-logs`) so a `KEEP=1` dry-run and a `KEEP=1` rollback run can
coexist.

```sh
cd ~/code/tinycld/tinycld
# Full automated driver (Docker + sim): builds the image, installs calendar-slots,
# boots the Release sim, and runs the assertion.
KEEP=1 OTA_E2E_EXPECT=rollback bash scripts/ota-e2e/run-ota-crash-rollback.sh
# Healthy variant (also asserts the booking tables exist):
KEEP=1 OTA_E2E_EXPECT=healthy bash scripts/ota-e2e/run-ota-crash-rollback.sh
```

Additional env knobs (on top of the happy-path ones above):

| Var | Default | Meaning |
|---|---|---|
| `OTA_E2E_EXPECT` | `rollback` | Which terminal outcome is the pass: `healthy` or `rollback`. |
| `PKG_SPEC` | `github:stefnnn/tinycld-calendar-slots` | The package whose OTA crash we reproduce (overridable for a fork/tag). |
| `CONTAINER` | `tinycld-ota-rollback-server` | Container name (distinct from the dry-run's). |
| `SERVER_PORT` | `7091` | Host port (distinct from the dry-run's `7090`). |

The TS assertion runner (`run-ota-crash-rollback.ts`) can also be invoked directly
(`pnpm run test:e2e:ota:rollback`) once a bundle is staged and the sim is booted +
connected — it is poll-only and never builds. (The driver passes
`OTA_E2E_SKIP_BUILD=1` for parity with the happy-path runner, but this runner
ignores it.)

## Update-is-live assertion (server boot beacon)

Both runners (the happy-path `run-ota-e2e.ts` and the crash-rollback runner's
HEALTHY outcome) verify the update is genuinely *live* — not merely that the
native `currentId` flipped. The native id comes from a read of which bundle
directory is promoted; it does **not** prove the new bundle's JS executed or that
the real app tree mounted (a bundle could promote, then crash before its JS runs,
and still report the new `currentId` natively).

The proof: the app's `BundleSentinel` (`core/lib/bundle-sentinel.tsx`) **POSTs a
boot beacon** to `POST /api/app/boot` — but only after the **real provider tree
commits** (it is mounted next to `MarkBundleHealthy` inside `<Providers>`, so it
can't fire from the blank gate placeholder). The server logs `app-boot: rendered`
with the bundle id; the harness polls `_logs` (`boot-beacon-poller.ts`) until the
new `build-<ts>-ios` id appears, and fails if it never does within 60s. This
proves the new bundle's JS executed and mounted.

**Why a server beacon and not a device-side signal.** The OTA path runs only in a
**Release** build, and a real-sim run proved both obvious device-side signals are
unobservable there: React Native does **not** route `console.log` to `os_log` in
Release (so `simctl log show` sees nothing), and a visually-hidden (`opacity:0`)
accessibility sentinel isn't surfaced by `idb`. The server channel is the one that
works in Release — it's the same `_logs` channel the `currentId`-flip assertion
already uses. The beacon is **not** `__DEV__`-gated (it must fire in Release), and
no-ops on web / until the server address resolves.

The running bundle id is also shown to users in **Settings → About** (a "Bundle"
row), independent of this harness — a human-facing "what's running" indicator.

## Not covered (future work)

Healthy-mark persistence across relaunch and Android. See
`docs/superpowers/specs/2026-06-12-ota-native-e2e-design.md` and
`docs/superpowers/specs/2026-06-24-ota-update-visible-assertion-design.md`.
