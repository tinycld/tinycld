// Pure version-comparison helpers + the shared types for the version-management
// UI. This is a LEAF module — no React, no store, no network — so both the data
// hook (use-package-versions) and the staging store can import from it without
// creating an import cycle. Mirrors the Go discovery/check endpoints
// (pkg_versions.go, pkg_compat.go).

export interface PackageVersionInfo {
    slug: string
    source: 'npm' | 'git' | 'unknown'
    current: string
    latest: string
    available: string[]
    hasUpdate: boolean
    error?: string
}

export interface CompatViolation {
    package: string
    requires: string
    range: string
    found: string
}

export interface DropReport {
    droppedCollections: string[]
    droppedFields: { collection: string; field: string }[]
}

// A pending per-package version selection (target differs from current).
export interface PendingChange {
    slug: string
    targetVersion: string
    isDowngrade: boolean
}

// compareVersions does a minimal numeric semver compare (ignoring pre-release
// tags): returns <0 if a<b, 0 if equal, >0 if a>b, or null if either is
// unparseable. Leading `v` is tolerated. This is intentionally small — the
// authoritative comparison runs server-side; this only drives the UI's
// confirmation gate and the "is this a no-op selection?" check.
// formatVersion renders a version for display with exactly one leading `v`.
// Discovered versions can be git tags that already include the `v` (e.g.
// "v0.0.3"); naively prepending "v" produced "vv0.0.3" in the confirm modal.
export function formatVersion(version: string): string {
    return `v${version.replace(/^v/, '')}`
}

export function compareVersions(a: string, b: string): number | null {
    const parse = (v: string) => {
        const core = v.replace(/^v/, '').split(/[-+]/)[0]
        const parts = core.split('.').map(n => Number.parseInt(n, 10))
        if (parts.some(Number.isNaN) || parts.length === 0) return null
        return parts
    }
    const pa = parse(a)
    const pb = parse(b)
    if (!pa || !pb) return null
    const len = Math.max(pa.length, pb.length)
    for (let i = 0; i < len; i++) {
        const d = (pa[i] ?? 0) - (pb[i] ?? 0)
        if (d !== 0) return d
    }
    return 0
}

// detectDowngrade decides whether moving a package to targetVersion is a
// downgrade. It prefers the published `available` order (newest-first, so a
// higher index == older == downgrade) when both versions are present; otherwise
// it falls back to a numeric semver compare. When it genuinely can't tell (e.g.
// the current version was yanked and isn't in `available` and neither parses),
// it returns true — a downgrade is the DESTRUCTIVE direction, so "unknown" must
// require confirmation rather than silently skip it.
export function detectDowngrade(info: PackageVersionInfo, targetVersion: string): boolean {
    const available = info.available ?? []
    const idx = available.indexOf(targetVersion)
    const curIdx = available.indexOf(info.current)
    if (idx >= 0 && curIdx >= 0) return idx > curIdx
    const cmp = compareVersions(targetVersion, info.current)
    if (cmp !== null) return cmp < 0
    return true // can't determine → treat as downgrade (requires confirmation)
}
