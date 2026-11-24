import { declaresTerms, useTermsBlocked } from '@tinycld/core/lib/packages/use-package-terms'
import { reportTermsGate } from '../lib/hub-rows'

/**
 * Whether a saved report may open, by the terms of the package that owns its
 * source: a report of a closed package opens its data, so it reads as missing.
 * `open` until the report's source is known.
 */
export function useReportTermsGate(source: string | undefined) {
    const { blocked, isReady } = useTermsBlocked()
    if (source === undefined) return 'open'
    return reportTermsGate({ source, blocked, isReady, declaresTerms })
}
