#!/usr/bin/env tsx
/**
 * Stage the assets that get compiled into the single-binary build.
 *
 * Three constraints drive this script:
 *   1. `go:embed` does not follow symlinks, and server/pb_migrations +
 *      server/pb_hooks are symlink farms the generator points at sibling
 *      repos. They must be materialized into real files.
 *   2. Source maps must never be embedded. They are the bulk of the web
 *      export and ship to Sentry separately via
 *      `expo export --source-maps external`.
 *   3. The SPA shell must land under the name the server reads (app.html, not
 *      Expo's index.html) — see ./app-shell.ts. Copying dist verbatim, as this
 *      script once did, yields a binary that serves / but 404s every deep link,
 *      including the first-run setup URL it prints on boot.
 */
import {
    copyFileSync,
    cpSync,
    existsSync,
    mkdirSync,
    readdirSync,
    realpathSync,
    rmSync,
    statSync,
} from 'node:fs'
import { basename, join } from 'node:path'
import { renameStagedShell } from './app-shell'

const appRoot = join(import.meta.dirname, '..')
const outDir = join(appRoot, 'server', 'embedded_assets')

const copyResolvedFiles = (srcDir: string, destDir: string) => {
    mkdirSync(destDir, { recursive: true })
    if (!existsSync(srcDir)) return 0
    let count = 0
    for (const entry of readdirSync(srcDir)) {
        const src = join(srcDir, entry)
        // statSync (not lstatSync) resolves symlinks to their target.
        if (!statSync(src).isFile()) continue
        // Copy the symlink's TARGET, not the link: cpSync given a symlinked
        // path treats it as a directory and fails with ERR_FS_EISDIR, and
        // go:embed would skip a copied link anyway.
        copyFileSync(realpathSync(src), join(destDir, basename(entry)))
        count++
    }
    return count
}

const copyWebBundle = (srcDir: string, destDir: string) => {
    if (!existsSync(srcDir)) {
        throw new Error(
            `web bundle not found at ${srcDir} — run \`pnpm exec expo export --platform web\` first`
        )
    }
    let skipped = 0
    cpSync(srcDir, destDir, {
        recursive: true,
        dereference: true,
        filter: src => {
            if (src.endsWith('.map')) {
                skipped++
                return false
            }
            return true
        },
    })
    return skipped
}

rmSync(outDir, { recursive: true, force: true })
mkdirSync(outDir, { recursive: true })

const webDir = join(outDir, 'web')
const skippedMaps = copyWebBundle(join(appRoot, 'dist'), webDir)
// index.html → app.html. Throws when the export produced no shell, rather than
// embedding a bundle whose every deep link 404s at runtime.
renameStagedShell(webDir)
const migrations = copyResolvedFiles(
    join(appRoot, 'server', 'pb_migrations'),
    join(outDir, 'pb_migrations')
)
const hooks = copyResolvedFiles(join(appRoot, 'server', 'pb_hooks'), join(outDir, 'pb_hooks'))

if (migrations === 0) {
    throw new Error('staged 0 migrations — run `pnpm run packages:generate` first')
}

console.log(
    `staged: web bundle (${skippedMaps} source maps excluded), ${migrations} migrations, ${hooks} hooks`
)
