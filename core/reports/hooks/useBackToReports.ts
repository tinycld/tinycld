import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { router } from 'expo-router'

// A report is opened from the hub, which the router history still holds. Going
// back returns to it; a deep link with nothing to go back to lands on the hub.
export function useBackToReports() {
    const orgHref = useOrgHref()
    return () => {
        if (router.canGoBack()) router.back()
        else router.replace(orgHref('reports'))
    }
}
