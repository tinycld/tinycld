#!/usr/bin/env bash
# Clone the feature packages that aren't bundled into this repo, as siblings of
# the app shell, so the npm workspace install picks them up as members. Runs
# during EAS builds (via `eas-build-post-install` in package.json) so the
# production iOS/Android bundle includes their routes, settings panels, etc.
#
# Workspace model: the app shell and every feature package are pnpm workspace
# members under a shared workspace root (one level up from this repo). We clone
# each feature next to the shell, ensure a workspace-root package.json +
# pnpm-workspace.yaml exist, then a root `pnpm install` (run by the EAS install
# step) links them all.
#
# Local dev does not need this — devs clone siblings next to the shell and run
# `pnpm install` at the workspace root.

set -euo pipefail

REPO_BASE="${TINYCLD_PACKAGES_REPO_BASE:-https://github.com/tinycld}"

# Standalone members cloned as siblings of the app shell. core is a member too
# (it is no longer bundled inside the shell); feature packages follow.
PACKAGES=(
    core
    mail
    contacts
    calendar
    drive
    calc
    text
    google-takeout-import
)

# The app shell is at <workspace>/app; siblings live at <workspace>/<pkg>.
SHELL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ROOT="$(cd "${SHELL_DIR}/.." && pwd)"

for pkg in "${PACKAGES[@]}"; do
    dest="${WORKSPACE_ROOT}/${pkg}"
    # Treat the member as present if its dir exists and is non-empty. We can't
    # key off ${dest}/.git: in the local-build flow EAS copies the whole
    # workspace into the build sandbox (per the root .easignore) WITHOUT each
    # member's .git, so the source is already here as a plain directory. Only
    # clone when the dir is genuinely missing/empty (the cloud bare-checkout case).
    if [ -d "${dest}" ] && [ -n "$(ls -A "${dest}" 2>/dev/null)" ]; then
        echo "==> ${pkg} already present at ${dest}"
    else
        echo "==> Cloning @tinycld/${pkg} -> ${dest}"
        git clone --depth=1 --single-branch "${REPO_BASE}/${pkg}.git" "${dest}"
    fi
done

# Ensure the workspace-root coordination files exist (package.json +
# pnpm-workspace.yaml). In CI/EAS the workspace root may be a bare checkout dir
# without them — the tinycld/workspace repo is not cloned, only the app shell is
# and this script reconstructs the siblings around it. pnpm discovers members
# from pnpm-workspace.yaml (not an npm "workspaces" array) and reads its linker /
# peer / build settings from there too.
if [ ! -f "${WORKSPACE_ROOT}/package.json" ]; then
    echo "==> Writing workspace-root package.json at ${WORKSPACE_ROOT}"
    cat > "${WORKSPACE_ROOT}/package.json" <<'JSON'
{
    "name": "@tinycld/workspace",
    "version": "0.0.1",
    "private": true,
    "type": "module",
    "scripts": {
        "postinstall": "tsx scripts/link-members.ts && cd app && pnpm run packages:generate && pnpm run assets:copy-pdfjs"
    },
    "devDependencies": {
        "tsx": "^4.21.0"
    },
    "packageManager": "pnpm@11.3.0"
}
JSON
fi

if [ ! -f "${WORKSPACE_ROOT}/pnpm-workspace.yaml" ]; then
    echo "==> Writing workspace-root pnpm-workspace.yaml at ${WORKSPACE_ROOT}"
    cat > "${WORKSPACE_ROOT}/pnpm-workspace.yaml" <<'YAML'
nodeLinker: hoisted
linkWorkspacePackages: true
strictPeerDependencies: false
enablePrePostScripts: true
packages:
  - app
  - app/package-scripts
  - core
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
# app/scripts/eas-workspace-files/.)
mkdir -p "${WORKSPACE_ROOT}/scripts"
if [ ! -f "${WORKSPACE_ROOT}/scripts/link-members.ts" ]; then
    cp "${SHELL_DIR}/scripts/eas-workspace-files/link-members.ts" "${WORKSPACE_ROOT}/scripts/link-members.ts"
fi
if [ ! -f "${WORKSPACE_ROOT}/tinycld.packages.ts" ]; then
    cp "${SHELL_DIR}/scripts/eas-workspace-files/tinycld.packages.ts" "${WORKSPACE_ROOT}/tinycld.packages.ts"
fi

# Verify each package landed as a sibling dir. Feature packages carry a
# manifest.ts; core does not (it is the shared lib, not a feature) — check its
# package.json instead. Without this guard a failed clone could let the build
# ship without feature routes.
missing=()
for pkg in "${PACKAGES[@]}"; do
    marker="manifest.ts"
    [ "${pkg}" = "core" ] && marker="package.json"
    if [ ! -f "${WORKSPACE_ROOT}/${pkg}/${marker}" ]; then
        missing+=("@tinycld/${pkg}")
    fi
done
if [ "${#missing[@]}" -gt 0 ]; then
    echo "FATAL: feature packages did not clone: ${missing[*]}" >&2
    ls -la "${WORKSPACE_ROOT}" >&2 || true
    exit 1
fi

# Install at the workspace root so members link, then generate (the root
# postinstall runs link-members + the generator). Use pnpm to match the
# workspace's package manager; --no-frozen-lockfile because the reconstructed
# root has no committed pnpm-lock.yaml on a fresh EAS checkout.
echo "==> Installing workspace from ${WORKSPACE_ROOT}"
(cd "${WORKSPACE_ROOT}" && corepack pnpm install --no-frozen-lockfile)
