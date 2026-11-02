import { describe, expect, it } from 'vitest'
import { CORE_FORM_HOST_ENTRY, deriveFormHosts } from '../../packages/derive-form-hosts'
import { widgetHost } from './fixtures'

describe('deriveFormHosts', () => {
    it('collects hosts with their package slug and ui loader', async () => {
        const ui = { components: {} }
        const hosts = deriveFormHosts([
            {
                manifest: { name: 'Gizmos', slug: 'gizmos' },
                formHosts: [widgetHost],
                formHostsUi: async () => ({ default: ui }),
            },
            { manifest: { name: 'Plain', slug: 'plain' } },
        ])
        expect(hosts).toHaveLength(1)
        expect(hosts[0]).toMatchObject({ key: 'gizmos:widget', pkgSlug: 'gizmos' })
        expect(hosts[0].pkgName).toBe('Gizmos')
        await expect(hosts[0].ui()).resolves.toBe(ui)
    })
    it('returns an empty ui when a package declares hosts without a ui module', async () => {
        const hosts = deriveFormHosts([
            { manifest: { name: 'Gizmos', slug: 'gizmos' }, formHosts: [widgetHost] },
        ])
        await expect(hosts[0].ui()).resolves.toEqual({ components: {} })
    })
    it('applies schema defaults to hand-authored input', () => {
        const hosts = deriveFormHosts([
            {
                manifest: { name: 'Gizmos', slug: 'gizmos' },
                formHosts: [
                    {
                        key: 'gizmos:widget',
                        label: 'Widget',
                        collection: 'gizmos_widgets',
                        formColumn: 'form',
                        fieldsColumn: 'fields',
                        columns: ['name'],
                        builtin: [{ key: 'name', label: 'Name', kind: 'text', storage: 'column' }],
                    },
                ],
            },
        ])
        expect(hosts[0].builtin[0]).toMatchObject({
            required: false,
            multiple: false,
            collaborative: false,
        })
    })
})

describe('core form hosts', () => {
    it('derives the web form host as a core host with an empty ui', async () => {
        const hosts = deriveFormHosts([CORE_FORM_HOST_ENTRY])
        expect(hosts.map(h => [h.key, h.pkgSlug, h.pkgName])).toEqual([
            ['core:web_form', 'core', 'Core'],
        ])
        expect(hosts[0].kinds).not.toContain('user')
        await expect(hosts[0].ui()).resolves.toEqual({ components: {} })
    })
})
