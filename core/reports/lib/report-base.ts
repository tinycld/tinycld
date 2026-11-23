import { createContext, createElement, type ReactNode, use } from 'react'

/** The route the report view lives under when its host names no other: core's own. */
export const REPORTS_BASE = 'reports'

// Scoped to one mounted report view, so two views on two routes never share a base.
const ReportBase = createContext<string>(REPORTS_BASE)

/** Sets the route a report view's drills and Back links are built under. */
export function ReportBaseProvider({ base, children }: { base: string; children: ReactNode }) {
    return createElement(ReportBase.Provider, { value: base }, children)
}

/**
 * The route the report view is drawn under, without the org prefix: `<base>/<id>`
 * is a report and `<base>` is the list its Back link opens. A host that draws
 * ReportScreen on its own route passes it as `basePath` and owns both routes.
 */
export function useReportBase(): string {
    return use(ReportBase)
}
