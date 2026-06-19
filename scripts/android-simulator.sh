#!/usr/bin/env bash
# Wrapper for `expo run:android` that targets an emulator. The Android
# counterpart to ios-simulator.sh — same dev (Debug + Metro proxy) vs
# --prod (Release, embedded bundle, OTA loader) split.
#
# By default it builds against whatever emulator/device is currently
# attached (adb's default). Pass --avd <name> to boot a specific AVD
# first, or set ANDROID_EMULATOR_AVD in ../.env. Any other flags are
# forwarded to expo (e.g. --no-bundler).
#
# Unlike iOS (which bakes RCT_METRO_PORT into the binary), the Android
# dev client reaches Metro over the network. The emulator's own
# localhost is the emulator, not the host, so we `adb reverse` the proxy
# ports (7100 user-facing proxy, 7102 Expo/Metro) back to the host. That
# routes the bundle fetch + /api + /_ through the same dev.ts proxy the
# iOS simulator uses.
#
# JDK note: the build needs JDK 17 (the machine default is newer). We pin
# JAVA_HOME the same way android:run / build:android do.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="$ROOT/../.env"

# dev.ts port block: proxy (user-facing), pb, expo = base, base+1, base+2.
PROXY_PORT=7100
EXPO_PORT=7102

AVD_OVERRIDE=""
PROD=0
EXTRA_ARGS=()
while [ $# -gt 0 ]; do
    case "$1" in
        --prod) PROD=1; shift ;;
        --avd)
            if [ $# -lt 2 ]; then
                echo "android-simulator: --avd requires a value" >&2
                exit 1
            fi
            AVD_OVERRIDE="$2"
            shift 2
            ;;
        --avd=*) AVD_OVERRIDE="${1#--avd=}"; shift ;;
        *) EXTRA_ARGS+=("$1"); shift ;;
    esac
done

# Read ANDROID_EMULATOR_AVD from ../.env if present and no flag was given.
if [ -z "$AVD_OVERRIDE" ] && [ -f "$ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
    AVD_OVERRIDE="${ANDROID_EMULATOR_AVD:-}"
fi

# Pin JDK 17 (machine default is newer; the Gradle build rejects it).
export JAVA_HOME="$(/usr/libexec/java_home -v 17 2>/dev/null || echo "${JAVA_HOME:-}")"
export PATH="$JAVA_HOME/bin:$PATH"

# Locate the emulator binary (rarely on PATH; resolve via the SDK root).
SDK="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Library/Android/sdk}}"
EMULATOR_BIN="$(command -v emulator || echo "$SDK/emulator/emulator")"

# Boot an emulator unless one is already attached. If no AVD was named
# (via --avd or ANDROID_EMULATOR_AVD) we auto-pick: the only AVD if
# there's exactly one, otherwise we list them and bail instead of
# hanging on wait-for-device.
if adb devices | grep -q "emulator-"; then
    echo "android-simulator: emulator already running, reusing it"
else
    if [ -z "$AVD_OVERRIDE" ]; then
        AVDS=()
        while IFS= read -r line; do
            [ -n "$line" ] && AVDS+=("$line")
        done < <("$EMULATOR_BIN" -list-avds 2>/dev/null)
        if [ "${#AVDS[@]}" -eq 0 ]; then
            echo "android-simulator: no AVDs found. Create one in Android Studio (Device Manager) first." >&2
            exit 1
        elif [ "${#AVDS[@]}" -eq 1 ]; then
            AVD_OVERRIDE="${AVDS[0]}"
        else
            echo "android-simulator: multiple AVDs found — pass --avd <name> or set ANDROID_EMULATOR_AVD in ../.env:" >&2
            printf '  - %s\n' "${AVDS[@]}" >&2
            exit 1
        fi
    fi
    echo "android-simulator: booting AVD '$AVD_OVERRIDE'…"
    "$EMULATOR_BIN" -avd "$AVD_OVERRIDE" -netdelay none -netspeed full >/dev/null 2>&1 &
fi

# Wait for the device to attach AND finish booting — wait-for-device only
# blocks until adb sees it, not until Android is usable, so we also poll
# sys.boot_completed. Time-box it so a stuck emulator fails loudly instead
# of hanging forever.
echo "android-simulator: waiting for device to boot…"
adb wait-for-device
BOOT_DEADLINE=$((SECONDS + 180))
until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
    if [ "$SECONDS" -ge "$BOOT_DEADLINE" ]; then
        echo "android-simulator: emulator did not finish booting within 180s" >&2
        exit 1
    fi
    sleep 2
done

# Map the proxy ports back to the host so the emulator's localhost:<port>
# reaches dev.ts on the dev machine.
adb reverse "tcp:${PROXY_PORT}" "tcp:${PROXY_PORT}"
adb reverse "tcp:${EXPO_PORT}" "tcp:${EXPO_PORT}"

cd "$ROOT"

if [ "$PROD" -eq 1 ]; then
    # Release build: JS is bundled into the embedded bundle at assemble
    # time, so there's no Metro to point at. This is the OTA path — with
    # no live bundler, the app falls through to the staged-OTA loader.
    echo "android-simulator: building Release (embedded bundle, OTA loader active)"
    npx expo run:android --variant release "${EXTRA_ARGS[@]}"
else
    # Point the packager proxy at the Expo port (reversed to the host),
    # matching ios-simulator.sh. The emulator speaks plain HTTP, so run
    # the dev server with --no-ssl (or delete assets/localhost*.pem) — a
    # TLS proxy would fail the bundle handshake.
    export EXPO_PACKAGER_PROXY_URL="http://localhost:${EXPO_PORT}"
    npx expo run:android --no-bundler "${EXTRA_ARGS[@]}"
fi
