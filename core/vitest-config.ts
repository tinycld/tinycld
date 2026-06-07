// Re-export the app shell's canonical vitest config under a @tinycld/* name so
// feature siblings inherit it by package specifier — `@tinycld/core/vitest-config`
// — instead of a cross-member relative path (`../tinycld/vitest.config`), which
// the standalone-member reorg exists to eliminate. The explicit `.ts` extension
// is required: siblings reach this file through core's package exports map, so
// the re-export resolves via Node's ESM loader (no extension inference). core is
// nested inside the tinycld member, so `../vitest.config.ts` is an in-repo path —
// the one place that cross-file coupling is allowed to live.
export { default } from '../vitest.config.ts'
