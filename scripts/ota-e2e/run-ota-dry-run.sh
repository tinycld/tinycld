#!/usr/bin/env bash
# End-to-end local driver for the OTA happy-path native E2E (iOS).
#
# Automates every step the harness needs so the whole flow is one command:
#   1. Pre-flight: docker running, sim UDID, Xcode, free Docker disk.
#   2. Build the server image (the install-harness Dockerfile) unless reused.
#   3. Boot the container on :7090 with bind-mounts; wait for first-boot health.
#   4. Scrape the first-run /admin bootstrap token from the container logs.
#   5. Drive the install spec's first two tests (bootstrap superuser + install
#      @tinycld/todo v1.0.0). The install runs `expo export --platform ios`,
#      minting a `build-<ts>-ios` bundle — the newer bundle the app reloads into.
#   6. Precheck: GET /api/app/update?platform=ios must now return 200 (not 204).
#   7. Build + boot the Release sim (scripts/ios-simulator.sh --prod).
#   8. Seed the app's cached server address (AsyncStorage `tinycld:server:app`)
#      so it auto-connects to :7090 without a manual /connect tap.
#   9. Run the TS assertion (`pnpm run test:e2e:ota`): poll the server's
#      structured _logs until a client reports q.currentId == the new bundle id.
#
# This is LOCAL-ONLY and not wired into CI (needs a Mac + Xcode + a booted sim).
#
# Env knobs:
#   IMAGE=<tag>        Reuse an existing server image (skip the docker build).
#   KEEP=1             Leave the container + sim running after the run (debug).
#   SKIP_INSTALL=1     Assume the container already has a staged ios bundle
#                      (reuse a KEEP=1 container from a prior run); skip step 5.
#   SERVER_PORT=7090   Host port the container serves on (loopback).
#   ADMIN_USER_LOGIN / ADMIN_USER_PW  Superuser creds (default: read from ../.env;
#                      these become OTA_E2E_SUPERUSER_*).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"        # the tinycld member
WS_ROOT="$(cd "${APP_DIR}/.." && pwd)"              # workspace root = docker build context
ENV_FILE="${WS_ROOT}/.env"

CONTAINER="${CONTAINER:-tinycld-ota-server}"
SERVER_PORT="${SERVER_PORT:-7090}"
SERVER_URL="http://localhost:${SERVER_PORT}"
BUNDLE_ID="org.tinycld.app"
LOG_DIR="${SCRIPT_DIR}/ota-dry-run-logs"
CONTAINER_LOG="${LOG_DIR}/container.live.log"
INSTALL_LOG="${LOG_DIR}/install-phase.log"
TAIL_PID=""
MOUNT_ROOT=""

# Minimum free space the Docker VM overlay needs before a rebuild. The image is
# ~8GB and the in-container rebuild (git clones + npm pack + expo export) needs
# several more; a full overlay fails the install with "no space left on device"
# at the resolve step (observed). Guard so that surfaces up-front, not 4 minutes in.
MIN_FREE_GB=15

log() { printf '[ota-dry-run] %s\n' "$*"; }
die() { printf '[ota-dry-run] FAIL: %s\n' "$*" >&2; exit 1; }

