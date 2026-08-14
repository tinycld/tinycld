import { describe, expect, it } from 'vitest'
import type { SerializableTriggerConfig } from '../triggers'

// The native editor receives triggers as JSON built by a HAND-MAINTAINED
// mapping in use-rich-editor.native.tsx. A field added to
// SerializableTriggerConfig but forgotten there reaches web and silently not
// native — which is exactly how `insertsMentionNode` shipped: mentions rendered
// as names on web and as raw `[[@id]]` tokens on the phone, with nothing
// failing to say so.
//
// This test pins the field list. When it fails, add the new field to the
// triggerSignature mapping FIRST, then update the list here.

const SERIALIZABLE_FIELDS = [
    'id',
    'char',
    'allItems',
    'limit',
    'insertTemplate',
    'insertsMentionNode',
] as const

describe('SerializableTriggerConfig', () => {
    it('carries exactly the fields the native init payload knows how to send', () => {
        // A total value: TypeScript fails to compile this if the type gains a
        // required field missing from SERIALIZABLE_FIELDS, and the runtime
        // assertion below catches an optional one.
        const complete: Required<SerializableTriggerConfig> = {
            id: 'cards-mention',
            char: '@',
            allItems: [],
            limit: 6,
            insertTemplate: '[[@{id}]] ',
            insertsMentionNode: true,
        }

        expect(Object.keys(complete).sort()).toEqual([...SERIALIZABLE_FIELDS].sort())
    })
})
