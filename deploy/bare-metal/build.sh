#!/usr/bin/env bash
#
# ⚠️  EXAMPLE SCRIPT — read it and adapt it before you run it.
#
# This is a reference implementation of a from-source bare-metal build, not a
# turnkey installer for every environment. It encodes specific choices that are
# unlikely to match yours unchanged:
#   - the host distro / package layout (paired install.sh assumes Debian/Ubuntu
#     apt + an x86-64 Go toolchain),
#   - fixed paths and a fixed service user ($BAKED_DIR, $STATE_DIR, $RUN_USER),
#   - that the app owns :80/:443 itself (autocert), with no reverse proxy.
# Treat it as a starting point: copy it, read it end to end, and edit it for your
# OS, architecture, paths, and TLS/proxy setup. The knobs below cover the common
# cases; anything else, change the script.
#
# build.sh — build TinyCld FROM SOURCE on a bare host (no Docker) and bake the
# result for the systemd service. Mirrors the runtime artifact that
# ../../Dockerfile produces, but assembled natively.
#
# This is the same pipeline the image build runs, just on the host:
#   bootstrap --assemble-only  ->  pnpm install  ->  packages:generate
#     ->  expo export (web)  ->  go build (server)  ->  bake to $BAKED_DIR
#
# Run as root (it writes $BAKED_DIR and chowns to $RUN_USER); the heavy build
# steps are dropped to $RUN_USER. Idempotent: re-running rebuilds from the pinned
# ref and atomically swaps the baked tree. install.sh installs the toolchain this
# needs and is expected to have run first.
#
# Configuration (environment variables; all have sensible defaults):
#   TINYCLD_VERSION    git ref/tag every repo is cloned at        (default: main)
#   TINYCLD_FEATURES   space-separated feature members to include
#                        (default: "mail contacts calendar drive calc text \
#                                   google-takeout-import")
#   TINYCLD_REPO_BASE  git base to clone from               (default HTTPS GitHub)
#   RUN_USER           unprivileged service/build user           (default: tinycld)
#   BAKED_DIR          where the pristine workspace is baked (default: /opt/tinycld-baked)
#   STATE_DIR          runtime state root (pb_data/builds/...)    (default: /workspace)
#   BUILD_DIR          scratch assembly dir            (default: /opt/tinycld-build)
#   PNPM_VERSION       pnpm to activate via corepack             (default: 11.3.0)
#   EXPO_PUBLIC_SENTRY_DSN   optional; inlined into the web bundle at export time
#
set -euo pipefail

TINYCLD_VERSION="${TINYCLD_VERSION:-main}"
TINYCLD_FEATURES="${TINYCLD_FEATURES:-mail contacts calendar drive calc text google-takeout-import}"
export TINYCLD_REPO_BASE="${TINYCLD_REPO_BASE:-https://github.com/tinycld}"
RUN_USER="${RUN_USER:-tinycld}"
BAKED_DIR="${BAKED_DIR:-/opt/tinycld-baked}"
STATE_DIR="${STATE_DIR:-/workspace}"
BUILD_DIR="${BUILD_DIR:-/opt/tinycld-build}"
PNPM_VERSION="${PNPM_VERSION:-11.3.0}"
EXPO_PUBLIC_SENTRY_DSN="${EXPO_PUBLIC_SENTRY_DSN:-}"

log() { echo "[tinycld-build] $*"; }
export PATH="/usr/local/go/bin:${PATH}"

# ------------------------------------------------------------------------------
# 1. Assemble + build in a scratch dir owned by $RUN_USER.
# ------------------------------------------------------------------------------
log "building TinyCld @ ${TINYCLD_VERSION} (features: ${TINYCLD_FEATURES})"

rm -rf "${BUILD_DIR}.tmp"
install -d -o "$RUN_USER" -g "$RUN_USER" "${BUILD_DIR}.tmp"

# Pin every cloned repo (the tinycld shell + each feature) to the requested ref.
with_args=""
for f in $TINYCLD_FEATURES; do
    with_args="$with_args --with ${f}@${TINYCLD_VERSION}"
done

runuser -u "$RUN_USER" -- bash -euo pipefail -s -- \
    "${BUILD_DIR}.tmp" "$TINYCLD_VERSION" "$PNPM_VERSION" "$with_args" "$EXPO_PUBLIC_SENTRY_DSN" <<'BUILD'
set -euo pipefail
SCRATCH="$1"; VERSION="$2"; PNPM_VERSION="$3"; WITH_ARGS="$4"; SENTRY_DSN="$5"
export TINYCLD_REPO_BASE="${TINYCLD_REPO_BASE:-https://github.com/tinycld}"
export COREPACK_ENABLE_DOWNLOAD_PROMPT=0
export PATH="/usr/local/go/bin:${PATH}"
export NODE_OPTIONS="--max-old-space-size=2048"

