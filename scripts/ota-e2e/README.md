# OTA happy-path native E2E (local-only, iOS)

Proves the over-the-air update pipe end-to-end on an iOS simulator: a **Release**
build launches on its embedded JS bundle, auto-checks the connected server, and
reloads into a newer server-served bundle. The harness observes the flip via the
server's existing `app-update: request` log line — no app/server changes, no
Maestro/Detox.

## What it does

1. Reads the embedded bundle id from `app.json` (`embedded-<version>`).
2. Prechecks `GET /api/app/update` — the server must already offer a newer iOS
   bundle (`build-<ts>-ios`). Fails loudly if not (status 204).
3. Builds + boots a Release sim via `scripts/ios-simulator.sh --prod`, pointed at
   the server through `EXPO_PUBLIC_PB_SERVER_ADDR`.
4. Tails the server log and passes when the client's reported `q.currentId`
   flips from the embedded id to the new bundle id.

## Prerequisites (manual, one-time per run)

- A booted iOS simulator; its UDID in the workspace-root `.env` as
  `IPHONE_SIMULATOR_UDID` (same var `ios-simulator.sh` uses).
- Xcode + the iOS toolchain (Release build runs `expo run:ios`).
- A **running local server on a loopback host** (e.g. `http://localhost:7200`,
  the `expo:test` port) **whose stdout is captured to a file**, AND which holds a
  newer iOS bundle than the app's embedded one. Two ways to get the bundle:
  - Trigger a server-side rebuild/install (the path
    `tests/install/run-todo-install.sh` drives), which runs `expo export
    --platform ios` and writes a `pkg_build` record; or
  - Point at an install-harness container that already built one.

  Capture stdout when you launch the server, e.g.:

  ```sh
  pnpm run expo:test 2>&1 | tee /tmp/ota-server.log
  ```

## Run

```sh
cd ~/code/tinycld/tinycld
OTA_E2E_SERVER_URL=http://localhost:7200 \
OTA_E2E_SERVER_LOG=/tmp/ota-server.log \
pnpm run test:e2e:ota
```

## Env knobs

| Var | Default | Meaning |
|---|---|---|
| `OTA_E2E_SERVER_URL` | `http://localhost:7200` | Server the sim connects to (must be loopback for plaintext). |
| `OTA_E2E_SERVER_LOG` | _(required)_ | File capturing the server's stdout. |
| `IPHONE_SIMULATOR_UDID` | from `.env` | Target simulator. |
| `OTA_E2E_TIMEOUT_MS` | `180000` | Max wait for the reload flip. |

## Interpreting failures

- **204 at precheck** → no newer bundle staged; the server build/export step
  didn't run or produced no iOS bundle.
- **timed out; last-seen currentId=embedded-…** → the app never reloaded. Check:
  build is genuinely Release (not Debug/Metro), server host is loopback
  (transport gating), `__DEV__`/web guards aren't active.
- **stream ended before a match** → the captured server log file stopped before
  the reload; confirm the server is still running and still writing to the file.
- **build/boot exited non-zero** → an `expo run:ios --configuration Release`
  failure; see the inline build output.

## Not covered (future work)

Healthy-mark persistence across relaunch, crash-rollback + server reconcile,
Android, and on-screen UI assertions. See
`docs/superpowers/specs/2026-06-12-ota-native-e2e-design.md`.
