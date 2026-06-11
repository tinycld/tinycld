#!/bin/sh
set -e

HEALTH_PORT=19876

# The application runs as this unprivileged user (uid/gid baked into the image
# as 1000:1000; see Dockerfile). The container itself starts as root only long
# enough to fix bind-mount ownership, then drops to RUN_AS via gosu.
RUN_AS=tinycld

# Mutable runtime state lives under /workspace (pb_data, releases, builds), OUTSIDE
# the per-build code tree the `current` symlink swaps. The Go binary reads this via
# resolveStateDir(); export it so every invocation (serve, health probe) agrees.
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
#   --migrationsDir → the active build's server/pb_migrations (the REAL dir the
#                     generator materializes for every build). We deliberately do
#                     NOT use the member-root pb_data→server/pb_migrations symlink:
#                     that symlink is created only by the Dockerfile for the baked
#                     image, NOT by the generator, so a freshly-assembled in-app
#                     build lacks it. Pointing at server/pb_migrations directly
#                     means a newly-installed package's migrations always load and
#                     apply on the post-swap boot.
PB_DATA_DIR=/workspace/pb_data
PB_SERVE_DIRS="--dir=${PB_DATA_DIR} --releasesDir=/workspace/releases --migrationsDir=${CURRENT_LINK}/server/pb_migrations"

echo "[entrypoint] starting; pwd=$(pwd) user=$(id -un) uid=$(id -u)"

# Seed the first build on a fresh deployment. When /workspace/current is missing or
# dangling (first boot, or a bind-mounted empty /workspace/builds), copy the pristine
# baked build into builds/build-baked and point current at it. Idempotent: a healthy
# current symlink short-circuits.
seed_baked_build() {
    if [ -e "$CURRENT_LINK/tinycld" ]; then
        return 0
    fi
    echo "[entrypoint] no live build; seeding from $BAKED_BUILD"
    if [ ! -d "$BAKED_BUILD/tinycld" ]; then
        echo "[entrypoint] ERROR: baked build $BAKED_BUILD missing — image is malformed" >&2
        exit 1
    fi
    dest=/workspace/builds/build-baked
    mkdir -p /workspace/builds
    if [ ! -e "$dest/tinycld/tinycld" ]; then
        rm -rf "$dest.tmp"
        cp -a "$BAKED_BUILD" "$dest.tmp"
        rm -rf "$dest"
        mv "$dest.tmp" "$dest"
    fi
    ln -sfn "$dest/tinycld" "$CURRENT_LINK.tmp"
    mv -T "$CURRENT_LINK.tmp" "$CURRENT_LINK"
    # The runtime user owns the build tree + symlink so a later in-app rebuild can
    # write sibling build dirs and atomically re-point current. Only when we're root.
    if [ "$(id -u)" = "0" ]; then
        chown -R "$RUN_AS:$RUN_AS" /workspace/builds 2>/dev/null || true
        chown -h "$RUN_AS:$RUN_AS" "$CURRENT_LINK" 2>/dev/null || true
    fi
    echo "[entrypoint] seeded current -> $(readlink "$CURRENT_LINK")"
}

# Ensure the bind-mounted data directories are writable by the runtime user.
#
# When a host bind-mount target (./pb_data → /workspace/tinycld/pb_data and
# ./types → /workspace/tinycld/core/types in docker-compose.yml) doesn't exist
# yet, the Docker daemon creates it owned by root:root. The
# unprivileged tinycld user then can't open the SQLite database — PocketBase
# fails with "unable to open database file (14)" and the container crash-loops.
# Reported in https://github.com/tinycld/app/issues/26.
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

    for dir in \
        /workspace/pb_data \
        /workspace/builds \
        /workspace/releases; do
        mkdir -p "$dir"
        # Skip the (potentially large) recursive chown when the top-level dir is
        # already owned correctly — the steady state after first run, so normal
        # restarts pay nothing. A fresh root-owned mount triggers the fix-up.
        owner=$(stat -c '%u:%g' "$dir" 2>/dev/null || echo '')
        if [ "$owner" != "1000:1000" ]; then
            echo "[entrypoint] fixing ownership of $dir (was '$owner') -> $RUN_AS"
            chown -R "$RUN_AS:$RUN_AS" "$dir"
        fi
    done
}

