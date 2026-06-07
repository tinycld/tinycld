// Re-export the app shell's CalDAV e2e helpers under a @tinycld/* name so
// calendar's e2e specs import them by package specifier
// (`@tinycld/core/e2e-caldav-helpers`) instead of
// `../../tinycld/tests/e2e/caldav-helpers`. See e2e-helpers.ts for the rationale
// and why the explicit `.ts` extension is required.
export * from '../tests/e2e/caldav-helpers.ts'
