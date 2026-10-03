#!/bin/sh
set -e

# The application runs as this unprivileged user (uid/gid baked into the image
# as 1000:1000; see Dockerfile). The container itself starts as root only long
# enough to fix bind-mount ownership and the git config below. The supervisor
# (exec'd at the end of this script) drops its OWN children to RUN_AS itself —
# do NOT gosu-wrap the supervisor, or its children inherit no privilege to bind
# :80/:443 (gosu would drop this script's process, not the supervisor's).
RUN_AS=tinycld

# Mutable runtime state lives under /workspace (pb_data, releases, builds), OUTSIDE
# the per-build code tree the `current` symlink swaps. The Go binary reads this via
# resolveStateDir(); export it so the supervisor and every `serve` child it starts
# agree on it.
export TINYCLD_STATE_DIR=/workspace

# Trust all git directories for the runtime user. The in-app package operations
# shell out to git (directly for ls-remote, and via `npm pack` for git specs).
# A local file:// remote (a self-hosted/air-gapped base, or the integration
# test's provisioned bare repo) can be owned by a different user, which makes git
# refuse with "detected dubious ownership" (exit 128). These are server-internal
# reads of a trusted, operator-configured remote.
#
# IMPORTANT: git honors `safe.directory=*` (the wildcard) ONLY from a config
# FILE — NOT from `-c safe.directory=*` or the GIT_CONFIG_* env vars (a
# deliberate git restriction so the wildcard can't be injected via the
# command line / environment). So we must WRITE it to the runtime user's global
# config. HOME is /workspace (writable by tinycld); run the config write as that
# user so the file is owned by and read by the git processes the server spawns.
seed_git_safe_directory() {
    if [ "$(id -u)" = "0" ]; then
        gosu "$RUN_AS" git config --global --add safe.directory '*' 2>/dev/null || true
    else
        git config --global --add safe.directory '*' 2>/dev/null || true
    fi
}

# The runnable code tree lives at /workspace/current → /workspace/builds/<id>/tinycld.
# The image bakes a pristine first build at /opt/tinycld-baked (an UNMOUNTED path so a
# bind-mounted /workspace/builds can't shadow it); first boot copies it into builds/
# and points `current` at it.
BAKED_BUILD=/opt/tinycld-baked
CURRENT_LINK=/workspace/current

# PocketBase resolves pb_data / migrations / releases relative to the binary
# unless overridden. Because the binary runs from the per-build
# /workspace/current tree, WITHOUT these every build would get its own pb_data
# (state LOST on each swap). Pin the stateful dirs at the persistent mounts so
# they survive the symlink swap, and migrationsDir at the ACTIVE build's
# migrations (code, which does travel with the build):
#   --dir          pb_data → /workspace/pb_data (persistent)
#   --releasesDir  promoted web bundles → /workspace/releases (persistent)
#   --websiteDir   marketing site → /workspace/website (persistent; populated by
#                  `utils/deploy.sh web`, NOT baked into the image — empty until
#                  the first web deploy, which the server tolerates)
#   --migrationsDir → the active build's server/pb_migrations (the REAL dir the
#                     generator materializes for every build). We deliberately do
#                     NOT use the member-root pb_data→server/pb_migrations symlink:
#                     that symlink is created only by the Dockerfile for the baked
#                     image, NOT by the generator, so a freshly-assembled in-app
#                     build lacks it. Pointing at server/pb_migrations directly
#                     means a newly-installed package's migrations always load and
#                     apply on the post-swap boot.
PB_DATA_DIR=/workspace/pb_data
PB_SERVE_DIRS="--dir=${PB_DATA_DIR} --releasesDir=/workspace/releases --websiteDir=/workspace/website --migrationsDir=${CURRENT_LINK}/server/pb_migrations"

echo "[entrypoint] starting; pwd=$(pwd) user=$(id -un) uid=$(id -u)"

# Adopt the image's baked build when it's newer than what's on the volume.
#
# Two situations both flow through here:
#   1. First boot / empty volume — no live `current`, so we must seed.
#   2. Image redeploy (`deploy.sh` → Dokku rebuild) — the volume already has a
#      `current` from the prior image (or an in-app install), but the NEW image
#      carries a NEWER baked build that must supersede it. A plain redeploy does
#      NOT otherwise re-point `current`, so without this the server keeps running
#      the OLD binary + serving the OLD web bundle (the image change is a no-op).
#
# The baked build is tagged with a unique release id (deploy.sh writes
# tinycld/.release-id; the Dockerfile stages it at
# /opt/tinycld-baked/tinycld/release-staging/<rid>/release-id.txt). We record the
# id we last adopted in /workspace/.baked-release-id and re-adopt only when the
# image's baked id differs — so this fires exactly once per new image and leaves
# in-app-installed builds (whose boots see an UNCHANGED baked id) untouched.
BAKED_MARKER=/workspace/.baked-release-id

