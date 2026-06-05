#!/usr/bin/env bash
# Local runner for the todo-install integration test. Builds a TinyCld
# image from the CURRENT working tree (so the git-spec validation change is
# present), boots it, scrapes the setup token, runs the Playwright spec in a
# standalone sandbox, and tears the container down.
#
# Env knobs:
#   IMAGE=<tag>   Skip the build and test an existing image tag.
#   KEEP=1        Leave the container running after the run (manual debug).
#   PW_BASE_URL   Override the base URL (default http://localhost:7090).
set -euo pipefail

# Resolve paths. This script lives at app/tests/install/; the app member is
# two levels up, and the workspace root (the docker build context) is three.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WS_ROOT="$(cd "${APP_DIR}/.." && pwd)"

CONTAINER=tinycld-todo-test
BASE_URL="${PW_BASE_URL:-http://localhost:7090}"
LOG_DIR="${SCRIPT_DIR}/todo-install-logs"
LIVE_LOG="${LOG_DIR}/container.live.log"
TAIL_PID=""

# IMAGE defaults to a local build tag; if the caller sets IMAGE we skip the build.
BUILD_IMAGE=1
if [ -n "${IMAGE:-}" ]; then
    BUILD_IMAGE=0
else
    IMAGE=tinycld-todo-test
fi

