#!/usr/bin/env bash
# Assemble a pnpm workspace from a pinned-release manifest.json (produced by
# utils/lib/pin-release.ts and uploaded as a GitHub Release asset). One source
# of truth for BOTH release workflows:
#
#   - docker-publish.yml, which builds the multi-arch container image
#   - release-binaries.yml, which cross-compiles the single-binary self-host
#     artifact
#
# Keeping the assembly here (rather than duplicated inline in two workflows)
# stops them from silently drifting: the container and the binary must be built
# from the SAME pinned member set, or a release ships two artifacts that don't
# agree about what code they contain.
#
# The resulting workspace is byte-identical to what the release author built
# locally, so rebuilding the same tag reproduces the same artifacts later.
#
# Usage:  ci-assemble-workspace.sh <assets-dir>
#   <assets-dir>  directory holding the downloaded manifest.json,
#                 pnpm-lock.yaml, and package-versions.json release assets
#
# Run from the workspace root (the dir holding the already-checked-out
# tinycld/ member). Clones each sibling at its pinned sha, lets bootstrap write
# the workspace-root scaffolding, and drops the pinned lockfile + version pins
# into place.
set -euo pipefail

ASSETS="${1:?usage: ci-assemble-workspace.sh <assets-dir>}"

MANIFEST="${ASSETS}/manifest.json"
LOCKFILE="${ASSETS}/pnpm-lock.yaml"
VERSIONS="${ASSETS}/package-versions.json"

for f in "$MANIFEST" "$LOCKFILE" "$VERSIONS"; do
    [ -f "$f" ] || { echo "::error::missing release asset: $f" >&2; exit 1; }
done

echo "Manifest contents:"
cat "$MANIFEST"

# Clone every sibling at the sha the release pinned. A tag would float if it
# were ever moved; the sha cannot.
jq -r '.members[] | "\(.name)\t\(.repo)\t\(.sha)"' "$MANIFEST" |
    while IFS=$'\t' read -r name repo sha; do
        echo "Cloning ${repo} @ ${sha} → ${name}"
        git clone --quiet "https://github.com/${repo}.git" "${name}"
        git -C "${name}" fetch --quiet origin "${sha}"
        git -C "${name}" checkout --quiet "${sha}"
    done

# bootstrap sees every member already cloned (tinycld + each sibling) and just
# writes the workspace-root scaffolding (package.json + pnpm-workspace.yaml +
# .npmrc + tinycld.packages.ts + scripts/link-members.ts). --assemble-only is
# non-destructive: it skips any member already present, cloning only what's
# missing. (Replaces the v1 --tooling flag, removed in bootstrap v2.)
TINYCLD_REPO_BASE=https://github.com/tinycld npx @tinycld/bootstrap@latest --assemble-only \
    --with contacts --with mail --with calendar --with drive \
    --with calc --with text --with google-takeout-import

# Drop the pinned lockfile into the workspace root so a later
# `pnpm install --frozen-lockfile` reproduces the exact dependency set.
cp "$LOCKFILE" pnpm-lock.yaml

# Drop the framework/native version pins into the workspace root. The
# Dockerfile COPYs package-versions.json into the image and the OTA rebuild
# reads it to regenerate the pnpm `overrides:` block. bootstrap doesn't write
# it, so the release asset is its only source in CI.
cp "$VERSIONS" package-versions.json

# Hand the manifest to the Docker build so it's baked into the image and served
# by /api/release (the About panel reads it). The wildcard COPY in the
# Dockerfile makes it optional, so this is the only wiring. Destination must be
# tinycld/ — the Dockerfile COPYs tinycld/.release-manifest*; anywhere else and
# the bake silently no-ops (the wildcard COPY tolerates the miss).
cp "$MANIFEST" tinycld/.release-manifest
