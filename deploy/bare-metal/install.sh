#!/usr/bin/env bash
#
# ⚠️  EXAMPLE SCRIPT — read it and adapt it before you run it.
#
# This is a reference bare-metal installer, not a one-size-fits-all tool. It bakes
# in choices you will likely need to change for your environment:
#   - Debian/Ubuntu on x86-64: it uses `apt`, the NodeSource apt repo, and the
#     linux-amd64 Go tarball. Other distros (RHEL/Alpine/…) or ARM hosts need the
#     package-install and Go/Node steps rewritten.
#   - It assumes TinyCld owns :80/:443 and terminates TLS itself via autocert. If
#     you run behind an existing reverse proxy or load balancer, drop autocert and
#     the low-port sysctl and serve plain HTTP on :7090 instead.
#   - It writes a systemd unit and uses fixed paths/user ($STATE_DIR, $BAKED_DIR,
#     $RUN_USER). Non-systemd init, or different paths, means edits.
# Copy it, read it end to end, and modify it to fit. It is meant to be a clear
# starting point you own, not a black box you curl-pipe blindly.
#
# install.sh — install TinyCld on a plain Debian/Ubuntu host as a systemd service
# (no Docker). Installs the runtime + build toolchain, creates the service user
# and state dirs, allows the unprivileged user to bind low ports, builds the app
# from source (via build.sh), and writes + starts the systemd unit.
#
# Run as root. Idempotent: safe to re-run (it also serves as the upgrade path —
# re-run after bumping TINYCLD_VERSION to rebuild and restart).
#
#   curl -fsSL https://raw.githubusercontent.com/tinycld/tinycld/main/deploy/bare-metal/install.sh \
#     | DOMAIN=tinycld.example.com bash
#
# Or clone the repo and run ./deploy/bare-metal/install.sh with DOMAIN set.
#
# Configuration (environment variables):
#   DOMAIN              REQUIRED. The canonical domain. Autocert provisions a
#                       Let's Encrypt cert for it and binds :80/:443 directly.
#   ADDITIONAL_DOMAINS  optional comma-separated extra cert domains.
#   TINYCLD_VERSION     git ref/tag to build                       (default: main)
#   TINYCLD_FEATURES    space-separated feature members      (default: the full set)
#   SENTRY_DSN          optional. Enables Sentry on BOTH the Go server (runtime)
#                       and the web bundle (inlined at build time).
#   ENV_EXTRA           optional. Newline-separated KEY=VALUE lines appended to the
#                       service env file (e.g. MAIL_PROVIDER, POSTMARK_* tokens).
#   RUN_USER STATE_DIR BAKED_DIR   advanced overrides (see build.sh defaults).
#
# TLS: the app terminates TLS itself (autocert) — no nginx/Caddy. Ensure :80 and
# :443 reach this host (and :465/:993 if you use the mail listeners). For a host
# behind NAT, forward those ports.
#
set -euo pipefail

DOMAIN="${DOMAIN:?Set DOMAIN=your.domain (the canonical domain autocert will cert)}"
ADDITIONAL_DOMAINS="${ADDITIONAL_DOMAINS:-}"
TINYCLD_VERSION="${TINYCLD_VERSION:-main}"
SENTRY_DSN="${SENTRY_DSN:-}"
ENV_EXTRA="${ENV_EXTRA:-}"

RUN_USER="${RUN_USER:-tinycld}"
STATE_DIR="${STATE_DIR:-/workspace}"
BAKED_DIR="${BAKED_DIR:-/opt/tinycld-baked}"

GO_VERSION="${GO_VERSION:-1.25.1}"
NODE_MAJOR="${NODE_MAJOR:-22}"
UNPRIV_PORT_START="${UNPRIV_PORT_START:-80}"

ENV_FILE=/etc/tinycld/tinycld.env
UNIT=/etc/systemd/system/tinycld.service
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

log() { echo "[tinycld-install] $*"; }
export DEBIAN_FRONTEND=noninteractive

# ------------------------------------------------------------------------------
# 1. Runtime + build dependencies.
#
# TinyCld self-rebuilds: its in-app package installer runs pnpm install + go build
# + expo export on the HOST, so the host needs the full toolchain, not just a
# runtime. This is the Dockerfile runtime-stage apt list + Node + Go.
#   cgo: libmupdf-dev (go-fitz), gcc/g++ (mupdf + goheif/libde265)
#   sqlite3 CLI: the installer's DB backup step; gosu: privilege drop in entrypoint
# ------------------------------------------------------------------------------
log "installing apt packages"
apt-get update -qq
apt-get install -y --no-install-recommends \
    ca-certificates libffi8 libmupdf-dev libcap2-bin curl git \
    build-essential gcc g++ sqlite3 gnupg gosu rsync

if ! command -v node >/dev/null 2>&1 || [ "$(node -v | cut -c2- | cut -d. -f1)" != "$NODE_MAJOR" ]; then
    log "installing Node ${NODE_MAJOR}.x"
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
    apt-get install -y --no-install-recommends nodejs
fi
corepack enable || true

