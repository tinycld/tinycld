import type { AutoUpgradeStatus } from '@tinycld/core/components/setup/auto-upgrade-logic'

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
        // A package without nav has no rail icon: it is a library, not an app.
        if (!row || !pkg.nav || pkg.slug === CORE_SLUG) return []
        return [
            {
                id: row.id,
                slug: pkg.slug,
                name: pkg.name,
                description: pkg.description,
                icon: pkg.nav.icon ?? '',
                isOn: row.status !== 'disabled',
            },
        ]
    })
}

// The wizard offers automatic updates only where the server can do them; on a
// build that cannot update itself the choice would be a promise it cannot keep.
// Hidden until the stored value has loaded: before that, `value` is the
// "missing row" false and a click would write the opposite of what the owner saw.
export function autoUpdateChoiceOf(
    status: AutoUpgradeStatus | undefined,
    value: boolean,
    isReady: boolean
): { isVisible: boolean; isOn: boolean } {
    return { isVisible: (status?.available ?? false) && isReady, isOn: value }
}
