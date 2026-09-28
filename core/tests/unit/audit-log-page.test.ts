import { describe, expect, it } from 'vitest'
import { AUDIT_PAGE_SIZE, auditWindowSize, hasMoreAuditPages } from '../../lib/audit-log-page'

// `audit_logs` is unbounded history, so the screen must never ask for it
// unfiltered. These are the two numbers that keep that true: how large a window
// the query asks the server for, and when "Load more" is spent.

describe('auditWindowSize', () => {
    it('asks for one page first, then one more page per press', () => {
        expect(auditWindowSize(1)).toBe(AUDIT_PAGE_SIZE)
        expect(auditWindowSize(3)).toBe(AUDIT_PAGE_SIZE * 3)
    })

    it('never asks for an empty or negative window', () => {
        expect(auditWindowSize(0)).toBe(AUDIT_PAGE_SIZE)
        expect(auditWindowSize(-2)).toBe(AUDIT_PAGE_SIZE)
    })
})

describe('hasMoreAuditPages', () => {
    it('offers another page while the window came back full', () => {
        expect(hasMoreAuditPages(AUDIT_PAGE_SIZE, 1)).toBe(true)
        expect(hasMoreAuditPages(AUDIT_PAGE_SIZE * 2, 2)).toBe(true)
    })

    it('stops as soon as a page returns fewer rows than it asked for', () => {
        expect(hasMoreAuditPages(AUDIT_PAGE_SIZE - 1, 1)).toBe(false)
        expect(hasMoreAuditPages(AUDIT_PAGE_SIZE + 1, 2)).toBe(false)
    })

    it('an empty log offers nothing to load', () => {
        expect(hasMoreAuditPages(0, 1)).toBe(false)
    })
})
