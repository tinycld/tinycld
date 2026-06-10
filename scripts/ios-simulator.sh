#!/usr/bin/env bash
# Wrapper for `expo run:ios` that targets a simulator UDID. By default reads
# the UDID from ../.env (IPHONE_SIMULATOR_UDID or IPAD_SIMULATOR_UDID); pass
# --ipad to switch to the iPad env var, or --udid <UDID> to skip the env file
# entirely and target an explicit simulator. Any other flags are forwarded to
# expo (e.g. --no-bundler).
#
# --prod builds a Release configuration instead of the default Debug+Metro dev
# loop. Release compiles the JS into the embedded main.jsbundle (no Metro), which
# is what makes AppDelegate.bundleURL() consult the self-hosted OTA loader
# (AppUpdaterBundle.stagedBundleURL) — the only way to exercise OTA staging/
# reload/rollback on a simulator. In --prod mode the dev-only packager-proxy env
# and --no-bundler flag are dropped (there's no packager to point at).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="$ROOT/../.env"

DEVICE="iphone"
UDID_OVERRIDE=""
PROD=0
EXTRA_ARGS=()
while [ $# -gt 0 ]; do
    case "$1" in
        --ipad) DEVICE="ipad"; shift ;;
        --iphone) DEVICE="iphone"; shift ;;
        --prod) PROD=1; shift ;;
        --udid)
            if [ $# -lt 2 ]; then
                echo "ios-simulator: --udid requires a value" >&2
                exit 1
            fi
            UDID_OVERRIDE="$2"
            shift 2
            ;;
        --udid=*) UDID_OVERRIDE="${1#--udid=}"; shift ;;
        *) EXTRA_ARGS+=("$1"); shift ;;
    esac
done

if [ -n "$UDID_OVERRIDE" ]; then
    UDID="$UDID_OVERRIDE"
else
    if [ ! -f "$ENV_FILE" ]; then
        echo "ios-simulator: $ENV_FILE not found (pass --udid <UDID> to bypass)" >&2
        exit 1
    fi

    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a

    if [ "$DEVICE" = "ipad" ]; then
        UDID="${IPAD_SIMULATOR_UDID:-}"
        UDID_VAR="IPAD_SIMULATOR_UDID"
    else
        UDID="${IPHONE_SIMULATOR_UDID:-}"
        UDID_VAR="IPHONE_SIMULATOR_UDID"
    fi

    if [ -z "$UDID" ]; then
        echo "ios-simulator: $UDID_VAR not set in $ENV_FILE (pass --udid <UDID> to override)" >&2
        exit 1
    fi
fi

cd "$ROOT"

if [ "$PROD" -eq 1 ]; then
    # Release build: JS is bundled into the embedded main.jsbundle at xcodebuild
    # time, so there's no Metro to point at — drop the packager-proxy env and the
    # --no-bundler flag. This is the OTA-testing path: with no live bundler,
    # AppDelegate.bundleURL() falls through to AppUpdaterBundle.stagedBundleURL(),
    # loading a staged OTA bundle (or the embedded one). Connect the booted app to
    # a server that ran a package install to exercise update → reload → rollback.
    echo "ios-simulator: building Release (embedded bundle, OTA loader active) on $UDID"
    npx expo run:ios --device "$UDID" --configuration Release "${EXTRA_ARGS[@]}"
else
    # RCT_METRO_PORT is baked into the iOS binary at xcodebuild time (via
    # RCTDefines.h), defaulting to 8081. We override it to 7100 — the dev.ts
    # proxy port — so the simulator loads the bundle through the same proxy
    # that routes /api and /_ to PocketBase. --port 7100 also tells the
    # (skipped) bundler to use that port if --no-bundler is removed.
    #
    # Important: the simulator speaks plain HTTP. dev.ts runs the proxy with
    # TLS by default when assets/localhost*.pem exist — run `npm start --
    # --no-ssl` (or delete the certs) when using the simulator, otherwise
    # the bundle fetch fails with a TLS handshake error.
    export EXPO_PACKAGER_PROXY_URL=http://localhost:7102
    npx expo run:ios --device "$UDID" --no-bundler "${EXTRA_ARGS[@]}"
fi