cleanup() {
    [ -n "${TAIL_PID}" ] && kill "${TAIL_PID}" >/dev/null 2>&1 || true
    if [ "${KEEP:-}" = "1" ]; then
        log "KEEP=1 — leaving container ${CONTAINER} up (server ${SERVER_URL})"
        [ -n "${MOUNT_ROOT}" ] && log "bind-mounts preserved at ${MOUNT_ROOT}"
        return
    fi
    docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
    [ -n "${MOUNT_ROOT}" ] && log "bind-mounts left at ${MOUNT_ROOT} (pb_data has the DB)"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Step 1 — pre-flight
# ---------------------------------------------------------------------------
preflight() {
    command -v docker >/dev/null || die "docker not found on PATH"
    docker version >/dev/null 2>&1 || die "docker daemon not running"
    command -v xcrun >/dev/null || die "xcrun not found (Xcode command-line tools required)"
    xcodebuild -version >/dev/null 2>&1 || die "Xcode not available (needed for the Release build)"

    # Resolve the simulator UDID the way ios-simulator.sh does.
    if [ -z "${IPHONE_SIMULATOR_UDID:-}" ] && [ -f "${ENV_FILE}" ]; then
        IPHONE_SIMULATOR_UDID="$(grep -E '^IPHONE_SIMULATOR_UDID=' "${ENV_FILE}" | tail -1 | cut -d= -f2- || true)"
    fi
    [ -n "${IPHONE_SIMULATOR_UDID:-}" ] || die "IPHONE_SIMULATOR_UDID not set (in env or ${ENV_FILE})"
    export IPHONE_SIMULATOR_UDID
    xcrun simctl list devices | grep -q "${IPHONE_SIMULATOR_UDID}" \
        || die "simulator ${IPHONE_SIMULATOR_UDID} not found in 'xcrun simctl list devices'"

    # Boot the sim if it isn't already (a Release install needs it running).
    if ! xcrun simctl list devices booted | grep -q "${IPHONE_SIMULATOR_UDID}"; then
        log "booting simulator ${IPHONE_SIMULATOR_UDID}"
        xcrun simctl boot "${IPHONE_SIMULATOR_UDID}" || true
        open -a Simulator || true
    fi

    # Superuser creds (default from .env's ADMIN_USER_*).
    if [ -z "${ADMIN_USER_LOGIN:-}" ] && [ -f "${ENV_FILE}" ]; then
        ADMIN_USER_LOGIN="$(grep -E '^ADMIN_USER_LOGIN=' "${ENV_FILE}" | tail -1 | cut -d= -f2- || true)"
    fi
    if [ -z "${ADMIN_USER_PW:-}" ] && [ -f "${ENV_FILE}" ]; then
        ADMIN_USER_PW="$(grep -E '^ADMIN_USER_PW=' "${ENV_FILE}" | tail -1 | cut -d= -f2- || true)"
    fi
    [ -n "${ADMIN_USER_LOGIN:-}" ] || die "ADMIN_USER_LOGIN not set (in env or ${ENV_FILE})"
    [ -n "${ADMIN_USER_PW:-}" ] || die "ADMIN_USER_PW not set (in env or ${ENV_FILE})"
    export ADMIN_USER_LOGIN ADMIN_USER_PW

    mkdir -p "${LOG_DIR}"
    log "pre-flight OK — sim ${IPHONE_SIMULATOR_UDID}, server ${SERVER_URL}, superuser ${ADMIN_USER_LOGIN}"
}

# Guard against a full Docker VM disk (the failure that aborts the rebuild).
check_docker_disk() {
    # Reclaim freely-prunable space first (build cache + dangling images never
    # include our running container's image), then assert headroom.
    docker builder prune -af >/dev/null 2>&1 || true
    docker image prune -f >/dev/null 2>&1 || true
    # Free GB on the Docker VM root, read from a throwaway alpine (df in the VM).
    local free_gb
    free_gb="$(docker run --rm alpine:latest df -BG / 2>/dev/null \
        | awk 'NR==2 {gsub(/G/,"",$4); print $4}')" || free_gb=""
    if [ -n "${free_gb}" ] && [ "${free_gb}" -lt "${MIN_FREE_GB}" ]; then
        die "Docker VM has only ${free_gb}GB free (need ≥${MIN_FREE_GB}GB). Reclaim with 'docker system prune -af' (note: removes unused images)."
    fi
    log "Docker disk OK (~${free_gb:-?}GB free on the VM)"
}

# ---------------------------------------------------------------------------
# Step 2/3 — build image + boot container
# ---------------------------------------------------------------------------
build_image() {
    if [ -n "${IMAGE:-}" ]; then
        log "reusing image ${IMAGE} (skipping build)"
        return
    fi
    IMAGE=tinycld-ota-server
    check_docker_disk
    log "building ${IMAGE} from ${WS_ROOT} (several minutes)…"
    docker build -f "${APP_DIR}/Dockerfile" -t "${IMAGE}" "${WS_ROOT}" \
        || die "docker build failed"
}

boot_container() {
    MOUNT_ROOT="$(mktemp -d -t tinycld-ota-mounts.XXXXXX)"
    mkdir -p "${MOUNT_ROOT}/pb_data" "${MOUNT_ROOT}/builds" "${MOUNT_ROOT}/releases"
    log "bind-mounts under ${MOUNT_ROOT}"

    docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
    docker run -d --name "${CONTAINER}" -p "${SERVER_PORT}:7090" \
        -v "${MOUNT_ROOT}/pb_data:/workspace/pb_data" \
        -v "${MOUNT_ROOT}/builds:/workspace/builds" \
        -v "${MOUNT_ROOT}/releases:/workspace/releases" \
        "${IMAGE}" >/dev/null || die "docker run failed"

    : > "${CONTAINER_LOG}"
    docker logs -f "${CONTAINER}" >> "${CONTAINER_LOG}" 2>&1 &
    TAIL_PID=$!

    log "waiting for first-boot health on ${SERVER_URL} (≤300s)…"
    local i
    for i in $(seq 1 300); do
        if curl -sf "${SERVER_URL}/api/health" >/dev/null 2>&1; then
            log "container healthy after ${i}s"
            return 0
        fi
        sleep 1
    done
    tail -30 "${CONTAINER_LOG}" >&2 || true
    die "container never became healthy within 300s"
}

scrape_token() {
    local i
    TOKEN=""
    for i in $(seq 1 30); do
        TOKEN="$(docker logs "${CONTAINER}" 2>&1 | grep -oE 'token=[a-f0-9]+' | head -1 | cut -d= -f2 || true)"
        [ -n "${TOKEN}" ] && break
        sleep 1
    done
    [ -n "${TOKEN}" ] || die "no first-run /admin bootstrap token printed within 30s"
    log "scraped bootstrap token (${#TOKEN} chars)"
}

# ---------------------------------------------------------------------------
# Step 5 — mint the ios bundle via the install spec's first two tests
# ---------------------------------------------------------------------------
# Returns 0 if a superuser already exists on the server (auth succeeds).
superuser_exists() {
    local code
    code="$(curl -s -o /dev/null -w '%{http_code}' \
        -X POST "${SERVER_URL}/api/collections/_superusers/auth-with-password" \
        -H 'Content-Type: application/json' \
        -d "{\"identity\":\"${ADMIN_USER_LOGIN}\",\"password\":\"${ADMIN_USER_PW}\"}")"
    [ "${code}" = "200" ]
}

mint_ios_bundle() {
    if [ "${SKIP_INSTALL:-}" = "1" ]; then
        log "SKIP_INSTALL=1 — assuming a staged ios bundle already exists"
        return
    fi
    log "ensuring chromium for playwright"
    (cd "${APP_DIR}" && pnpm exec playwright install chromium >/dev/null 2>&1) || true

    # The /admin?token= setup wizard only renders while pb_data has NO superuser,
    # and the token is one-shot. On a fresh container we run bootstrap+install; on
    # a reused (already-bootstrapped) container we run install-only, which logs in
    # as the existing superuser. Pick the right test set by probing for the user.
    local grep_expr
    if superuser_exists; then
        log "superuser already exists — running install-only (skipping the one-shot bootstrap)"
        grep_expr='install @tinycld/todo pinned to v1.0.0'
    else
        log "fresh container — running bootstrap + install"
        grep_expr='bootstrap superuser via /admin wizard|install @tinycld/todo pinned to v1.0.0'
    fi

    log "installing @tinycld/todo v1.0.0 (mints the ios bundle via expo export — several minutes)…"
    (
        cd "${APP_DIR}/tests/install"
        PW_BASE_URL="${SERVER_URL}" \
        PW_TODO_SETUP_TOKEN="${TOKEN}" \
        ADMIN_USER_LOGIN="${ADMIN_USER_LOGIN}" \
        ADMIN_USER_PW="${ADMIN_USER_PW}" \
        RUN_TODO_INSTALL_TEST=1 \
        CI=true FORCE_COLOR=0 \
        pnpm exec playwright test todo-install.spec.ts --reporter=line -g "${grep_expr}"
    ) > "${INSTALL_LOG}" 2>&1 || {
        tail -40 "${INSTALL_LOG}" >&2 || true
        die "install phase failed (see ${INSTALL_LOG} and ${CONTAINER_LOG})"
    }
    log "install phase complete"
}

# ---------------------------------------------------------------------------
# Step 6 — precheck the server now offers a newer ios bundle
# ---------------------------------------------------------------------------
precheck_bundle() {
    local version embedded code
    version="$(node -p "require('${APP_DIR}/app.json').expo.version")"
    embedded="embedded-${version}"
    code="$(curl -s -o /dev/null -w '%{http_code}' \
        "${SERVER_URL}/api/app/update?platform=ios&runtimeVersion=${version}&currentId=${embedded}&currentHash=")"
    if [ "${code}" = "204" ]; then
        die "server still returns 204 for platform=ios — no native bundle was staged (the install didn't export one)"
    fi
    [ "${code}" = "200" ] || die "unexpected precheck status ${code} (want 200)"
    log "precheck OK — server offers a newer ios bundle (HTTP 200 for ${embedded})"
}

# ---------------------------------------------------------------------------
# Step 7/8 — build+boot the Release sim, then seed its cached server address
# ---------------------------------------------------------------------------
build_release_sim() {
    # Kill any stale expo run:ios so it doesn't contend for the sim/build system.
    pkill -f "expo run:ios.*${IPHONE_SIMULATOR_UDID}" >/dev/null 2>&1 || true

    log "building + booting Release on ${IPHONE_SIMULATOR_UDID} (Xcode build — several minutes)…"
    # ios-simulator.sh --prod runs `npx expo run:ios --configuration Release`, which
    # builds, installs, launches — then STAYS FOREGROUND tailing device logs and
    # never exits on its own. So run it in the background and gate on the app's data
    # container appearing (the signal the install finished), then kill the tailer.
    # The cached-address seed (next step) points the app at our server; no server
    # addr is passed to the build (the app reads none from env).
    local build_log="${LOG_DIR}/release-build.log"
    (cd "${APP_DIR}" && ./scripts/ios-simulator.sh --prod) > "${build_log}" 2>&1 &
    local build_pid=$!

    # Wait up to ~12 min for the install to land (cold Xcode build is slow). The
    # data container exists once the app is installed; that's our "build done" gate.
    local i
    for i in $(seq 1 720); do
        if ! kill -0 "${build_pid}" 2>/dev/null; then
            # The build process exited before we saw an install — a real failure.
            tail -30 "${build_log}" >&2 || true
            die "ios-simulator.sh --prod exited before the app installed (see ${build_log})"
        fi
        if xcrun simctl get_app_container "${IPHONE_SIMULATOR_UDID}" "${BUNDLE_ID}" data >/dev/null 2>&1 \
            && grep -q "Logs for your project will appear below" "${build_log}" 2>/dev/null; then
            log "Release app installed + launched after ~${i}s"
            break
        fi
        sleep 1
    done

    # Stop the foreground log-tailer so the script can proceed; the app stays
    # installed + running on the sim.
    kill "${build_pid}" >/dev/null 2>&1 || true
    pkill -f "expo run:ios.*${IPHONE_SIMULATOR_UDID}" >/dev/null 2>&1 || true

    xcrun simctl get_app_container "${IPHONE_SIMULATOR_UDID}" "${BUNDLE_ID}" data >/dev/null 2>&1 \
        || die "Release app never installed within the build window (see ${build_log})"
}

seed_server_address() {
    # The app resolves its server from AsyncStorage key `tinycld:server:app`
    # (core/lib/server-address.ts). The community AsyncStorage (2.2.0) stores small
    # values inline in a manifest.json, located under the app's data container at
    # `Library/Application Support/<bundle-id>/RCTAsyncLocalStorage_V1/` (NOT
    # Documents/ — verified empirically). Seeding it lets the app auto-connect to
    # our server with no /connect tap. Terminate first: the manifest is read at
    # startup and rewritten after mutations, so a running app would clobber the seed.
    local container_dir storage_dir manifest
    container_dir="$(xcrun simctl get_app_container "${IPHONE_SIMULATOR_UDID}" "${BUNDLE_ID}" data 2>/dev/null)" \
        || die "could not locate ${BUNDLE_ID} data container — is the app installed?"

    xcrun simctl terminate "${IPHONE_SIMULATOR_UDID}" "${BUNDLE_ID}" >/dev/null 2>&1 || true

    # Prefer an existing manifest (the app creates the real one once it has run);
    # fall back to the canonical Application Support path for a never-launched app.
    manifest="$(find "${container_dir}/Library/Application Support" \
        -path '*RCTAsyncLocalStorage_V1/manifest.json' 2>/dev/null | head -1)"
    if [ -z "${manifest}" ]; then
        storage_dir="${container_dir}/Library/Application Support/${BUNDLE_ID}/RCTAsyncLocalStorage_V1"
        mkdir -p "${storage_dir}"
        manifest="${storage_dir}/manifest.json"
    fi

    # Merge our key into the manifest, preserving any other keys the app wrote.
    node -e '
        const fs = require("fs");
        const path = process.argv[1];
        const url = process.argv[2];
        let obj = {};
        try { obj = JSON.parse(fs.readFileSync(path, "utf8")); } catch {}
        obj["tinycld:server:app"] = url;
        fs.writeFileSync(path, JSON.stringify(obj));
        console.log("seeded " + path);
    ' "${manifest}" "${SERVER_URL}"

    # Relaunch so the app reads the freshly-seeded address.
    xcrun simctl launch "${IPHONE_SIMULATOR_UDID}" "${BUNDLE_ID}" >/dev/null 2>&1 || true
    log "seeded cached server address (${SERVER_URL}) and relaunched the app"
}

# ---------------------------------------------------------------------------
# Step 9 — run the TS assertion (polls _logs for the bundle-id flip)
# ---------------------------------------------------------------------------
assert_flip() {
    log "running the OTA flip assertion (polling _logs)…"
    (
        cd "${APP_DIR}"
        OTA_E2E_SERVER_URL="${SERVER_URL}" \
        OTA_E2E_SUPERUSER_EMAIL="${ADMIN_USER_LOGIN}" \
        OTA_E2E_SUPERUSER_PASSWORD="${ADMIN_USER_PW}" \
        OTA_E2E_SKIP_BUILD=1 \
        pnpm run test:e2e:ota
    ) || die "OTA flip assertion failed (see above + ${CONTAINER_LOG})"
    log "✅ OTA dry run PASSED — the app reloaded into the new bundle"
}

main() {
    preflight
    build_image
    boot_container
    scrape_token
    mint_ios_bundle
    precheck_bundle
    build_release_sim
    seed_server_address
    assert_flip
}

main "$@"
