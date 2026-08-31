#!/usr/bin/env bash
# Local runner for the HOSTED (hosting) browser-level install suite — the
# finish line of DESIGN-org-package-agency goal 2 ("same UX"): the same
# todo-install.spec.ts phases the single-tenant runner drives, run against an
# ORG SUBDOMAIN of a real serve-router router. Everything the org admin does
# happens in the browser through the org's own SPA (served by the tenant from
# its artifact's pb_public); every install-class operation rides the hosted
# pipeline — router-side build, in-tenant downs, deploy proposal, evict +
# respawn — instead of the single-tenant exit-75 rebuild.
#
# What runs, and what deliberately does not:
#   RUNS   install v1 → verify v1 (+ add a todo) → upgrade v2 → verify v2
#          (+ tag a todo) → downgrade v1 (in-tenant DOWNs) → verify downs →
#          uninstall → verify registry row gone + collections dropped.
#   SKIPS  the /admin bootstrap wizard (host-only; hosted onboarding is the
#          control-plane org create + tenant superuser upsert below), the
#          buggy-fixture rollbacks (they exercise the single-tenant
#          entrypoint's health-probe rollback; the hosted revert path is
#          covered at protocol level by TestHostedDeployE2E), build-history
#          revert (pkg_build archives are single-tenant machinery), and the
#          core upgrade/downgrade phases (they provision a git base remote;
#          the hosted base rides the npm lockfile instead).
#   COVERS the OTA assertions: a tenant serves /api/app/update from its own
#          build artifact, so each package change must advertise a NEW
#          content-addressed bundle id per platform (design §6 closed).
#
# Fixture packages come from a LOCAL npm registry (hosted-npm-registry.mjs):
# the tinycld base packed from the sibling checkout, @tinycld/todo packed from
# its repo's v1.0.0/v2.0.0 tags. MT_NPM_REGISTRY points the router's builder,
# /v1/resolve and /v1/versions at it — member fetches only; the build
# workspace's own pnpm install keeps normal registry resolution.
#
# Env knobs:
#   MT_PORT                  Router port (default 7092 — 7090 is the todo
#                            container, 7091 the old hosting runner, 7200 e2e).
#   TODO_REPO                Todo fixture repo URL or local path
#                            (default https://github.com/tinycld/todo).
#   MT_SUPERUSER_EMAIL/_PASSWORD    Control-plane superuser (defaults below).
#   TENANT_ADMIN_EMAIL/_PASSWORD    The org's tenant superuser the spec logs
#                            in as (defaults below; forwarded to the spec as
#                            ADMIN_USER_LOGIN/ADMIN_USER_PW).
#   PNPM_STORE               pnpm store the builder hardlinks from (default:
#                            `pnpm store path` — strongly recommended).
#   KEEP=1                   Leave the router + registry (and work dir) running
#                            after the run for manual debugging.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WS_ROOT="$(cd "${APP_DIR}/.." && pwd)"
HOSTING_DIR="${WS_ROOT}/hosting"

PORT="${MT_PORT:-7092}"
BASE_DOMAIN="localhost"
ORG_SLUG="acme"
ORG_URL="http://${ORG_SLUG}.${BASE_DOMAIN}:${PORT}"
SUPERUSER_EMAIL="${MT_SUPERUSER_EMAIL:-hosted-smoke@example.com}"
SUPERUSER_PASSWORD="${MT_SUPERUSER_PASSWORD:-HostedSmoke1234!}"
TENANT_ADMIN_EMAIL="${TENANT_ADMIN_EMAIL:-hosted-admin@example.com}"
TENANT_ADMIN_PASSWORD="${TENANT_ADMIN_PASSWORD:-HostedAdmin1234!}"
TODO_REPO="${TODO_REPO:-https://github.com/tinycld/todo}"

LOG_DIR="${SCRIPT_DIR}/hosted-install-logs"
SERVER_LOG="${LOG_DIR}/serve-router.log"
REGISTRY_LOG="${LOG_DIR}/npm-registry.log"
mkdir -p "${LOG_DIR}"
: >"${SERVER_LOG}"
: >"${REGISTRY_LOG}"

if [ ! -d "${HOSTING_DIR}" ]; then
    echo "hosting repo not found at ${HOSTING_DIR} — assemble it as a workspace sibling first" >&2
    exit 1
fi

