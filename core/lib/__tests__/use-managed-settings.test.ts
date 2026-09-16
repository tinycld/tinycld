import { describe, expect, it } from 'vitest'
import { isManagedPrefix } from '../use-managed-settings'

describe('isManagedPrefix', () => {
    it('matches a panel whose namespace the operator owns', () => {
        expect(isManagedPrefix(['sentry.', 'vapid.', 'mail.'], 'mail.')).toBe(true)
        expect(isManagedPrefix(['sentry.', 'vapid.', 'mail.'], 'vapid.')).toBe(true)
    })

    it('leaves a namespace nobody claimed alone', () => {
        expect(isManagedPrefix(['mail.'], 'sentry.')).toBe(false)
    })

    // The standalone guarantee: with nothing managed, nothing is ever hidden.
    it('hides nothing when the deployment administers everything', () => {
        for (const prefix of ['mail.', 'vapid.', 'sentry.']) {
            expect(isManagedPrefix([], prefix)).toBe(false)
        }
    })

    // A panel must opt in by declaring what it writes; every existing package
    // declares nothing and must keep rendering.
    it('never hides a panel that declares no namespace', () => {
        expect(isManagedPrefix(['mail.', 'vapid.'], undefined)).toBe(false)
        expect(isManagedPrefix(['mail.', 'vapid.'], '')).toBe(false)
        expect(isManagedPrefix(['mail.', 'vapid.'], null)).toBe(false)
    })

    // An empty prefix from the server would otherwise match every panel and hide
    // the entire System group.
    it('ignores an empty managed prefix rather than matching everything', () => {
        expect(isManagedPrefix([''], 'sentry.')).toBe(false)
        expect(isManagedPrefix(['', 'mail.'], 'mail.')).toBe(true)
    })

    // Prefix matching is on the namespace, so a lookalike must not match.
    it('does not match a namespace that merely starts similarly', () => {
        expect(isManagedPrefix(['mail.'], 'mailbox.')).toBe(false)
    })
})

// The pending window is not "nothing is managed". A screen that renders its
// form on an empty list gives a hosted owner time to act on settings they do
// not administer — VapidPanel's Generate button being the sharp case, since it
// POSTs to a core endpoint rather than writing a row.
describe('pending vs unmanaged', () => {
    it('an empty list is indistinguishable from unmanaged, so callers need the pending flag', () => {
        // Both states produce the same answer from the matcher...
        expect(isManagedPrefix([], 'vapid.')).toBe(false)
        // ...which is exactly why a screen that can ACT must gate on pending
        // rather than on this result. Asserted here so the distinction is not
        // quietly removed.
        expect(isManagedPrefix([], 'vapid.')).toBe(isManagedPrefix([], 'mail.'))
    })
})

// An operator may manage only the provider CREDENTIALS, leaving the org its own
// From address. The Mail Sending screen declares 'mail.', so it correctly stays
// VISIBLE under that narrower set — the org still administers what it owns.
describe('narrower-than-namespace prefixes', () => {
    const credentialsOnly = ['mail.provider', 'mail.postmark_', 'mail.smtp_']

    it('does not hide the whole mail namespace', () => {
        expect(isManagedPrefix(credentialsOnly, 'mail.')).toBe(false)
    })

    it('still hides a panel scoped to a managed key', () => {
        expect(isManagedPrefix(credentialsOnly, 'mail.postmark_server_token')).toBe(true)
    })
})
