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
        systemSettings: (manifest.systemSettings ?? []).map(s => ({
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
        ...(manifest.search ? { search: manifest.search } : {}),
        ...(manifest.automation ? { automation: manifest.automation.definitions } : {}),
        eventSources: (manifest.eventSources ?? []).map(s => ({
            target: s.target,
            id: s.id,
            label: s.label,
            module: s.module,
            ...(s.color ? { color: s.color } : {}),
            order: s.order ?? 0,
        })),
        eventSourceHost: Boolean(manifest.eventSourceHost),
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
 * Cross-package validation for `nav.shortcut`. Two packages claiming the same
 * letter make `t <letter>` ambiguous: CoreShortcuts registers both under
 * different ids, and the matcher fires whichever it reaches first, so one
 * package simply becomes unreachable by keyboard.
 *
 * docs/keyboard-shortcuts.md has always claimed generation "fails fast if two
 * manifests claim the same letter". It did not, and the e2e shortcut stub and
 * boards silently collided on 'k' until this was added.
 */
export function validateNavShortcuts(pkgs: ConfigPkg[]): void {
    const claimedBy = new Map<string, string>()
    for (const p of pkgs) {
        // ConfigPkg.manifest carries extra fields under an index signature, so
        // nav arrives as `unknown` and has to be narrowed here.
        const nav = p.manifest.nav as { shortcut?: string } | undefined
        const letter = nav?.shortcut
        if (!letter) continue
        const existing = claimedBy.get(letter)
        if (existing) {
            throw new Error(
                `[generate] nav.shortcut '${letter}' is claimed by both '${existing}' and '${p.slug}'. Each package needs its own letter — 't ${letter}' cannot resolve to two packages.`
            )
        }
        claimedBy.set(letter, p.slug)
    }
}

/**
 * Cross-package validation for sidebar contributions. Run once with the full set
 * of present packages. Targets that aren't in this workspace are tolerated
 * (normal for a partial checkout — the contribution just won't appear). A
 * contribution targeting a present package's UNKNOWN slot is a build error.
 */
/**
 * Cross-package validation for event-source contributions. An absent target is
 * tolerated with a warning (partial checkout — the source is silently
 * inactive); a PRESENT target that does not declare `eventSourceHost: true` is
 * a build error (version drift should fail fast, not render nothing). Source
 * ids are embedded in `:`-delimited synthetic event ids by the host, so they
 * are restricted to [a-z0-9-], and (target, id) must be unique across the
 * workspace or two sources' items would collide.
 */
export function validateEventSources(pkgs: ConfigPkg[]): void {
    const hostBySlug = new Map<string, boolean>()
    for (const p of pkgs) {
        hostBySlug.set(p.slug, p.eventSourceHost)
    }
    const seen = new Map<string, string>()
    for (const p of pkgs) {
        for (const s of p.eventSources) {
            if (!/^[a-z0-9-]+$/.test(s.id)) {
                throw new Error(
                    `[generate] ${p.slug}: eventSource id '${s.id}' is invalid — ids are embedded in synthetic event ids and must match [a-z0-9-]+`
                )
            }
            const key = `${s.target}:${s.id}`
            const existing = seen.get(key)
            if (existing) {
                throw new Error(
                    `[generate] eventSource id '${s.id}' targeting '${s.target}' is declared by both '${existing}' and '${p.slug}' — (target, id) must be unique`
                )
            }
            seen.set(key, p.slug)
            const host = hostBySlug.get(s.target)
            if (host === undefined) {
                console.warn(
                    `[generate] ${p.slug}: eventSource targets '${s.target}' which is not installed in this workspace — source will be silently inactive`
                )
                continue
            }
            if (!host) {
                throw new Error(
                    `[generate] ${p.slug}: eventSource targets '${s.target}', which does not declare eventSourceHost: true. Upgrade '${s.target}' to a version that hosts event sources or drop the contribution.`
                )
            }
        }
    }
}

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
