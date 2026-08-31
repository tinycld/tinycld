#!/usr/bin/env bash
# Dev convenience (NOT a test): boot a local hosting router with two orgs
# and a shared app user, then keep serving until Ctrl-C.
#
# What it does:
#   1. starts the fixture npm registry (hosted-npm-registry.mjs) over the base
#      sibling + every feature sibling present, so org lockfiles resolve from
#      the working tree;
#   2. boots serve-router (plain HTTP, base domain `localhost`, unconfined
#      control sockets so the in-app Packages UI works on macOS);
#   3. creates the orgs (default: acme + globex) from a base+features
#      lockfile — the first org pays the real artifact build (minutes warm),
#      the second is a cache hit (seconds). Re-runs skip existing orgs;
#   4. mints each org's tenant superuser from ADMIN_USER_LOGIN/ADMIN_USER_PW
#      (workspace .env; fallbacks below) — sign in at /setup;
#   5. creates the TEST_USER_LOGIN/TEST_USER_PW app user (workspace .env) as
#      a member of EVERY org, so one login works on both subdomains — handy
#      for the cross-org switcher.
#
# Then browse:  http://acme.localhost:<port>  and  http://globex.localhost:<port>
# (Browsers resolve *.localhost to loopback natively; curl needs -H "Host: ...".)
#
# State lives in MT_ROOT (default /tmp/mt-local) and is REUSED across runs —
# artifact cache hits make restarts fast. `rm -rf /tmp/mt-local` for a fresh
# slate.
#
# Env knobs:
#   MT_PORT            Router port (default 7093 — clear of the test runners).
#   MT_ROOT            State root (default /tmp/mt-local; keep it SHORT —
#                      build jobs put TMPDIR under it and long paths break
#                      unix-socket creation).
#   ORGS               Space-separated slugs (default "acme globex").
#   FEATURES           Feature siblings to pack + include in the org lockfile
#                      (default: every feature dir present). FEATURES="" gives
#                      lean base-only orgs (fastest first build).
#   TEST_USER_ROLE     Role for the shared app user (default owner).
#   ADMIN_USER_LOGIN/_PW, TEST_USER_LOGIN/_PW   From workspace .env; already-
#                      exported values win; fallbacks baked below.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WS_ROOT="$(cd "${APP_DIR}/.." && pwd)"
HOSTING_DIR="${WS_ROOT}/hosting"

# Pull credentials from the workspace .env unless already exported.
if [ -f "${WS_ROOT}/.env" ]; then
    for _k in ADMIN_USER_LOGIN ADMIN_USER_PW TEST_USER_LOGIN TEST_USER_PW; do
        if [ -z "$(eval "printf '%s' \"\${${_k}:-}\"")" ]; then
            _v=$(grep -E "^${_k}=" "${WS_ROOT}/.env" | tail -1 | cut -d= -f2-)
            [ -n "${_v}" ] && export "${_k}=${_v}"
        fi
    done
    unset _k _v
fi
ADMIN_USER_LOGIN="${ADMIN_USER_LOGIN:-local-admin@example.com}"
ADMIN_USER_PW="${ADMIN_USER_PW:-LocalAdmin1234!}"
TEST_USER_LOGIN="${TEST_USER_LOGIN:-local-user@example.com}"
TEST_USER_PW="${TEST_USER_PW:-LocalUser1234!}"
TEST_USER_ROLE="${TEST_USER_ROLE:-owner}"
# The node subprocess that builds the user-create JSON reads these from its
# environment — a plain shell assignment is invisible to it (a blank role
# once failed the create with validation_required).
export ADMIN_USER_LOGIN ADMIN_USER_PW TEST_USER_LOGIN TEST_USER_PW TEST_USER_ROLE

PORT="${MT_PORT:-7093}"
BASE_DOMAIN="localhost"
MT_ROOT_DIR="${MT_ROOT:-/tmp/mt-local}"
ORGS="${ORGS:-acme globex}"
SUPERUSER_EMAIL="local-router@example.com"
SUPERUSER_PASSWORD="LocalRouter1234!"

