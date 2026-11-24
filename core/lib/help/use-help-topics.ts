import { packageHelp } from '@tinycld/app-generated/package-help'
import { useMemo } from 'react'
import { useTermsBlocked } from '../packages/use-package-terms'
import { isManagedPrefix, useManagedSettingPrefixes } from '../use-managed-settings'
import type { HelpGroup, HelpTopic, HelpTopicId } from './types'

const allGroups = packageHelp as unknown as HelpGroup[]

// Every hook below filters through the same rule, so the hub, the per-package
// list, the drawer and the search palette can never disagree about what this
// deployment documents. A topic reachable from search alone would still be a
// set of instructions for a screen that is not here.
//
// A package whose terms the owner has not accepted contributes no topics: its
// help describes screens nobody can open yet.
export function visibleGroups(
    groups: readonly HelpGroup[],
    managed: readonly string[],
    blocked: ReadonlySet<string>
): readonly HelpGroup[] {
    if (managed.length === 0 && blocked.size === 0) return groups
    const out: HelpGroup[] = []
    for (const group of groups) {
        if (blocked.has(group.pkgSlug)) continue
        const topics = group.topics.filter(t => !isManagedPrefix(managed, t.keyPrefix))
        // A package whose every topic is hidden contributes no section at all,
        // rather than an empty heading.
        if (topics.length > 0) out.push({ ...group, topics })
    }
    return out
}

export function useHelpGroups(): readonly HelpGroup[] {
    const managed = useManagedSettingPrefixes()
    const { blocked } = useTermsBlocked()
    return useMemo(() => visibleGroups(allGroups, managed, blocked), [managed, blocked])
}

export function useHelpTopics(): HelpTopic[] {
    const groups = useHelpGroups()
    return useMemo(() => groups.flatMap(g => g.topics), [groups])
}

export function useHelpTopic(id: HelpTopicId | null | undefined): HelpTopic | null {
    const topics = useHelpTopics()
    return useMemo(() => {
        if (!id) return null
        // Resolved against the VISIBLE set, so a deep link to a hidden topic
        // renders not-found instead of stale instructions.
        return topics.find(t => t.id === id) ?? null
    }, [id, topics])
}

export function useHelpGroupForPackage(pkgSlug: string | null | undefined): HelpGroup | null {
    const groups = useHelpGroups()
    return useMemo(
        () => (pkgSlug ? (groups.find(g => g.pkgSlug === pkgSlug) ?? null) : null),
        [pkgSlug, groups]
    )
}
