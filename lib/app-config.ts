import type { CoreConfig } from '@tinycld/core'
import Constants from 'expo-constants'
import { Platform } from 'react-native'

// The connect-screen default server URL, derived from the platform we're
// actually running on so each simulator points at the right host without
// manual edits. The dev proxy serves cleartext by default (see dev.ts), so
// simulators use plain http:
//
//   - Android emulator: the host loopback (localhost/127.0.0.1) resolves to
//     the emulator VM itself; the host is exposed at the AVD alias 10.0.2.2.
//     Debug builds permit cleartext (usesCleartextTraffic).
//   - iOS simulator: localhost reaches the host directly. ATS allows the
//     cleartext call via NSAllowsLocalNetworking, so no https needed.
//   - Physical device (Constants.isDevice): keeps https+localhost as a
//     placeholder; a real device needs a LAN IP regardless (separate concern).
//
// Run the proxy with `--ssl` only if you specifically want TLS in dev; then
// point the connect screen at the https URL manually. Web never calls this —
// it resolves PB from window.location.origin.
function devDefaultServer(): string {
    if (!Constants.isDevice) {
        if (Platform.OS === 'android') return 'http://10.0.2.2:7100'
        if (Platform.OS === 'ios') return 'http://localhost:7100'
    }
    return 'https://localhost:7100'
}

// Public (non-secret) system config the server injects into the web HTML before
// the bundle loads — see coreserver/static.go::publicConfigScript. Web only;
// native has no window/HTML and uses the build-time path below.
declare global {
    var __TINYCLD_PUBLIC_CONFIG__: { sentryDsn?: string } | undefined
}

// Resolve the Sentry DSN at startup. Order:
//   1. window.__TINYCLD_PUBLIC_CONFIG__ — web, injected by the app server at
//      serve time, so changing it in /admin Settings takes effect on next load
//      with no rebuild.
//   2. process.env.EXPO_PUBLIC_SENTRY_DSN — native, inlined at `expo export`
//      from the stored value by the rebuild pipeline (build input, not a runtime
//      env read).
//   3. '' — no DSN configured; Sentry init no-ops (see core/lib/sentry.ts).
// No hardcoded production DSN: a third-party host must supply its own.
function resolveSentryDsn(): string {
    if (typeof globalThis !== 'undefined' && globalThis.__TINYCLD_PUBLIC_CONFIG__?.sentryDsn) {
        return globalThis.__TINYCLD_PUBLIC_CONFIG__.sentryDsn
    }
    return process.env.EXPO_PUBLIC_SENTRY_DSN ?? ''
}

// App config handed to @tinycld/core at startup. Web resolves the PB address
// from the page origin (the dev proxy / app server routes /api to PocketBase
// same-origin); native uses defaultServer on the connect screen.
//
// brandName flows from app.json's expo.name so a fork rebrands by editing
// app.json alone. The fallback covers the rare case Constants.expoConfig is
// unavailable (e.g. some unit-test environments).
//
// Sentry DSN is resolved at startup (resolveSentryDsn): web reads the
// server-injected window global; native uses EXPO_PUBLIC_SENTRY_DSN inlined at
// build time. There is NO hardcoded production DSN — a host (incl. ours, via EAS
// env) must supply its own, so a third-party deployment never reports to us.
export const appConfig: CoreConfig = {
    brandName: Constants.expoConfig?.name ?? 'TinyCld',
    serverShortcuts: {},
    // Web resolves PB from the page origin. Guard on Platform.OS === 'web', NOT
    // `typeof window`: React Native defines a partial `window` WITHOUT `location`,
    // so a `typeof window` check passes on native and `window.location.origin`
    // throws "Cannot read property 'origin' of undefined". This crashed the
    // server-exported OTA bundle specifically, where EXPO_PUBLIC_ENV is baked as
    // 'web' (the web export), driving resolveEnvAddress down the web branch at
    // configureCore() module-init — before any screen renders.
    webShortcut: () =>
        Platform.OS === 'web' && typeof window !== 'undefined' ? window.location.origin : null,
    defaultServer: __DEV__ ? devDefaultServer() : 'https://tinycld.org',
    sentryDsn: resolveSentryDsn(),
    privacyUrl: 'https://tinycld.org/privacy',
    sourceUrl: 'https://github.com/tinycld/tinycld',
}
