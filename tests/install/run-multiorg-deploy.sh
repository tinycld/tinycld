#!/usr/bin/env bash
# Local runner for the multi-org package-deploy integration test — the
# multi-org sibling of run-todo-install.sh. Builds serve-multi + serve-org
# from the multi-org sibling repo's working tree, boots the router on plain
# HTTP (MT_TLS_MODE=proxy) with a localhost base domain, runs
# multiorg-deploy.spec.ts against it over real HTTP, and tears it down.
#
# Unlike the single-tenant runner there is no docker image and no exit-75
# restart protocol: the router is one host process that lazily spawns per-org
# serve-org children, and a deploy is re-materialize + evict (the next
# request respawns the tenant). On macOS the spawner is a plain subprocess
# (no uid/namespace confinement — that's Linux-only and not under test here).
#
# When DESIGN-org-package-agency §7 step 4 lands (tenant-side Packages UI),
# todo-install.spec.ts itself becomes runnable against an org subdomain and
# supersedes most of this; the spec here keeps covering the router-side
# operator flow (publish/create/deploy over HTTP).
#
# Env knobs:
#   MT_PORT                  Router port (default 7091 — 7090 is the todo
#                            container, 7200 the e2e webServer).
#   MT_SUPERUSER_EMAIL/_PASSWORD
#                            Control-plane superuser (defaults baked below,
#                            mirroring the spec's fallbacks).
#   KEEP=1                   Leave the router (and its work dir) running
#                            after the run for manual debugging.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WS_ROOT="$(cd "${APP_DIR}/.." && pwd)"
MULTIORG_DIR="${WS_ROOT}/multi-org"

PORT="${MT_PORT:-7091}"
BASE_DOMAIN="localhost"
SUPERUSER_EMAIL="${MT_SUPERUSER_EMAIL:-multiorg-smoke@example.com}"
SUPERUSER_PASSWORD="${MT_SUPERUSER_PASSWORD:-MultiOrgSmoke1234!}"

LOG_DIR="${SCRIPT_DIR}/multiorg-deploy-logs"
SERVER_LOG="${LOG_DIR}/serve-multi.log"
mkdir -p "${LOG_DIR}"
: >"${SERVER_LOG}"

if [ ! -d "${MULTIORG_DIR}" ]; then
    echo "multi-org repo not found at ${MULTIORG_DIR} — assemble it as a workspace sibling first" >&2
    exit 1
fi

WORK_DIR="$(mktemp -d -t tinycld-multiorg-XXXXXX)"
BIN_DIR="${WORK_DIR}/bin"
MT_ROOT_DIR="${WORK_DIR}/mt_root"
mkdir -p "${BIN_DIR}" "${MT_ROOT_DIR}"

SERVER_PID=""

cleanup() {
    if [ "${KEEP:-}" = "1" ]; then
        echo "KEEP=1 — router left running (pid ${SERVER_PID}, root ${MT_ROOT_DIR}, log ${SERVER_LOG})"
        return
    fi
    if [ -n "${SERVER_PID}" ]; then
        # SIGTERM the router; its shutdown reaps the per-org tenant children.
        kill "${SERVER_PID}" >/dev/null 2>&1 || true
        wait "${SERVER_PID}" 2>/dev/null || true
    fi
    rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

echo "== Building serve-multi + serve-org from ${MULTIORG_DIR}"
(cd "${MULTIORG_DIR}" && go build -o "${BIN_DIR}/serve-multi" ./cmd/serve-multi)
(cd "${MULTIORG_DIR}" && go build -o "${BIN_DIR}/serve-org" ./cmd/serve-org)

echo "== Starting router on 127.0.0.1:${PORT} (base domain ${BASE_DOMAIN}, root ${MT_ROOT_DIR})"
env \
    MT_ROOT="${MT_ROOT_DIR}" \
    MT_BASE_DOMAIN="${BASE_DOMAIN}" \
    MT_ADDR="127.0.0.1:${PORT}" \
    MT_TLS_MODE=proxy \
    MT_TENANT_BINARY="${BIN_DIR}/serve-org" \
    MT_SUPERUSER_EMAIL="${SUPERUSER_EMAIL}" \
    MT_SUPERUSER_PASSWORD="${SUPERUSER_PASSWORD}" \
    "${BIN_DIR}/serve-multi" >>"${SERVER_LOG}" 2>&1 &
SERVER_PID=$!

echo "== Waiting for the control plane to answer /api/health"
HEALTH_DEADLINE=$((SECONDS + 60))
until curl -sf -H "Host: admin.${BASE_DOMAIN}:${PORT}" "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1; do
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
        echo "serve-multi exited during startup — log tail:" >&2
        tail -30 "${SERVER_LOG}" >&2
        exit 1
    fi
    if [ ${SECONDS} -ge ${HEALTH_DEADLINE} ]; then
        echo "control plane never became healthy — log tail:" >&2
        tail -30 "${SERVER_LOG}" >&2
        exit 1
    fi
    sleep 1
done
echo "   healthy."

echo "== Running multiorg-deploy.spec.ts"
STATUS=0
(
    cd "${SCRIPT_DIR}"
    env \
        RUN_MULTIORG_DEPLOY_TEST=1 \
        PW_MULTIORG_URL="http://127.0.0.1:${PORT}" \
        PW_MULTIORG_BASE_DOMAIN="${BASE_DOMAIN}" \
        MT_SUPERUSER_EMAIL="${SUPERUSER_EMAIL}" \
        MT_SUPERUSER_PASSWORD="${SUPERUSER_PASSWORD}" \
        CI=true FORCE_COLOR=0 \
        pnpm exec playwright test multiorg-deploy.spec.ts --reporter=line
) || STATUS=$?

if [ ${STATUS} -ne 0 ]; then
    echo "== FAILED (status ${STATUS}) — router log tail:" >&2
    tail -60 "${SERVER_LOG}" >&2
else
    echo "== PASSED"
fi
exit ${STATUS}