baked_release_id() {
    # The staging dir holds exactly one <rid>/ with a release-id.txt.
    for f in "$BAKED_BUILD"/tinycld/release-staging/*/release-id.txt; do
        [ -f "$f" ] && { tr -d '[:space:]' < "$f"; return 0; }
    done
    return 1
}

seed_baked_build() {
    if [ ! -d "$BAKED_BUILD/tinycld" ]; then
        echo "[entrypoint] ERROR: baked build $BAKED_BUILD missing — image is malformed" >&2
        exit 1
    fi

    baked_id=$(baked_release_id || true)
    adopted_id=$(cat "$BAKED_MARKER" 2>/dev/null || echo '')

    # Already on a live build AND this image's baked build is the one we adopted
    # (or an in-app install built on top of it) → nothing to do.
    if [ -e "$CURRENT_LINK/tinycld" ] && [ -n "$baked_id" ] && [ "$baked_id" = "$adopted_id" ]; then
        return 0
    fi

    if [ -e "$CURRENT_LINK/tinycld" ]; then
        echo "[entrypoint] new image detected (baked='$baked_id' adopted='$adopted_id'); adopting baked build over the volume's current"
    else
        echo "[entrypoint] no live build; seeding from $BAKED_BUILD (baked='$baked_id')"
    fi

    # Name the build dir by its baked id so successive images coexist (and the
    # Go pruneBuilds can manage them). Fall back to build-baked when id is absent.
    dest="/workspace/builds/build-baked-${baked_id:-unknown}"
    mkdir -p /workspace/builds
    if [ ! -e "$dest/tinycld/tinycld" ]; then
        rm -rf "$dest.tmp"
        cp -a "$BAKED_BUILD" "$dest.tmp"
        rm -rf "$dest"
        mv "$dest.tmp" "$dest"
    fi

    # Record the outgoing build id so the supervisor's cold-rollback path (Go,
    # State.RollbackCurrent) has a target.
    prev_dest=$(readlink "$CURRENT_LINK" 2>/dev/null || echo '')
    if [ -n "$prev_dest" ]; then
        prev_id=$(basename "$(dirname "$prev_dest")")
        [ -n "$prev_id" ] && [ "$prev_id" != "build-baked-${baked_id:-unknown}" ] && \
            printf '%s' "$prev_id" > /workspace/.previous-build 2>/dev/null || true
    fi

    ln -sfn "$dest/tinycld" "$CURRENT_LINK.tmp"
    mv -T "$CURRENT_LINK.tmp" "$CURRENT_LINK"
    [ -n "$baked_id" ] && printf '%s' "$baked_id" > "$BAKED_MARKER" 2>/dev/null || true

    # The runtime user owns the build tree + symlink so a later in-app rebuild can
    # write sibling build dirs and atomically re-point current. Only when we're root.
    if [ "$(id -u)" = "0" ]; then
        chown -R "$RUN_AS:$RUN_AS" /workspace/builds 2>/dev/null || true
        chown -h "$RUN_AS:$RUN_AS" "$CURRENT_LINK" 2>/dev/null || true
        chown "$RUN_AS:$RUN_AS" "$BAKED_MARKER" 2>/dev/null || true
    fi
    echo "[entrypoint] seeded current -> $(readlink "$CURRENT_LINK")"
}

# Ensure the bind-mounted data directories are writable by the runtime user.
#
# When a host bind-mount target (./pb_data → /workspace/pb_data in
# docker-compose.yml) doesn't exist yet, the Docker daemon creates it owned by
# root:root. The unprivileged tinycld user then can't open the SQLite database —
# PocketBase fails with "unable to open database file (14)" and the container
# crash-loops. Reported in https://github.com/tinycld/app/issues/26.
# (core/types is NOT bind-mounted — it's regenerated inside each build tree on
# boot, so it never needs an ownership fix-up here.)
#
# The same applies to ./builds and ./releases when an operator backs them with a
# persistent volume/bind-mount so installed-package archives and the promoted web
# bundle (incl. the native OTA bundles staged into each build's release dir)
# survive container restarts. Without the chown the install pipeline can't write
# the archive and fails at "archive build".
#
# We run this as root (the container's start user) and chown the dirs to the
# runtime user before dropping privileges. Only runs when we're actually root;
# if an operator overrode the start user to non-root they're responsible for
# host-side ownership (and the chown would fail anyway), so we skip silently.
fix_data_dir_ownership() {
    [ "$(id -u)" = "0" ] || return 0

    # The expected owner is whatever uid:gid $RUN_AS actually resolves to. The
    # Dockerfile lets an operator build with a non-default uid/gid
    # (--build-arg TINYCLD_UID/GID), so we must not hardcode 1000:1000 — that
    # would re-run the full recursive chown on every boot for a non-default uid.
    want_owner="$(id -u "$RUN_AS"):$(id -g "$RUN_AS")"

    for dir in \
        /workspace/pb_data \
        /workspace/builds \
        /workspace/releases; do
        mkdir -p "$dir"
        # Skip the (potentially large) recursive chown when the top-level dir is
        # already owned correctly — the steady state after first run, so normal
        # restarts pay nothing. A fresh root-owned mount triggers the fix-up.
        owner=$(stat -c '%u:%g' "$dir" 2>/dev/null || echo '')
        if [ "$owner" != "$want_owner" ]; then
            echo "[entrypoint] fixing ownership of $dir (was '$owner') -> $RUN_AS"
            chown -R "$RUN_AS:$RUN_AS" "$dir"
        fi
    done
}

fix_data_dir_ownership
seed_git_safe_directory
seed_baked_build

# Promote the staged release to /workspace/tinycld/releases/. Runs on every
# container start; idempotent.
#
# /workspace/tinycld/releases is typically the container's writable layer
# (compose-style deploys) and starts empty on every fresh container; Dokku-style
# deploys may back it with a persistent volume so old releases survive container
# replacement. Either way the promote logic below is the same: copy the staged
# tree off the image, swap the `current` symlink atomically.
#
# Layout produced under /workspace/tinycld/releases/:
#   <id>/             per-release dir: app.html + release-id.txt
#   _static/          cross-release asset pool:
#     _expo/static/...    (hashed names; newest release wins a collision)
#     assets/...           (mostly hashed; a few stable names like
#                           app-icon.png get overwritten per deploy)
#   current → <id>    SPA fallback reads <current>/app.html
#
# Why a pool: asset filenames are hashed, so files from different releases
# mostly coexist without collision. Stale tabs that dynamic-import a chunk
# see their hashed filename in the pool until a future prune wipes old
# entries. The Go server serves /_expo/static/ and /assets/ from _static/
# directly — there is no per-request release lookup.
#
# The hashes are NOT content hashes: Expo leaves the paths of a chunk's async
# imports out of the hash, so a bundle can keep its name while the chunk table
# inside it changes. cp -a below lets the newest release's bytes win, and the
# server serves the pool no-cache (revalidated by mtime) and pins the shell's
# asset URLs to the release id, so a browser never reuses a same-named copy
# from an earlier release. See coreserver.PoolAssets.
promote_release() {
    staging_dir=/workspace/current/release-staging
    releases_dir=/workspace/releases
    pool_dir="$releases_dir/_static"

    echo "[entrypoint] promote_release: staging=$staging_dir releases=$releases_dir"
    echo "[entrypoint] staging contents:"
    ls -la "$staging_dir" 2>&1 | sed 's/^/[entrypoint]   /' || true
    echo "[entrypoint] releases dir contents (before):"
    ls -la "$releases_dir" 2>&1 | sed 's/^/[entrypoint]   /' || true

    [ -d "$staging_dir" ] || {
        echo "[entrypoint] WARN: $staging_dir missing; skipping release promotion (SPA fallback will 404)"
        return 0
    }

    mkdir -p "$releases_dir" "$pool_dir/_expo/static" "$pool_dir/assets"

    # Pick the MOST RECENTLY MODIFIED staging dir, not the first by glob order.
    # After an in-app package install the staging dir holds BOTH the base image's
    # release (e.g. 2026-…-deadbee, staged at image-build time and never removed)
    # AND the install's freshly-built bundle (install-<ts>). Globbing alphabetically
    # would pick the base dir (`2026-…` sorts before `install-…`) and promote a
    # bundle WITHOUT the just-installed package's routes — the SPA then 404s
    # ("Unmatched Route") on that package. Newest-mtime always selects the install's
    # bundle (or, on first boot, the only dir present). `ls -1dt` lists dirs
    # newest-first; we take the first one that carries a release-id.txt.
    release_id=""
    for d in $(ls -1dt "$staging_dir"/*/ 2>/dev/null); do
        [ -d "$d" ] || continue
        if [ -f "$d/release-id.txt" ]; then
            release_id=$(cat "$d/release-id.txt")
            echo "[entrypoint] found release-id.txt in $d -> '$release_id' (newest staging dir)"
            break
        else
            echo "[entrypoint] WARN: $d has no release-id.txt"
        fi
    done

    if [ -z "$release_id" ]; then
        echo "[entrypoint] ERROR: no release-id.txt found under $staging_dir; aborting"
        exit 1
    fi

    src="$staging_dir/$release_id"
    dst="$releases_dir/$release_id"

    # Merge this release's asset trees into the cross-release pool.
    # cp -a (no -n) is used deliberately: the current release's copy must
    # win a name collision — Expo can re-emit a bundle under an unchanged
    # name with a different chunk table (see the pool note above), and the
    # unhashed names under assets/ (app-icon.png, app-splash.png) change
    # per deploy. cp -a keeps each file's build mtime, which is the
    # validator the server's no-cache policy revalidates against. The
    # whole tree is a few MB so the redundant rewrites cost nothing.
    if [ -d "$src/_expo/static" ]; then
        echo "[entrypoint] merging _expo/static into pool"
        cp -a "$src/_expo/static/." "$pool_dir/_expo/static/"
    fi
    if [ -d "$src/assets" ]; then
        echo "[entrypoint] merging assets into pool"
        cp -a "$src/assets/." "$pool_dir/assets/"
    fi

    # Treat a previously-promoted dst as valid only if it has app.html.
    # If a prior boot left a half-promoted tree (interrupted copy, etc.),
    # the [ -d "$dst" ] check below would skip and reuse the corrupt
    # tree; this guard wipes it so the next attempt re-promotes cleanly.
    if [ -d "$dst" ] && [ ! -f "$dst/app.html" ]; then
        echo "[entrypoint] WARN: $dst exists but app.html is missing; clearing for re-promote"
        rm -rf "$dst"
    fi

    if [ ! -d "$dst" ]; then
        echo "[entrypoint] promoting release $release_id ($src -> $dst, app.html + release-id.txt + manifest.json)"
        rm -rf "$dst.tmp"
        mkdir "$dst.tmp"
        cp -a "$src/app.html" "$dst.tmp/"
        cp -a "$src/release-id.txt" "$dst.tmp/"
        # manifest.json is present only on release builds (pinned-release recipe);
        # the /api/release handler degrades gracefully when it's absent.
        if [ -f "$src/manifest.json" ]; then
            cp -a "$src/manifest.json" "$dst.tmp/"
        fi
        mv "$dst.tmp" "$dst"
        echo "[entrypoint] promotion complete; size=$(du -sh "$dst" 2>/dev/null | cut -f1)"
    else
        echo "[entrypoint] release $release_id already on volume; skipping per-release copy"
    fi

    # Atomic symlink swap: write to current.tmp, then mv -T over current.
    ln -sfn "$release_id" "$releases_dir/current.tmp"
    mv -T "$releases_dir/current.tmp" "$releases_dir/current"
    echo "[entrypoint] current -> $(readlink "$releases_dir/current")"

    if [ -f "$releases_dir/current/app.html" ]; then
        echo "[entrypoint] app.html present ($(wc -c < "$releases_dir/current/app.html") bytes)"
    else
        echo "[entrypoint] ERROR: $releases_dir/current/app.html missing — SPA fallback will 404"
        ls -la "$releases_dir/current/" 2>&1 | sed 's/^/[entrypoint]   /' || true
        exit 1
    fi

    pool_size=$(du -sh "$pool_dir" 2>/dev/null | cut -f1)
    echo "[entrypoint] pool size: $pool_size"
}

