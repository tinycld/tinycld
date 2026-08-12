import { describe, expect, it } from 'vitest'
import {
    ALL_OPS,
    humanizeFieldKey,
    NO_VALUE_OPS,
    OPERATORS_BY_TYPE,
    parseRef,
    qualifyRef,
} from '../helpers'

describe('humanizeFieldKey', () => {
    it('replaces underscores and capitalizes the first letter only', () => {
        expect(humanizeFieldKey('has_attachments')).toBe('Has attachments')
        expect(humanizeFieldKey('subject')).toBe('Subject')
        expect(humanizeFieldKey('sender_email')).toBe('Sender email')
    })
})

describe('refs', () => {
    it('round-trips a qualified ref', () => {
        expect(qualifyRef('mail', 'message-received')).toBe('mail:message-received')
        expect(parseRef('mail:message-received')).toEqual({ pkg: 'mail', id: 'message-received' })
    })

    it('throws on a malformed ref', () => {
        expect(() => parseRef('no-colon')).toThrow(/malformed automation ref/)
    })
})

describe('operator sets', () => {
    it('covers every field type', () => {
        expect(Object.keys(OPERATORS_BY_TYPE).sort()).toEqual([
            'boolean',
            'date',
            'number',
            'relation',
            'select',
            'text',
        ])
    })

    it('every operator appears in ALL_OPS exactly once', () => {
        const flattened = Object.values(OPERATORS_BY_TYPE).flat()
        expect(new Set(flattened).size).toBe(flattened.length)
        expect([...flattened].sort()).toEqual([...ALL_OPS].sort())
    })

    it('value-less operators are exactly the is_true/is_false/is_empty set', () => {
        expect([...NO_VALUE_OPS].sort()).toEqual(['is_empty', 'is_false', 'is_true'])
    })
})
