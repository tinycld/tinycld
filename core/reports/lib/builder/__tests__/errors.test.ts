import { ClientResponseError } from 'pocketbase'
import { describe, expect, it } from 'vitest'
import { describeProblem, problemsFor, problemsFromError, sectionOf } from '../errors'

describe('builder errors', () => {
    it('puts each spec path in its builder section', () => {
        expect(sectionOf('columns.2.path.1.link')).toBe('columns')
        expect(sectionOf('filter.args.0')).toBe('filter')
        expect(sectionOf('params.0.default')).toBe('filter')
        expect(sectionOf('groups.1.bucket')).toBe('group')
        expect(sectionOf('sort.0.col')).toBe('group')
        expect(sectionOf('pivot.cell')).toBe('pivot')
        expect(sectionOf('layout.3.formula')).toBe('layout')
        expect(sectionOf('source')).toBe('details')
        expect(sectionOf('title')).toBe('details')
    })

    it('names the part a problem is about', () => {
        expect(
            describeProblem({
                path: 'columns.2.path.1.link',
                code: 'unknown_link',
                message: 'no link "x"',
            })
        ).toBe('Column 3: no link "x"')
        expect(
            describeProblem({ path: 'groups.0.col', code: 'invalid', message: 'pick one' })
        ).toBe('Group 1: pick one')
        expect(describeProblem({ path: 'source', code: 'unknown_source', message: 'gone' })).toBe(
            'gone'
        )
    })

    it('reads a save refusal and filters it by section', () => {
        const error = new ClientResponseError({
            status: 400,
            response: {
                message: 'The report is not valid.',
                data: {
                    'columns.0.id': { code: 'invalid', message: 'bad id' },
                    title: { code: 'validation_required', message: 'Cannot be blank.' },
                },
            },
        })
        const problems = problemsFromError(error)
        expect(problemsFor('columns', problems)).toEqual([
            { path: 'columns.0.id', code: '', message: 'bad id' },
        ])
        expect(problemsFor('details', problems)).toEqual([
            { path: 'title', code: '', message: 'Cannot be blank.' },
        ])
    })
})