cleanup() {
    # Stop the live log tail first so it doesn't dangle.
    if [ -n "${TAIL_PID}" ]; then
        kill "${TAIL_PID}" >/dev/null 2>&1 || true
    fi
    if [ "${KEEP:-}" = "1" ]; then
        echo "[runner] KEEP=1 — leaving container ${CONTAINER} up"
        echo "[runner] live container log: ${LIVE_LOG}"
        return
    fi
    docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Start streaming the container's stdout/stderr to a file in the background so
# the full install trace (npm pack / pnpm / go build / expo, now echoed to the
# server's stdout) is captured LIVE — even if the test later hangs. A final
# dump_logs still snapshots the complete log on failure.
start_live_log() {
    mkdir -p "${LOG_DIR}"
    : > "${LIVE_LOG}"
    docker logs -f "${CONTAINER}" >> "${LIVE_LOG}" 2>&1 &
    TAIL_PID=$!
    echo "[runner] streaming container logs → ${LIVE_LOG} (pid ${TAIL_PID})"
}

dump_logs() {
    mkdir -p "${LOG_DIR}"
    docker logs "${CONTAINER}" > "${LOG_DIR}/container.log" 2>&1 || true
    echo "[runner] container logs → ${LOG_DIR}/container.log"
    echo "[runner] (live trace also at ${LIVE_LOG})"
    echo "[runner] --- last 40 lines of container log ---"
    tail -40 "${LOG_DIR}/container.log" || true
    echo "[runner] --- end container log tail ---"
}

wait_healthy() {
    local label="$1"
    for i in $(seq 1 120); do
        if curl -sf "${BASE_URL}/api/health" >/dev/null 2>&1; then
            echo "[runner] ${label}: healthy after ${i}s"
            return 0
        fi
        sleep 1
    done
    echo "[runner] ERROR: ${label}: container never became healthy" >&2
    dump_logs
    return 1
}

# Wait for the server to go DOWN after a restart was requested. Returns as
# soon as /api/health stops responding. If it never goes down within the
# window (a restart faster than our poll, or one we missed), warn and return
# 0 — the subsequent wait_healthy still gates on the server being back up, so
# the worst case is we proceed against a server that never actually restarted,
# which the post-restart test's own login-retry loop will still exercise.
wait_unhealthy() {
    local label="$1"
    for i in $(seq 1 60); do
        if ! curl -sf "${BASE_URL}/api/health" >/dev/null 2>&1; then
            echo "[runner] ${label}: server went down after ${i}s"
            return 0
        fi
        sleep 1
    done
    echo "[runner] WARN: ${label}: server never went down within 60s (fast or missed restart)"
    return 0
}

# 1. Build image from the working tree (unless IMAGE points at an existing tag).
if [ "${BUILD_IMAGE}" = "1" ]; then
    echo "[runner] building ${IMAGE} from ${WS_ROOT}"
    echo "[runner] NOTE: the build context is the assembled workspace root."
    echo "[runner] If 'pnpm install --frozen-lockfile' fails on a missing member,"
    echo "[runner] the local workspace has members the Dockerfile doesn't COPY —"
    echo "[runner] that surfaces here as a build failure, by design."
    docker build -f "${APP_DIR}/Dockerfile" -t "${IMAGE}" "${WS_ROOT}"
else
    echo "[runner] using existing image ${IMAGE} (skipping build)"
fi

# 2. Boot.
docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
docker run -d --name "${CONTAINER}" -p 7090:7090 "${IMAGE}"

# 2a. Begin streaming container logs to a file immediately, so we capture the
# full install trace live even if a later step hangs.
start_live_log

# 3. Wait for first-boot health.
wait_healthy "first boot"

# 4. Scrape the setup token from logs.
TOKEN=$(docker logs "${CONTAINER}" 2>&1 | grep -oE 'token=[a-f0-9]+' | head -1 | cut -d= -f2 || true)
if [ -z "${TOKEN}" ]; then
    echo "[runner] ERROR: no setup token printed in logs" >&2
    dump_logs
    exit 1
fi
echo "[runner] scraped setup token (${#TOKEN} chars)"

# 5. Build the standalone Playwright sandbox (mirrors smoke-test-image.yml):
#    @playwright/test installed locally + the spec + config copied in + a
#    minimal tsconfig so the TS loader doesn't walk up to app/tsconfig.json.
PW_VER=$(node -e "console.log(require('${APP_DIR}/package.json').devDependencies['@playwright/test'].replace(/^[\^~>=<]+/, ''))")
PW_ROOT=$(mktemp -d)
echo "[runner] sandbox at ${PW_ROOT} (playwright ${PW_VER})"
(
    cd "${PW_ROOT}"
    npm init -y >/dev/null
    npm install --ignore-scripts "@playwright/test@${PW_VER}" >/dev/null 2>&1
    cat > tsconfig.json <<'TSCFG'
{ "compilerOptions": { "target": "es2022", "module": "commonjs", "moduleResolution": "node", "esModuleInterop": true, "strict": false, "skipLibCheck": true } }
TSCFG
    cp "${SCRIPT_DIR}/playwright.config.ts" .
    cp "${SCRIPT_DIR}/todo-install.spec.ts" .
    ./node_modules/.bin/playwright install --with-deps chromium >/dev/null
)

# 6. Run bootstrap + install tests (everything except the post-restart check).
echo "[runner] running bootstrap + install"
(
    cd "${PW_ROOT}"
    PW_BASE_URL="${BASE_URL}" \
    PW_TODO_SETUP_TOKEN="${TOKEN}" \
    CI=true FORCE_COLOR=0 \
    ./node_modules/.bin/playwright test --reporter=line,list \
        -g 'bootstrap|install @tinycld/todo'
) || { echo "[runner] install phase failed"; dump_logs; exit 1; }

# 7. The install requested a restart. Wait for the container to come back.
# The restart kills the `docker logs -f` stream, so re-attach it after the
# container is healthy again to keep capturing the post-restart boot trace.
echo "[runner] waiting for post-install restart"
wait_unhealthy "restart down"   # observe the old server exit first
wait_healthy "post-restart"     # then wait for the new binary to come up
start_live_log                  # re-attach the live log to the restarted container

# 8. Run the post-restart verification test.
echo "[runner] running post-restart verification"
(
    cd "${PW_ROOT}"
    PW_BASE_URL="${BASE_URL}" \
    CI=true FORCE_COLOR=0 \
    ./node_modules/.bin/playwright test --reporter=line,list \
        -g 'after restart'
) || { echo "[runner] post-restart phase failed"; dump_logs; exit 1; }

echo "[runner] ✅ todo install integration test passed"
