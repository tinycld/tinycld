import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { router } from 'expo-router'
import { reportBack } from '../lib/report-back'
import { useReportRecord } from './useReportRecord'

// A report is opened from the hub, which the router history still holds. Going
// back returns to it; a deep link with nothing to go back to lands on the hub.
export function useBackToReports() {
    const orgHref = useOrgHref()
    return () => {
        if (router.canGoBack()) router.back()
        else router.replace(orgHref('reports'))
    }
}

/**
 * The back link of a report opened from report `fromId`. It returns to the source as
 * the reader left it (history back), or, opened from a pasted link with no history,
 * opens the source with its defaults.
 */
export function useReportBack(fromId: string): { label: string; onBack: () => void } {
    const orgHref = useOrgHref()
    const source = useReportRecord(fromId)
    const back = reportBack(source.state, source.record?.title)
    const onBack = () => {
        if (back.to === 'hub') {
            router.replace(orgHref('reports'))
            return
        }
        // A failed read stays failed for the next screen's query of the same record
        // until it is recovered, so the source screen would open on its error.
        if (source.state === 'error') source.retry()
        if (router.canGoBack()) router.back()
        else router.replace(orgHref(`reports/${fromId}`))
    }
    return { label: back.label, onBack }
}
