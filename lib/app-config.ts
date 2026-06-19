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

// App config handed to @tinycld/core at startup. Web resolves the PB address
// from the page origin (the dev proxy / app server routes /api to PocketBase
// same-origin); native uses defaultServer on the connect screen.
//
// brandName flows from app.json's expo.name so a fork rebrands by editing
// app.json alone. The fallback covers the rare case Constants.expoConfig is
// unavailable (e.g. some unit-test environments).
//
// Sentry DSN is hardcoded (matches the EXPO_PUBLIC_SENTRY_DSN value EAS injects
// from its production environment) so it ships in every build without depending
// on build-time env plumbing.
export const appConfig: CoreConfig = {
    brandName: Constants.expoConfig?.name ?? 'TinyCld',
    serverShortcuts: {},
    webShortcut: () => (typeof window !== 'undefined' ? window.location.origin : null),
    defaultServer: __DEV__ ? devDefaultServer() : 'https://tinycld.org',
    sentryDsn:
        'https://bfba682150acd66a9b75f51ddbced312@o4510361420431360.ingest.us.sentry.io/4511261359013888',
    privacyUrl: 'https://tinycld.org/privacy',
    sourceUrl: 'https://github.com/tinycld/tinycld',
}
