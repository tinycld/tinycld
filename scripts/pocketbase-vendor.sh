#!/usr/bin/env bash
# Imports a pristine upstream PocketBase release onto the vendor/pocketbase
# branch. Merge that branch afterwards to bring the release into the fork.
# See third_party/pocketbase/FORK.md.
#
# Usage: scripts/pocketbase-vendor.sh <upstream tag, e.g. v0.40.4>
set -euo pipefail

tag="${1:?usage: $0 <upstream tag>}"
branch="vendor/pocketbase"
prefix="third_party/pocketbase"
upstream="https://github.com/pocketbase/pocketbase.git"

repo="$(git rev-parse --show-toplevel)"
cd "$repo"

if ! git rev-parse --verify --quiet "$branch" >/dev/null; then
    git fetch origin "$branch:$branch"
fi

work="$(mktemp -d)"
trap 'git -C "$repo" worktree remove --force "$work/wt" 2>/dev/null || true; rm -rf "$work"' EXIT

git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$tag" "$upstream" "$work/upstream"
git worktree add --quiet "$work/wt" "$branch"

rm -rf "${work:?}/wt/$prefix"
mkdir -p "$work/wt/$prefix"
git -C "$work/upstream" archive "$tag" | tar -x -C "$work/wt/$prefix"

# The fork embeds only the prebuilt admin UI. Keep ui/dist and the files that
# embed it; drop the UI sources.
find "$work/wt/$prefix/ui" -mindepth 1 -maxdepth 1 \
    ! -name dist ! -name embed.go ! -name embed_no_ui.go ! -name README.md \
    ! -name .env ! -name .env.development ! -name .gitignore \
    -exec rm -rf {} +

git -C "$work/wt" add -A "$prefix"
if git -C "$work/wt" diff --cached --quiet; then
    echo "$branch already holds $tag"
    exit 0
fi
git -C "$work/wt" commit --quiet -m "vendor: pocketbase $tag (pristine upstream, ui sources stripped)"

echo "Imported $tag onto $branch. Next:"
echo "  git merge $branch"
echo "  git push origin $branch"
