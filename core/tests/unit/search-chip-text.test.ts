import { chipsToText, textAfterChips } from '@tinycld/core/lib/search/chip-text'
import { describe, expect, it } from 'vitest'

describe('chipsToText', () => {
    it('renders one chip as a single colon-space prefix', () => {
        expect(chipsToText(['mail'])).toBe('mail: ')
    })

    it('renders multiple chips in order, each with its own colon-space', () => {
        expect(chipsToText(['mail', 'drive'])).toBe('mail: drive: ')
    })

    it('renders no chips as an empty string', () => {
        expect(chipsToText([])).toBe('')
    })
})

describe('textAfterChips', () => {
    it('strips exactly the rendered chip prefix, leaving the typed remainder', () => {
        expect(textAfterChips('mail: budget report', ['mail'])).toBe('budget report')
    })

    it('returns the full text unchanged when there are no chips', () => {
        expect(textAfterChips('budget report', [])).toBe('budget report')
    })

    it('returns an empty string when the text is exactly the chip prefix', () => {
        expect(textAfterChips('mail: ', ['mail'])).toBe('')
    })

    // Regression guard for the backspace-pop path: the prefix must be computed
    // from the CURRENT chip list, not the previous one, or popping a chip would
    // strip the wrong length and truncate real query text.
    it('recomputes against a shorter chip list rather than the text that produced it', () => {
        // Text was built from two chips; simulate having already popped one so
        // only 'mail' remains — the remainder must start right after 'mail: '.
        expect(textAfterChips('mail: drive: budget', ['mail'])).toBe('drive: budget')
    })
})
