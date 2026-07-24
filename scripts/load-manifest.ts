import * as fs from 'node:fs'
import * as path from 'node:path'
import { pathToFileURL } from 'node:url'

export interface PackageManifest {
    name: string
    slug: string
    version: string
    description: string
    routes?: { directory: string }
    publicRoutes?: { directory: string }
    nav?: { label: string; icon: string; order?: number; shortcut?: string }
    migrations?: { directory: string }
    hooks?: { directory: string }
    collections?: { register: string; types: string }
    sidebar?: { component: string }
    provider?: { component: string }
    settings?: { slug: string; component: string; label: string }[]
    systemSettings?: { slug: string; component: string; label: string }[]
    slots?: string[]
    sidebarContributions?: {
        target: string
        slot: string
        component: string
        order?: number
    }[]
    seed?: { script: string }
    tests?: { directory: string }
    build?: { script: string }
    server?: { package: string; module: string }
    help?: { directory: string }
    repository?: { url: string; issueTemplate?: string }
    dependencies?: string[]
    // Host-side capability config, read by core from bundled-packages.json and
    // interpreted by the shared Go capability packages (tinycld.org/core/{fts,
    // carddav,audit}). A feature contributes these as DATA — no feature Go runs
    // in the multi-org host. See tinycld/core/server/{fts,carddav,audit}.
    //
    // Full-text search: core registers index-sync record hooks + a
    // GET /api/<slug>/search route from this block.
    fts?: {
        collection: string
        table: string
        columns: { fts: string; field: string; strip?: boolean }[]
        owner: { field: string; via: string; userField: string }
        // Columns the search route returns per hit. `type` coerces the JSON shape
        // to match the schema ('bool'|'number'; omit for string). Kept as a broad
        // string so a manifest object literal doesn't need `as const`; core
        // validates the value.
        output: { column: string; type?: string }[]
        softDeleteField?: string
    }
    // CardDAV: core serves this collection as vCards over /carddav, scoped per org.
    carddav?: {
        collection: string
        listFilter: string
        sort?: string
        ownerField: string
        uidField: string
        softDeleteField?: string
        vcard: {
            version?: string
            name: { given: string; family: string }
            simple?: Record<string, string>
            revField?: string
        }
    }
    // Audit: core registers audit hooks for these collections from config,
    // replacing the per-package audit.RegisterCollection Go call.
    audit?: {
        collection: string
        resolveOrg?: { field: string; collection: string; orgField: string }
        labelFields?: string[]
        labelJoin?: string
    }[]
    // Semver ranges this version requires of other packages / @tinycld/core,
    // enforced by the version-management compatibility solver. See the
    // PackageManifest doc in core/lib/packages/types.ts.
    peerVersions?: Record<string, string>
}

// Import a member's manifest.ts (ESM default export). This file is run via tsx,
// so a dynamic import of a .ts file works.
export async function loadManifest(packageDir: string): Promise<PackageManifest> {
    const candidate = ['manifest.ts', 'manifest.js']
        .map(f => path.join(packageDir, f))
        .find(p => fs.existsSync(p))
    if (!candidate) throw new Error(`No manifest found in ${packageDir}`)
    const mod = await import(pathToFileURL(candidate).href)
    return mod.default as PackageManifest
}
