// The SPA shell filename convention, in one place.
//
// Expo exports the shell as `dist/index.html` — that name is hardcoded in the
// CLI exporter (`@expo/cli`'s exportApp.js), with no flag or app.config field
// to change it, so every consumer of a web export has to do the rename itself.
//
// The server looks for `app.html`, NOT `index.html`, because the static handler
// tries literal files before falling back to the shell: a shell named
// index.html would also be reachable as a static hit, and a stray
// `public/index.html` would silently shadow it. The distinct name keeps the
// shell unambiguous.
//
// Three places consume a web export — release promotion (promote-release.ts),
// single-binary embedding (stage-embed-assets.ts), and the in-place package
// build (core/server/pkgbuild/pipeline.go, Go, which cannot share this code but
// points here). The embed path originally forgot the rename, which shipped a
// binary that served `/` but 404'd every deep link including the first-run
// setup URL. Route new consumers through here so that cannot recur.

import * as fs from 'node:fs'
import * as path from 'node:path'

/** What Expo names the exported shell. */
export const EXPORTED_SHELL = 'index.html'

/** What the server reads the shell from. Also `app.html` in pkgbuild's Go port. */
export const SERVED_SHELL = 'app.html'

/**
 * Asserts a web export exists and returns the path to its shell. Throws with a
 * fix-it message rather than letting a missing export surface later as a 404.
 */
export function exportedShellPath(distDir: string): string {
    const shell = path.join(distDir, EXPORTED_SHELL)
    if (!fs.existsSync(shell)) {
        throw new Error(
            `${shell} missing — run \`pnpm exec expo export --platform web\` to produce a web bundle`
        )
    }
    return shell
}

/**
 * Renames an already-staged export's shell in place, for consumers that copy
 * the whole dist tree first. No-ops when the staged dir already holds the
 * served name, so it is safe to call twice.
 */
export function renameStagedShell(stagedDir: string): void {
    const staged = path.join(stagedDir, EXPORTED_SHELL)
    const served = path.join(stagedDir, SERVED_SHELL)
    if (fs.existsSync(served)) return
    if (!fs.existsSync(staged)) {
        throw new Error(
            `${staged} missing — staged export has no SPA shell, so every deep link would 404`
        )
    }
    fs.renameSync(staged, served)
}