LOG_DIR="${SCRIPT_DIR}/local-hosting-logs"
SERVER_LOG="${LOG_DIR}/serve-router.log"
REGISTRY_LOG="${LOG_DIR}/npm-registry.log"
mkdir -p "${LOG_DIR}" "${MT_ROOT_DIR}"
: >"${SERVER_LOG}"
: >"${REGISTRY_LOG}"

if [ ! -d "${HOSTING_DIR}" ]; then
    echo "hosting repo not found at ${HOSTING_DIR}" >&2
    exit 1
fi

# Default FEATURES = every feature sibling present (manifest.ts + package.json,
# excluding the non-feature members).
if [ -z "${FEATURES+x}" ]; then
    FEATURES=""
    for d in "${WS_ROOT}"/*/; do
        name="$(basename "$d")"
        case "$name" in tinycld | bootstrap | hosting | web | utils | node_modules) continue ;; esac
        if [ -f "$d/manifest.ts" ] && [ -f "$d/package.json" ]; then
            FEATURES="${FEATURES} ${name}"
        fi
    done
fi

REGISTRY_PID=""
SERVER_PID=""
cleanup() {
    [ -n "${SERVER_PID}" ] && kill "${SERVER_PID}" >/dev/null 2>&1 || true
    [ -n "${SERVER_PID}" ] && wait "${SERVER_PID}" 2>/dev/null || true
    [ -n "${REGISTRY_PID}" ] && kill "${REGISTRY_PID}" >/dev/null 2>&1 || true
    echo "[local] stopped (state kept at ${MT_ROOT_DIR} — next start is a cache hit)"
}
trap cleanup EXIT

# ---- 1. fixture npm registry over the working tree ----
PACK_ARGS=(--pack "${WS_ROOT}/tinycld")
for f in ${FEATURES}; do PACK_ARGS+=(--pack "${WS_ROOT}/${f}"); done
echo "[local] starting fixture npm registry (base${FEATURES:+ +${FEATURES}})"
node "${SCRIPT_DIR}/hosted-npm-registry.mjs" "${PACK_ARGS[@]}" >>"${REGISTRY_LOG}" 2>&1 &
REGISTRY_PID=$!

REGISTRY_URL=""
DEADLINE=$((SECONDS + 300))
until [ -n "${REGISTRY_URL}" ]; do
    kill -0 "${REGISTRY_PID}" 2>/dev/null || { tail -20 "${REGISTRY_LOG}" >&2; exit 1; }
    [ ${SECONDS} -ge ${DEADLINE} ] && { echo "registry never became ready" >&2; exit 1; }
    sleep 1
    REGISTRY_URL="$(grep -oE 'REGISTRY_URL=http://[0-9.:]+' "${REGISTRY_LOG}" | head -1 | cut -d= -f2 || true)"
done
echo "[local] registry at ${REGISTRY_URL}"
grep '^PACKED ' "${REGISTRY_LOG}" | sed 's/^/[local]   /'

# Lockfile JSON from the PACKED lines: base + every packed feature.
LOCKFILE_JSON="$(grep '^PACKED ' "${REGISTRY_LOG}" | sed 's/^PACKED //' | node -e '
const lines = require("fs").readFileSync(0, "utf8").trim().split("\n")
const lock = {}
for (const l of lines) {
    const at = l.lastIndexOf("@")
    lock[l.slice(0, at)] = l.slice(at + 1)
}
console.log(JSON.stringify(lock))
')"
echo "[local] org lockfile: ${LOCKFILE_JSON}"

# ---- 2. the router ----
echo "[local] building serve-router"
(cd "${HOSTING_DIR}" && go build -o "${MT_ROOT_DIR}/serve-router" ./cmd/serve-router)

echo "[local] starting router on 127.0.0.1:${PORT} (root ${MT_ROOT_DIR})"
env \
    MT_ROOT="${MT_ROOT_DIR}" \
    MT_BASE_DOMAIN="${BASE_DOMAIN}" \
    MT_ADDR="127.0.0.1:${PORT}" \
    MT_TLS_MODE=proxy \
    MT_SCAFFOLD_ROOT="${WS_ROOT}" \
    MT_NPM_REGISTRY="${REGISTRY_URL}" \
    MT_BUILDER_PNPM_STORE="$(cd "${WS_ROOT}" && pnpm store path 2>/dev/null || true)" \
    MT_ALLOW_UNCONFINED_CONTROL=true \
    MT_SUPERUSER_EMAIL="${SUPERUSER_EMAIL}" \
    MT_SUPERUSER_PASSWORD="${SUPERUSER_PASSWORD}" \
    "${MT_ROOT_DIR}/serve-router" >>"${SERVER_LOG}" 2>&1 &
SERVER_PID=$!

admin_api() {
    local method="$1" path="$2"
    shift 2
    curl -sf -X "${method}" -H "Host: admin.${BASE_DOMAIN}:${PORT}" \
        -H "Content-Type: application/json" "$@" "http://127.0.0.1:${PORT}${path}"
}
json_field() { node -e "console.log(JSON.parse(require('fs').readFileSync(0,'utf8'))$1 ?? '')"; }

DEADLINE=$((SECONDS + 60))
until curl -sf -H "Host: admin.${BASE_DOMAIN}:${PORT}" "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1; do
    kill -0 "${SERVER_PID}" 2>/dev/null || { tail -30 "${SERVER_LOG}" >&2; exit 1; }
    [ ${SECONDS} -ge ${DEADLINE} ] && { echo "control plane never became healthy" >&2; tail -30 "${SERVER_LOG}" >&2; exit 1; }
    sleep 1
done

SU_TOKEN="$(admin_api POST /api/collections/_superusers/auth-with-password \
    -d "{\"identity\":\"${SUPERUSER_EMAIL}\",\"password\":\"${SUPERUSER_PASSWORD}\"}" | json_field '.token')"

org_filter() { python3 -c "import urllib.parse,sys;print(urllib.parse.quote(\"slug='\"+sys.argv[1]+\"'\"))" "$1"; }

org_status() {
    admin_api GET "/api/collections/orgs/records?filter=$(org_filter "$1")" -H "Authorization: ${SU_TOKEN}" \
        | json_field '.items?.[0]?.status'
}

org_recipe_hash() {
    admin_api GET "/api/collections/orgs/records?filter=$(org_filter "$1")" -H "Authorization: ${SU_TOKEN}" \
        | json_field '.items?.[0]?.recipe_hash'
}

wait_org_healthy() {
    local slug="$1" deadline=$((SECONDS + 120))
    until curl -sf -H "Host: ${slug}.${BASE_DOMAIN}:${PORT}" "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1; do
        [ ${SECONDS} -ge ${deadline} ] && { echo "org ${slug} never became healthy" >&2; tail -30 "${SERVER_LOG}" >&2; exit 1; }
        sleep 1
    done
}

# tenant_api <slug> <method> <path> [curl args...]
tenant_api() {
    local slug="$1" method="$2" path="$3"
    shift 3
    curl -s -X "${method}" -H "Host: ${slug}.${BASE_DOMAIN}:${PORT}" \
        -H "Content-Type: application/json" "$@" "http://127.0.0.1:${PORT}${path}"
}

# tenant_su_token <slug> — mints the org's superuser token, retrying through
# the post-resume window (the respawned tenant can briefly refuse before its
# routes are live; an empty token here once sent the app-user create out
# unauthenticated, which PB 400s for the managed verified/role fields).
tenant_su_token() {
    local slug="$1" token="" i
    for i in $(seq 1 15); do
        token="$(tenant_api "${slug}" POST /api/collections/_superusers/auth-with-password \
            -d "{\"identity\":\"${ADMIN_USER_LOGIN}\",\"password\":\"${ADMIN_USER_PW}\"}" \
            | json_field '.token' 2>/dev/null || true)"
        [ -n "${token}" ] && { printf '%s' "${token}"; return 0; }
        sleep 2
    done
    echo "could not authenticate org '${slug}' superuser ${ADMIN_USER_LOGIN}" >&2
    return 1
}

# ---- 3–5. provision each org ----
for slug in ${ORGS}; do
    status="$(org_status "${slug}")"
    if [ "${status}" = "active" ]; then
        echo "[local] org '${slug}' already exists — skipping create"
    else
        echo "[local] creating org '${slug}' (first build takes minutes on a cold cache; later ones are cache hits)"
        admin_api POST /api/orgs -H "Authorization: ${SU_TOKEN}" --max-time 3600 \
            -d "{\"slug\":\"${slug}\",\"display_name\":\"${slug}\",\"lockfile\":${LOCKFILE_JSON}}" >/dev/null \
            || { echo "create org ${slug} failed" >&2; tail -30 "${SERVER_LOG}" >&2; exit 1; }
        echo "[local]   created."
    fi

    # Tenant superuser (idempotent upsert) — run the org's OWN artifact binary
    # against its DB while the tenant process is stopped (no in-band
    # onboarding flow exists yet).
    HASH="$(org_recipe_hash "${slug}")"
    ARTIFACT="${MT_ROOT_DIR}/builds/${HASH#sha256:}/tinycld"
    [ -x "${ARTIFACT}" ] || { echo "artifact binary missing for ${slug} (${HASH})" >&2; exit 1; }
    admin_api POST "/api/orgs/${slug}/suspend" -H "Authorization: ${SU_TOKEN}" >/dev/null
    sleep 2 # let the evicted tenant release the DB
    (cd "${MT_ROOT_DIR}/pb_orgs/${slug}" && "${ARTIFACT}" superuser upsert \
        "${ADMIN_USER_LOGIN}" "${ADMIN_USER_PW}" \
        --dir "${MT_ROOT_DIR}/pb_orgs/${slug}/pb_data") >/dev/null
    admin_api POST "/api/orgs/${slug}/resume" -H "Authorization: ${SU_TOKEN}" >/dev/null
    wait_org_healthy "${slug}"

    # The shared app user, in THIS org's own users collection — same
    # email/password in every org, so one login works on both subdomains.
    TT="$(tenant_su_token "${slug}")"
    CREATE_BODY="$(node -e "console.log(JSON.stringify({
        email: process.env.TEST_USER_LOGIN,
        password: process.env.TEST_USER_PW,
        passwordConfirm: process.env.TEST_USER_PW,
        name: 'Test User',
        username: 'testuser',
        verified: true,
        role: process.env.TEST_USER_ROLE,
    }))")"
    CREATE_RES="$(tenant_api "${slug}" POST /api/collections/users/records \
        -H "Authorization: ${TT}" -d "${CREATE_BODY}")"
    # Ground truth is a real sign-in, not the create's status code: a 400 can
    # mean "already exists" OR a masked validation failure.
    LOGIN_OK="$(tenant_api "${slug}" POST /api/collections/users/auth-with-password \
        -d "{\"identity\":\"${TEST_USER_LOGIN}\",\"password\":\"${TEST_USER_PW}\"}" | json_field '.token')"
    if [ -n "${LOGIN_OK}" ]; then
        echo "[local] org '${slug}': app user ${TEST_USER_LOGIN} signs in (role ${TEST_USER_ROLE})"
    else
        echo "app user in '${slug}' cannot sign in — create response: ${CREATE_RES}" >&2
        exit 1
    fi
done

echo
echo "[local] ✅ ready — serving until Ctrl-C"
for slug in ${ORGS}; do
    echo "[local]   http://${slug}.${BASE_DOMAIN}:${PORT}"
done
echo "[local]   app user:          ${TEST_USER_LOGIN} (member of every org above)"
echo "[local]   org superuser:     ${ADMIN_USER_LOGIN} (sign in at /setup)"
echo "[local]   control plane:     http://127.0.0.1:${PORT} with Host: admin.${BASE_DOMAIN}:${PORT} (${SUPERUSER_EMAIL})"
echo "[local]   router log:        ${SERVER_LOG}"

wait "${SERVER_PID}"
