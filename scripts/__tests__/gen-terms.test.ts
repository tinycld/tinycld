import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { resolveTerms, TERMS_MAX_BYTES } from '../gen-terms'
import type { PackageManifest } from '../load-manifest'

const dirs: string[] = []

// A fictional package directory with the given files.
function packageDir(files: Record<string, string>): string {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'gen-terms-'))
    dirs.push(dir)
    for (const [name, text] of Object.entries(files)) {
        fs.mkdirSync(path.dirname(path.join(dir, name)), { recursive: true })
        fs.writeFileSync(path.join(dir, name), text)
    }
    return dir
}

afterEach(() => {
    for (const dir of dirs.splice(0)) fs.rmSync(dir, { recursive: true, force: true })
})

describe('resolveTerms', () => {
    it('returns undefined for a package without terms', () => {
        expect(resolveTerms(packageDir({}), 'gizmos', undefined)).toBeUndefined()
    })

    it('reads the file and keeps the version', () => {
        const dir = packageDir({ 'TERMS.md': '# Gizmos terms\n\nBe kind.\n' })
        expect(resolveTerms(dir, 'gizmos', { file: 'TERMS.md', version: '2026-11-01' })).toEqual({
            version: '2026-11-01',
            body: '# Gizmos terms\n\nBe kind.',
        })
    })

    it('reads a file in a subdirectory of the package', () => {
        const dir = packageDir({ 'legal/terms.md': 'Text' })
        expect(resolveTerms(dir, 'gizmos', { file: 'legal/terms.md', version: '3' })?.body).toBe(
            'Text'
        )
    })

    it.each([
        ['an empty version', { file: 'TERMS.md', version: '' }, /gizmos: manifest\.terms version/],
        [
            'a version with a space',
            { file: 'TERMS.md', version: 'v 1' },
            /gizmos: manifest\.terms version/,
        ],
        [
            'a version of 51 characters',
            { file: 'TERMS.md', version: 'v'.repeat(51) },
            /gizmos: manifest\.terms version/,
        ],
        ['a file that is not markdown', { file: 'TERMS.txt', version: '1' }, /must be a \.md file/],
        [
            'an uppercase extension',
            { file: 'TERMS.MD', version: '1' },
            /must be a \.md file \(lowercase extension\)/,
        ],
        [
            'a version that is not a string',
            { file: 'TERMS.md', version: 3 },
            /gizmos: manifest\.terms version 3 is not valid/,
        ],
        [
            'a file that is not a string',
            { file: 7, version: '1' },
            /gizmos: manifest\.terms file 7 must be a \.md file/,
        ],
        ['a missing file', { file: 'MISSING.md', version: '1' }, /does not exist/],
        [
            'a file outside the package',
            { file: '../TERMS.md', version: '1' },
            /outside the package/,
        ],
        ['an empty file', { file: 'EMPTY.md', version: '1' }, /is empty/],
        ['a file that is too large', { file: 'BIG.md', version: '1' }, /is larger than/],
    ])('refuses %s', (_name, terms, message) => {
        const dir = packageDir({
            'TERMS.md': 'Text',
            'TERMS.txt': 'Text',
            'EMPTY.md': ' \n',
            'BIG.md': 'x'.repeat(TERMS_MAX_BYTES + 1),
        })
        const declared = terms as unknown as PackageManifest['terms']
        expect(() => resolveTerms(dir, 'gizmos', declared)).toThrow(message)
    })

    it('refuses a symlink that points outside the package', () => {
        const outside = packageDir({ 'secret.md': 'Not for the package' })
        const dir = packageDir({})
        fs.symlinkSync(path.join(outside, 'secret.md'), path.join(dir, 'TERMS.md'))
        expect(() => resolveTerms(dir, 'gizmos', { file: 'TERMS.md', version: '1' })).toThrow(
            /outside the package/
        )
    })

    it('refuses a directory named like a markdown file', () => {
        const dir = packageDir({ 'TERMS.md/inner.txt': 'x' })
        expect(() => resolveTerms(dir, 'gizmos', { file: 'TERMS.md', version: '1' })).toThrow(
            /does not exist/
        )
    })
})