promote_release

# Validate the domain env vars and derive TINYCLD_PUBLIC_URL. The supervisor
# (exec'd at the end of this script) reads PRIMARY_DOMAIN, AUTOCERT_ENABLED,
# ADDITIONAL_DOMAINS and HTTP_ADDR itself from its environment and builds its
# own `--http=… --https=…` mode flags — this script no longer builds serve
# args. It still validates up front, so a bad domain fails fast with a clear
# message instead of surfacing from inside the supervisor or autocert.
#
#   PRIMARY_DOMAIN     the canonical domain (first cert SAN; also feeds the
#                      user-facing setup URL via TINYCLD_PUBLIC_URL below).
#   ADDITIONAL_DOMAINS comma-separated extra domains added to the cert request.
#   AUTOCERT_ENABLED   true/false — whether to provision Let's Encrypt certs
#                      and bind :80/:443 directly.
#   HTTP_ADDR          plain-HTTP bind when autocert is off (default :7090).

# Trim surrounding whitespace; a value of "" or "   " counts as unset (users
# frequently leave `PRIMARY_DOMAIN:` blank in compose YAML to disable autocert).
# Re-export: this script's own validation/URL logic needs the trimmed value,
# and the exported value is what the supervisor's child process inherits.
PRIMARY_DOMAIN=$(printf '%s' "${PRIMARY_DOMAIN:-}" | awk '{$1=$1};1')
export PRIMARY_DOMAIN

