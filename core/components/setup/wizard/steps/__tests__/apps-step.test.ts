import { describe, expect, it } from 'vitest'
import { appChoicesOf, autoUpdateChoiceOf } from '../app-choices'

const bundled = [
    { slug: 'alpha', name: 'Alpha', description: 'First app', nav: { icon: 'mail' } },
    { slug: 'beta', name: 'Beta', description: 'Second app', nav: { icon: 'calendar' } },
]

describe('appChoicesOf', () => {
    it('lists bundled apps in build order with their on state', () => {
        const rows = [
            { id: 'r2', slug: 'beta', status: 'disabled' },
            { id: 'r1', slug: 'alpha', status: 'bundled' },
        ]
        expect(appChoicesOf(rows, bundled)).toEqual([
            {
                id: 'r1',
                slug: 'alpha',
                name: 'Alpha',
                description: 'First app',
                icon: 'mail',
                isOn: true,
                needsTerms: false,
            },
            {
                id: 'r2',
                slug: 'beta',
                name: 'Beta',
                description: 'Second app',
                icon: 'calendar',
                isOn: false,
                needsTerms: false,
            },
        ])
    })
    it('shows an app with terms the owner has not accepted as off, whatever its status', () => {
        const rows = [
            { id: 'r1', slug: 'alpha', status: 'bundled' },
            { id: 'r2', slug: 'beta', status: 'bundled' },
        ]
        const choices = appChoicesOf(rows, bundled, new Set(['beta']))
        expect(choices.map(c => [c.slug, c.isOn, c.needsTerms])).toEqual([
            ['alpha', true, false],
            ['beta', false, true],
        ])
    })
    it('leaves out core and rows not compiled into this build', () => {
        const rows = [
            { id: 'c', slug: 'core', status: 'bundled' },
            { id: 'x', slug: 'extra', status: 'disabled' },
        ]
        expect(
            appChoicesOf(rows, [...bundled, { slug: 'core', name: 'Core', description: '' }])
        ).toEqual([])
    })
    it('leaves out a bundled package with no nav entry, which is not an app people open', () => {
        const rows = [{ id: 'l', slug: 'lib', status: 'bundled' }]
        expect(appChoicesOf(rows, [{ slug: 'lib', name: 'Lib', description: '' }])).toEqual([])
    })
    it('leaves out a bundled app that has no registry row yet', () => {
        expect(appChoicesOf([], bundled)).toEqual([])
    })
})

describe('autoUpdateChoiceOf', () => {
    const status = { available: true, lastRun: '', lastResult: '', nextCheck: '' }
    it('is hidden until the server says updates are available', () => {
        expect(autoUpdateChoiceOf(undefined, true, true)).toEqual({ isVisible: false, isOn: true })
        expect(
            autoUpdateChoiceOf(
                { available: false, reason: 'x', lastRun: '', lastResult: '', nextCheck: '' },
                true,
                true
            )
        ).toEqual({ isVisible: false, isOn: true })
    })
    it('is hidden until the stored setting has loaded', () => {
        expect(autoUpdateChoiceOf(status, false, false)).toEqual({ isVisible: false, isOn: false })
    })
    it('shows the stored value when available and loaded', () => {
        expect(autoUpdateChoiceOf(status, false, true)).toEqual({ isVisible: true, isOn: false })
    })
})
