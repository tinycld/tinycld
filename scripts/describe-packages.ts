import type { ConfigPkg } from './gen-config'
import type { PackageManifest } from './load-manifest'

export function schemaTypeName(slug: string): string {
    const camel = slug.replace(/-([a-z])/g, (_m, c) => c.toUpperCase())
    return `${camel.charAt(0).toUpperCase()}${camel.slice(1)}Schema`
}

export function manifestToConfigPkg(packageName: string, manifest: PackageManifest): ConfigPkg {
    const hasRegister = Boolean(manifest.collections?.register)
    const slots = manifest.slots ?? []
    const seenSlots = new Set<string>()
    for (const s of slots) {
        if (seenSlots.has(s)) {
            throw new Error(
                `[generate] ${manifest.slug}: duplicate slot name '${s}' in manifest.slots`
            )
        }
        seenSlots.add(s)
    }
    return {
        packageName,
        slug: manifest.slug,
        schemaType: hasRegister ? schemaTypeName(manifest.slug) : '',
        hasRegister,
        hasSidebar: Boolean(manifest.sidebar?.component),
        hasProvider: Boolean(manifest.provider?.component),
        hasSeed: Boolean(manifest.seed?.script),
        settings: (manifest.settings ?? []).map(s => ({
            slug: s.slug,
            label: s.label,
            component: s.component,
        })),
        slots,
        sidebarContributions: (manifest.sidebarContributions ?? []).map(c => ({
            target: c.target,
            slot: c.slot,
            component: c.component,
            order: c.order ?? 0,
        })),
        manifest: {
            name: manifest.name,
            slug: manifest.slug,
            version: manifest.version,
            description: manifest.description,
            ...(manifest.nav ? { nav: manifest.nav } : {}),
            ...(manifest.routes ? { routes: manifest.routes } : {}),
            ...(manifest.publicRoutes ? { publicRoutes: manifest.publicRoutes } : {}),
            ...(manifest.repository ? { repository: manifest.repository } : {}),
            ...(manifest.dependencies ? { dependencies: manifest.dependencies } : {}),
        },
    }
}

/**
 * Cross-package validation for sidebar contributions. Run once with the full set
 * of present packages. Targets that aren't in this workspace are tolerated
 * (normal for a partial checkout — the contribution just won't appear). A
 * contribution targeting a present package's UNKNOWN slot is a build error.
 */
export function validateSidebarContributions(pkgs: ConfigPkg[]): void {
    const slotsByTarget = new Map<string, Set<string>>()
    for (const p of pkgs) {
        slotsByTarget.set(p.slug, new Set(p.slots))
    }
    for (const p of pkgs) {
        for (const c of p.sidebarContributions) {
            const targetSlots = slotsByTarget.get(c.target)
            if (!targetSlots) {
                console.warn(
                    `[generate] ${p.slug}: sidebarContribution targets '${c.target}' which is not installed in this workspace — contribution will be silently inactive`
                )
                continue
            }
            if (!targetSlots.has(c.slot)) {
                throw new Error(
                    `[generate] ${p.slug}: sidebarContribution targets unknown slot '${c.target}:${c.slot}'. The '${c.target}' package declares slots: [${[...targetSlots].map(s => `'${s}'`).join(', ') || '(none)'}]`
                )
            }
        }
    }
}
