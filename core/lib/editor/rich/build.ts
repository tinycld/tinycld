import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { bundleWebViewEditor } from '../webview-bundler/build'

// Build entry for the shared rich editor's WebView page. Invoked by the
// generator (scripts/generate.ts step 0) and, standalone, via:
//   pnpm exec tsx core/lib/editor/rich/build.ts
//
// Emits webview/build/editorHtml.ts — a self-contained HTML string that
// use-rich-editor.native.tsx hands to TenTap's customSource. Owning the page
// is what lets markdown be the editor's native format on mobile: the WebView
// runs our own Tiptap with @tiptap/markdown, instead of TenTap's prebuilt page
// whose bridge only speaks HTML.
//
// Unlike a feature's build this one is declared explicitly in the generator,
// because core has no manifest to carry a `build.script` field.

const here = dirname(fileURLToPath(import.meta.url))
const sourceDir = resolve(here, 'webview/source')
const buildDir = resolve(here, 'webview/build')

// The bundle's imports (@tiptap/*, react-dom, yjs) resolve out of the app
// shell's and workspace root's node_modules — core has none of its own. The
// generator sets both roots; the fallbacks let the script run standalone.
// core/ is nested inside the app shell, so APP_ROOT is two levels up from
// lib/editor/rich → core/ → the shell.
const appShellRoot = process.env.TINYCLD_APP_ROOT ?? resolve(here, '../../..')
const wsRoot = process.env.TINYCLD_WS_ROOT ?? resolve(appShellRoot, '..')

async function main() {
    const result = await bundleWebViewEditor({
        entryHtml: resolve(sourceDir, 'index.html'),
        entryScript: resolve(sourceDir, 'entry.tsx'),
        outFile: resolve(buildDir, 'editorHtml.ts'),
        nodePaths: [resolve(appShellRoot, 'node_modules'), resolve(wsRoot, 'node_modules')],
    })
    // biome-ignore lint/suspicious/noConsole: build-time helper; surfacing the bundle size is intentional
    console.log(`[core rich-editor] bundled ${result.htmlSize} bytes → ${result.outFile}`)
}

main().catch(err => {
    // biome-ignore lint/suspicious/noConsole: build-time helper; reporting the failure is intentional
    console.error('[core rich-editor] build failed:', err)
    process.exit(1)
})
