import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import type { ComponentType, LazyExoticComponent, ReactNode } from 'react'
import type { PackageSettingsPanel, SidebarContribution } from './config-types'

interface SidebarProps {
    isCollapsed: boolean
}
interface ProviderProps {
    children: ReactNode
}
type SidebarComp = ComponentType<SidebarProps> | LazyExoticComponent<ComponentType<SidebarProps>>
type ProviderComp = ComponentType<ProviderProps> | LazyExoticComponent<ComponentType<ProviderProps>>

type ComponentEntryLike = {
    manifest: { slug: string }
    sidebar?: SidebarComp | null
    provider?: ProviderComp | null
}

/** slug → sidebar component (null when the package contributes none). */
export function deriveSidebars(
    entries: readonly ComponentEntryLike[]
): Record<string, SidebarComp | null> {
    const out: Record<string, SidebarComp | null> = {}
    for (const e of entries) out[e.manifest.slug] = e.sidebar ?? null
    return out
}

/** slug → context provider component (null when the package contributes none). */
export function deriveProviders(
    entries: readonly ComponentEntryLike[]
): Record<string, ProviderComp | null> {
    const out: Record<string, ProviderComp | null> = {}
    for (const e of entries) out[e.manifest.slug] = e.provider ?? null
    return out
}

export interface PackageSettingsGroup {
    packageName: string
    pkgSlug: string
    panels: PackageSettingsPanel[]
}

type SettingsEntryLike = {
    manifest: { name: string; slug: string }
    settings?: PackageSettingsPanel[]
}

/** Settings panels grouped by package, omitting packages that contribute none. */
export function deriveSettings(entries: readonly SettingsEntryLike[]): PackageSettingsGroup[] {
    const out: PackageSettingsGroup[] = []
    for (const e of entries) {
        if (e.settings && e.settings.length > 0) {
            out.push({ packageName: e.manifest.name, pkgSlug: e.manifest.slug, panels: e.settings })
        }
    }
    return out
}

export interface SidebarContributionEntry {
    contributorSlug: string
    order: number
    Component: ComponentType | LazyExoticComponent<ComponentType>
}

type ContributionEntryLike = {
    manifest: { slug: string }
    sidebarContributions?: SidebarContribution[]
}

/**
 * target slug → slot name → sorted contributions.
 * Stable sort: ascending by `order`, ties broken by contributor slug.
 */
export function deriveSidebarContributions(
    entries: readonly ContributionEntryLike[]
): Record<string, Record<string, SidebarContributionEntry[]>> {
    const out: Record<string, Record<string, SidebarContributionEntry[]>> = {}
    for (const e of entries) {
        const contributions = e.sidebarContributions
        if (!contributions || contributions.length === 0) continue
        for (const c of contributions) {
            let bySlot = out[c.target]
            if (!bySlot) {
                bySlot = {}
                out[c.target] = bySlot
            }
            let list = bySlot[c.slot]
            if (!list) {
                list = []
                bySlot[c.slot] = list
            }
            list.push({
                contributorSlug: e.manifest.slug,
                order: c.order,
                Component: c.Component,
            })
        }
    }
    for (const target of Object.keys(out)) {
        for (const slot of Object.keys(out[target])) {
            out[target][slot].sort((a, b) => {
                if (a.order !== b.order) return a.order - b.order
                return a.contributorSlug.localeCompare(b.contributorSlug)
            })
        }
    }
    return out
}

export const packageSidebars = deriveSidebars(tinycldConfig)
export const packageProviders = deriveProviders(tinycldConfig)
export const packageSettings = deriveSettings(tinycldConfig)
export const packageSidebarContributions = deriveSidebarContributions(tinycldConfig)