# A SHORT work dir, deliberately not mktemp's $TMPDIR default: build jobs run
# with TMPDIR=<jobDir>/tmp, and tools that create unix sockets there (tsx's
# IPC pipe during the workspace postinstall) hit the ~104-byte sun_path limit
# when the job dir nests under macOS's already-long /var/folders temp root —
# `listen EINVAL` deep inside pnpm install. /tmp keeps the whole job path
# comfortably under the limit.
WORK_DIR="$(mktemp -d /tmp/tchosted.XXXXXX)"
BIN_DIR="${WORK_DIR}/bin"
MT_ROOT_DIR="${WORK_DIR}/mt_root"
mkdir -p "${BIN_DIR}" "${MT_ROOT_DIR}"

SERVER_PID=""
REGISTRY_PID=""

cleanup() {
    if [ "${KEEP:-}" = "1" ]; then
        echo "KEEP=1 — router left running (pid ${SERVER_PID}, root ${MT_ROOT_DIR})"
        echo "  server log:   ${SERVER_LOG}"
        echo "  registry log: ${REGISTRY_LOG} (pid ${REGISTRY_PID})"
        return
    fi
    if [ -n "${SERVER_PID}" ]; then
        # SIGTERM the router; its shutdown reaps the per-org tenant children.
        kill "${SERVER_PID}" >/dev/null 2>&1 || true
        wait "${SERVER_PID}" 2>/dev/null || true
    fi
    if [ -n "${REGISTRY_PID}" ]; then
        kill "${REGISTRY_PID}" >/dev/null 2>&1 || true
    fi
    rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

dump_server_log() {
    echo "== router log tail:" >&2
    tail -60 "${SERVER_LOG}" >&2
}

# ---- 1. the local npm registry with the fixture set ----
echo "== Starting fixture npm registry (base sibling + todo v1/v2 from ${TODO_REPO})"
node "${SCRIPT_DIR}/hosted-npm-registry.mjs" \
    --pack "${WS_ROOT}/tinycld" \
    --pack-git "${TODO_REPO}#v1.0.0" \
    --pack-git "${TODO_REPO}#v2.0.0" \
    >>"${REGISTRY_LOG}" 2>&1 &
REGISTRY_PID=$!

REGISTRY_URL=""
REG_DEADLINE=$((SECONDS + 300)) # git clone + three npm packs
until [ -n "${REGISTRY_URL}" ]; do
    if ! kill -0 "${REGISTRY_PID}" 2>/dev/null; then
        echo "fixture registry exited during startup — log tail:" >&2
        tail -30 "${REGISTRY_LOG}" >&2
        exit 1
    fi
    if [ ${SECONDS} -ge ${REG_DEADLINE} ]; then
        echo "fixture registry never became ready — log tail:" >&2
        tail -30 "${REGISTRY_LOG}" >&2
        exit 1
    fi
    sleep 1
    REGISTRY_URL="$(grep -oE 'REGISTRY_URL=http://[0-9.:]+' "${REGISTRY_LOG}" | head -1 | cut -d= -f2 || true)"
done
echo "   registry at ${REGISTRY_URL}"
grep '^PACKED ' "${REGISTRY_LOG}" | sed 's/^/   /'

BASE_VERSION="$(node -p "require('${WS_ROOT}/tinycld/package.json').version")"
echo "   base version: tinycld@${BASE_VERSION}"

# ---- 2. build + boot the router ----
echo "== Building serve-router from ${HOSTING_DIR}"
(cd "${HOSTING_DIR}" && go build -o "${BIN_DIR}/serve-router" ./cmd/serve-router)

PNPM_STORE="${PNPM_STORE:-$(cd "${WS_ROOT}" && pnpm store path 2>/dev/null || true)}"
[ -n "${PNPM_STORE}" ] && echo "   builder pnpm store: ${PNPM_STORE}"

echo "== Starting router on 127.0.0.1:${PORT} (base domain ${BASE_DOMAIN}, root ${MT_ROOT_DIR})"
env \
    MT_ROOT="${MT_ROOT_DIR}" \
    MT_BASE_DOMAIN="${BASE_DOMAIN}" \
    MT_ADDR="127.0.0.1:${PORT}" \
    MT_TLS_MODE=proxy \
    MT_SCAFFOLD_ROOT="${WS_ROOT}" \
    MT_NPM_REGISTRY="${REGISTRY_URL}" \
    MT_ALLOW_UNCONFINED_CONTROL=true \
    MT_BUILDER_PNPM_STORE="${PNPM_STORE}" \
    MT_SUPERUSER_EMAIL="${SUPERUSER_EMAIL}" \
    MT_SUPERUSER_PASSWORD="${SUPERUSER_PASSWORD}" \
    "${BIN_DIR}/serve-router" >>"${SERVER_LOG}" 2>&1 &
SERVER_PID=$!

# admin_api <method> <path> [curl args...] — control-plane call via the Host
# header (no DNS assumptions), printing the response body.
admin_api() {
    local method="$1" path="$2"
    shift 2
    curl -sf -X "${method}" \
        -H "Host: admin.${BASE_DOMAIN}:${PORT}" \
        -H "Content-Type: application/json" \
        "$@" \
        "http://127.0.0.1:${PORT}${path}"
}

echo "== Waiting for the control plane to answer /api/health"
HEALTH_DEADLINE=$((SECONDS + 60))
until curl -sf -H "Host: admin.${BASE_DOMAIN}:${PORT}" "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1; do
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
        echo "serve-router exited during startup" >&2
        dump_server_log
        exit 1
    fi
    if [ ${SECONDS} -ge ${HEALTH_DEADLINE} ]; then
        echo "control plane never became healthy" >&2
        dump_server_log
        exit 1
    fi
    sleep 1
done
echo "   healthy."

SU_TOKEN="$(admin_api POST /api/collections/_superusers/auth-with-password \
    -d "{\"identity\":\"${SUPERUSER_EMAIL}\",\"password\":\"${SUPERUSER_PASSWORD}\"}" \
    | node -p 'JSON.parse(require("fs").readFileSync(0,"utf8")).token')"

# ---- 3. hosted-org onboarding ----
# CreateOrg builds the base-only artifact through the trusted builder (the
# heavy step: pnpm install + go build + expo export on a cold cache), then
# boots and readiness-verifies the tenant before returning.
#
# owner_email is REQUIRED by the provisioner: a tenant serves no /setup wizard,
# so an org created without one has no `users` row that can log in. We pass the
# same identity the tenant-superuser step below upserts, so the org's owner
# account and its PB superuser are one login rather than two divergent ones.
echo "== Creating org '${ORG_SLUG}' from lockfile {tinycld: ${BASE_VERSION}} (first build — minutes on a cold cache)"
CREATE_OUT="${WORK_DIR}/create-org.json"
CREATE_STATUS="$(curl -s -o "${CREATE_OUT}" -w '%{http_code}' -X POST \
    -H "Host: admin.${BASE_DOMAIN}:${PORT}" \
    -H "Content-Type: application/json" \
    -H "Authorization: ${SU_TOKEN}" \
    --max-time 3600 \
    -d "{\"slug\":\"${ORG_SLUG}\",\"display_name\":\"Acme\",\"lockfile\":{\"tinycld\":\"${BASE_VERSION}\"},\"owner_email\":\"${TENANT_ADMIN_EMAIL}\",\"owner_password\":\"${TENANT_ADMIN_PASSWORD}\"}" \
    "http://127.0.0.1:${PORT}/api/orgs")" || {
    echo "create org failed: curl exit $? (status ${CREATE_STATUS:-none})" >&2
    cat "${CREATE_OUT}" >&2 2>/dev/null || true
    dump_server_log
    exit 1
}
if [ "${CREATE_STATUS}" != "200" ]; then
    echo "create org failed: HTTP ${CREATE_STATUS}" >&2
    cat "${CREATE_OUT}" >&2
    dump_server_log
    exit 1