# Normalize AUTOCERT_ENABLED to a strict 1/0 (accepts true/TRUE/yes/1) for this
# script's own branching; export the ORIGINAL value so the supervisor (which
# does its own, identical normalization) sees what the operator set.
export AUTOCERT_ENABLED
case "$(printf '%s' "${AUTOCERT_ENABLED:-}" | tr '[:upper:]' '[:lower:]' | awk '{$1=$1};1')" in
    1|true|yes|on) AUTOCERT_ON=1 ;;
    *)             AUTOCERT_ON=0 ;;
esac

# ADDITIONAL_DOMAINS and HTTP_ADDR are read only by the supervisor below (this
# script only validates ADDITIONAL_DOMAINS' entries); export unconditionally so
# a value set by the operator — or HTTP_ADDR's default, assigned below — always
# reaches the child, even when it was never in this process's own environment.
export ADDITIONAL_DOMAINS

# validate_domain: reject shell-truthy-but-bogus values (no dot, illegal
# characters, leading/trailing dot or hyphen) before autocert fails them at a
# less-obvious layer inside the supervisor's child.
validate_domain() {
    case "$1" in
        *[!A-Za-z0-9.-]*|.*|*.|-*|*-)
            echo "[entrypoint] ERROR: invalid domain token: '$1'" >&2
            echo "[entrypoint] Domains must be hostnames like 'tinycld.example.com'." >&2
            exit 1
            ;;
    esac
    case "$1" in
        *.*) ;;
        *)
            echo "[entrypoint] ERROR: domain token has no dot, doesn't look like a domain: '$1'" >&2
            echo "[entrypoint] Domains must be hostnames like 'tinycld.example.com'." >&2
            exit 1
            ;;
    esac
}