if [ ! -x /usr/local/go/bin/go ] || ! /usr/local/go/bin/go version | grep -q "go${GO_VERSION}"; then
    log "installing Go ${GO_VERSION}"
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz
    rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm -f /tmp/go.tgz
fi
[ -f /etc/profile.d/go.sh ] || echo 'export PATH=/usr/local/go/bin:$PATH' > /etc/profile.d/go.sh

# ------------------------------------------------------------------------------
# 2. Service user + state dirs.
#
# A dedicated system user with an AUTO-ALLOCATED uid/gid (do NOT pin 1000 — it is
# usually the first human login user). TinyCld does not require a specific uid:
# the entrypoint chowns state to whatever user runs it; the Dockerfile's 1000 is
# only a bind-mount-ownership convenience.
# ------------------------------------------------------------------------------
if ! id -u "$RUN_USER" >/dev/null 2>&1; then
    log "creating service user ${RUN_USER}"
    useradd --system --user-group --home-dir "$STATE_DIR" \
        --no-create-home --shell /usr/sbin/nologin "$RUN_USER"
fi
install -d -o "$RUN_USER" -g "$RUN_USER" \
    "$STATE_DIR" "$STATE_DIR/pb_data" "$STATE_DIR/releases" "$STATE_DIR/builds"
install -d -m 0755 /etc/tinycld

# ------------------------------------------------------------------------------
# 3. Let the unprivileged user bind low ports.
#
# TinyCld binds :80/:443 (autocert) and :465/:993 (mail) but drops to the
# unprivileged $RUN_USER (gosu, in the entrypoint). Under Docker this Just Works
# because containers default net.ipv4.ip_unprivileged_port_start=0. On bare metal
# it defaults to 1024, AND the entrypoint chowns the build tree on every boot
# (chown strips file capabilities) — so a CAP_NET_BIND_SERVICE approach (file cap
# or systemd AmbientCapabilities through gosu, which clears the ambient set)
# cannot hold. Lowering the unprivileged-port floor mirrors the Docker behavior,
# needs no caps, and survives every rebuild.
# ------------------------------------------------------------------------------
log "setting net.ipv4.ip_unprivileged_port_start=${UNPRIV_PORT_START}"
echo "net.ipv4.ip_unprivileged_port_start=${UNPRIV_PORT_START}" > /etc/sysctl.d/60-tinycld-lowports.conf
sysctl -w "net.ipv4.ip_unprivileged_port_start=${UNPRIV_PORT_START}"

# ------------------------------------------------------------------------------
# 4. Service env file (root-only). Secrets + optional Sentry/extra config.
# ------------------------------------------------------------------------------
log "writing ${ENV_FILE}"
umask 077
: > "$ENV_FILE"
if [ -n "$SENTRY_DSN" ]; then
    echo "SENTRY_DSN=${SENTRY_DSN}" >> "$ENV_FILE"      # server-side (Go) Sentry
fi
if [ -n "$ENV_EXTRA" ]; then
    printf '%s\n' "$ENV_EXTRA" >> "$ENV_FILE"
fi
chmod 0600 "$ENV_FILE"

# ------------------------------------------------------------------------------
# 5. Build from source + bake (build.sh installs /opt/tinycld-entrypoint.sh).
# ------------------------------------------------------------------------------
log "building from source (this takes a few minutes)"
TINYCLD_VERSION="$TINYCLD_VERSION" \
RUN_USER="$RUN_USER" STATE_DIR="$STATE_DIR" BAKED_DIR="$BAKED_DIR" \
EXPO_PUBLIC_SENTRY_DSN="$SENTRY_DSN" \
${TINYCLD_FEATURES:+TINYCLD_FEATURES="$TINYCLD_FEATURES"} \
    bash "${SCRIPT_DIR}/build.sh"

# ------------------------------------------------------------------------------
# 6. systemd unit. ExecStart is the ENTRYPOINT (the supervisor), not the binary —
#    it owns first-boot seeding, release promotion, and the in-app installer's
#    exit-75 / health-probe / rollback loop. Starts as root so it can chown state
#    dirs, then drops to $RUN_USER via gosu.
# ------------------------------------------------------------------------------
log "writing ${UNIT}"
cat > "$UNIT" <<EOF
[Unit]
Description=TinyCld (${DOMAIN})
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${STATE_DIR}
Environment=TINYCLD_STATE_DIR=${STATE_DIR}
Environment=PRIMARY_DOMAIN=${DOMAIN}
Environment=ADDITIONAL_DOMAINS=${ADDITIONAL_DOMAINS}
Environment=AUTOCERT_ENABLED=true
Environment=PUBLIC_SCHEME=https
EnvironmentFile=${ENV_FILE}
ExecStart=/opt/tinycld-entrypoint.sh
Restart=always
RestartSec=5
# An in-app rebuild runs expo export + go build on the box; give it room.
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
EOF

log "enabling + starting service"
systemctl daemon-reload
systemctl enable tinycld
systemctl restart tinycld

log "done. follow logs with: journalctl -u tinycld -f"
log "the app is serving https://${DOMAIN} once autocert finishes provisioning."