fi
echo "   created: $(cat "${CREATE_OUT}")"

# The org's tenant superuser — the account the spec's /setup login uses. No
# in-band flow mints it yet (hosted onboarding UX is future work), so provision
# it the way TestHostedDeployE2E does: run the org's OWN artifact binary in
# host mode against the org's pb_data. Suspend first so no tenant process holds
# the DB, then resume; the next request respawns the tenant.
echo "== Onboarding tenant superuser ${TENANT_ADMIN_EMAIL}"
RECIPE_HASH="$(admin_api GET "/api/collections/orgs/records?filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"slug='${ORG_SLUG}'\"))")" \
    -H "Authorization: ${SU_TOKEN}" \
    | node -p 'JSON.parse(require("fs").readFileSync(0,"utf8")).items[0].recipe_hash')"
ARTIFACT_DIR="${MT_ROOT_DIR}/builds/${RECIPE_HASH#sha256:}"
ORG_DIR="${MT_ROOT_DIR}/pb_orgs/${ORG_SLUG}"
if [ ! -x "${ARTIFACT_DIR}/tinycld" ]; then
    echo "artifact binary not found at ${ARTIFACT_DIR}/tinycld (recipe ${RECIPE_HASH})" >&2
    dump_server_log
    exit 1
fi

admin_api POST "/api/orgs/${ORG_SLUG}/suspend" -H "Authorization: ${SU_TOKEN}" >/dev/null
sleep 2 # let the evicted tenant finish its drain and release the DB
(cd "${ORG_DIR}" && "${ARTIFACT_DIR}/tinycld" superuser upsert \
    "${TENANT_ADMIN_EMAIL}" "${TENANT_ADMIN_PASSWORD}" \
    --dir "${ORG_DIR}/pb_data") >/dev/null
