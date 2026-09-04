import { beforeEach, describe, expect, it } from 'vitest'
import {
    mentionTokensToHtml,
    resetMentionLabels,
    serializeMentionToken,
    setMentionLabels,
} from '../mention-node'

// The mention node shows a NAME while storing an ID. Both halves matter: the
// name is what makes a description readable, and the id is what boards' Go flush
// hook parses to notify someone. A bug in either direction is silent — a
// mention that renders correctly but serializes wrong notifies nobody.

const TRIGGER = 'boards-mention'

describe('mention labels', () => {
    beforeEach(() => {
        resetMentionLabels()
        setMentionLabels(TRIGGER, [
            { id: 'u1', label: 'Ada Lovelace' },
            { id: 'u2', label: 'Grace Hopper' },
        ])
    })

    it('renders a known id as that person’s name', () => {
        expect(mentionTokensToHtml('hi [[@u1]]', TRIGGER)).toBe(
            'hi <span data-mention-id="u1">@Ada Lovelace</span>'
        )
    })

    it('rewrites every mention in a body', () => {
        const html = mentionTokensToHtml('[[@u1]] and [[@u2]]', TRIGGER)
        expect(html).toContain('@Ada Lovelace')
        expect(html).toContain('@Grace Hopper')
    })

    // The editor serializes through markdown, where `[` is syntax, so a stored
    // token round-trips escaped. Matching only the bare spelling is what left
    // raw tokens on screen before — the same trap boards' own regex documents.
    it('matches the backslash-escaped spelling markdown produces', () => {
        expect(mentionTokensToHtml('hi \\[\\[@u1\\]\\]', TRIGGER)).toContain('@Ada Lovelace')
    })

    // A token with no name AND no roster entry is the only case left with
    // nothing to show — a legacy `[[@id]]` naming someone who has since left.
    it('falls back to a placeholder for an unknown id with no name', () => {
        expect(mentionTokensToHtml('hi [[@ghost]]', TRIGGER)).toBe(
            'hi <span data-mention-id="ghost">@someone</span>'
        )
    })

    // The whole point of carrying the name: leaving the board does not un-say
    // the sentence that named you, so the mention stays readable even though
    // the roster cannot resolve the id.
    it('uses the name in the token when the roster does not know the id', () => {
        const html = mentionTokensToHtml('hi [[@gone|Grace Hopper]]', TRIGGER)
        expect(html).toContain('@Grace Hopper')
        expect(html).not.toContain('@someone')
    })

    // The roster is live, so a rename shows up without rewriting stored text.
    it('prefers the live roster over the name baked into the token', () => {
        expect(mentionTokensToHtml('hi [[@u1|Old Name]]', TRIGGER)).toContain('@Ada Lovelace')
    })

    describe('the wire token', () => {
        it('omits the name when there is none', () => {
            expect(serializeMentionToken('u1')).toBe('[[@u1]]')
        })

        it('carries the name when there is one', () => {
            expect(serializeMentionToken('u1', 'Ada Lovelace')).toBe('[[@u1|Ada Lovelace]]')
        })

        // `]` and `|` would end the token early and spill the rest of the name
        // into the document as visible text. Percent-encoded, not backslashed:
        // markdown strips a backslash before the token is ever matched.
        it('encodes delimiters a user could type into their own name', () => {
            const token = serializeMentionToken('u1', 'a]b|c')
            expect(token).toBe('[[@u1|a%5Db%7Cc]]')
            expect(mentionTokensToHtml(token, 'unregistered')).toContain('@a]b|c')
        })

        it('round-trips a percent sign without corrupting it', () => {
            const token = serializeMentionToken('u1', '100%5D')
            expect(mentionTokensToHtml(token, 'unregistered')).toContain('@100%5D')
        })
    })

    it('leaves a body with no tokens untouched', () => {
        expect(mentionTokensToHtml('nothing here', TRIGGER)).toBe('nothing here')
    })

    it('preserves the id, not the label, in the markup', () => {
        // The id is the wire format — it must survive into the node's attribute
        // so serializing back reproduces the token the server parses.
        expect(mentionTokensToHtml('[[@u1]]', TRIGGER)).toContain('data-mention-id="u1"')
    })

    // Two triggers on one editor must not resolve each other's ids.
    it('scopes labels to their own trigger', () => {
        setMentionLabels('other', [{ id: 'u1', label: 'Wrong Person' }])
        expect(mentionTokensToHtml('[[@u1]]', TRIGGER)).toContain('@Ada Lovelace')
        expect(mentionTokensToHtml('[[@u1]]', 'other')).toContain('@Wrong Person')
    })

    it('placeholders everything when the trigger is unknown', () => {
        expect(mentionTokensToHtml('[[@u1]]', 'never-registered')).toContain('@someone')
    })
})
