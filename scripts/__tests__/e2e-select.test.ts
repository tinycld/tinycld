import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { AREAS, selectSpecs, shardsFor } from '../e2e-select'

const SPECS = [
    'tests/e2e/avatar.spec.ts',
    'tests/e2e/forms-builder.spec.ts',
    'tests/e2e/reports-view.spec.ts',
    'tests/e2e/rules.spec.ts',
    'tests/e2e/web-forms.spec.ts',
]
const FORMS = [
    'tests/e2e/forms-builder.spec.ts',
    'tests/e2e/rules.spec.ts',
    'tests/e2e/web-forms.spec.ts',
]
const REPORTS = ['tests/e2e/reports-view.spec.ts']

describe('selectSpecs', () => {
    it('runs only the reports specs for a change inside reports', () => {
        const changed = ['core/reports/lib/run.ts', 'core/server/reports/run.go']
        expect(selectSpecs(changed, SPECS)).toEqual({ mode: 'some', specs: REPORTS })
    })

    it('runs the forms and the reports specs for a change inside forms', () => {
        // The report builder imports the forms components.
        expect(selectSpecs(['core/ui/forms/ValueInput.tsx'], SPECS)).toEqual({
            mode: 'some',
            specs: [...FORMS.slice(0, 1), ...REPORTS, ...FORMS.slice(1)],
        })
    })

    it('runs every spec when one changed file is outside every area', () => {
        const changed = ['core/reports/lib/run.ts', 'core/ui/button/index.tsx']
        expect(selectSpecs(changed, SPECS)).toEqual({ mode: 'all', specs: [] })
    })

    it('runs every spec for a change to a shared e2e helper or to CI', () => {
        expect(selectSpecs(['tests/e2e/helpers.ts'], SPECS).mode).toBe('all')
        expect(selectSpecs(['.github/workflows/ci.yml'], SPECS).mode).toBe('all')
        expect(selectSpecs(['scripts/e2e-select.ts'], SPECS).mode).toBe('all')
    })

    it('runs a changed spec outside every area alone, with the areas changed beside it', () => {
        expect(selectSpecs(['tests/e2e/avatar.spec.ts'], SPECS)).toEqual({
            mode: 'some',
            specs: ['tests/e2e/avatar.spec.ts'],
        })
        const changed = ['tests/e2e/avatar.spec.ts', 'tests/e2e/reports-helpers.ts']
        expect(selectSpecs(changed, SPECS)).toEqual({
            mode: 'some',
            specs: ['tests/e2e/avatar.spec.ts', ...REPORTS],
        })
    })

    it('runs nothing for a change to documents only, or to a deleted spec', () => {
        expect(selectSpecs(['docs/packages.md', 'README.md'], SPECS).mode).toBe('none')
        expect(selectSpecs(['tests/e2e/gone.spec.ts'], SPECS).mode).toBe('none')
        expect(selectSpecs([], SPECS).mode).toBe('none')
    })

    it('does not take a help topic for a document: the app bundles it', () => {
        expect(selectSpecs(['core/help/reports.md'], SPECS).mode).toBe('all')
    })
})

describe('shardsFor', () => {
    it('grows with the test count and stops at six', () => {
        expect(shardsFor(1)).toEqual([1])
        expect(shardsFor(45)).toEqual([1])
        expect(shardsFor(46)).toEqual([1, 2])
        expect(shardsFor(1000)).toEqual([1, 2, 3, 4, 5, 6])
    })
})

// A directory that was renamed or removed leaves its area's map false without a sign.
describe('AREAS', () => {
    const root = resolve(import.meta.dirname, '..', '..')
    const paths = AREAS.flatMap(area => area.sources).filter(
        source => source.endsWith('/') || source.endsWith('.ts') || source.endsWith('.tsx')
    )
    it.each(paths)('%s exists', source => {
        expect(existsSync(resolve(root, source))).toBe(true)
    })
})
