#!/usr/bin/env bash
# Local runner for the todo-install integration test. Builds a TinyCld
# image from the CURRENT working tree (so the git-spec validation change is
# present), boots it, scrapes the first-run /admin bootstrap token, runs the
# Playwright spec in a standalone sandbox, and tears the container down.
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
        [ -n "${MOUNT_ROOT:-}" ] && echo "[runner] bind-mount dirs preserved at ${MOUNT_ROOT}"
        return
    fi
    docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
    # Leave MOUNT_ROOT on disk for post-mortem DB inspection; it's a mktemp dir
    # under /tmp the OS reaps. Print it so a failed run's DB is findable.
    [ -n "${MOUNT_ROOT:-}" ] && echo "[runner] bind-mount dirs at ${MOUNT_ROOT} (pb_data/ has the DB for inspection)"
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
#
# Mount pb_data, builds and releases from host dirs (mirroring bin/local-docker
# and the operator's deployment). This is REQUIRED to reproduce the field bug
# "registry update: FAILED: database disk image is malformed (11)" on
# install/delete/rollback: that corruption only manifests when the SQLite DB
# lives on a Docker bind-mount (the VirtioFS/gRPC-FUSE layer doesn't keep WAL's
# shared-memory + file locks coherent across the live server and the installer's
# sqlite3/migrate subprocesses). A container running the DB on its own overlayfs
# (the old no-mount runner) never reproduces it. Fresh dirs each run for a clean
# first-boot bootstrap; KEEP=1 leaves them (and the container) for inspection.
MOUNT_ROOT="${MOUNT_ROOT:-$(mktemp -d -t tinycld-todo-mounts.XXXXXX)}"
PB_DATA_DIR="${MOUNT_ROOT}/pb_data"
BUILDS_DIR="${MOUNT_ROOT}/builds"
RELEASES_DIR="${MOUNT_ROOT}/releases"
# The first-run setup token (which we scrape to drive the /admin wizard) is only
# printed by PocketBase's InstallerFunc when the DB has NO superusers — i.e. a
# truly empty pb_data. A fresh mktemp MOUNT_ROOT is already empty, but if the
# caller reuses MOUNT_ROOT (the `:-` default above) a stale DB from a prior run
# would suppress the token and the bootstrap scrape would fail. Wipe the mounts
# before boot so an empty-DB first run — and thus the printed token — is
# guaranteed regardless of how MOUNT_ROOT was chosen.
rm -rf "${PB_DATA_DIR}" "${BUILDS_DIR}" "${RELEASES_DIR}"
mkdir -p "${PB_DATA_DIR}" "${BUILDS_DIR}" "${RELEASES_DIR}"
echo "[runner] bind-mounting host dirs under ${MOUNT_ROOT}"
echo "[runner]   pb_data  → ${PB_DATA_DIR}"
echo "[runner]   builds   → ${BUILDS_DIR}"
echo "[runner]   releases → ${RELEASES_DIR}"

docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
docker run -d --name "${CONTAINER}" -p 7090:7090 \
    -v "${PB_DATA_DIR}:/workspace/pb_data" \
    -v "${BUILDS_DIR}:/workspace/builds" \
    -v "${RELEASES_DIR}:/workspace/releases" \
    "${IMAGE}"

# 2a. Begin streaming container logs to a file immediately, so we capture the
# full install trace live even if a later step hangs.
start_live_log

# 3. Wait for first-boot health.
wait_healthy "first boot"

# 4. Scrape the first-run bootstrap token from logs. The server prints a
#    `${url}/admin?token=…` line on first boot; the path doesn't matter here —
#    we match the `token=` query param, so this is unaffected by /setup→/admin.
#
#    The setup-token banner is printed AFTER /api/health starts answering (the
#    banner comes near the very end of boot, once PocketBase finishes the
#    InstallerFunc check), so a one-shot grep right after wait_healthy races the
#    banner and can find nothing. Poll the logs for up to 30s instead.
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

# Runs a subset of the serial spec, selected by a title grep. The first phase
# needs the bootstrap token (for the first-run /admin wizard); later phases
# don't. Each
# call shares the container's persisted state from the prior phase. $1 is the
# title grep, $2 a human label for failure messages.
run_phase() {
    local grep_expr="$1" label="$2"
    echo "[runner] running ${label}"
    (
        cd "${PW_ROOT}"
        PW_BASE_URL="${BASE_URL}" \
        PW_TODO_SETUP_TOKEN="${TOKEN}" \
        PW_CORE_CUR="${CORE_CUR:-}" \
        PW_CORE_NEXT="${CORE_NEXT:-}" \
        RUN_TODO_INSTALL_TEST=1 \
        CI=true FORCE_COLOR=0 \
        ./node_modules/.bin/playwright test --reporter=line,list -g "${grep_expr}"
    ) || { echo "[runner] ${label} phase failed"; dump_logs; exit 1; }
}

