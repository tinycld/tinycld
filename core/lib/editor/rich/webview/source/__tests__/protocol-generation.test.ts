import { describe, expect, it } from 'vitest'
import { makeMessage } from '../../../../message-bus/types'
import { APP_INIT, APP_PARK, type RichEditorInitPayload } from '../protocol'

/**
 * A warm editor is re-initialized rather than remounted, so the page needs a
 * way to tell a NEW configuration from a re-delivery of the one it already
 * applied. The generation is that discriminator: the page keys its Tiptap
 * subtree on it, so a bumped value is a full stage-two reconstruction and a
 * repeated value is a no-op.
 */
describe('init generation', () => {
    it('rides in the init payload so the page can rebuild on a bump', () => {
        const payload: RichEditorInitPayload = {
            generation: 3,
            contentFormat: 'markdown',
            initialContent: 'hello',
            placeholder: '',
            editable: true,
            autofocus: false,
            colors: { bg: '#000', fg: '#fff', placeholder: '#888', primary: '#0f0' },
        }
        const message = makeMessage('app', APP_INIT, payload)

        expect(message.namespace).toBe('app')
        expect((message.payload as RichEditorInitPayload).generation).toBe(3)
    })

    it('names a park message so a released editor can drop to stage one', () => {
        expect(APP_PARK).toBe('park')
        expect(makeMessage('app', APP_PARK, null).type).toBe('park')
    })
})