if [ "$AUTOCERT_ON" = "1" ] && [ -n "$PRIMARY_DOMAIN" ]; then
    validate_domain "$PRIMARY_DOMAIN"

    # Validate each ADDITIONAL_DOMAINS entry (comma-separated; surrounding
    # whitespace per entry is tolerated) up front. The supervisor re-splits and
    # uses them itself; this is purely a fail-fast check.
    OLD_IFS=$IFS
    IFS=','
    for dom in ${ADDITIONAL_DOMAINS:-}; do
        IFS=$OLD_IFS
        dom=$(printf '%s' "$dom" | awk '{$1=$1};1')
        [ -z "$dom" ] && { IFS=','; continue; }
        validate_domain "$dom"
        IFS=','
    done
    IFS=$OLD_IFS

    echo "[entrypoint] Running with autocert HTTPS on: $PRIMARY_DOMAIN${ADDITIONAL_DOMAINS:+, $ADDITIONAL_DOMAINS}"

    # Setup URL (and any other user-facing URL) should use the canonical
    # HTTPS domain, not PB's bind address. Only set if the operator hasn't
    # pinned TINYCLD_PUBLIC_URL explicitly.
    export TINYCLD_PUBLIC_URL="${TINYCLD_PUBLIC_URL:-https://$PRIMARY_DOMAIN}"