cd "$SCRATCH"

echo "[build] bootstrap --assemble-only --with tinycld@${VERSION} ${WITH_ARGS}"
# The tinycld shell repo is always cloned; pin it to the ref via --with too.
# shellcheck disable=SC2086
npx -y @tinycld/bootstrap@latest --assemble-only --with tinycld@"${VERSION}" ${WITH_ARGS}

echo "[build] activating pnpm@${PNPM_VERSION}"
corepack enable
corepack prepare "pnpm@${PNPM_VERSION}" --activate

# Pin the pnpm store inside the workspace so the in-app installer reuses it
# (matches the Dockerfile's storeDir pin). Append to the build copy only.
printf '\nstoreDir: %s/.pnpm-store\n' "$SCRATCH" >> pnpm-workspace.yaml

# NOT --frozen-lockfile: `bootstrap --assemble-only` does not emit a root
# pnpm-lock.yaml (it writes package.json + pnpm-workspace.yaml and clones the
# members; the lockfile is generated here). The Dockerfile uses --frozen-lockfile
# because its CI context bakes in a pinned lockfile asset, which this does not.
echo "[build] pnpm install (runs the generator postinstall)"
pnpm install

cd tinycld

echo "[build] packages:generate"
pnpm run packages:generate

# Materialize the generator's migration/hook symlinks into real files (the
# Dockerfile does this before the Go build).
for d in server/pb_migrations server/pb_hooks; do
    [ -d "$d" ] || continue
    find "$d" -type l -exec sh -c 'target=$(readlink -f "$1") && rm "$1" && cp "$target" "$1"' _ {} \;
done

echo "[build] expo export --platform web"
export EXPO_PUBLIC_ENV=web
# Web Sentry DSN is inlined into the bundle at export time. Empty = disabled.
export EXPO_PUBLIC_SENTRY_DSN="$SENTRY_DSN"
RID="$(date -u +%Y-%m-%d-%H%M%S)-$(git -C . rev-parse --short HEAD 2>/dev/null || echo deadbeef)"
export EXPO_PUBLIC_RELEASE_ID="$RID"
npx expo export --platform web
mkdir -p release-staging
mv dist "release-staging/${RID}"
printf '%s' "$RID" > "release-staging/${RID}/release-id.txt"
mv "release-staging/${RID}/index.html" "release-staging/${RID}/app.html"

echo "[build] go build server binary"
cd server
CGO_ENABLED=1 GOOS=linux go build -o ../tinycld .
if [ -f go.work ]; then go work sync; fi
cd ..

echo "[build] assembled tree ready at ${SCRATCH}/tinycld/tinycld"
BUILD

# ------------------------------------------------------------------------------
# 2. Promote the scratch tree to $BAKED_DIR atomically.
#
# Bake the WHOLE workspace (root manifests + node_modules + every feature sibling
# + the tinycld member), exactly like the Dockerfile bakes /opt/tinycld-baked:
# node_modules/@tinycld/<x> are RELATIVE symlinks, so a build dir must contain the
# entire workspace, not just tinycld/.
# ------------------------------------------------------------------------------
log "promoting build to ${BAKED_DIR}"
rm -rf "${BAKED_DIR}.old"
[ -e "$BAKED_DIR" ] && mv "$BAKED_DIR" "${BAKED_DIR}.old"
mv "${BUILD_DIR}.tmp" "$BAKED_DIR"
chown -R "$RUN_USER:$RUN_USER" "$BAKED_DIR"
rm -rf "${BAKED_DIR}.old"

# ------------------------------------------------------------------------------
# 3. Install the entrypoint at the fixed path the systemd unit invokes, and clear
#    the stale seeded build so the NEW bake is what gets served on restart.
#
# The entrypoint's seed_baked_build() short-circuits when $STATE_DIR/current
# already resolves to a binary — so on an UPDATE it would keep serving the old
# seeded copy and never re-seed from the freshly-baked tree. Removing the
# disposable build trees + the current symlink forces a clean re-seed.
#
# SAFE: this touches only CODE under $STATE_DIR/builds and the symlink. The
# database ($STATE_DIR/pb_data) and promoted web bundles ($STATE_DIR/releases)
# are siblings and are left untouched; the entrypoint re-promotes releases from
# the new bake on start.
# ------------------------------------------------------------------------------
log "installing entrypoint + clearing stale seeded build"
install -m 0755 -o "$RUN_USER" -g "$RUN_USER" \
    "${BAKED_DIR}/tinycld/config/entrypoint.sh" /opt/tinycld-entrypoint.sh
rm -rf "${STATE_DIR}/builds" "${STATE_DIR}/current"
install -d -o "$RUN_USER" -g "$RUN_USER" "${STATE_DIR}/builds"

log "build complete. start/restart the service to pick it up (systemctl restart tinycld)."
