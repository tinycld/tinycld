#!/bin/sh
set -e

HEALTH_PORT=19876

# The application runs as this unprivileged user (uid/gid baked into the image
# as 1000:1000; see Dockerfile). The container itself starts as root only long
# enough to fix bind-mount ownership, then drops to RUN_AS via gosu.
RUN_AS=tinycld

echo "[entrypoint] starting; pwd=$(pwd) user=$(id -un) uid=$(id -u)"

# Ensure the bind-mounted data directories are writable by the runtime user.
#
# When a host bind-mount target (./pb_data, ./types in docker-compose.yml)
# doesn't exist yet, the Docker daemon creates it owned by root:root. The
# unprivileged tinycld user then can't open the SQLite database — PocketBase
# fails with "unable to open database file (14)" and the container crash-loops.
# Reported in https://github.com/tinycld/app/issues/26.
#
# We run this as root (the container's start user) and chown the dirs to the
# runtime user before dropping privileges. Only runs when we're actually root;
# if an operator overrode the start user to non-root they're responsible for
# host-side ownership (and the chown would fail anyway), so we skip silently.
fix_data_dir_ownership() {
    [ "$(id -u)" = "0" ] || return 0

    for dir in /app/pb_data /app/types; do
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
run_tinycld() {
    if [ "$(id -u)" = "0" ]; then
        gosu "$RUN_AS" ./tinycld "$@"
    else
        ./tinycld "$@"
    fi
}

fix_data_dir_ownership

# Promote the staged release to /app/releases/. Runs on every container
# start; idempotent.
#
# /app/releases is typically the container's writable layer (compose-style
# deploys) and starts empty on every fresh container; Dokku-style deploys
# may back it with a persistent volume so old releases survive container
# replacement. Either way the promote logic below is the same: copy the
# staged tree off the image, swap the `current` symlink atomically.
#
# Layout produced under /app/releases/:
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
    staging_dir=/app/release-staging
    releases_dir=/app/releases
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

    release_id=""
    for d in "$staging_dir"/*/; do
        [ -d "$d" ] || continue
        if [ -f "$d/release-id.txt" ]; then
            release_id=$(cat "$d/release-id.txt")
            echo "[entrypoint] found release-id.txt in $d -> '$release_id'"
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
        echo "[entrypoint] promoting release $release_id ($src -> $dst, app.html + release-id.txt only)"
        rm -rf "$dst.tmp"
        mkdir "$dst.tmp"
        cp -a "$src/app.html" "$dst.tmp/"
        cp -a "$src/release-id.txt" "$dst.tmp/"
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
    run_tinycld serve "$@"
    EXIT_CODE=$?

    if [ $EXIT_CODE -eq 75 ]; then
        echo "[entrypoint] Restart requested (exit code 75)"

        # Health check: start new binary on a temp port, verify /api/health responds
        run_tinycld serve --http=127.0.0.1:${HEALTH_PORT} &
        HEALTH_PID=$!

        HEALTHY=false
        for i in 1 2 3 4 5 6 7 8 9 10; do
            if curl -sf http://127.0.0.1:${HEALTH_PORT}/api/health >/dev/null 2>&1; then
                HEALTHY=true
                break
            fi
            sleep 1
        done

        kill $HEALTH_PID 2>/dev/null
        wait $HEALTH_PID 2>/dev/null || true

        if [ "$HEALTHY" = "true" ]; then
            echo "[entrypoint] Health check passed, restarting server"
            continue
        else
            echo "[entrypoint] Health check failed, attempting rollback"
            if [ -f ./tinycld.prev ]; then
                mv ./tinycld ./tinycld.failed
                mv ./tinycld.prev ./tinycld
                echo "[entrypoint] Rolled back to previous binary"
            fi
            continue
        fi
    fi

    # Normal exit (not a restart request)
    echo "[entrypoint] Server exited with code $EXIT_CODE"
    exit $EXIT_CODE
done
