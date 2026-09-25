// The core row is the shell itself, not an app that can be hidden.
export const CORE_SLUG = 'core'

export interface AppChoice {
    id: string
    slug: string
    name: string
    description: string
    icon: string
    isOn: boolean
}

/**
 * The apps this step offers: packages compiled into this build (the static
 * registry is exactly that set), in build order, each with its registry row.
 * An app with no row yet has nothing to toggle, so it is left out until the
 * server seeds it.
 */
export function appChoicesOf(
    rows: readonly { id: string; slug: string; status: string }[],
    bundled: readonly {
        slug: string
        name: string
        description: string
        nav?: { icon?: string }
    }[]
): AppChoice[] {
    const rowBySlug = new Map(rows.map(r => [r.slug, r]))
    return bundled.flatMap(pkg => {
        const row = rowBySlug.get(pkg.slug)
        if (!row || pkg.slug === CORE_SLUG) return []
        return [
            {
                id: row.id,
                slug: pkg.slug,
                name: pkg.name,
                description: pkg.description,
                icon: pkg.nav?.icon ?? '',
                isOn: row.status !== 'disabled',
            },
        ]
    })
}
