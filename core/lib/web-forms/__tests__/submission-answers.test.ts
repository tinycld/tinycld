import { describe, expect, it } from 'vitest'
import { definitionSchema } from '../../forms/definition'
import { answersOf, previewOf } from '../submission-answers'

const def = definitionSchema.parse({
    version: 1,
    pages: [{ key: 'main', title: 'Main' }],
    fields: [
        { key: 'name', page: 'main', label: 'Name', kind: 'text' },
        { key: 'note', page: 'main', label: 'Note', kind: 'textarea' },
        { key: 'city', page: 'main', label: 'City', kind: 'text' },
        { key: 'ok', page: 'main', label: 'OK', kind: 'toggle' },
        {
            key: 'role',
            page: 'main',
            label: 'Role',
            kind: 'select',
            options: [{ value: 'dev', label: 'Developer' }],
        },
        { key: 'cv', page: 'main', label: 'CV', kind: 'file' },
    ],
})

describe('submission answers', () => {
    it('lists each answer in form order, without files', () => {
        const answers = answersOf(def, { name: 'Ada', ok: true, role: 'dev', cv: ['f1'] })
        expect(answers.map(a => [a.label, a.text])).toEqual([
            ['Name', 'Ada'],
            ['Note', ''],
            ['City', ''],
            ['OK', 'Yes'],
            ['Role', 'Developer'],
        ])
    })

    it('previews the first two text answers that have a value', () => {
        expect(previewOf(def, { name: 'Ada', note: '', city: 'London' })).toBe('Ada · London')
        expect(previewOf(def, null)).toBe('')
    })
})
