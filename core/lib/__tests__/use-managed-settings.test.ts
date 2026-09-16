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
