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
    // mailListeners: this package serves mail protocols, so the multi-org
    // ROUTER creates per-org mail sockets for orgs whose set includes it; the
    // package's single Register discovers them via coreserver's TenantContext
    // (host mode binds its own ports instead).
    server?: { package: string; module: string; mailListeners?: boolean }
    // Go payload package (dir relative to the member root, e.g. 'server/api')
    // holding the exported HTTP request/response structs. The generator emits
    // lib/generated/<slug>-api.ts from it (see scripts/gen-payload-types.ts);
    // TS imports the types as @tinycld/app-generated/<slug>-api.
    payloads?: { package: string }
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
    // Storage-bearing collections. core/quota enforces the ceilings from this
    // as record hooks, so no write path can skip them. A source with no
    // ownerField counts toward the org ceiling only.
    quota?: {
        collection: string
        sizeField: string
        ownerField?: string
    }[]
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
        // Binds the feature's per-user soft-delete state; when set, a DAV
        // DELETE stamps it instead of destroying the record.
        trash?: {
            collection: string
            itemField: string
            userField: string
            trashedAtField: string
        }
    }
    caldav?: {
        prefix?: string
        calendarCollection: string
        eventCollection: string
        calendar: { name: string; description?: string }
        event: {
            calendar: string
            uid: string
            owner: string
            title: string
            description?: string
            location?: string
            start: string
            end: string
            allDay?: string
            recurrence?: string
            guests?: string
            reminder?: string
            busyStatus?: string
            visibility?: string
            updated?: string
            created?: string
            // Values for required select fields a minimal client payload omits.
            defaults?: Record<string, string>
        }
    }
    // CLI commands this package contributes to the `tinycld` binary.
    // package/module mirror `server` and drive gen-cli.ts; `scopes` feeds the
    // OAuth scope registry. Cobra owns the command list and --help. See the
    // PackageManifest doc in core/lib/packages/types.ts.
    cli?: {
        package: string
        module: string
        scopes?: string[]
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
