// Re-export the app shell's canonical Playwright e2e helpers (login,
// navigateToPackage, clickSidebarItem, ORG_SLUG, …) under a @tinycld/* name so
// feature siblings' e2e specs import them by package specifier —
// `@tinycld/core/e2e-helpers` — instead of a cross-member relative path
// (`../../tinycld/tests/e2e/helpers`), which the standalone-member reorg exists
// to eliminate. Playwright has no `~/` alias, so package-name resolution is the
// only non-relative option. The explicit `.ts` extension is required because
// siblings reach this through core's exports map, so the re-export is resolved
// by Node's ESM loader (no extension inference); see vitest-config.ts.
export * from '../tests/e2e/helpers.ts'
