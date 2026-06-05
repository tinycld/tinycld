import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { loadManifest } from '../load-manifest'

// loadManifest is a dynamic-import wrapper around a member's
// manifest.ts. The interesting behavior is "find the file then return
// its default export"; we exercise that against a tmpdir-staged fake
// member rather than a real installed sibling so the test doesn't pull
// any feature member into the bootstrap closure.

describe('loadManifest', () => {
    let tmp: string
    beforeEach(() => {
        tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-manifest-'))
    })
    afterEach(() => fs.rmSync(tmp, { recursive: true, force: true }))

    it('returns the default export of manifest.ts in the given dir', async () => {
        // Write a minimal manifest matching the PackageManifest shape
        // loadManifest's return type promises. The collections sub-
        // object is the field the original assertion checked, so we
        // include it to keep that branch covered.
        fs.writeFileSync(
            path.join(tmp, 'manifest.ts'),
            `export default {
                name: 'Demo',
                slug: 'demo',
                version: '0.1.0',
                description: 'tmpdir fixture',
                collections: { register: 'collections', types: 'types' },
            } as const\n`
        )
        const m = await loadManifest(tmp)
        expect(m.slug).toBe('demo')
        expect(m.name).toBe('Demo')
        expect(m.collections).toMatchObject({ register: 'collections', types: 'types' })
    })

    it('throws when the dir has no manifest.ts/.js', async () => {
        await expect(loadManifest(tmp)).rejects.toThrow(/No manifest found/)
    })
})
