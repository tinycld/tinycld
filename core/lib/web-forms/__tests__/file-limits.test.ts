import { describe, expect, it } from 'vitest'
import { definitionSchema } from '../../forms/definition'
import { fileLimitIssues } from '../file-limits'

const def = definitionSchema.parse({
    version: 1,
    pages: [{ key: 'main', title: 'Main' }],
    fields: [
        {
            key: 'cv',
            page: 'main',
            label: 'CV',
            kind: 'file',
            accept: ['application/pdf', '.docx'],
            maxSizeMb: 1,
        },
        { key: 'docs', page: 'main', label: 'Docs', kind: 'file', multiple: true },
    ],
})
const MB = 1024 * 1024
const file = (name: string, type: string, size: number) => ({ name, type, size })

describe('fileLimitIssues', () => {
    it('passes files within the limits', () => {
        const files = {
            a: file('cv.pdf', 'application/pdf', MB / 2),
            b: file('CV.DOCX', 'application/zip', 10),
        }
        expect(fileLimitIssues(def, { cv: 'a' }, files, 25)).toEqual([])
        expect(fileLimitIssues(def, { cv: 'b' }, files, 25)).toEqual([])
    })

    it('names a file over its field limit', () => {
        const files = { a: file('big.pdf', 'application/pdf', 2 * MB) }
        expect(fileLimitIssues(def, { cv: 'a' }, files, 25)).toEqual([
            { path: ['cv'], message: 'big.pdf is larger than 1 MB' },
        ])
    })

    it('names a file of a type the field does not accept', () => {
        const files = { a: file('notes.txt', 'text/plain', 10) }
        expect(fileLimitIssues(def, { cv: 'a' }, files, 25)).toEqual([
            { path: ['cv'], message: 'notes.txt is not an accepted type' },
        ])
    })

    it('flags the total over the submission cap on the last file field', () => {
        const files = { a: file('a.bin', 'x/y', 6 * MB), b: file('b.bin', 'x/y', 6 * MB) }
        expect(fileLimitIssues(def, { docs: ['a', 'b'] }, files, 10)).toEqual([
            { path: ['docs'], message: 'The files together are larger than 10 MB' },
        ])
    })
})
