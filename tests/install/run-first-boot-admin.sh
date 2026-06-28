#!/usr/bin/env bash
# Smoke test for the first-boot /admin console: builds (or reuses) a TinyCld
# image, boots it on a FRESH blank DB so the first-run setup token is printed,
# scrapes that token, and runs setup-and-packages.spec.ts against the live
# container — the bootstrap wizard, the package list, org creation, super-admin
# grant, and the system-settings panels (Sentry DSN injection + VAPID generate).
#
# This is the runner setup-and-packages.spec.ts always needed (run-todo-install.sh
# only drives the heavy todo-install spec). Always boots a clean DB so the run is
# deterministic and self-isolating — no hand-running against a reused container.
#
# Env knobs (mirror run-todo-install.sh):
#   IMAGE=<tag>   Skip the build and test an existing local image tag.
#   PULL=1        Pull from a registry before running (skips the build).
#   KEEP=1        Leave the container running afterward (manual debugging).
#   PW_BASE_URL   Override the base URL (default http://localhost:7090).
set -euo pipefail

DEFAULT_PULL_IMAGE="ghcr.io/tinycld/tinycld:latest"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WS_ROOT="$(cd "${APP_DIR}/.." && pwd)"

CONTAINER=tinycld-first-boot-test
BASE_URL="${PW_BASE_URL:-http://localhost:7090}"
LOG_DIR="${SCRIPT_DIR}/first-boot-logs"
LIVE_LOG="${LOG_DIR}/container.live.log"
TAIL_PID=""

# Image source: PULL=1 → pull; IMAGE=<tag> → reuse a local tag; neither → build
# from the current working tree.
BUILD_IMAGE=1
PULL_IMAGE=0
if [ "${PULL:-}" = "1" ]; then
    BUILD_IMAGE=0
    PULL_IMAGE=1
    IMAGE="${IMAGE:-${DEFAULT_PULL_IMAGE}}"
elif [ -n "${IMAGE:-}" ]; then
    BUILD_IMAGE=0
else
    IMAGE=tinycld-first-boot-test
fi

cleanup() {
    if [ -n "${TAIL_PID}" ]; then kill "${TAIL_PID}" >/dev/null 2>&1 || true; fi
    if [ "${KEEP:-}" = "1" ]; then
        echo "[runner] KEEP=1 — leaving container ${CONTAINER} up (live log: ${LIVE_LOG})"
        return
    fi
    docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
    [ -n "${MOUNT_ROOT:-}" ] && echo "[runner] bind-mounts at ${MOUNT_ROOT}"
}
trap cleanup EXIT

dump_logs() {
    mkdir -p "${LOG_DIR}"
    docker logs "${CONTAINER}" > "${LOG_DIR}/container.log" 2>&1 || true
    echo "[runner] --- last 40 lines of container log ---"
    tail -40 "${LOG_DIR}/container.log" || true
}

wait_healthy() {
    local timeout="${1:-300}"
    for i in $(seq 1 "${timeout}"); do
        if curl -sf "${BASE_URL}/api/health" >/dev/null 2>&1; then
            echo "[runner] healthy after ${i}s"
            return 0
        fi
        sleep 1
    done
    echo "[runner] ERROR: container never became healthy (waited ${timeout}s)" >&2
    dump_logs
    return 1
}

# 1. Obtain the image.
if [ "${PULL_IMAGE}" = "1" ]; then
    echo "[runner] pulling ${IMAGE}"
    docker pull "${IMAGE}"
elif [ "${BUILD_IMAGE}" = "1" ]; then
    echo "[runner] building ${IMAGE} from ${WS_ROOT}"
    docker build -f "${APP_DIR}/Dockerfile" -t "${IMAGE}" "${WS_ROOT}"
else
    echo "[runner] using existing local image ${IMAGE}"
fi

# 2. Boot on a FRESH blank DB. The setup token is only printed when pb_data has no
#    superusers, so a clean mount guarantees it — and makes every run isolated.
MOUNT_ROOT="$(mktemp -d -t tinycld-first-boot.XXXXXX)"
mkdir -p "${MOUNT_ROOT}/pb_data" "${MOUNT_ROOT}/builds" "${MOUNT_ROOT}/releases"
docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
docker run -d --name "${CONTAINER}" -p 7090:7090 \
    -v "${MOUNT_ROOT}/pb_data:/workspace/pb_data" \
    -v "${MOUNT_ROOT}/builds:/workspace/builds" \
    -v "${MOUNT_ROOT}/releases:/workspace/releases" \
    "${IMAGE}" >/dev/null

mkdir -p "${LOG_DIR}"
: > "${LIVE_LOG}"
docker logs -f "${CONTAINER}" >> "${LIVE_LOG}" 2>&1 &
TAIL_PID=$!

# 3. Wait for first-boot health (cold boot does schema-gen + seeding).
wait_healthy 300

# 4. Scrape the first-run token. It's printed near the end of boot (after health
#    answers), so poll the logs for up to 30s.
TOKEN=""
for i in $(seq 1 30); do
    TOKEN=$(docker logs "${CONTAINER}" 2>&1 | grep -oE 'token=[a-f0-9]+' | head -1 | cut -d= -f2 || true)
    [ -n "${TOKEN}" ] && break
    sleep 1
done
if [ -z "${TOKEN}" ]; then
    echo "[runner] ERROR: no bootstrap token printed in logs after 30s" >&2
    dump_logs
    exit 1
fi
echo "[runner] scraped bootstrap token (${#TOKEN} chars)"

# 5. Ensure chromium is present (no-op once cached), then run the spec in place
#    against the live container.
echo "[runner] ensuring chromium is installed for playwright"
(cd "${APP_DIR}" && pnpm exec playwright install chromium >/dev/null)

echo "[runner] running setup-and-packages.spec.ts"
(
    cd "${SCRIPT_DIR}"
    PW_BASE_URL="${BASE_URL}" \
    PW_SETUP_TOKEN="${TOKEN}" \
    CI=true FORCE_COLOR=0 \
    pnpm exec playwright test setup-and-packages.spec.ts --reporter=line,list
) || { echo "[runner] spec failed"; dump_logs; exit 1; }

echo "[runner] ✅ first-boot /admin smoke test passed"