else
    if [ "$AUTOCERT_ON" = "1" ] && [ -z "$PRIMARY_DOMAIN" ]; then
        echo "[entrypoint] AUTOCERT_ENABLED is set but PRIMARY_DOMAIN is empty; falling back to plain HTTP" >&2
    fi
    HTTP_ADDR="${HTTP_ADDR:-0.0.0.0:7090}"
    export HTTP_ADDR
    echo "[entrypoint] Serving plain HTTP on $HTTP_ADDR (map a host port to this with -p / compose ports)"

    # Behind a reverse proxy on PRIMARY_DOMAIN, the public URL is still that
    # domain, but the container can't tell whether the proxy terminates TLS.
    # Default the scheme to https (the common production case) and let operators
    # override with PUBLIC_SCHEME=http for a plain-HTTP proxy, or pin the whole
    # URL via TINYCLD_PUBLIC_URL. Derived here so the printed setup URL is right.
    if [ -n "$PRIMARY_DOMAIN" ]; then
        validate_domain "$PRIMARY_DOMAIN"
        PUBLIC_SCHEME=$(printf '%s' "${PUBLIC_SCHEME:-https}" | tr '[:upper:]' '[:lower:]' | awk '{$1=$1};1')
        case "$PUBLIC_SCHEME" in
            http|https) ;;
            *)
                echo "[entrypoint] WARN: PUBLIC_SCHEME='$PUBLIC_SCHEME' is not http/https; defaulting to https" >&2
                PUBLIC_SCHEME=https
                ;;
        esac
        export TINYCLD_PUBLIC_URL="${TINYCLD_PUBLIC_URL:-$PUBLIC_SCHEME://$PRIMARY_DOMAIN}"
    fi
fi

# RECOVERY HATCH. With TINYCLD_RESCUE=1 the container runs the given command (or
# an interactive shell) INSTEAD of handing over to the supervisor below.
#
# Why this exists: without it, a command passed to `docker run` / `dokku run` is
# never executed as a command — it would be handed to the supervisor (and from
# there to `serve`) as FLAGS. When a boot-time failure (a migration that cannot
# apply, a corrupt pb_data) kills the server before it binds a port, `dokku
# enter` has no running container to attach to and `dokku run <cmd>` just
# reboots the crashing server. The instance is then unreachable by any in-band
# route, and repairing it means host root access to the volume. That is what
# turned a failed boards install into a full outage on tinycld.org.
#
# The hatch never triggers on its own: it is opt-in via an env var an operator
# sets deliberately, e.g.
#
#   dokku run -e TINYCLD_RESCUE=1 tinycld.org sqlite3 /workspace/pb_data/data.db '.tables'
#
# It runs as the unprivileged runtime user, exactly like serve, so it grants no
# privilege that a normal boot does not already have.
if [ "${TINYCLD_RESCUE:-}" = "1" ]; then
    echo "[entrypoint] RESCUE MODE: skipping serve"
    echo "[entrypoint] state dir: $TINYCLD_STATE_DIR  current build: $(readlink -f "$CURRENT_LINK" 2>/dev/null || echo '<unresolved>')"
    if [ "$#" -eq 0 ]; then
        set -- /bin/sh
    fi
    echo "[entrypoint] running: $*"
    RESCUE_CODE=0
    if [ "$(id -u)" = "0" ]; then
        gosu "$RUN_AS" "$@" || RESCUE_CODE=$?
    else
        "$@" || RESCUE_CODE=$?
    fi
    echo "[entrypoint] rescue command exited with code $RESCUE_CODE"
    exit "$RESCUE_CODE"
fi

# Hand over to the supervisor. It holds the public ports, starts `serve`
# children with PB_SERVE_DIRS, builds its own mode flags from the env
# exported above, runs the restart/health-check/rollback cycle that used to
# live in this script (core/server/supervise), and drops ITS OWN children to
# $RUN_AS — so this exec must NOT go through gosu even when we're root (gosu
# would drop this script's privilege, not the children the supervisor forks
# after it).
#
# exec (not a backgrounded call checked for an exit code): the supervisor is
# the long-lived PID this container/unit tracks from here on, same as `serve`
# was before this script owned a restart loop. The baked binary is used
# deliberately, never $CURRENT_LINK/tinycld: only the baked binary carries the
# cap_net_bind_service capability (needed to bind :80/:443 as a non-root
# $RUN_AS), and chown strips capabilities, so a rebuilt binary never has it.
echo "[entrypoint] handing over to the supervisor"
exec /opt/tinycld-baked/tinycld/tinycld supervise $PB_SERVE_DIRS