# Run a tinycld subcommand as the unprivileged runtime user.
#
# When the container starts as root we drop privileges with gosu, which preserves
# the binary's cap_net_bind_service file capability (needed to bind :80/:443
# under autocert). If we're already non-root — e.g. an operator pinned USER to
# something else — run the binary directly.
#
# This deliberately does NOT exec: the serve loop below inspects the exit code
# (75 = in-app package-install restart) and the health check runs a copy in the
# background, so control must return here.
# The binary runs from the active build's tinycld dir, reached via the `current`
# symlink. cd there so its relative code/asset lookups (server/, lib/, app/) resolve
# inside the active build, while pb_data/releases come from TINYCLD_STATE_DIR.
run_tinycld() {
    if [ "$(id -u)" = "0" ]; then
        gosu "$RUN_AS" sh -c 'cd "$0" && exec ./tinycld "$@"' "$CURRENT_LINK" "$@"
    else
        ( cd "$CURRENT_LINK" && exec ./tinycld "$@" )
    fi
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
#     _expo/static/...    (content-hashed, immutable)
#     assets/...           (mostly hashed; a few stable names like
#                           app-icon.png get overwritten per deploy)
#   current → <id>    SPA fallback reads <current>/app.html
#
# Why a pool: asset filenames are content-hashed, so files from different
# releases coexist without collision. Stale tabs that dynamic-import a
# chunk see their hashed filename in the pool until a future prune wipes
# old entries. The Go server serves /_expo/static/ and /assets/ from
# _static/ directly — there is no per-request release lookup.
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
    # cp -a (no -n) is used deliberately: same hashed filename = same
    # content, so re-copying is a no-op in effect; for the handful of
    # unhashed names under assets/ (app-icon.png, app-splash.png), the
    # current release's copy wins, which is the desired behavior. The
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

# Build serve arguments from three env vars:
#
#   PRIMARY_DOMAIN     the canonical domain (first cert SAN; also feeds the
#                      user-facing setup URL via TINYCLD_PUBLIC_URL below).
#   ADDITIONAL_DOMAINS comma-separated extra domains added to the cert request.
#   AUTOCERT_ENABLED   true/false — whether to provision Let's Encrypt certs
#                      and bind :80/:443 directly.
#
# Mode 1 — Autocert HTTPS (AUTOCERT_ENABLED=true AND PRIMARY_DOMAIN set):
#   pass the domains as positional args to PocketBase's `serve`, which binds
#   :80 (HTTP-01 challenge + redirect) and :443 (HTTPS) directly. These are
#   privileged ports; the binary has `cap_net_bind_service` set at build time
#   so the unprivileged runtime user can still bind them. PRIMARY_DOMAIN is
#   passed first so the cert's primary SAN and DomainArgs()[0] (used for the
#   demo-reset URL) are the canonical domain.
#
# Mode 2 — Plain HTTP (autocert off, or enabled without a PRIMARY_DOMAIN):
#   bind an unprivileged port (:7090 by default) and let an upstream reverse
#   proxy or Docker port-mapping handle TLS termination and ingress routing.
#   Override the bind with HTTP_ADDR (e.g. a dev sidecar on a different port).

# Trim surrounding whitespace; a value of "" or "   " counts as unset (users
# frequently leave `PRIMARY_DOMAIN:` blank in compose YAML to disable autocert).
PRIMARY_DOMAIN=$(printf '%s' "${PRIMARY_DOMAIN:-}" | awk '{$1=$1};1')

# Normalize AUTOCERT_ENABLED to a strict 1/0 (accepts true/TRUE/yes/1).
case "$(printf '%s' "${AUTOCERT_ENABLED:-}" | tr '[:upper:]' '[:lower:]' | awk '{$1=$1};1')" in
    1|true|yes|on) AUTOCERT_ON=1 ;;
    *)             AUTOCERT_ON=0 ;;
esac

