// Resolves a /settings URL's (pkgSlug, panelSlug) segments to a contributed
// panel. Shared by the two settings route trees, which deliberately resolve
// against DIFFERENT registries:
//
//   settings/[...section]         → packageSettings       (org-scoped)
//   settings/system/[...section]  → packageSystemSettings (deployment-wide)
//
// They must stay separate because one package may declare the same slug in
// both: mail declares `provider` as an org panel (mail domains) AND as a system
// panel (provider choice + credentials). Resolved through a single registry the
// pair ('mail','provider') is ambiguous — whichever list is searched first
// wins and the other panel has no addressable URL at all.

export interface PanelGroupLike {
    pkgSlug: string
    packageName: string
    panels: readonly { slug: string }[]
}

/**
 * Finds the group and panel for a URL's segments, or null when either segment
 * is missing or names something no installed package contributes.
 *
 * Generic over the caller's own group type so the returned panel keeps its
 * concrete shape (label, Component, …) rather than widening to the constraint.
 */
export function resolvePanel<Group extends PanelGroupLike>(
    groups: readonly Group[],
    pkgSlug: string | undefined,
    panelSlug: string | undefined
): { group: Group; panel: Group['panels'][number] } | null {
    if (!pkgSlug || !panelSlug) return null
    const group = groups.find(g => g.pkgSlug === pkgSlug)
    if (!group) return null
    const panel = group.panels.find(p => p.slug === panelSlug)
    if (!panel) return null
    return { group, panel }
}
