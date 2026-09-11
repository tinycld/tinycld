import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'

/**
 * The deployment's single branding row (logo + crop). Absent until an admin
 * uploads a logo — the migration creates no default row — so callers must
 * handle `null` rather than assuming one exists. `brandingCollection` is
 * exposed alongside so a mutation site doesn't need a second `useStore` call.
 */
export function useOrgBranding() {
    const [brandingCollection] = useStore('org_branding')
    const { data } = useOrgLiveQuery(query => query.from({ branding: brandingCollection }))
    return { branding: data?.[0] ?? null, brandingCollection }
}
