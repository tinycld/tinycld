import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { captureException } from '@tinycld/core/lib/errors'
import type PocketBase from 'pocketbase'
import { useCallback, useEffect, useMemo, useState } from 'react'

// Shared types for the version-management UI. Mirrors the Go discovery/check
// endpoints (pkg_versions.go, pkg_compat.go).

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

interface RegistryRow {
    slug: string
    name: string
    version: string
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

// compareVersions does a minimal numeric semver compare (ignoring pre-release
// tags): returns <0 if a<b, 0 if equal, >0 if a>b, or null if either is
// unparseable. Leading `v` is tolerated. This is intentionally small — the
// authoritative comparison runs server-side; this only drives the UI's
// confirmation gate.
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

async function adminFetch<T>(pb: PocketBase, path: string, init?: RequestInit): Promise<T> {
    const res = await fetch(`${PB_SERVER_ADDR}/api/admin/packages${path}`, {
        ...init,
        headers: {
            'Content-Type': 'application/json',
            Authorization: pb.authStore.token,
            ...(init?.headers ?? {}),
        },
    })
    if (!res.ok) {
        throw new Error(`${path} failed: ${res.status}`)
    }
    return res.json() as Promise<T>
}

// usePackageVersions owns all data + selection + mutation state for the version
// tab, keeping the component JSX declarative. Selection is a map of slug →
// target version; clearing a target (set to current) removes it from the set.
export function usePackageVersions(pb: PocketBase) {
    const [versions, setVersions] = useState<PackageVersionInfo[]>([])
    const [names, setNames] = useState<Record<string, string>>({})
    const [isLoading, setIsLoading] = useState(true)
    const [targets, setTargets] = useState<Record<string, string>>({})
    const [violations, setViolations] = useState<CompatViolation[]>([])
    const [isChecking, setIsChecking] = useState(false)
    const [applyJobId, setApplyJobId] = useState<string | null>(null)

    const fetchVersions = useCallback(async () => {
        setIsLoading(true)
        try {
            const [reg, vers] = await Promise.all([
                pb.collection('pkg_registry').getFullList<RegistryRow>({ sort: 'name' }),
                adminFetch<{ packages: PackageVersionInfo[] }>(pb, '/versions'),
            ])
            setNames(Object.fromEntries(reg.map(r => [r.slug, r.name])))
            setVersions(vers.packages)
        } catch (err) {
            captureException('versions.fetch', err)
        } finally {
            setIsLoading(false)
        }
    }, [pb])

    useEffect(() => {
        fetchVersions()
    }, [fetchVersions])

    const currentBySlug = useMemo(
        () => Object.fromEntries(versions.map(v => [v.slug, v.current])),
        [versions]
    )

    const setTarget = useCallback(
        (slug: string, version: string) => {
            setTargets(prev => {
                const next = { ...prev }
                if (version === currentBySlug[slug] || !version) {
                    delete next[slug]
                } else {
                    next[slug] = version
                }
                return next
            })
        },
        [currentBySlug]
    )

    const pendingChanges = useMemo<PendingChange[]>(() => {
        const infoBySlug = Object.fromEntries(versions.map(v => [v.slug, v]))
        return Object.entries(targets).map(([slug, targetVersion]) => {
            const info = infoBySlug[slug]
            // No discovery info for this slug → can't tell the direction; treat as
            // a downgrade so it still requires confirmation (the safe default).
            const isDowngrade = info ? detectDowngrade(info, targetVersion) : true
            return { slug, targetVersion, isDowngrade }
        })
    }, [targets, versions])

    const selectAllUpdates = useCallback(() => {
        setTargets(() => {
            const next: Record<string, string> = {}
            for (const v of versions) {
                if (v.hasUpdate && v.latest) next[v.slug] = v.latest
            }
            return next
        })
    }, [versions])

    const clearSelection = useCallback(() => setTargets({}), [])

    // Re-check compatibility whenever the target set changes.
    useEffect(() => {
        const changeList = Object.entries(targets)
        if (changeList.length === 0) {
            setViolations([])
            return
        }
        let cancelled = false
        setIsChecking(true)
        const handle = setTimeout(async () => {
            try {
                const result = await adminFetch<{ ok: boolean; violations: CompatViolation[] }>(
                    pb,
                    '/versions/check',
                    { method: 'POST', body: JSON.stringify({ changes: targets }) }
                )
                if (!cancelled) setViolations(result.violations ?? [])
            } catch (err) {
                captureException('versions.check', err)
                if (!cancelled) setViolations([])
            } finally {
                if (!cancelled) setIsChecking(false)
            }
        }, 300)
        return () => {
            cancelled = true
            clearTimeout(handle)
        }
    }, [targets, pb])

    const fetchDropReport = useCallback(
        async (slug: string, targetVersion: string): Promise<DropReport> => {
            return adminFetch<DropReport>(pb, '/versions/drop-report', {
                method: 'POST',
                body: JSON.stringify({ slug, targetVersion }),
            })
        },
        [pb]
    )

    const applyChanges = useCallback(async () => {
        const changes = pendingChanges.map(c => ({
            slug: c.slug,
            targetVersion: c.targetVersion,
        }))
        try {
            const result = await adminFetch<{ jobId: string }>(pb, '/versions/apply', {
                method: 'POST',
                body: JSON.stringify({ changes }),
            })
            setApplyJobId(result.jobId)
        } catch (err) {
            captureException('versions.apply', err)
            throw err
        }
    }, [pb, pendingChanges])

    const onApplyComplete = useCallback(() => {
        setApplyJobId(null)
        clearSelection()
        fetchVersions()
    }, [clearSelection, fetchVersions])

    return {
        versions,
        names,
        isLoading,
        targets,
        setTarget,
        pendingChanges,
        hasDowngrade: pendingChanges.some(c => c.isDowngrade),
        violations,
        isChecking,
        canApply: pendingChanges.length > 0 && violations.length === 0 && !isChecking,
        selectAllUpdates,
        clearSelection,
        fetchDropReport,
        applyChanges,
        applyJobId,
        onApplyComplete,
        refresh: fetchVersions,
    }
}
