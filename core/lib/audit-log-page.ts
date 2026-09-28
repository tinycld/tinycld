/**
 * Paging arithmetic for the audit log, kept pure so it can be asserted without
 * rendering the screen.
 *
 * `audit_logs` is unbounded history, admin-readable, and the screen's default
 * state has no filter — so an unfiltered live query would fetch every entry a
 * deployment ever wrote. The screen therefore always asks for a bounded window
 * (`limit(PAGE_SIZE * page)`) and grows it a page at a time.
 */
export const AUDIT_PAGE_SIZE = 50

export function auditWindowSize(page: number) {
    return AUDIT_PAGE_SIZE * Math.max(1, page)
}

/**
 * A page short of the window it asked for means the server has nothing more to
 * give, so "Load more" is spent. Equality is the only case that can still hide
 * rows — the window was filled exactly.
 */
export function hasMoreAuditPages(loadedCount: number, page: number) {
    return loadedCount >= auditWindowSize(page)
}
