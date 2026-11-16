import { describe, expect, it } from 'vitest'
import {
    emailDefaults,
    parseAddresses,
    reportEmailSchema,
    sentToastTitle,
} from '../export-email-form'

const VALID = {
    to: 'a@x.test',
    cc: '',
    subject: 'Stub team balance — May 2039',
    message: 'Hello,',
    format: 'pdf' as const,
}

/** The messages of the schema's issues on one field. */
function errorsOn(values: Record<string, unknown>, field: string): string[] {
    const result = reportEmailSchema.safeParse(values)
    if (result.success) return []
    return result.error.issues.filter(i => i.path[0] === field).map(i => i.message)
}

function addresses(prefix: string, n: number): string {
    return Array.from({ length: n }, (_, i) => `${prefix}${i}@x.test`).join(', ')
}

describe('parseAddresses', () => {
    it('splits on commas, semicolons, spaces and line breaks, trims, drops empties and keeps order', () => {
        expect(
            parseAddresses(' c@x.test, a@x.test;b@x.test   d@x.test\ne@x.test\r\n,; f@x.test ,')
        ).toEqual(['c@x.test', 'a@x.test', 'b@x.test', 'd@x.test', 'e@x.test', 'f@x.test'])
    })

    it('gives no address for an empty field', () => {
        expect(parseAddresses('  , ; \n')).toEqual([])
    })
})

describe('reportEmailSchema', () => {
    it('parses a valid form', () => {
        expect(reportEmailSchema.safeParse({ ...VALID, cc: 'b@x.test; c@x.test' }).success).toBe(
            true
        )
    })

    it('asks for one address in To', () => {
        expect(errorsOn({ ...VALID, to: '  ' }, 'to')).toEqual(['Enter at least one address'])
    })

    it('names a bad address on its own field', () => {
        expect(errorsOn({ ...VALID, to: 'a@x.test, nope, b@x.test' }, 'to')).toEqual([
            'nope is not an email address',
        ])
        const cc = errorsOn({ ...VALID, cc: 'b@x.test; not-an-address' }, 'cc')
        expect(cc).toEqual(['not-an-address is not an email address'])
        expect(errorsOn({ ...VALID, cc: 'b@x.test; not-an-address' }, 'to')).toEqual([])
    })

    it('takes at most 10 addresses over To and Cc, and says so on To', () => {
        expect(errorsOn({ ...VALID, to: addresses('t', 6), cc: addresses('c', 5) }, 'to')).toEqual([
            'Use at most 10 addresses in To and Cc',
        ])
        expect(
            reportEmailSchema.safeParse({ ...VALID, to: addresses('t', 6), cc: addresses('c', 4) })
                .success
        ).toBe(true)
    })

    it('asks for a subject of at most 200 characters', () => {
        expect(errorsOn({ ...VALID, subject: '   ' }, 'subject')).toEqual(['Enter a subject'])
        expect(errorsOn({ ...VALID, subject: 's'.repeat(201) }, 'subject')).toHaveLength(1)
        expect(errorsOn({ ...VALID, subject: 's'.repeat(200) }, 'subject')).toEqual([])
    })

    it('takes a message of at most 5 000 characters', () => {
        expect(errorsOn({ ...VALID, message: 'm'.repeat(5001) }, 'message')).toHaveLength(1)
        expect(errorsOn({ ...VALID, message: 'm'.repeat(5000) }, 'message')).toEqual([])
    })
})

describe('emailDefaults', () => {
    it("fills the subject and the message from the report and the sender's name", () => {
        expect(
            emailDefaults({
                title: 'Stub team balance',
                heading: 'Stub Sales Co',
                periodWords: 'May 1 – 31, 2039',
                senderName: 'Ann Lee',
            })
        ).toEqual({
            to: '',
            cc: '',
            subject: 'Stub team balance — May 1 – 31, 2039',
            message:
                'Hello,\n\nAttached is the Stub team balance report of Stub Sales Co, May 1 – 31, 2039.\n\nAnn Lee',
            format: 'pdf',
        })
    })

    it('leaves the period out of a report with none', () => {
        const values = emailDefaults({
            title: 'Stub larger side',
            heading: 'Stub Sales Co',
            periodWords: '',
            senderName: 'Ann Lee',
        })
        expect(values.subject).toBe('Stub larger side')
        expect(values.message).toBe(
            'Hello,\n\nAttached is the Stub larger side report of Stub Sales Co.\n\nAnn Lee'
        )
    })
})

describe('sentToastTitle', () => {
    it('names the one address', () => {
        expect(sentToastTitle(['a@x.test'], [])).toBe('Report sent to a@x.test')
    })

    it('counts the others over To and Cc', () => {
        expect(sentToastTitle(['a@x.test', 'b@x.test'], ['c@x.test'])).toBe(
            'Report sent to a@x.test and 2 others'
        )
    })

    it('says "1 other" for two addresses', () => {
        expect(sentToastTitle(['a@x.test'], ['b@x.test'])).toBe(
            'Report sent to a@x.test and 1 other'
        )
    })
})