# validate_domain: reject shell-truthy-but-bogus values (no dot, illegal
# characters, leading/trailing dot or hyphen) before PocketBase autocert fails
# them at a less-obvious layer.
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

    # PRIMARY_DOMAIN first, then each ADDITIONAL_DOMAINS entry (comma-separated;
    # surrounding whitespace per entry is tolerated). Build a space-separated
    # positional list for `serve`.
    set -- "$PRIMARY_DOMAIN"
    OLD_IFS=$IFS
    IFS=','
    for dom in ${ADDITIONAL_DOMAINS:-}; do
        IFS=$OLD_IFS
        dom=$(printf '%s' "$dom" | awk '{$1=$1};1')
        [ -z "$dom" ] && { IFS=','; continue; }
        validate_domain "$dom"
        set -- "$@" "$dom"
        IFS=','
    done
    IFS=$OLD_IFS

    echo "[entrypoint] Running with autocert HTTPS on: $*"
    set -- "$@" --http --https

    # Setup URL (and any other user-facing URL) should use the canonical
    # HTTPS domain, not PB's bind address. Only set if the operator hasn't
    # pinned TINYCLD_PUBLIC_URL explicitly.
    export TINYCLD_PUBLIC_URL="${TINYCLD_PUBLIC_URL:-https://$PRIMARY_DOMAIN}"
else
    if [ "$AUTOCERT_ON" = "1" ] && [ -z "$PRIMARY_DOMAIN" ]; then
        echo "[entrypoint] AUTOCERT_ENABLED is set but PRIMARY_DOMAIN is empty; falling back to plain HTTP" >&2
    fi
    HTTP_ADDR="${HTTP_ADDR:-0.0.0.0:7090}"
    echo "[entrypoint] Serving plain HTTP on $HTTP_ADDR (map a host port to this with -p / compose ports)"
    set -- "--http=$HTTP_ADDR"

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