admin_api POST "/api/orgs/${ORG_SLUG}/resume" -H "Authorization: ${SU_TOKEN}" >/dev/null

echo "== Waiting for the org subdomain to serve"
ORG_DEADLINE=$((SECONDS + 120))
until curl -sf "${ORG_URL}/api/health" >/dev/null 2>&1; do
    if [ ${SECONDS} -ge ${ORG_DEADLINE} ]; then
        echo "org subdomain never became healthy at ${ORG_URL}" >&2
        dump_server_log
        exit 1
    fi
    sleep 1
done
# The tenant serves its SPA itself (artifact pb_public); a shell on / is the
# browser-level precondition every phase below depends on.
if ! curl -sf "${ORG_URL}/" | grep -qi '<html'; then
    echo "org subdomain serves no SPA shell at ${ORG_URL}/" >&2
    dump_server_log
    exit 1
fi
echo "   ${ORG_URL} is serving (API + SPA)."

# ---- 4. drive the spec phases against the org subdomain ----
echo "== ensuring chromium is installed for playwright"
(cd "${APP_DIR}" && pnpm exec playwright install chromium >/dev/null)

# Runs a subset of the serial spec by title grep, exactly like
# run-todo-install.sh's run_phase — but against the org subdomain, with npm
# specs (hosted refuses git specs) and the hosted progress-bar floor (the
# router-side build streams no mid-build progress). OTA assertions now run
# here too: the tenant serves /api/app/update from its own build artifact,
# advertising content-addressed recipe-<hash>-<platform> bundle ids.
run_phase() {
    local grep_expr="$1" label="$2"
    echo "== running ${label}"
    (
        cd "${SCRIPT_DIR}"
        PW_BASE_URL="${ORG_URL}" \
        ADMIN_USER_LOGIN="${TENANT_ADMIN_EMAIL}" \
        ADMIN_USER_PW="${TENANT_ADMIN_PASSWORD}" \
        PW_TODO_SPEC_V1="@tinycld/todo@1.0.0" \
        PW_PROGRESS_MIN_PCT=10 \
        RUN_TODO_INSTALL_TEST=1 \
        CI=true FORCE_COLOR=0 \
        pnpm exec playwright test todo-install.spec.ts --reporter=line,list -g "${grep_expr}"
    ) || {
        echo "== ${label} phase FAILED"
        dump_server_log
        exit 1
    }
}

# Install-class phases end with the router evicting + respawning the tenant;
# the spec's own status polling rides that out (waitForOpStatus tolerates the
# restart window), so no runner-side restart choreography is needed.

# Phase 1 — install todo pinned to 1.0.0 through the hosted installer UI.
run_phase 'install @tinycld/todo' 'install v1.0.0'

# Phase 2 — verify v1 live (no tags schema), mint the app user, add a todo.
run_phase 'v1.0.0 is live' 'verify v1.0.0'

# Phase 3 — upgrade to 2.0.0 via the Packages version picker (UP migration
# runs on the respawned tenant's boot).
run_phase 'upgrade todo to v2.0.0' 'upgrade to v2.0.0'

# Phase 4 — verify v2 live (tags schema present) and tag a todo in the UI.
run_phase 'v2.0.0 is live' 'verify v2.0.0 + tag'

# Phase 5 — downgrade back to 1.0.0: drop-report modal, typed-slug confirm,
# in-tenant DOWN migrations before the deploy proposal.
run_phase 'downgrade todo to v1.0.0' 'downgrade to v1.0.0'

# Phase 6 — verify the downs ran: tags schema dropped, TAGS editor gone.
run_phase 'down migration ran' 'verify down migration'

# Phase 7 — uninstall todo (registry row deleted by the respawned tenant's
# reconcile; collections already dropped by the downgrade above).
run_phase 'uninstalling todo succeeds' 'uninstall todo'

# Phase 8 — verify the uninstall landed: row gone + collections gone.
run_phase 'delete landed' 'verify uninstall'

echo "== ✅ hosted install/upgrade/downgrade/uninstall browser suite passed against ${ORG_URL}"
