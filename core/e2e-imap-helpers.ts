// Re-export the app shell's IMAP e2e helpers under a @tinycld/* name so mail's
// e2e specs import them by package specifier (`@tinycld/core/e2e-imap-helpers`)
// instead of `../../tinycld/tests/e2e/imap-helpers`. See e2e-helpers.ts for the
// rationale and why the explicit `.ts` extension is required.
export * from '../tests/e2e/imap-helpers.ts'
