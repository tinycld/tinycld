import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { captureException } from '@tinycld/core/lib/errors'
import type PocketBase from 'pocketbase'
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
    type CompatViolation,
    type DropReport,
    detectDowngrade,
    type PackageVersionInfo,
    type PendingChange,
} from './version-compare'
import { useVersionStagingStore } from './version-staging-store'

// The pure version helpers + their types live in the leaf module version-compare
// (no React/store/network), so the hook and the staging store can both use them
// without an import cycle. Re-exported here for existing call sites that import
// these from the hook.
export {
    type CompatViolation,
    compareVersions,
    type DropReport,
    detectDowngrade,
    type PackageVersionInfo,
    type PendingChange,
} from './version-compare'

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

// usePackageVersions owns the version discovery + selection + compat/apply
// mutation state for the merged Packages screen, keeping the JSX declarative.
// Selection (the staged target per slug) lives in the version-staging store;
// this hook layers the server data and mutations on top of it.
//
// `enabled` gates the initial `/versions` fetch on the Packages tab being shown
// (the dashboard keeps every tab's component mounted). Deferring it avoids
// firing the discovery — which shells out to `git ls-remote` server-side — until
// it's actually needed.
export function usePackageVersions(pb: PocketBase, enabled = true) {
    const [versions, setVersions] = useState<PackageVersionInfo[]>([])
    const [isLoading, setIsLoading] = useState(true)
    // Staged version targets live in a store so the row selects, the apply
    // footer, and the per-row "staged" lock all share one source of truth.
    const targets = useVersionStagingStore(s => s.targets)
    const setTargetInStore = useVersionStagingStore(s => s.setTarget)
    const setAllTargets = useVersionStagingStore(s => s.setAll)
    const clearTargets = useVersionStagingStore(s => s.clear)
    const [violations, setViolations] = useState<CompatViolation[]>([])
    const [isChecking, setIsChecking] = useState(false)
    const [applyJobId, setApplyJobId] = useState<string | null>(null)

    // Only fetches `/versions` (the per-package discovery, which shells out to
    // `git ls-remote` server-side). The merged Packages screen owns the
    // pkg_registry list, so this hook no longer reads that collection — which
    // also removes the same-collection auto-cancel race that two simultaneous
    // pkg_registry reads caused.
    const fetchVersions = useCallback(async () => {
        setIsLoading(true)
        try {
            const vers = await adminFetch<{ packages: PackageVersionInfo[] }>(pb, '/versions')
            setVersions(vers.packages)
        } catch (err) {
            captureException('versions.fetch', err)
        } finally {
            setIsLoading(false)
        }
    }, [pb])

    useEffect(() => {
        if (enabled) fetchVersions()
    }, [enabled, fetchVersions])

    const currentBySlug = useMemo(
        () => Object.fromEntries(versions.map(v => [v.slug, v.current])),
        [versions]
    )

    const setTarget = useCallback(
        (slug: string, version: string) => {
            setTargetInStore(slug, version, currentBySlug[slug] ?? '')
        },
        [currentBySlug, setTargetInStore]
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
        const next: Record<string, string> = {}
        for (const v of versions) {
            if (v.hasUpdate && v.latest) next[v.slug] = v.latest
        }
        setAllTargets(next)
    }, [versions, setAllTargets])

    const clearSelection = useCallback(() => clearTargets(), [clearTargets])

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
