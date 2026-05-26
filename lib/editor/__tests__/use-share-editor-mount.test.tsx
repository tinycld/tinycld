// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import type { ShareSession } from '../../anon-identity'
import { buildAnonMount } from '../use-share-editor-mount'

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
