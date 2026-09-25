import { describe, expect, it } from 'vitest'
import { appChoicesOf } from '../app-choices'

const bundled = [
    { slug: 'alpha', name: 'Alpha', description: 'First app', nav: { icon: 'mail' } },
    { slug: 'beta', name: 'Beta', description: 'Second app' },
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
            },
            {
                id: 'r2',
                slug: 'beta',
                name: 'Beta',
                description: 'Second app',
                icon: '',
                isOn: false,
            },
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
    it('leaves out a bundled app that has no registry row yet', () => {
        expect(appChoicesOf([], bundled)).toEqual([])
    })
})
