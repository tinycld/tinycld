#!/usr/bin/env bash
# Emit the OCI image-inventory key=value pairs derived from a pinned-release
# manifest.json (produced by utils/lib/pin-release.ts and uploaded as a GitHub
# Release asset). One source of truth for BOTH consumers in docker-publish.yml:
#
#   - the per-arch build, which feeds these as `labels:` to build-push-action
#   - the merge job, which turns them into `--annotation=index:<k>=<v>` args on
#     the multi-arch manifest list
#
# Keeping the jq here (rather than duplicated inline in two workflow steps)
# stops the two consumers from silently drifting — labels and index annotations
# must stay identical.
#
# Usage:  release-image-inventory.sh <manifest.json> <repository>
#   <manifest.json>  path to the downloaded manifest
#   <repository>     owner/repo, for the image.source label (pass $GITHUB_REPOSITORY)
#
# Output: lines of `key=value`, e.g.
#   org.opencontainers.image.version=v0.0.4
#   org.tinycld.packages=app=v0.0.4,mail=v0.0.2,...
#
# org.tinycld.packages is a compact comma-separated app=tag,member=tag list.
set -euo pipefail

MANIFEST="${1:?usage: release-image-inventory.sh <manifest.json> <repository>}"
REPOSITORY="${2:?usage: release-image-inventory.sh <manifest.json> <repository>}"

APP_TAG=$(jq -r '.appTag' "$MANIFEST")
APP_SHA=$(jq -r '.appSha' "$MANIFEST")
RELEASED_AT=$(jq -r '.releasedAt' "$MANIFEST")
PKGS=$(jq -r '[ "app=" + .appTag ] + [ .members[] | .name + "=" + .tag ] | join(",")' "$MANIFEST")

cat <<EOF
org.opencontainers.image.version=${APP_TAG}
org.opencontainers.image.revision=${APP_SHA}
org.opencontainers.image.created=${RELEASED_AT}
org.opencontainers.image.source=https://github.com/${REPOSITORY}
org.tinycld.packages=${PKGS}
EOF
