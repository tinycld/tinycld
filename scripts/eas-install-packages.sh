#!/usr/bin/env bash
# Assemble the pnpm workspace for an EAS build, then install it.
#
# Runs as `eas-build-pre-install` (in package.json) so the workspace is in place
# BEFORE EAS's own install phase — otherwise that phase tries to resolve the
# unpublished @tinycld/core / @tinycld/package-scripts workspace members from the
# npm registry and 404s.
#
# Topology. EAS clones THIS repo (the tinycld app shell) into a build dir and runs
# the build from there. @tinycld/core and @tinycld/package-scripts ship nested
# inside this repo (tinycld/core, tinycld/package-scripts), so they arrive with
# the clone — we do NOT clone them. The optional FEATURE packages (mail, drive, …)
# are independent repos; we clone each as a sibling of the app dir. The workspace
# root is the app dir's parent.
#
#   <workspace root>/                 <- WORKSPACE_ROOT (we write coordination files here)
#     <app dir>/                      <- SHELL_DIR (this repo; EAS names it "build" in cloud)
#       core/                         <- @tinycld/core (nested, ships with the clone)
#       package-scripts/              <- @tinycld/package-scripts (nested, ships with clone)
#     mail/ drive/ calc/ …            <- feature repos we clone as siblings
#
# Local builds (eas:build:ios:local, EAS_NO_VCS=1 + EAS_PROJECT_ROOT=..) copy the
# already-assembled on-disk workspace into the sandbox, so members and the real
# coordination files are already present — the guards below make every step a
# no-op in that case.

set -euo pipefail

REPO_BASE="${TINYCLD_PACKAGES_REPO_BASE:-https://github.com/tinycld}"

# Independent FEATURE repos, cloned as siblings of the app dir. core and
# package-scripts are NOT here — they ship nested inside this repo.
FEATURES=(
    mail
    contacts
    calendar
    drive
    calc
    text
    google-takeout-import
)

SHELL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ROOT="$(cd "${SHELL_DIR}/.." && pwd)"
# The app dir's name varies: EAS clones it as "build" in the cloud; locally it's
# the real shell dir. The reconstructed workspace files must reference it by its
# actual name so pnpm finds the app + its nested core/package-scripts members.
APP_DIR_NAME="$(basename "${SHELL_DIR}")"

for pkg in "${FEATURES[@]}"; do
    dest="${WORKSPACE_ROOT}/${pkg}"
    # Present (non-empty) means the local-build copy already placed it — skip.
    # Only clone the genuinely-missing dir (the cloud case).
    if [ -d "${dest}" ] && [ -n "$(ls -A "${dest}" 2>/dev/null)" ]; then
        echo "==> ${pkg} already present at ${dest}"
    else
        echo "==> Cloning @tinycld/${pkg} -> ${dest}"
        git clone --depth=1 --single-branch "${REPO_BASE}/${pkg}.git" "${dest}"
    fi
done

# Reconstruct the workspace-root coordination files when EAS gave us a bare
# checkout (only the app repo, no committed workspace root). The local-build flow
# already has the real files, so these guards no-op there.
if [ ! -f "${WORKSPACE_ROOT}/package.json" ]; then
    echo "==> Writing workspace-root package.json at ${WORKSPACE_ROOT}"
    cat > "${WORKSPACE_ROOT}/package.json" <<JSON
{
    "name": "@tinycld/workspace",
    "version": "0.0.1",
    "private": true,
    "type": "module",
    "scripts": {
        "postinstall": "tsx scripts/link-members.ts && cd ${APP_DIR_NAME} && pnpm run packages:generate && pnpm run assets:copy-pdfjs"
    },
    "devDependencies": {
        "tsx": "^4.21.0"
    }
}
JSON
fi

if [ ! -f "${WORKSPACE_ROOT}/pnpm-workspace.yaml" ]; then
    echo "==> Writing workspace-root pnpm-workspace.yaml at ${WORKSPACE_ROOT}"
    cat > "${WORKSPACE_ROOT}/pnpm-workspace.yaml" <<YAML
nodeLinker: hoisted
linkWorkspacePackages: true
strictPeerDependencies: false
enablePrePostScripts: true
packages:
  - ${APP_DIR_NAME}
  - ${APP_DIR_NAME}/core
  - ${APP_DIR_NAME}/package-scripts
  - contacts
  - mail
  - calendar
  - drive
  - calc
  - text
  - google-takeout-import
allowBuilds:
  esbuild: true
  '@sentry/cli': true
YAML
fi

# The workspace-root postinstall needs scripts/link-members.ts and
# tinycld.packages.ts (the member-discovery helper the generator also imports).
# These live in the tinycld/workspace repo, which EAS does not clone — so copy
# the canonical versions that ship inside the app shell's repo. (Kept in sync at
# scripts/eas-workspace-files/.)
mkdir -p "${WORKSPACE_ROOT}/scripts"
if [ ! -f "${WORKSPACE_ROOT}/scripts/link-members.ts" ]; then
    cp "${SHELL_DIR}/scripts/eas-workspace-files/link-members.ts" "${WORKSPACE_ROOT}/scripts/link-members.ts"
fi
if [ ! -f "${WORKSPACE_ROOT}/tinycld.packages.ts" ]; then
    cp "${SHELL_DIR}/scripts/eas-workspace-files/tinycld.packages.ts" "${WORKSPACE_ROOT}/tinycld.packages.ts"
fi

# Verify each feature landed as a sibling dir (each carries a manifest.ts).
# Without this guard a failed clone could silently ship a build missing routes.
missing=()
for pkg in "${FEATURES[@]}"; do
    if [ ! -f "${WORKSPACE_ROOT}/${pkg}/manifest.ts" ]; then
        missing+=("@tinycld/${pkg}")
    fi
done
# core + package-scripts must be present nested in the app dir (they ship with
# the clone); verify so a malformed checkout fails loudly rather than 404ing.
[ -f "${SHELL_DIR}/core/package.json" ] || missing+=("@tinycld/core")
[ -f "${SHELL_DIR}/package-scripts/package.json" ] || missing+=("@tinycld/package-scripts")
if [ "${#missing[@]}" -gt 0 ]; then
    echo "FATAL: workspace members not present: ${missing[*]}" >&2
    ls -la "${WORKSPACE_ROOT}" "${SHELL_DIR}" >&2 || true
    exit 1
fi

# Install at the workspace root so members link, then the root postinstall runs
# link-members + the generator. Use the pnpm EAS already provisioned (from this
# repo's packageManager field) — NOT `corepack pnpm`, which re-fetches a pinned
# pnpm and crashes with ERR_VM_DYNAMIC_IMPORT_CALLBACK_MISSING under Node 20.19.
# --no-frozen-lockfile because the reconstructed root has no committed lockfile.
# Export TINYCLD_APP_DIR so link-members.ts (run by the postinstall) resolves the
# nested core / generated dirs under the real app dir name (EAS uses "build").
echo "==> Installing workspace from ${WORKSPACE_ROOT}"
(cd "${WORKSPACE_ROOT}" && TINYCLD_APP_DIR="${APP_DIR_NAME}" pnpm install --no-frozen-lockfile)
