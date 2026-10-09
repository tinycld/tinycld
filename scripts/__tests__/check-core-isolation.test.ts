import { describe, expect, it } from 'vitest'
import { buildPattern, isSharedE2EHelper, isTestFile } from '../check-core-isolation'

// Regression coverage for the gap that let tests/e2e/imap-helpers.ts carry
// mail's mailbox layout into core unscanned: isTestFile() treated every file
// under a `tests/` (or `__tests__`/`testdata`) path segment as a test file,
// so a SHARED e2e helper — library code a `*.spec.ts` imports, not a test
// itself — was silently exempt. A fictional package ('acme') stands in for
// the real one so this can't regress for any package, not just mail's.
describe('isSharedE2EHelper', () => {
    it('treats a non-spec file under tests/e2e/ as a shared helper', () => {
        expect(isSharedE2EHelper('tests/e2e/acme-helpers.ts')).toBe(true)
    })

    it('treats a core/e2e-*.ts re-export wrapper as a shared helper', () => {
        expect(isSharedE2EHelper('core/e2e-acme-helpers.ts')).toBe(true)
    })

    it('does not treat a tests/e2e/*.spec.ts file as a shared helper', () => {
        expect(isSharedE2EHelper('tests/e2e/acme.spec.ts')).toBe(false)
    })

    it('does not treat a file in a deeper tests/e2e/ subdirectory as a shared helper', () => {
        expect(isSharedE2EHelper('tests/e2e/fixtures/acme-helpers.ts')).toBe(false)
    })

    it('does not treat an unrelated tests/ file as a shared helper', () => {
        expect(isSharedE2EHelper('core/tests/unit/acme.test.ts')).toBe(false)
    })
})

describe('isTestFile', () => {
    it('scans a shared e2e helper even though it sits under tests/', () => {
        expect(isTestFile('tests/e2e/acme-helpers.ts')).toBe(false)
    })

    it('still skips a real spec file under tests/e2e/', () => {
        expect(isTestFile('tests/e2e/acme.spec.ts')).toBe(true)
    })

    it('still skips an ordinary unit test file outside tests/e2e/', () => {
        expect(isTestFile('core/tests/unit/acme.test.ts')).toBe(true)
    })

    it('still skips files under __tests__ and testdata', () => {
        expect(isTestFile('scripts/__tests__/acme.ts')).toBe(true)
        expect(isTestFile('scripts/testdata/acme.ts')).toBe(true)
    })
})

// buildPattern is what actually catches a leaked package reference in a
// shared e2e helper's source — prove a fictional package's name, scope
// import and collection are all caught in the kind of line a helper like
// the old imap-helpers.ts carried (a comment is stripped by the caller
// before this pattern ever sees it, so these lines are plain code/strings).
describe('buildPattern', () => {
    const pattern = buildPattern(['acme'], ['acme_widgets'])

    it('matches a package-scoped import', () => {
        expect(pattern.test("import { foo } from '@tinycld/acme'")).toBe(true)
    })

    it('matches an API route for the package', () => {
        expect(pattern.test("fetch('/api/acme/widgets')")).toBe(true)
    })

    it('matches a package-owned collection name', () => {
        expect(pattern.test("useStore('acme_widgets')")).toBe(true)
    })

    it('does not match unrelated code', () => {
        expect(pattern.test("import { foo } from '@tinycld/core'")).toBe(false)
    })
})