# Restart loop: exit code 75 signals a package install restart request.
# Serve args are in $@ (positional params) so a multi-domain list survives
# without re-splitting.
while true; do
    # Capture the serve exit code WITHOUT letting `set -e` abort the script.
    # The in-app installer signals a restart by exiting the serve process with
    # code 75; under `set -e` a bare `run_tinycld serve` would make the shell
    # exit immediately on that non-zero code, before the 75-handling below ever
    # runs (the container would just exit 75 instead of restarting in place).
    # `|| EXIT_CODE=$?` swallows the non-zero for set -e and records the code;
    # reset to 0 first so a clean exit is captured too.
    EXIT_CODE=0
    run_tinycld serve $PB_SERVE_DIRS "$@" || EXIT_CODE=$?

    if [ $EXIT_CODE -eq 75 ]; then
        echo "[entrypoint] Restart requested (exit code 75)"

        # Health check: start the new binary on a temp HTTP port and verify
        # /api/health responds. Disable the mail package's IMAP (:993) and SMTP
        # (:465) listeners FOR THE PROBE ONLY via IMAP_ENABLED/SMTP_ENABLED=false
        # — otherwise the probe binds those fixed ports, and after we kill it the
        # ports aren't released before the real server restarts, so the restart
        # crashes with "listen tcp :993: bind: address already in use". The probe
        # only needs the HTTP listener to answer /api/health. The real serve
        # below (the `continue`d loop iteration) starts with mail enabled as
        # normal. Export the vars inside a backgrounded subshell so gosu passes
        # them to the child and they don't leak to the real server.
        #
        # Run the probe in its OWN process group via setsid, so teardown can kill
        # the WHOLE tree. Without setsid, `kill $HEALTH_PID` only kills the
        # subshell — the gosu→tinycld grandchild it spawned SURVIVES, keeping
        # :${HEALTH_PORT} bound. Across the install→upgrade→downgrade sequence
        # (three exit-75 restarts) those leaked probe servers accumulate; the next
        # restart's probe then can't bind and crashes with "listen tcp
        # 127.0.0.1:${HEALTH_PORT}: bind: address already in use", the health check
        # flaps, and the container is declared unhealthy. setsid makes $! the group
        # leader (PID == PGID), so `kill -- -$PGID` reaps gosu + tinycld together.
        # Preserve the same privilege drop as run_tinycld: gosu when root (which
        # also keeps the binary's cap_net_bind_service), direct otherwise. `exec`
        # so the gosu/tinycld process IS the group leader (no extra sh layer left
        # holding the group open).
        # The probe runs the NEW build's binary via the (just-flipped) current
        # symlink, cd'd into it so its relative lookups resolve in the new tree.
        if [ "$(id -u)" = "0" ]; then
            PROBE_CMD='cd '"$CURRENT_LINK"' && exec gosu '"$RUN_AS"' ./tinycld serve '"$PB_SERVE_DIRS"' --http=127.0.0.1:'"${HEALTH_PORT}"
        else
            PROBE_CMD='cd '"$CURRENT_LINK"' && exec ./tinycld serve '"$PB_SERVE_DIRS"' --http=127.0.0.1:'"${HEALTH_PORT}"
        fi
        setsid sh -c '
            export IMAP_ENABLED=false SMTP_ENABLED=false
            '"$PROBE_CMD"'
        ' &
        HEALTH_PID=$!

        # Poll the probe for up to 60s. The probe boots a FULL server — it runs
        # pending migrations, regenerates the PB schema types, and seeds bundled
        # packages before /api/health answers. After a version change (especially a
        # downgrade, which reverts a migration and rebuilds) that cold boot can take
        # well over 10s, more so under the concurrent load of the install
        # integration test. A too-short window declares a healthy server "failed",
        # trips the rollback path, and the container dies right after the probe
        # starts (observed: post-downgrade restart never reaching the real :7090
        # serve). 60s matches the real server's own cold-boot budget.
        HEALTHY=false
        for i in $(seq 1 60); do
            if curl -sf http://127.0.0.1:${HEALTH_PORT}/api/health >/dev/null 2>&1; then
                HEALTHY=true
                break
            fi
            sleep 1
        done

        # Kill the probe's entire process group (negative PID), then reap. The
        # group leader's PID equals the PGID because setsid created the group.
        # The trailing `|| true` is REQUIRED under `set -e`: if the probe already
        # exited on its own, BOTH kills return non-zero and the bare compound would
        # abort the entrypoint (pid 1) → the container dies right here, which looks
        # exactly like a failed restart. Never let probe teardown kill the script.
        { kill -- "-${HEALTH_PID}" 2>/dev/null || kill "${HEALTH_PID}" 2>/dev/null; } || true
        wait "${HEALTH_PID}" 2>/dev/null || true

        # Belt-and-suspenders: wait out the kernel releasing :${HEALTH_PORT} before
        # the next iteration's probe (or the real serve) tries to bind it. The
        # group kill is synchronous-ish but the socket close + TIME_WAIT teardown
        # is not; poll until the port is free (max ~5s) so a fast restart loop
        # can't race the previous probe's socket.
        for _ in 1 2 3 4 5 6 7 8 9 10; do
            curl -sf http://127.0.0.1:${HEALTH_PORT}/api/health >/dev/null 2>&1 || break
            sleep 0.5
        done

        if [ "$HEALTHY" = "true" ]; then
            echo "[entrypoint] Health check passed, restarting server"
            # Re-promote before re-serving. The in-app installer / version-change /
            # revert pipelines build a new web bundle and leave it in
            # release-staging/<id>, relying on promote_release to point
            # releases/current at it (see stageRelease's doc comment). Because this
            # exit-75 "restart" is an IN-PROCESS loop (the entrypoint stays alive
            # and `continue`s) rather than a full container restart, promote_release
            # — which otherwise runs only once at container start — must run again
            # here, or the server keeps serving the OLD bundle and a
            # newly-installed package's routes 404 ("Unmatched Route").
            promote_release
            continue
        else
            echo "[entrypoint] Health check failed, attempting rollback"
            # The whole build tree swaps via the `current` symlink, so rollback is a
            # symlink flip back to the previous build dir (recorded by activateBuild in
            # /workspace/.previous-build) — not a binary mv. The DB was already restored
            # in-process by the failed rebuild's restore step; here we only revert code.
            if [ -f /workspace/.previous-build ]; then
                prev=$(cat /workspace/.previous-build)
                if [ -d "/workspace/builds/$prev/tinycld" ]; then
                    ln -sfn "/workspace/builds/$prev/tinycld" "$CURRENT_LINK.tmp"
                    mv -T "$CURRENT_LINK.tmp" "$CURRENT_LINK"
                    echo "[entrypoint] Rolled back current -> $prev"
                else
                    echo "[entrypoint] WARN: previous build $prev not on disk; cannot roll back symlink" >&2
                fi
            else
                echo "[entrypoint] WARN: no /workspace/.previous-build; cannot roll back" >&2
            fi
            continue
        fi
    fi

    # Normal exit (not a restart request)
    echo "[entrypoint] Server exited with code $EXIT_CODE"
    exit $EXIT_CODE
done
