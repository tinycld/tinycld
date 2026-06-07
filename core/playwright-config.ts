// Re-export the app shell's canonical Playwright config under a @tinycld/* name
// so feature siblings inherit it by package specifier —
// `@tinycld/core/playwright-config` — instead of a cross-member relative path
// (`../tinycld/playwright.config`), which the standalone-member reorg exists to
// eliminate. The explicit `.ts` extension is required because siblings reach
// this through core's exports map, so the re-export is resolved by Node's ESM
// loader (no extension inference); see vitest-config.ts for the full rationale.
export { default } from '../playwright.config.ts'
