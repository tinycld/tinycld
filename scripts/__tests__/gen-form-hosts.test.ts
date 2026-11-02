import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { loadFormHosts, mergeAllFormHosts, mergeFormHosts } from '../gen-form-hosts'

const tmpDirs: string[] = []
afterEach(() => {
    for (const d of tmpDirs.splice(0)) fs.rmSync(d, { recursive: true, force: true })
})

function makePkg(exportsMap: Record<string, string>, files: Record<string, string>): string {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'gen-form-hosts-'))
    tmpDirs.push(dir)
    fs.writeFileSync(
        path.join(dir, 'package.json'),
        JSON.stringify({ name: '@tinycld/fake', exports: exportsMap })
    )
    for (const [rel, content] of Object.entries(files)) {
        const abs = path.join(dir, rel)
        fs.mkdirSync(path.dirname(abs), { recursive: true })
        fs.writeFileSync(abs, content)
    }
    return dir
}

const hostJs = `export default [{ key: 'gizmos:widget', label: 'Widget', collection: 'gizmos_widgets', formColumn: 'form', fieldsColumn: 'fields', columns: ['name'], builtin: [{ key: 'name', label: 'Name', kind: 'text', storage: 'column', required: true }] }]\n`

describe('loadFormHosts', () => {
    it('imports the module through the exports map', async () => {
        const dir = makePkg(
            { './forms': './tinycld/fake/forms.js' },
            { 'tinycld/fake/forms.js': hostJs }
        )
        const hosts = await loadFormHosts(dir, '@tinycld/fake', 'forms')
        expect(hosts[0]?.key).toBe('gizmos:widget')
    })
    it('throws when the exports map lacks the subpath', async () => {
        const dir = makePkg({}, {})
        await expect(loadFormHosts(dir, '@tinycld/fake', 'forms')).rejects.toThrow(
            /no '\.\/forms' entry/
        )
    })
})

describe('mergeFormHosts', () => {
    const widget = {
        key: 'gizmos:widget',
        label: 'Widget',
        collection: 'gizmos_widgets',
        formColumn: 'form',
        fieldsColumn: 'fields',
        columns: ['name'],
        builtin: [{ key: 'name', label: 'Name', kind: 'text', storage: 'column', required: true }],
    }
    it('stamps the package slug and sorts by key', () => {
        const merged = mergeFormHosts([
            { slug: 'zeta', hosts: [{ ...widget, key: 'zeta:thing' }] },
            { slug: 'gizmos', hosts: [widget] },
        ])
        expect(merged.hosts.map(h => [h.key, h.pkgSlug])).toEqual([
            ['gizmos:widget', 'gizmos'],
            ['zeta:thing', 'zeta'],
        ])
    })
    it('rejects a host whose key prefix is not the package slug', () => {
        expect(() => mergeFormHosts([{ slug: 'other', hosts: [widget] }])).toThrow(
            /gizmos:widget.*other/
        )
    })
    it('rejects an invalid host', () => {
        expect(() =>
            mergeFormHosts([{ slug: 'gizmos', hosts: [{ ...widget, kinds: ['nope'] }] }])
        ).toThrow(/kinds/)
    })
    it('keeps a host layout, so server/form_hosts.json carries it to Go', () => {
        const layout = {
            pages: [
                { key: 'details', title: 'Details' },
                { key: 'more', title: 'More' },
            ],
            fields: { name: { width: 'half' } },
            rules: [
                {
                    key: 'show_more',
                    match: 'all',
                    when: [{ field: 'name', op: 'is_empty' }],
                    actions: [{ action: 'show', page: 'more' }],
                },
            ],
        }
        const merged = mergeFormHosts([{ slug: 'gizmos', hosts: [{ ...widget, layout }] }])
        expect(merged.hosts[0].layout).toEqual(layout)
    })
    it('rejects a duplicate key across packages', () => {
        expect(() =>
            mergeFormHosts([
                { slug: 'gizmos', hosts: [widget] },
                { slug: 'gizmos', hosts: [widget] },
            ])
        ).toThrow(/duplicate/)
    })
})

describe('mergeFormHosts with computed and table built-ins', () => {
    it('keeps expression, decimals and columns on the emitted host', () => {
        const merged = mergeFormHosts([
            {
                slug: 'gizmos',
                hosts: [
                    {
                        key: 'gizmos:invoice',
                        label: 'Invoice',
                        collection: 'gizmos_invoices',
                        formColumn: 'form',
                        fieldsColumn: 'fields',
                        columns: ['subtotal', 'form', 'fields'],
                        builtin: [
                            {
                                key: 'lines',
                                label: 'Lines',
                                kind: 'table',
                                storage: 'rows',
                                columns: [{ key: 'qty', kind: 'number', label: 'Qty' }],
                            },
                            {
                                key: 'subtotal',
                                label: 'Subtotal',
                                kind: 'computed',
                                storage: 'column',
                                decimals: 2,
                                expression: { sumOf: [{ var: 'lines' }, 'qty'] },
                            },
                        ],
                    },
                ],
            },
        ])
        const builtins = merged.hosts[0].builtin
        expect(builtins[0].columns?.[0].key).toBe('qty')
        expect(builtins[1].expression).toEqual({ sumOf: [{ var: 'lines' }, 'qty'] })
        expect(builtins[1].decimals).toBe(2)
    })
})

describe('mergeAllFormHosts', () => {
    it('always includes the core web form host', () => {
        const merged = mergeAllFormHosts([])
        expect(merged.hosts.map(h => h.key)).toEqual(['core:web_form'])
        expect(merged.hosts[0]).toMatchObject({
            pkgSlug: 'core',
            collection: 'form_submissions',
            builtin: [],
        })
    })

    it('rejects a package that uses the core slug', () => {
        expect(() => mergeAllFormHosts([{ slug: 'core', hosts: [] }])).toThrow(/reserved/)
    })
})
