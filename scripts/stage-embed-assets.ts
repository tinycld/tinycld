#!/usr/bin/env tsx
/**
 * Stage the assets that get compiled into the single-binary build.
 *
 * Two constraints drive this script:
 *   1. `go:embed` does not follow symlinks, and server/pb_migrations +
 *      server/pb_hooks are symlink farms the generator points at sibling
 *      repos. They must be materialized into real files.
 *   2. Source maps must never be embedded. They are the bulk of the web
 *      export and ship to Sentry separately via
 *      `expo export --source-maps external`.
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

const skippedMaps = copyWebBundle(join(appRoot, 'dist'), join(outDir, 'web'))
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
