// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import type { ShareSession } from '../../anon-identity'
import { buildAnonMount, buildGuestMount } from '../use-share-editor-mount'

// viewer session fixture — the canonical test case for View 3 (read-only).
const viewerSession: ShareSession = {
    sessionToken: 'tok-viewer-abc',
    anonId: 'anon-id-123',
    displayName: 'Anon Panther',
    role: 'viewer',
    itemId: 'item-xyz',
    name: 'Q1 Report',
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    orgName: 'Acme Inc',
    orgSlug: 'acme',
}

const commentorSession: ShareSession = {
    ...viewerSession,
    sessionToken: 'tok-commentor-def',
    role: 'commentor',
}

const editorSession: ShareSession = {
    ...viewerSession,
    sessionToken: 'tok-editor-ghi',
    role: 'editor',
}

describe('buildAnonMount', () => {
    it('produces an anon-kind identity', () => {
        const mount = buildAnonMount(viewerSession)
        expect(mount.identity.kind).toBe('anon')
    })

    it('has no userId or userOrgId on the identity', () => {
        const mount = buildAnonMount(viewerSession)
        expect(mount.identity.userId).toBeUndefined()
        expect(mount.identity.userOrgId).toBeUndefined()
    })

    it('sets displayName from the session', () => {
        const mount = buildAnonMount(viewerSession)
        expect(mount.identity.displayName).toBe('Anon Panther')
    })

    it('generates a non-empty deterministic color from anonId', () => {
        const m1 = buildAnonMount(viewerSession)
        const m2 = buildAnonMount(viewerSession)
        expect(m1.identity.color).toBeTruthy()
        expect(m1.identity.color).toBe(m2.identity.color)
    })

    it('color differs for different anonIds', () => {
        const a = buildAnonMount({ ...viewerSession, anonId: 'anon-aaa' })
        const b = buildAnonMount({ ...viewerSession, anonId: 'anon-bbb' })
        // Different ids should (in practice) produce different hues.
        // This is probabilistic but the strings are sufficiently different.
        expect(a.identity.color).not.toBe(b.identity.color)
    })

    it('sets itemId, itemName, mimeType from session', () => {
        const mount = buildAnonMount(viewerSession)
        expect(mount.itemId).toBe('item-xyz')
        expect(mount.itemName).toBe('Q1 Report')
        expect(mount.mimeType).toBe(
            'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        )
    })

    it('sets itemFile to empty string (bytes arrive via realtime WS bootstrap)', () => {
        const mount = buildAnonMount(viewerSession)
        expect(mount.itemFile).toBe('')
    })

    it('sets realtimeCredential to shareSession with the sessionToken', () => {
        const mount = buildAnonMount(viewerSession)
        expect(mount.realtimeCredential).toEqual({
            kind: 'shareSession',
            token: 'tok-viewer-abc',
        })
    })

    it('viewer: all capabilities false', () => {
        const mount = buildAnonMount(viewerSession)
        expect(mount.capabilities).toEqual({
            canEdit: false,
            canComment: false,
            canUseFileActions: false,
            canMention: false,
        })
    })

    it('commentor: all capabilities false (comment capability deferred to Phase 3)', () => {
        const mount = buildAnonMount(commentorSession)
        expect(mount.capabilities).toEqual({
            canEdit: false,
            canComment: false,
            canUseFileActions: false,
            canMention: false,
        })
    })

    it('editor: all capabilities false (anon editor experience deferred)', () => {
        const mount = buildAnonMount(editorSession)
        expect(mount.capabilities).toEqual({
            canEdit: false,
            canComment: false,
            canUseFileActions: false,
            canMention: false,
        })
    })

    it('role is carried through from the session', () => {
        expect(buildAnonMount(viewerSession).role).toBe('viewer')
        expect(buildAnonMount(commentorSession).role).toBe('commentor')
        expect(buildAnonMount(editorSession).role).toBe('editor')
    })
})

describe('buildGuestMount', () => {
    const baseInput = {
        session: viewerSession,
        userId: 'user-guest-1',
        userOrgId: 'uo-guest-1',
        userName: 'Guest Person',
    }

    it('produces a guest-kind identity with userId and userOrgId', () => {
        const mount = buildGuestMount({ ...baseInput, role: 'viewer' })
        expect(mount.identity.kind).toBe('guest')
        expect(mount.identity.userId).toBe('user-guest-1')
        expect(mount.identity.userOrgId).toBe('uo-guest-1')
    })

    it('uses the auth user name as displayName (not the anon session name)', () => {
        const mount = buildGuestMount({ ...baseInput, role: 'commentor' })
        expect(mount.identity.displayName).toBe('Guest Person')
    })

    it('color is stable for the same userId', () => {
        const a = buildGuestMount({ ...baseInput, role: 'commentor' })
        const b = buildGuestMount({ ...baseInput, role: 'editor' })
        expect(a.identity.color).toBeTruthy()
        expect(a.identity.color).toBe(b.identity.color)
    })

    it('color differs for different userIds', () => {
        const a = buildGuestMount({
            ...baseInput,
            userId: 'u-aaa',
            role: 'commentor',
        })
        const b = buildGuestMount({
            ...baseInput,
            userId: 'u-bbb',
            role: 'commentor',
        })
        expect(a.identity.color).not.toBe(b.identity.color)
    })

    it('realtimeCredential is { kind: "auth" } (real PB token, not shareSession)', () => {
        const mount = buildGuestMount({ ...baseInput, role: 'editor' })
        expect(mount.realtimeCredential).toEqual({ kind: 'auth' })
    })

    it('viewer role: all capabilities false (defensive)', () => {
        const mount = buildGuestMount({ ...baseInput, role: 'viewer' })
        expect(mount.capabilities).toEqual({
            canEdit: false,
            canComment: false,
            canUseFileActions: false,
            canMention: false,
        })
        expect(mount.role).toBe('viewer')
    })

    it('commentor role: canComment only — NO file actions, NO mentions', () => {
        const mount = buildGuestMount({ ...baseInput, role: 'commentor' })
        expect(mount.capabilities).toEqual({
            canEdit: false,
            canComment: true,
            canUseFileActions: false,
            canMention: false,
        })
        expect(mount.role).toBe('commentor')
    })

    it('editor role: canEdit + canComment — STILL no file actions, no mentions', () => {
        const mount = buildGuestMount({ ...baseInput, role: 'editor' })
        expect(mount.capabilities).toEqual({
            canEdit: true,
            canComment: true,
            canUseFileActions: false,
            canMention: false,
        })
        expect(mount.role).toBe('editor')
    })

    it('itemId, itemName, mimeType, itemFile carry through from session', () => {
        const mount = buildGuestMount({ ...baseInput, role: 'editor' })
        expect(mount.itemId).toBe('item-xyz')
        expect(mount.itemName).toBe('Q1 Report')
        expect(mount.mimeType).toBe(
            'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        )
        // Like anon, bytes arrive via the realtime WS bootstrap.
        expect(mount.itemFile).toBe('')
    })

    it('guests NEVER get canUseFileActions or canMention at any role', () => {
        for (const role of ['viewer', 'commentor', 'editor'] as const) {
            const mount = buildGuestMount({ ...baseInput, role })
            expect(mount.capabilities.canUseFileActions).toBe(false)
            expect(mount.capabilities.canMention).toBe(false)
        }
    })
})
