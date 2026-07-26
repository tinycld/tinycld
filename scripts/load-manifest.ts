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
    // Protocol capabilities. Core serves these; a package contributes only the
    // config, so a multi-org tenant (which links no feature Go) still gets the
    // protocol. The host materializes these blocks into the tenant's runtime
    // config — see multi-org's controlplane/capabilities.go.
    carddav?: {
        collection: string
        listFilter: string
        sort?: string
        ownerField: string
        uidField: string
        softDeleteField?: string
        vcard: {
            version: string
            name: { given: string; family: string }
            simple: Record<string, string>
            revField?: string
        }
    }
    webdav?: {
        prefix: string
        collection: string
        fields: {
            name: string
            parent: string
            isFolder: string
            size: string
            file: string
            owner: string
            mimeType?: string
            updated?: string
        }
    }
    help?: { directory: string }
    repository?: { url: string; issueTemplate?: string }
    dependencies?: string[]
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