# Asserts the armed-backup rollback protocol committed after a HEALTHY restart
# (review finding H3). On the success path the rebuild leaves data.db.backup +
# .db-backup-armed in pb_data ARMED across the exit-75 restart; the entrypoint's
# in-process loop must DELETE both once the new binary's health probe passes
# ("commit"). By the time wait_healthy returns, that verdict has already run, so
# a backup/marker still present here means the commit step never fired — the DB
# would be left needlessly armed (and a future crash could wrongly roll it back).
# The DB lives on the host bind-mount (${PB_DATA_DIR}), so check it directly.
assert_backup_committed() {
    local label="$1"
    if [ -f "${PB_DATA_DIR}/.db-backup-armed" ] || [ -f "${PB_DATA_DIR}/data.db.backup" ]; then
        echo "[runner] ERROR: ${label}: DB backup still armed after a healthy restart — commit step did not fire" >&2
        ls -la "${PB_DATA_DIR}" 2>&1 | sed 's/^/[runner]   /' || true
        dump_logs
        exit 1
    fi
    echo "[runner] ${label}: DB backup committed (disarmed) after healthy restart"
}

# Waits out one install-class exit-75 restart and re-attaches the live log.
# The restart kills the `docker logs -f` stream, so re-attach after the new
# binary is healthy again to keep capturing the post-restart boot trace.
await_restart() {
    local label="$1"
    echo "[runner] waiting for ${label} restart"
    wait_unhealthy "${label} restart down"   # observe the old server exit first
    wait_healthy "${label} post-restart"     # then wait for the new binary up
    assert_backup_committed "${label}"        # armed backup must be committed (H3)
    start_live_log                            # re-attach to the restarted container
}

# The flow has THREE install-class restarts (install-v1, upgrade-v2,
# downgrade-v1). Phases that don't trigger a restart (the verify/tag steps) run
# in the same invocation as the step before them where possible, but here each
# verify follows a restart, so they're driven as their own post-restart phase.

# Phase 1 — bootstrap the superuser, then install todo pinned to v1.0.0.
run_phase 'bootstrap|install @tinycld/todo' 'bootstrap + install v1.0.0'
await_restart "post-install"

# Phase 2 — verify v1.0.0 is live (no tags schema) and seed an org + a todo.
run_phase 'v1.0.0 is live' 'verify v1.0.0'

# Phase 3 — upgrade to v2.0.0 via the Versions tab (applies create_tags UP).
run_phase 'upgrade todo to v2.0.0' 'upgrade to v2.0.0'
await_restart "post-upgrade"

# Phase 4 — verify v2.0.0 is live (tags schema present) and tag a todo.
run_phase 'v2.0.0 is live' 'verify v2.0.0 + tag'

# Phase 5 — downgrade to v1.0.0 via the Versions tab (runs create_tags DOWN).
run_phase 'downgrade todo to v1.0.0' 'downgrade to v1.0.0'
await_restart "post-downgrade"

# Phase 6 — verify the down migration ran: tags schema dropped, TAGS editor gone.
run_phase 'down migration ran' 'verify down migration'

# Phase 7 — ROLLBACK: revert to the archived v2.0.0 build (restores its binary +
# schema). This is one of the ops the operator reported failing with the
# malformed-DB error; it restarts on success.
run_phase 'revert to the archived v2.0.0 build' 'rollback to v2.0.0 build'
await_restart "post-rollback"

# Phase 8 — verify the rollback landed: v2.0.0 current, tags schema restored.
run_phase 'rollback landed' 'verify rollback'

# Phase 9 — DELETE: uninstall todo. The second op the operator reported failing.
# Restarts on success.
run_phase 'uninstalling todo succeeds' 'delete (uninstall) todo'
await_restart "post-delete"

# Phase 10 — verify the delete landed: todo registry row marked disabled.
run_phase 'delete landed' 'verify delete'

