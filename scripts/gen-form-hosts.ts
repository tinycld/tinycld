import * as fs from 'node:fs'
import * as path from 'node:path'
import { pathToFileURL } from 'node:url'
import { CORE_FORM_HOST_SLUG, CORE_FORM_HOSTS } from '../core/lib/forms/core-hosts'
import { type FormHostDef, hostDefListSchema } from '../core/lib/forms/host'
import { SERVER_DIR } from './paths'

export interface MergedFormHosts {
    hosts: (FormHostDef & { pkgSlug: string })[]
}

function parseHosts(owner: string, raw: unknown): FormHostDef[] {
    const parsed = hostDefListSchema.safeParse(raw)
    if (!parsed.success) {
        const issues = parsed.error.issues
            .map(i => `${i.path.join('.')}: ${i.message}`)
            .join('\n  ')
        throw new Error(`[generate] ${owner}: invalid forms hosts:\n  ${issues}`)
    }
    return parsed.data
}

// Resolve an exports-map subpath to a file and import it, like loadAutomationDefs.
export async function loadFormHosts(
    packageDir: string,
    packageName: string,
    subpath: string
): Promise<FormHostDef[]> {
    const pkgJsonPath = path.join(packageDir, 'package.json')
    const exportsMap = (JSON.parse(fs.readFileSync(pkgJsonPath, 'utf8')).exports ?? {}) as Record<
        string,
        string | Record<string, unknown>
    >
    const rel = exportsMap[`./${subpath}`]
    if (!rel) {
        throw new Error(
            `[generate] ${packageName}: manifest declares forms hosts '${subpath}' but package.json exports has no './${subpath}' entry`
        )
    }
    if (typeof rel !== 'string') {
        throw new Error(
            `[generate] ${packageName}: exports entry './${subpath}' is not a plain string (conditional exports are unsupported here)`
        )
    }
    const mod = (await import(pathToFileURL(path.join(packageDir, rel)).href)) as {
        default?: unknown
    }
    if (!mod.default) {
        throw new Error(
            `[generate] ${packageName}: module '${rel}' must default-export FormHostDef[]`
        )
    }
    return parseHosts(packageName, mod.default)
}

export function mergeFormHosts(features: { slug: string; hosts: unknown }[]): MergedFormHosts {
    const out: MergedFormHosts = { hosts: [] }
    const seen = new Set<string>()
    for (const f of features) {
        for (const h of parseHosts(f.slug, f.hosts)) {
            if (!h.key.startsWith(`${f.slug}:`)) {
                throw new Error(
                    `[generate] form host ${h.key} must be prefixed with its package slug ${f.slug}`
                )
            }
            if (seen.has(h.key)) throw new Error(`[generate] duplicate form host ${h.key}`)
            seen.add(h.key)
            out.hosts.push({ ...h, pkgSlug: f.slug })
        }
    }
    out.hosts.sort((a, b) => a.key.localeCompare(b.key))
    return out
}

/** The package hosts plus the hosts core owns; a package may not claim core's slug. */
export function mergeAllFormHosts(features: { slug: string; hosts: unknown }[]): MergedFormHosts {
    for (const f of features)
        if (f.slug === CORE_FORM_HOST_SLUG)
            throw new Error(
                `[generate] the form host slug ${CORE_FORM_HOST_SLUG} is reserved for core`
            )
    return mergeFormHosts([{ slug: CORE_FORM_HOST_SLUG, hosts: CORE_FORM_HOSTS }, ...features])
}

export function emitFormHosts(merged: MergedFormHosts): void {
    fs.writeFileSync(
        path.join(SERVER_DIR, 'form_hosts.json'),
        `${JSON.stringify(merged, null, 4)}\n`
    )
}
