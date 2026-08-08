import { existsSync } from 'node:fs'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { afterAll, describe, expect, it } from 'vitest'
import { bundleWebViewEditor } from '../../webview-bundler/build'

/**
 * Bundles the real WebView editor source.
 *
 * The native markdown path has no mechanical coverage below this line — an
 * unresolvable import or a broken entry surfaces only as a blank WebView on a
 * device. This catches that in CI without one.
 */
const SOURCE_DIR = resolve(__dirname, '../webview/source')
const CORE_ROOT = resolve(__dirname, '../../../..')
const APP_ROOT = resolve(CORE_ROOT, '..')
const WS_ROOT = resolve(APP_ROOT, '..')

let tempDir: string | null = null

afterAll(async () => {
    if (tempDir) await rm(tempDir, { recursive: true, force: true })
})

describe('rich editor WebView bundle', () => {
    it('bundles to a self-contained page', async () => {
        tempDir = await mkdtemp(join(tmpdir(), 'rich-editor-bundle-'))
        const outFile = join(tempDir, 'editorHtml.ts')

        const result = await bundleWebViewEditor({
            entryHtml: join(SOURCE_DIR, 'index.html'),
            entryScript: join(SOURCE_DIR, 'entry.tsx'),
            outFile,
            nodePaths: [join(APP_ROOT, 'node_modules'), join(WS_ROOT, 'node_modules')],
        })

        expect(existsSync(outFile)).toBe(true)
        const emitted = await readFile(outFile, 'utf-8')
        expect(emitted).toContain('export const editorHtml')

        // Assert against the HTML the WebView actually receives, not the
        // module source — the emitted file escapes it into a JS string
        // literal, so this also proves the escaping survives a round-trip.
        const html = JSON.parse(
            emitted.slice(emitted.indexOf('"'), emitted.lastIndexOf('"') + 1)
        ) as string

        expect(html).toContain('<div id="root">')
        // The page must carry its own script inline — a leftover <script src>
        // would load nothing inside a WebView with no origin to resolve it.
        expect(html).not.toMatch(/<script\s+src=/)

        // Guards against a bundle that "succeeds" while tree-shaking the
        // editor away: markdown being native to this page is the whole point.
        expect(result.htmlSize).toBeGreaterThan(100_000)
        expect(html).toContain('ProseMirror')
    }, 120_000)
})
