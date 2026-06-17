import type { CoreConfig } from '@tinycld/core'
import Constants from 'expo-constants'

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
    defaultServer: __DEV__ ? 'https://localhost:7100' : 'https://tinycld.org',
    sentryDsn:
        'https://bfba682150acd66a9b75f51ddbced312@o4510361420431360.ingest.us.sentry.io/4511261359013888',
    privacyUrl: 'https://tinycld.org/privacy',
    sourceUrl: 'https://github.com/tinycld/tinycld',
}
