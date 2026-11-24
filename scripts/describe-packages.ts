import { parseRecordLabel, type RecordLabelDecl } from '../core/lib/record-labels/label'
import { CORE_SLOT_TARGET, CORE_SLOTS } from '../core/lib/setup/core-slots'
import { isValidOrderKey } from '../core/lib/setup/order'
import type { ConfigPkg, ConfigSetupStep } from './gen-config'
import type { PackageManifest, PackagePort } from './load-manifest'

// Names the supervisor reserves for the public HTTP(S) listeners it owns
// itself — a package cannot claim one of these as a port name.
const RESERVED_PORT_NAMES = new Set(['http', 'https', 'http-redirect'])

// Ports the supervisor itself binds — :80/:443 for the autocert HTTP-01 /
// HTTPS listeners (core/server/supervise/config.go autocertHTTPAddr /
// autocertHTTPSAddr), :7090 for its default plain-HTTP listener
// (defaultHTTPAddr). A package claiming one of these would make the
// supervisor's own bind collide with — or silently lose to — a package's.
const RESERVED_SUPERVISOR_PORTS = new Set([80, 443, 7090])

export interface PortsFeature {
    slug: string
    ports?: PackagePort[]
}

export function schemaTypeName(slug: string): string {
    const camel = slug.replace(/-([a-z])/g, (_m, c) => c.toUpperCase())
    return `${camel.charAt(0).toUpperCase()}${camel.slice(1)}Schema`
}

function toSetupSteps(manifest: PackageManifest): ConfigSetupStep[] {
    return (manifest.setupSteps ?? []).map(s => {
        if (!/^[a-z0-9-]+$/.test(s.id)) {
            throw new Error(
                `[generate] ${manifest.slug}: setupStep id '${s.id}' is invalid — ids appear in URLs and must match [a-z0-9-]+`
            )
        }
        if (s.order !== undefined && !isValidOrderKey(s.order)) {
            throw new Error(
                `[generate] ${manifest.slug}: setupStep '${s.id}' has order '${s.order}', which is not a fractional-indexing key (core uses a0–a3)`
            )
        }
        return { id: s.id, label: s.label, module: s.module, order: s.order ?? null }
    })
}

// A package labels only the collections it owns: the same `<slug>_*` naming
// convention core/server/pkgaccess resolves ownership by. Relabelling core's
// `users`, or another package's collection, would change every picker over it
// from outside the package that knows what its rows mean.
function ownsCollection(slug: string, collection: string): boolean {
    const prefix = slug.replace(/-/g, '_')
    return collection === prefix || collection.startsWith(`${prefix}_`)
}

function toRecordLabels(manifest: PackageManifest): Record<string, RecordLabelDecl> | undefined {
    if (!manifest.recordLabels) return undefined
    for (const [collection, decl] of Object.entries(manifest.recordLabels)) {
        if (!ownsCollection(manifest.slug, collection)) {
            throw new Error(
                `[generate] ${manifest.slug}: recordLabels names '${collection}', which is not one of its own collections (they are named ${manifest.slug.replace(/-/g, '_')}_*)`
            )
        }
        try {
            parseRecordLabel(decl)
        } catch (err) {
            const reason = err instanceof Error ? err.message : String(err)
            throw new Error(`[generate] ${manifest.slug}: recordLabels.${collection}: ${reason}`)
        }
    }
    return manifest.recordLabels
}

