import type { EditorMessage } from '@tinycld/core/lib/editor/message-bus/types'
import {
    type AnchoredOverlayRequest,
    anchoredOverlayReducer,
    decodeUiMessage,
    initialAnchoredOverlayState,
    resolvePopoverPosition,
} from '@tinycld/core/lib/editor/overlay/anchored-overlay-state'
import { describe, expect, it } from 'vitest'

const RECT = { top: 100, left: 40, width: 2, height: 18, scrollX: 0, scrollY: 0 }

function open(requestId = 'req1'): AnchoredOverlayRequest {
    return { kind: 'trigger:cards-mention', requestId, rect: RECT, payload: null }
}

describe('decodeUiMessage', () => {
    it('decodes show-popover', () => {
        const message: EditorMessage = {
            namespace: 'ui',
            type: 'show-popover',
            requestId: 'req1',
            payload: { kind: 'trigger:cards-mention', rect: RECT, payload: { items: [] } },
        }
        expect(decodeUiMessage(message)).toEqual({
            type: 'show',
            request: {
                kind: 'trigger:cards-mention',
                requestId: 'req1',
                rect: RECT,
                payload: { items: [] },
            },
        })
    })

    it('UNWRAPS popover-update, so the body is not handed a payload.payload', () => {
        // The wire carries the contents alongside routing metadata; the body
        // expects only the contents. Getting this wrong renders an empty
        // popover that looks like a data problem rather than a decode bug.
        const message: EditorMessage = {
            namespace: 'ui',
            type: 'popover-update',
            requestId: 'req1',
            payload: { payload: { items: [{ id: 'u1' }] }, editorInstanceId: 'rich-1' },
        }
        expect(decodeUiMessage(message)).toEqual({
            type: 'update',
            requestId: 'req1',
            payload: { items: [{ id: 'u1' }] },
        })
    })

    it('decodes popover-exited and dismiss-on-scroll', () => {
        expect(
            decodeUiMessage({
                namespace: 'ui',
                type: 'popover-exited',
                requestId: 'req1',
                payload: null,
            })
        ).toEqual({ type: 'webview-exited', requestId: 'req1' })

        expect(
            decodeUiMessage({
                namespace: 'ui',
                type: 'popover-dismiss-on-scroll',
                payload: null,
            })
        ).toEqual({ type: 'dismiss-on-scroll' })
    })

    it('rejects a show-popover with no requestId — there would be nobody to answer', () => {
        expect(
            decodeUiMessage({
                namespace: 'ui',
                type: 'show-popover',
                payload: { kind: 'x', rect: RECT, payload: null },
            })
        ).toBeNull()
    })

    it('rejects a malformed rect rather than drawing somewhere arbitrary', () => {
        expect(
            decodeUiMessage({
                namespace: 'ui',
                type: 'show-popover',
                requestId: 'req1',
                payload: { kind: 'x', rect: { top: 'nope' }, payload: null },
            })
        ).toBeNull()
    })

    it('ignores other namespaces', () => {
        expect(
            decodeUiMessage({ namespace: 'markdown', type: 'show-popover', payload: null })
        ).toBeNull()
    })
})

describe('anchoredOverlayReducer', () => {
    it('opens, then re-renders on a matching update', () => {
        const shown = anchoredOverlayReducer(initialAnchoredOverlayState, {
            type: 'show',
            request: open(),
        })
        const updated = anchoredOverlayReducer(shown, {
            type: 'update',
            requestId: 'req1',
            payload: { items: [1] },
        })
        expect(updated.open?.payload).toEqual({ items: [1] })
    })

    it('drops an update for a request that is no longer open', () => {
        const shown = anchoredOverlayReducer(initialAnchoredOverlayState, {
            type: 'show',
            request: open('req1'),
        })
        const stale = anchoredOverlayReducer(shown, {
            type: 'update',
            requestId: 'req-old',
            payload: { items: [1] },
        })
        expect(stale).toBe(shown)
    })

    it('closes on a matching respond and ignores a stale one', () => {
        const shown = anchoredOverlayReducer(initialAnchoredOverlayState, {
            type: 'show',
            request: open('req1'),
        })
        expect(
            anchoredOverlayReducer(shown, { type: 'respond', requestId: 'req1' }).open
        ).toBeNull()
        expect(anchoredOverlayReducer(shown, { type: 'respond', requestId: 'other' })).toBe(shown)
    })

    it('a scroll closes whatever is open, without needing a requestId', () => {
        const shown = anchoredOverlayReducer(initialAnchoredOverlayState, {
            type: 'show',
            request: open(),
        })
        expect(anchoredOverlayReducer(shown, { type: 'dismiss-on-scroll' }).open).toBeNull()
    })

    it('a newer show replaces an open one', () => {
        const first = anchoredOverlayReducer(initialAnchoredOverlayState, {
            type: 'show',
            request: open('req1'),
        })
        const second = anchoredOverlayReducer(first, { type: 'show', request: open('req2') })
        expect(second.open?.requestId).toBe('req2')
    })
})

describe('resolvePopoverPosition', () => {
    const base = {
        rect: RECT,
        webViewOriginX: 0,
        webViewOriginY: 200,
        viewportWidth: 400,
        viewportHeight: 800,
        popoverWidth: 260,
        popoverHeightEstimate: 220,
        gap: 4,
    }

    it('sits below the anchor when there is room', () => {
        const { top } = resolvePopoverPosition(base)
        expect(top).toBe(200 + 100 + 18 + 4)
    })

    it('flips above the anchor when it would overflow the bottom', () => {
        // Otherwise the picker lands off-screen exactly when someone is typing
        // at the bottom of a long description.
        const { top } = resolvePopoverPosition({ ...base, viewportHeight: 340 })
        expect(top).toBeLessThan(200 + 100)
    })

    it('clamps to the left edge instead of going negative', () => {
        const { left } = resolvePopoverPosition({
            ...base,
            rect: { ...RECT, left: -50 },
        })
        expect(left).toBeGreaterThanOrEqual(0)
    })

    it('clamps to the right edge so the popover stays on screen', () => {
        const { left } = resolvePopoverPosition({ ...base, rect: { ...RECT, left: 390 } })
        expect(left + base.popoverWidth).toBeLessThanOrEqual(base.viewportWidth)
    })
})
