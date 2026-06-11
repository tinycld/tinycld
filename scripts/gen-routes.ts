import * as fs from 'node:fs'
import * as path from 'node:path'

// Recursively list files (relative paths) under dir.
function walkFiles(dir: string, prefix = ''): string[] {
    if (!fs.existsSync(dir)) return []
    const out: string[] = []
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const rel = prefix ? path.join(prefix, entry.name) : entry.name
        if (entry.isDirectory()) out.push(...walkFiles(path.join(dir, entry.name), rel))
        else out.push(rel)
    }
    return out
}

const ROUTE_EXTS = new Set(['.tsx', '.ts', '.jsx', '.js'])

export interface EmitRoutesOpts {
    packageName: string
    slug: string
    packageDir: string
    routesDir: string // path RELATIVE to packageDir where screen files live
    importSubpath: string // the package.json exports subpath, e.g. 'screens'
    routesBase: string // app/app/a/[orgSlug]
}

// Emit one `export { default } from '<pkg>/<subpath>/<file>'` per screen file,
// preserving directory structure, under routesBase/<slug>/.
export function emitRoutes(opts: EmitRoutesOpts): string[] {
    const screensDir = path.join(opts.packageDir, opts.routesDir)
    const files = walkFiles(screensDir).filter(f => ROUTE_EXTS.has(path.extname(f)))
    const pkgRouteDir = path.join(opts.routesBase, opts.slug)
    fs.mkdirSync(pkgRouteDir, { recursive: true })

    const written: string[] = []
    for (const file of files) {
        const withoutExt = file.replace(/\.[^.]+$/, '')
        const importPath = `${opts.packageName}/${opts.importSubpath}/${withoutExt}`
        const outFile = path.join(pkgRouteDir, file)
        fs.mkdirSync(path.dirname(outFile), { recursive: true })
        fs.writeFileSync(outFile, `export { default } from '${importPath}'\n`)
        written.push(outFile)
    }
    return written
}

export interface EmitPublicRoutesOpts {
    packageName: string
    slug: string
    packageDir: string
    routesDir: string // relative to packageDir
    importSubpath: string // e.g. 'public-screens'
    publicRoutesBase: string // app/app/p
}

// A child-dir name is prunable only if it's a plain, single-segment slug — the
// same defense-in-depth guard generate.ts applies before any slug-joined rmSync.
// (A '/'/'..'/absolute name should never appear as a real dir entry, but if one
// somehow does, refuse to touch it rather than risk escaping `base`.)
function isPrunableSlug(name: string): boolean {
    return !(name.includes('/') || name.includes('..') || path.isAbsolute(name))
}

// Remove orphan generated route dirs left behind when a package is uninstalled.
// `base` is ROUTES_BASE or PUBLIC_ROUTES_BASE; `presentSlugs` are the slugs of
// packages routed in THIS run; `appOwnedEntries` are dir names under `base` that
// the app owns (never package route dirs). Only direct CHILD DIRECTORIES are
// considered — app-owned files (_layout.tsx, index.tsx, demo.tsx) are never
// touched, and any dir that's app-owned, a present slug, or fails the slug guard
// is left alone. Everything else is, by construction, an orphan package route
// dir (each present package re-creates its own dir via emit*Routes), so it's
// safe to rm -rf. Returns the names of the dirs it pruned (for logging/tests).
export function pruneOrphanRouteDirs(
    base: string,
    presentSlugs: Set<string>,
    appOwnedEntries: Set<string>
): string[] {
    if (!fs.existsSync(base)) return []
    const pruned: string[] = []
    for (const entry of fs.readdirSync(base, { withFileTypes: true })) {
        if (!entry.isDirectory()) continue
        const name = entry.name
        if (presentSlugs.has(name) || appOwnedEntries.has(name)) continue
        if (!isPrunableSlug(name)) continue
        fs.rmSync(path.join(base, name), { recursive: true, force: true })
        pruned.push(name)
    }
    return pruned
}

export function emitPublicRoutes(opts: EmitPublicRoutesOpts): string[] {
    const sourceDir = path.join(opts.packageDir, opts.routesDir)
    const files = walkFiles(sourceDir).filter(f => ROUTE_EXTS.has(path.extname(f)))
    const pkgRouteDir = path.join(opts.publicRoutesBase, opts.slug)
    fs.mkdirSync(pkgRouteDir, { recursive: true })

    const written: string[] = []
    for (const file of files) {
        const withoutExt = file.replace(/\.[^.]+$/, '')
        const importPath = `${opts.packageName}/${opts.importSubpath}/${withoutExt}`
        const outFile = path.join(pkgRouteDir, file)
        fs.mkdirSync(path.dirname(outFile), { recursive: true })
        fs.writeFileSync(outFile, `export { default } from '${importPath}'\n`)
        written.push(outFile)
    }
    return written
}
