import * as fs from 'node:fs'
import * as path from 'node:path'
import { afterAll, describe, expect, it } from 'vitest'
import { isPackageLinked } from '../e2e/helpers'

// isPackageLinked resolves "<workspaceRoot>/<slug>/manifest.ts" from helpers.ts's
// own location. These tests exercise that contract against the real workspace so
// a future path drift (like the old "packages/@tinycld/<slug>" layout) is caught.
describe('isPackageLinked', () => {
    // helpers.ts lives at tinycld/tests/e2e/helpers.ts, so the workspace root is
    // three levels up from tests/unit/ (this file) as well.
    const workspaceRoot = path.resolve(import.meta.dirname, '..', '..', '..')
    const scratchSlug = '__tinycld_test_scratch_pkg__'
    const scratchDir = path.join(workspaceRoot, scratchSlug)

    afterAll(() => {
        fs.rmSync(scratchDir, { recursive: true, force: true })
    })

    it('returns false for a slug with no sibling member dir', () => {
        expect(isPackageLinked('__definitely_not_a_real_package__')).toBe(false)
    })

    it('returns false for a sibling dir that lacks a manifest.ts', () => {
        fs.mkdirSync(scratchDir, { recursive: true })
        expect(isPackageLinked(scratchSlug)).toBe(false)
    })

    it('returns true once the sibling dir has a manifest.ts', () => {
        fs.mkdirSync(scratchDir, { recursive: true })
        fs.writeFileSync(path.join(scratchDir, 'manifest.ts'), 'export default {}\n')
        expect(isPackageLinked(scratchSlug)).toBe(true)
    })
})