export function manifestToConfigPkg(packageName: string, manifest: PackageManifest): ConfigPkg {
    const recordLabels = toRecordLabels(manifest)
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
        accountSettings: (manifest.accountSettings ?? []).map(s => ({
            slug: s.slug,
            label: s.label,
            component: s.component,
        })),
        systemSettings: (manifest.systemSettings ?? []).map(s => ({
            slug: s.slug,
            label: s.label,
            component: s.component,
            keyPrefix: s.keyPrefix,
        })),
        slots,
        sidebarContributions: (manifest.sidebarContributions ?? []).map(c => ({
            target: c.target,
            slot: c.slot,
            component: c.component,
            order: c.order ?? 0,
            ...(c.label ? { label: c.label } : {}),
        })),
        ...(manifest.search ? { search: manifest.search } : {}),
        ...(manifest.automation ? { automation: manifest.automation.definitions } : {}),
        ...(manifest.notifications ? { notifications: manifest.notifications } : {}),
        ...(manifest.forms ? { forms: manifest.forms } : {}),
        eventSources: (manifest.eventSources ?? []).map(s => ({
            target: s.target,
            id: s.id,
            label: s.label,
            module: s.module,
            ...(s.color ? { color: s.color } : {}),
            order: s.order ?? 0,
        })),
        eventSourceHost: Boolean(manifest.eventSourceHost),
        setupSteps: toSetupSteps(manifest),
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
            ...(recordLabels ? { recordLabels } : {}),
            // The client closes a package that declares terms (static-registry).
            ...(manifest.terms ? { terms: manifest.terms } : {}),
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
 * Cross-package validation for `recordLabels`. Ownership is a name prefix, so
 * two slugs where one prefixes the other (`gizmo`, `gizmo-x`) can both claim
 * `gizmo_x_*`; the picker would show whichever loaded first.
 */
export function validateRecordLabels(pkgs: ConfigPkg[]): void {
    const claimedBy = new Map<string, string>()
    for (const p of pkgs) {
        const labels = p.manifest.recordLabels as Record<string, unknown> | undefined
        for (const collection of Object.keys(labels ?? {})) {
            const existing = claimedBy.get(collection)
            if (existing) {
                throw new Error(
                    `[generate] recordLabels for '${collection}' are declared by both '${existing}' and '${p.slug}'. Only the package that owns a collection may label it.`
                )
            }
            claimedBy.set(collection, p.slug)
        }
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

/**
 * Cross-package validation for `ports`. Names and port numbers must be
 * unique across the whole installed set — the supervisor binds each one
 * exactly once and looks it up by name, so a collision would make one
 * package's listener silently win (or bind the wrong socket). Reserved
 * names belong to the supervisor's own HTTP(S) listeners.
 */
export function validatePorts(features: PortsFeature[]): void {
    const slugByPort = new Map<number, string>()
    const slugByName = new Map<string, string>()
    for (const f of features) {
        for (const p of f.ports ?? []) {
            if (!Number.isInteger(p.port) || p.port < 1 || p.port > 65535) {
                throw new Error(
                    `[generate] ${f.slug}: port '${p.name}' has value ${p.port}, which is outside 1-65535`
                )
            }
            if (!/^[a-z0-9-]+$/.test(p.name)) {
                throw new Error(
                    `[generate] ${f.slug}: port name '${p.name}' is invalid — names must match [a-z0-9-]+`
                )
            }
            if (RESERVED_PORT_NAMES.has(p.name)) {
                throw new Error(
                    `[generate] ${f.slug}: port name '${p.name}' is reserved for the supervisor's own listener`
                )
            }
            if (RESERVED_SUPERVISOR_PORTS.has(p.port)) {
                throw new Error(
                    `[generate] ${f.slug}: port ${p.port} is reserved — the supervisor binds it itself`
                )
            }
            const portOwner = slugByPort.get(p.port)
            if (portOwner) {
                throw new Error(
                    `[generate] port ${p.port} is declared by both '${portOwner}' and '${f.slug}' — each public port must be claimed once`
                )
            }
            slugByPort.set(p.port, f.slug)
            const nameOwner = slugByName.get(p.name)
            if (nameOwner) {
                throw new Error(
                    `[generate] port name '${p.name}' is declared by both '${nameOwner}' and '${f.slug}' — each name must be unique`
                )
            }
            slugByName.set(p.name, f.slug)
        }
    }
}

export function validateSidebarContributions(pkgs: ConfigPkg[]): void {
    const slotsByTarget = new Map<string, Set<string>>()
    slotsByTarget.set(CORE_SLOT_TARGET, new Set(CORE_SLOTS))
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
