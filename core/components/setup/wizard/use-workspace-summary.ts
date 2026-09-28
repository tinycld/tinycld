import { inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useSetupWizardState } from '@tinycld/core/lib/setup/use-setup-wizard-state'
import { chosenOrgName } from '@tinycld/core/lib/setup/wizard-logic'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'

export function initialsOf(name: string): string {
    return name
        .split(/\s+/)
        .filter(Boolean)
        .slice(0, 2)
        .map(part => part[0]?.toUpperCase() ?? '')
        .join('')
}

/** The workspace name as the wizard shows it; see chosenOrgName. */
export function useChosenOrgName(): string {
    const { org } = useOrgInfo()
    const { state } = useSetupWizardState()
    return chosenOrgName(org?.name ?? '', state)
}

/** What the Done screen sums up: the chosen name, the apps on, and the people in. */
export function useWorkspaceSummary() {
    const name = useChosenOrgName()
    const [pkgRegistry, users] = useStore('pkg_registry', 'users')
    const packages = usePackages()

    const { data: enabled = [] } = useLiveQuery(query =>
        query
            .from({ p: pkgRegistry })
            .where(({ p }) => inArray(p.status, ['bundled', 'installed']))
            .select(({ p }) => ({ slug: p.slug }))
    )
    const { data: people = [] } = useLiveQuery(query =>
        query.from({ u: users }).select(({ u }) => ({ id: u.id }))
    )

    const enabledSlugs = new Set(enabled.map(e => e.slug))
    const appCount = packages.filter(p => p.nav && enabledSlugs.has(p.slug)).length

    return { name: name.trim(), appCount, memberCount: people.length }
}
