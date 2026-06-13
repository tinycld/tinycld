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

## Not covered (future work)

Healthy-mark persistence across relaunch, crash-rollback + server reconcile,
Android, and on-screen UI assertions. See
`docs/superpowers/specs/2026-06-12-ota-native-e2e-design.md`.