# --- Core (base) upgrade/downgrade phases -------------------------------------
#
# The base-update pipeline clones the base repo named in pkg_registry.npm_package.
# To drive a real, deterministic core upgrade without touching the shared remote,
# we build a LOCAL bare git remote INSIDE the container from the running tree,
# add a minimal v-next commit (bump core/package.json + one trivial core
# migration), tag it, and repoint core's registry source at that file:// remote.
# The downgrade target is the current baked version (v<CORE_CUR>).

CORE_CUR=$(docker exec "${CONTAINER}" node -e "console.log(require('/workspace/current/core/package.json').version)")
echo "[runner] current base version: ${CORE_CUR}"
CORE_NEXT="0.0.5"   # synthetic upgrade target; must be > CORE_CUR (0.0.4)

provision_base_remote() {
    echo "[runner] provisioning local base remote with v${CORE_CUR} + v${CORE_NEXT}"
    docker exec "${CONTAINER}" sh -lc '
        set -e
        BARE=/workspace/base-remote.git
        WORK=/workspace/base-work
        rm -rf "$BARE" "$WORK"
        git init -q "$WORK"
        cd "$WORK"
        git config user.email t@t.local && git config user.name t
        # Copy the live base source into the work tree: EVERYTHING a runnable base
        # needs, mirroring the runtime tinycld COPY set in the Dockerfile, minus
        # generated/runtime state (dist-*, release-staging, node_modules, the
        # compiled tinycld binary, bundled-packages.json, the pb_migrations symlink,
        # tinycld.config.ts/seeds.ts which the generator re-emits). Missing any of
        # these breaks a base rebuild: package-scripts -> pnpm 404; plugins/modules
        # -> expo export PluginError (app.json references the with-app-updater plugin
        # and metro maps the app-updater specifier to modules); assets/lib/public/
        # global.css/babel/tsconfig/uniwind -> metro resolution failures.
        # NOTE: this comment lives inside a single-quoted sh -lc body, so NO
        # apostrophes here (one would close the quote and break the parse).
        for d in app assets babel.config.cjs core expo-env.d.ts global.css lib \
                 metro.config.cjs modules package-scripts plugins public scripts \
                 server tsconfig.json uniwind-types.d.ts app.json package.json; do
            [ -e "/workspace/current/$d" ] && cp -a "/workspace/current/$d" .
        done
        git add -A && git commit -qm "base v'"${CORE_CUR}"'"
        git tag "v'"${CORE_CUR}"'"
        # v-next: bump core/package.json version + add one trivial core migration.
        node -e "const f=\"core/package.json\";const j=require(\"./\"+f);j.version=\"'"${CORE_NEXT}"'\";require(\"fs\").writeFileSync(f,JSON.stringify(j,null,4)+\"\n\")"
        mkdir -p core/server/pb_migrations
        cat > core/server/pb_migrations/1990000000_create_base_probe.js <<"MIG"
/// <reference path="../pb_data/types.d.ts" />
migrate(
    app => {
        const collection = new Collection({
            id: "pbc_base_probe_01",
            name: "base_probe",
            type: "base",
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: "bp_note",
                    name: "note",
                    type: "text",
                    max: 2000,
                },
            ],
        })
        app.save(collection)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId("base_probe")
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
MIG
        git add -A && git commit -qm "base v'"${CORE_NEXT}"'"
        git tag "v'"${CORE_NEXT}"'"
        git clone -q --bare "$WORK" "$BARE"
    '
    # Repoint core registry source at the local bare remote (git+file://).
    docker exec "${CONTAINER}" sqlite3 /workspace/pb_data/data.db \
        "UPDATE pkg_registry SET npm_package='git+file:///workspace/base-remote.git' WHERE slug='core';"
}

provision_base_remote

# Phase C1 — upgrade core to v-next via the base row's version picker.
run_phase 'upgrade core to v0.0.5' 'upgrade core'
await_restart "post-core-upgrade"

# Phase C2 — verify the upgrade: registry version advanced + base_probe exists.
run_phase 'core upgrade landed' 'verify core upgrade'

# Phase C3 — downgrade core back to the baked version (runs base_probe DOWN).
run_phase 'downgrade core to v0.0.4' 'downgrade core'
await_restart "post-core-downgrade"

# Phase C4 — verify the downgrade: version reverted + base_probe dropped.
run_phase 'core downgrade landed' 'verify core downgrade'

echo "[runner] ✅ todo install/upgrade/downgrade/rollback/delete + core base upgrade/downgrade integration test passed"
