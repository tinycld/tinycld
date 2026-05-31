import type { CoreConfig } from '@tinycld/core'
import Constants from 'expo-constants'

// Minimal app config for the ./new dev app. Web resolves the PB address from
// the page origin (the dev proxy / app server routes /api to PocketBase
// same-origin). Sentry/review-mode/demo are intentionally omitted for the spike.
//
// brandName flows from app.json's expo.name so a fork rebrands by editing
// app.json alone. The fallback covers the rare case Constants.expoConfig is
// unavailable (e.g. some unit-test environments).
export const appConfig: CoreConfig = {
    brandName: Constants.expoConfig?.name ?? 'TinyCld',
    serverShortcuts: {},
    webShortcut: () => (typeof window !== 'undefined' ? window.location.origin : null),
    defaultServer: 'http://localhost:7100',
    privacyUrl: 'https://tinycld.org/privacy',
    sourceUrl: 'https://github.com/tinycld/tinycld',
}
